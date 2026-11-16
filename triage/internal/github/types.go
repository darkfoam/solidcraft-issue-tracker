package github

import "time"

type User struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

type Label struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

type PullRequestRef struct {
	URL      string     `json:"url"`
	HTMLURL  string     `json:"html_url"`
	MergedAt *time.Time `json:"merged_at"`
}

type Issue struct {
	ID          int64           `json:"id"`
	Number      int             `json:"number"`
	Title       string          `json:"title"`
	Body        string          `json:"body"`
	State       string          `json:"state"`
	StateReason string          `json:"state_reason"`
	User        *User           `json:"user"`
	Labels      []Label         `json:"labels"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	ClosedAt    *time.Time      `json:"closed_at"`
	URL         string          `json:"url"`
	HTMLURL     string          `json:"html_url"`
	Comments    int             `json:"comments"`
	Locked      bool            `json:"locked"`
	PullRequest *PullRequestRef `json:"pull_request"`
}

type Comment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	User      *User     `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	URL       string    `json:"url"`
	HTMLURL   string    `json:"html_url"`
	IssueURL  string    `json:"issue_url"`
}

func (u *User) LoginOrEmpty() string {
	if u == nil {
		return ""
	}
	return u.Login
}

func (u *User) TypeOrEmpty() string {
	if u == nil {
		return ""
	}
	return u.Type
}
