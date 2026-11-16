package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.github.com"

type Client struct {
	HTTP      *http.Client
	Token     string
	BaseURL   string
	UserAgent string
	Logf      func(format string, args ...any)
}

func New(token string) *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 90 * time.Second},
		Token:     token,
		BaseURL:   DefaultBaseURL,
		UserAgent: "solidcraft-triage/0.1",
		Logf:      func(string, ...any) {},
	}
}

// Get fetches path (relative to BaseURL) with query q.
func (c *Client) Get(ctx context.Context, path string, q url.Values) ([]byte, http.Header, error) {
	u := strings.TrimRight(c.BaseURL, "/") + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return c.getURL(ctx, u)
}

// GetURL fetches an absolute URL, which is what a GitHub Link header gives us.
func (c *Client) GetURL(ctx context.Context, u string) ([]byte, http.Header, error) {
	return c.getURL(ctx, u)
}

func (c *Client) getURL(ctx context.Context, u string) ([]byte, http.Header, error) {
	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		if attempt > 0 {
			if err := sleep(ctx, backoff(attempt)); err != nil {
				return nil, nil, err
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		req.Header.Set("User-Agent", c.UserAgent)
		if c.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}

		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err
			c.Logf("network error: %v (retrying)", err)
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			c.Logf("read error: %v (retrying)", readErr)
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			c.logRate(resp.Header)
			return body, resp.Header, nil
		}

		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return nil, resp.Header, fmt.Errorf("GitHub rejected the token (401). Check GITHUB_TOKEN or run the wrapper so it can use `gh auth token`")
		case http.StatusForbidden, http.StatusTooManyRequests:
			wait := rateLimitWait(resp.Header)
			if wait <= 0 {
				wait = backoff(attempt)
			}
			lastErr = fmt.Errorf("GitHub rate limit or abuse response: %s", strings.TrimSpace(truncate(string(body), 300)))
			c.Logf("%v; waiting %s", lastErr, wait.Round(time.Second))
			if err := sleep(ctx, wait); err != nil {
				return nil, resp.Header, err
			}
		case http.StatusNotFound:
			return nil, resp.Header, fmt.Errorf("GitHub 404 for %s", u)
		default:
			if resp.StatusCode >= 500 {
				lastErr = fmt.Errorf("GitHub %d: %s", resp.StatusCode, strings.TrimSpace(truncate(string(body), 300)))
				c.Logf("%v (retrying)", lastErr)
				continue
			}
			return nil, resp.Header, fmt.Errorf("GitHub %d: %s", resp.StatusCode, strings.TrimSpace(truncate(string(body), 500)))
		}
	}
	return nil, nil, fmt.Errorf("giving up after retries: %w", lastErr)
}

func (c *Client) logRate(h http.Header) {
	remaining := h.Get("X-RateLimit-Remaining")
	if remaining == "" {
		return
	}
	reset := h.Get("X-RateLimit-Reset")
	if reset == "" {
		c.Logf("rate limit: %s remaining", remaining)
		return
	}
	epoch, err := strconv.ParseInt(reset, 10, 64)
	if err != nil {
		c.Logf("rate limit: %s remaining", remaining)
		return
	}
	c.Logf("rate limit: %s remaining, resets %s", remaining, time.Unix(epoch, 0).Format(time.RFC3339))
}

func rateLimitWait(h http.Header) time.Duration {
	if ra := h.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil {
			return time.Duration(secs+2) * time.Second
		}
		if t, err := http.ParseTime(ra); err == nil {
			return time.Until(t) + 2*time.Second
		}
	}
	if h.Get("X-RateLimit-Remaining") == "0" {
		if reset := h.Get("X-RateLimit-Reset"); reset != "" {
			if epoch, err := strconv.ParseInt(reset, 10, 64); err == nil {
				d := time.Until(time.Unix(epoch, 0)) + 2*time.Second
				if d > 0 {
					return d
				}
			}
		}
	}
	return 0
}

func backoff(attempt int) time.Duration {
	d := time.Second << min(attempt, 6)
	if d > 60*time.Second {
		d = 60 * time.Second
	}
	return d
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// NextLink returns the rel="next" URL from a GitHub Link header, or "".
func NextLink(h http.Header) string {
	for _, part := range strings.Split(h.Get("Link"), ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="next"`) {
			continue
		}
		start := strings.IndexByte(part, '<')
		end := strings.IndexByte(part, '>')
		if start >= 0 && end > start+1 {
			return part[start+1 : end]
		}
	}
	return ""
}

// Page parses a JSON array while keeping each element's original bytes.
func Page(body []byte) ([]json.RawMessage, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, err
	}
	return items, nil
}
