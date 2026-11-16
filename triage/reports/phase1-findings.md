# Phase 1 — What SolidCraft's issue-reporting pipeline does today

**Scope:** `tangency-labs/solidcraft`, checkout on `main`, read-only.
**Method:** source and docs read only. No builds, tests, GitHub writes, or project files. The local triage DB exists and is reserved for Phase 2.
**Legend:** **[V]** verified in the named file; **[I]** inference from the source; **[U]** could not verify from this checkout.

---

## 1. The short version

- **The desktop app's default transport is a private ticket server, not GitHub.** `Report a bug`, Help › Report an Issue, a dialog's `Create Issue…`, the status toast's `Report…`, and a crash report's `Report…` all open the same in-app form. It creates a draft on `tickets.solidcraft.io`, uploads ticked attachments, and opens the draft page in the browser **[V]** (`crates/ui-egui/src/issue_report/ticket.rs:34-37`, `crates/ui-egui/src/tickets.rs:1-14`, `docs/problem-reports.md:1-44`).
- **GitHub is the fallback and an explicit preference.** With `report_on_github` on, or where the ticket HTTP service is absent (notably the web build), the form opens a prefilled `github.com/.../issues/new` URL and copies the full text to the clipboard when the URL is too long **[V/I]** (`crates/ui-egui/src/issue_report.rs:655-665`, `:966`; `crates/ui-egui/src/prefs.rs:1085-1087`; `apps/solidcraft/src/main.rs:763`; `docs/problem-reports.md:76-81`).
- **Crash reporting is mature and local.** Panics, kernel panics the guard catches, unclean exits, and native Windows/macOS crashes leave a local folder with `report.json`, and when possible `design.sc` and `replay.json`. Nothing is uploaded; sharing is a manual reviewed draft **[V]** (`crates/engine/src/crash.rs:1-34`; `docs/crash-reports.md:1-7`).
- **Non-fatal operation failures are not treated as reports.** The command returns an `EngineError`; the UI shows a dialog error, a status toast, or a timeline feature mark. There is no automatic local report and no creation-site stack/context record. The error type is converted to a `String` at layer boundaries **[V]** (`crates/engine/src/lib.rs:77-136`, `:1850-2044`; `crates/doc/src/eval.rs:1454-1463`).
- **The best existing reproduction data is the whole design file plus the command.** A failed operation's report can carry the full `.sc` (off by default) and the command/params; a crash report carries the design from before the command and a replay script. There is no minimal-operand extraction **[V]** (`crates/ui-egui/src/issue_report/ticket.rs:120-175`; `crates/engine/src/crash.rs:700-736`, `:1097-1142`; `docs/problem-reports.md:61-69`).
- **Wrong results are only partly covered.** The kernel can run SMLib's full validity check after each feature (default `each_feature`), and a body it refuses fails the feature. That check is used for validity, not as a “succeeded but wrong” report trigger, and the timeline error mark has no report action **[V]** (`crates/kernel/src/validation.rs:1-40`; `crates/ui-egui/src/prefs.rs:1274-1277`; `crates/ui-egui/src/timeline.rs:574-700`).

---

## 2. Entry points

| Entry point                          | What it opens                                                     | Failure kinds it can serve                | Evidence                                                                                                                                                                                |
| ------------------------------------ | ----------------------------------------------------------------- | ----------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Title-bar / app-bar **Report a bug** | In-app ticket form (default), GitHub draft (fallback/preference)  | all; generic report                       | `crates/ui-egui/src/toolbar.rs:256-263`; `crates/ui-egui/src/issue_report.rs:667-673`, `:966`                                                                                           |
| Help › **Report an Issue…**          | Same form; captures the open dialog's error and the preview error | non-fatal, wrong result, general          | `crates/ui-egui/src/help.rs:81-84`, `:152`                                                                                                                                              |
| Dialog **Create Issue…**             | Same form, pre-filled for the failed dialog operation             | non-fatal operation failure               | `crates/ui-egui/src/dialogs.rs:3706-3709`                                                                                                                                               |
| Status toast **Report…**             | Same form, with the toast's error text                            | non-fatal command failure                 | `crates/ui-egui/src/viewport.rs:7094-7121`; only when `ticket::enabled` is true (`:7095`)                                                                                               |
| Crash Reports window **Report…**     | Crash-origin form; can attach the crash JSON                      | fatal/unclean/kernel panic                | `crates/ui-egui/src/crash_ui.rs:158-162`                                                                                                                                                |
| Agent `diagnostics.report_issue`     | App-side report dialog or direct ticket draft                     | agent-reported any kind; requires the app | `crates/engine/src/cmd/diagnostics.rs:24-45`, `:237-265`; `crates/ui-egui/src/tickets/agent.rs:56-140`                                                                                  |
| GitHub issue forms                   | Public GitHub issue                                               | non-code, bug/feature                     | `.github/ISSUE_TEMPLATE/bug.yml:1-43`; `.github/ISSUE_TEMPLATE/feature.yml`                                                                                                             |
| `solidcraft-cli crashes`             | Lists/reads/replays **local** crash reports; it does not file one | fatal only                                | `apps/solidcraft-cli/src/crashes.rs:1-24`; `docs/crash-reports.md:63-85`                                                                                                                |
| Web build / no ticket service        | GitHub draft fallback                                             | same as app, public                       | **[I]** `crates/ui-egui/src/issue_report/ticket.rs:34-37`, `crates/ui-egui/src/issue_report.rs:966`; `tickets_http` is installed in the desktop app (`apps/solidcraft/src/main.rs:763`) |

**Non-obvious gaps in entry points**

- **Timeline feature errors have no report action.** They show an error mark and tooltip (`crates/ui-egui/src/timeline.rs:574-700`), but the only `Create Issue…` is in dialog errors (`crates/ui-egui/src/dialogs.rs:3706`). A feature that fails during a rebuild, import, or parameter change is therefore less reportable than a dialog failure.
- **The status-toast Report… action is ticket-only.** `viewport.rs:7095` requires `ticket::enabled(app)`, so the GitHub fallback has no action on the toast; the user must use the title-bar or Help entry.
- **Crash install is desktop-only.** `solidcraft_engine::crash::install` is called in the desktop app (`apps/solidcraft/src/main.rs:1287-1295`); the code says tests, the CLI and the web build never install it (`crates/engine/src/crash.rs:18-20`). Native handlers are Windows/macOS only (`apps/solidcraft/src/main.rs:1300-1305`). Linux native crashes rely on the unclean-exit journal only.
- **Many UI command calls discard errors** with `let _ = app.run(...)` (examples: `crates/ui-egui/src/timeline.rs:39,400`; `crates/ui-egui/src/tools.rs:981`; `crates/ui-egui/src/browser_prompts.rs:77,81`). Some are only selection/view state; some are real operations. This is a silent-failure surface.
- **A report can be opened from any error string in a dialog**, including a preview error; the dialog passes `Some(&error)` and the current dialog (`crates/ui-egui/src/dialogs.rs:3706-3709`).

---

## 3. Data captured

### 3.1 The in-app ticket form

Defaults are in `payload::Ticks`: **system, repro, log, screenshot on; design and crash off** **[V]** (`crates/ui-egui/src/tickets/payload.rs:27-43`).

| Evidence                     | What it contains                                                                                                                                                    | Source                                                                                                                  |
| ---------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| System info                  | App version, commit, build date, target, OS, arch, renderer (GPU/CPU), GPU name, graphics API, up to 4 adapters, window system, window size and pixel ratio, locale | `crates/ui-egui/src/issue_report/ticket.rs:39-83`                                                                       |
| Failed operation (`Failing`) | Command id, params exactly as the command API takes them, error string, origin (`solidcore-ops`/`solidcore-kernel`/`app`), code (`kind (id)`)                       | `crates/ui-egui/src/tickets/payload.rs:45-58`; `crates/ui-egui/src/issue_report/ticket.rs:155-175`                      |
| Repro/session                | `session_start` (opened/new), design SHA-256, format version, undo count, redo count, commit                                                                        | `crates/ui-egui/src/issue_report/ticket.rs:122-150`                                                                     |
| Log                          | Last 300 lines of the design's command log and Messages, home folder replaced by `~`                                                                                | `crates/ui-egui/src/issue_report/ticket.rs:86-103`                                                                      |
| Screenshot                   | Current 3D view as PNG; taken locally; not auto-uploaded to GitHub                                                                                                  | `crates/ui-egui/src/issue_report/ticket.rs:196-215`; `crates/ui-egui/src/issue_report/form.rs:31-115`                   |
| Design                       | Entire open `.sc` file, with history; off by default                                                                                                                | `crates/ui-egui/src/issue_report/ticket.rs:120-150`, `:284-296`; `crates/ui-egui/src/tickets/payload.rs:235-239`        |
| Crash                        | Crash report id + full JSON; off by default                                                                                                                         | `crates/ui-egui/src/issue_report.rs:543-614`; `crates/ui-egui/src/issue_report/ticket.rs:331-334`; `payload.rs:240-244` |
| Consent                      | Ticked kinds, time, client, `via: dialog                                                                                                                            | agent`, `confirmed_by` when an agent request was confirmed by the person                                                | `crates/ui-egui/src/tickets/payload.rs:176-204` |

**User-typed:** type (bug/feature/question), title, description. The failure template pre-fills title (`<Command>: <error>`) and description (failed command, key settings, error, “What were you trying to do?”) **[V]** (`crates/ui-egui/src/issue_report/ticket.rs:177-194`, `:226-258`).

**Automatic, but only when the report opens:** command parameters are rebuilt from the dialog by `apply_commands` at report time, not at failure time; `failed_command` is captured then **[V]** (`crates/ui-egui/src/issue_report.rs:469-538`). If the dialog/model changed after the failure, this is not a creation-site record.

### 3.2 The GitHub fallback

- Title max 160; body max 24,000 chars; URL limit 6,000 and sign-in limit 7,000; if the body does not fit, only the title goes in the URL and the body is copied to the clipboard **[V]** (`crates/ui-egui/src/issue_report.rs:20-30`, `:655-665`).
- It includes the user's answers and the collected technical text, but **not the design bytes** and **not the screenshot automatically**; the UI provides Copy/Save screenshot **[V]** (`crates/ui-egui/src/issue_report/form.rs:31-115`; `docs/crash-reports.md:9-16`).
- The GitHub path is public by definition; the form says reports are public and a GitHub account is required (`crates/ui-egui/src/issue_report/form.rs:173-178`).

### 3.3 Crash reports (local)

- Folder `crashes/<time>-<signature>/` with `report.json`; when known `design.sc`, `replay.json`, and on Windows a `minidump.dmp` **[V]** (`crates/engine/src/crash.rs:33-34`, `:1150-1230`).
- `report.json` carries: format/id/kind/signature/count/first/last seen; app version/commit/build; OS/arch/environment; session uptime; message/location/thread/backtrace; command and params; base; up to 32 recent commands; files; reproduce command; notes; startup phase; native record **[V]** (`crates/engine/src/crash.rs:1200-1221`).
- Backtrace frames strip the panic machinery, home folder, rustc paths; capped at 60 **[V]** (`crates/engine/src/crash.rs:298-335`).
- Signature is place + message with digits ignored; repeats count instead of writing again **[V]** (`crates/engine/src/crash.rs:221`, `:1-34`).
- Cost: two small unbuffered appends per top-level command; 512 KB journal; params capped 4 KB; backtrace only on the first signature **[V]** (`crates/engine/src/crash.rs:18-23`, `:759-850`).

### 3.4 What is not captured

- **No stack trace at the creation site of a non-fatal error**; only panics get a backtrace.
- **No stable error code/origin for ordinary command failures** in `Failing`: `origin` and `code` are set only for crash-origin reports (`crates/ui-egui/src/issue_report/ticket.rs:155-174`). Normal view failures get `None`.
- **No exact operand bodies/features** of the failing operation; only the whole design and, for dialogs, the command params.
- **No failure-site document snapshot** for non-fatal failures; the design is whatever is open when the report opens. The engine does keep the `before` snapshot and restores it on failure (`crates/engine/src/lib.rs:1876-1882`, `:2021-2030`), but the report does not take it.
- **No full command history for non-fatal failures** in the report. The crash journal records recent commands (including failed ones with a 300-char error) but is only used for crash reports (`crates/engine/src/crash.rs:759-850`, `:1097-1142`).
- **No structured kernel context** such as tolerances, face/edge IDs, or the exact `KernelError` variant; most is stringified before the UI (`crates/engine/src/lib.rs:103-136`).

---

## 4. Transport and destination

### Default: ticket server

- Server: `https://tickets.solidcraft.io`; test override `SOLIDCRAFT_TICKETS_URL` (HTTPS, or HTTP to localhost only) **[V]** (`crates/ui-egui/src/tickets/api.rs:9-25`).
- Send path: create draft (`POST /api/drafts`), open resumable uploads (`POST /api/uploads`, SHA-256), upload parts (`PUT /api/uploads/:id/parts/:n`), complete (`POST /api/uploads/:id/complete`), open the draft page in the browser **[V]** (`crates/ui-egui/src/tickets/api.rs:150-212`; `crates/ui-egui/src/tickets.rs:1-14`; `docs/problem-reports.md:28-44`).
- No app account or token; the draft's upload token stays in memory, and no token is in the ticket **[V]** (`crates/ui-egui/src/tickets.rs:1-14`; `docs/problem-reports.md:39-40`).
- Offline/transient failure: the report waits in `<data>/report-queue`, at most 20 reports / 1 GB, and retries on user action **[V]** (`crates/ui-egui/src/tickets/queue.rs:1-24`, `:37-70`; `docs/problem-reports.md:42-44`).
- Failure text/progress: Sending/Cancel, Thanks/link, error/Retry/Copy details **[V]** (`crates/ui-egui/src/issue_report/ticket.rs:349-438`).
- Server implementation, retention, access control, and encryption are **not in this checkout** **[U]**; `docs/problem-reports.md:4-6` states private-by-default with reporter/triager/admin access.

### Fallback: GitHub

- URL `https://github.com/tangency-labs/solidcraft/issues/new?title=…&body=…`; no direct API call; browser and clipboard only **[V]** (`crates/ui-egui/src/issue_report.rs:25`, `:655-665`; `docs/crash-reports.md:9-23`).
- Blank issues are deliberately enabled so the app URL works **[V]** (`.github/ISSUE_TEMPLATE/config.yml:1-3`).
- If the browser does not open, the user copies the text; failure behavior is advisory, not a submitted report **[V]** (`docs/crash-reports.md:25-31`).

---

## 5. Destination templates, labels and bots

### GitHub forms

- Bug form: `bug` label; required “What happened”, “Steps to reproduce”, version/OS; optional screenshots/recording; points at `solidcraft-cli script` and the `solidcraft-reporting` skill **[V]** (`.github/ISSUE_TEMPLATE/bug.yml:1-43`).
- Feature form: `enhancement` label; goal and idea **[V]** (`.github/ISSUE_TEMPLATE/feature.yml:1-16`).
- Blank issues enabled for the app's `issues/new` link **[V]** (`.github/ISSUE_TEMPLATE/config.yml:1-3`).
- There is **no crash or failed-operation form**; the app uses blank issues, and the triage bot recognizes the app's sections instead **[V]** (`scripts/triage/needs-info.js:20-23`; `scripts/triage/triage.js:61-65`).

### Triage bot

- `.github/workflows/triage.yml` runs `scripts/triage` on issue open/edit/comment/label and a daily sweep (`triage.yml:1-86`).
- New issue labels: crash title → `bug` + `priority:P1`; in-app failure title (`<COMMAND>: operation failed` / `unexpected result`) or `## Actual behavior` → `bug`; feature title → `enhancement`; area from title words, command prefix, or the app's `Settings and selection`/`Command` section; no priority → `needs-triage` **[V]** (`scripts/triage/triage.js:51-117`, `:156-185`).
- Area labels are coarse: `area:kernel`, `area:sketch`, `area:doc`, `area:io`, `area:render`, `area:ui`, `area:infra` **[V]** (`scripts/triage/triage.js:14-59`).
- `needs-info`: short hand-written bug reports are asked once for steps (and version if absent), then closed after 14 days; app reports with `Settings and selection`, `Backtrace` or `Recent commands` are exempt; enhancement and app reports are not asked **[V]** (`scripts/triage/needs-info.js:20-49`, `:74-90`, `:104-180`).
- `/triage` lets maintainers add/remove labels and set `P0`–`P3`; it only adds labels automatically, so corrections stick **[V]** (`scripts/triage/triage.js:1-4`, `:202-271`).

### Documentation drift

- `docs/problem-reports.md` documents the current ticket-server flow **[V]**.
- `docs/crash-reports.md:5-7` and `:20-23` still say crash reports are shared through a GitHub issue draft and that desktop/web submit through GitHub. That is stale for the current desktop default; `crates/ui-egui/src/issue_report.rs:1` also still says “never publishes or uploads a report” even though the ticket module uploads. **[V]** (stale text; not a behavior claim).

---

## 6. Privacy and consent

- **Private-by-default ticket** (reporter/triagers/admins) versus **public GitHub issue**; the form says reports are public and a GitHub account is required **[V]** (`docs/problem-reports.md:4-6`; `crates/ui-egui/src/issue_report/form.rs:173-178`).
- **Defaults collect:** system info, repro/session hashes, last log lines, screenshot. **Design and crash JSON are off** until ticked. Consent records exactly the ticked kinds **[V]** (`crates/ui-egui/src/tickets/payload.rs:27-43`, `:176-204`).
- **Design warning:** a failure report with the design box off says plainly the failure probably cannot be reproduced without it **[V]** (`crates/ui-egui/src/issue_report/ticket.rs:284-296`; `docs/problem-reports.md:13-16`).
- **Redaction implemented:** home folder → `~` in logs/messages (`crates/ui-egui/src/issue_report/ticket.rs:86-103`; `crates/ui-egui/src/issue_report.rs:402-413`); crash backtrace home and rustc paths (`crates/engine/src/crash.rs:300-335`); design **file name** strips path separators (`crates/ui-egui/src/issue_report/ticket.rs:106-118`).
- **Not redacted / not previewed:**
  - The server request adds **hostname** (`COMPUTERNAME`/`HOSTNAME`/`/etc/hostname`) to the draft body outside the ticket's `consent.kinds` field and outside the visible system-info preview **[V]** (`crates/ui-egui/src/tickets/api.rs:135-158`; `docs/problem-reports.md:18-24`).
  - Logs and messages redact only the home folder; other absolute paths, usernames, project names, server paths, and proprietary names can remain.
  - Feature names, parameter names, body names, dimensions, and error text are included; these can carry proprietary design meaning.
  - Screenshot is on by default and can show proprietary geometry; the user can remove/retake.
  - The queue stores `report.json` and attachment files (including the design if ticked) unencrypted on disk **[V]** (`crates/ui-egui/src/tickets/queue.rs:1-24`, `:37-70`; `docs/problem-reports.md:42-44`).
- **Agent consent:** an agent can ask for a report; design and crash are never sent on the agent's word. The dialog opens with those boxes off; only the person can tick them **[V]** (`crates/ui-egui/src/tickets/agent.rs:56-140`; `docs/problem-reports.md:46-59`).
- **No telemetry:** network only on send/retry or an agent request; crash reports are local and never sent **[V]** (`docs/problem-reports.md:71-74`; `crates/engine/src/crash.rs:1-34`).
- **Old keyring token** from test builds is deleted once; the app keeps no account token **[V]** (`crates/ui-egui/src/tickets.rs:20-24`, `:124-136`; `docs/problem-reports.md:39-40`).

---

## 7. Reproducibility support that already exists

| Asset                        | What it gives                                                                                                | Where                                                                                                                               |
| ---------------------------- | ------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------- |
| `.sc` design file            | Full document with history; lossless enough to rebuild the design; SHA-256 in report                         | `crates/ui-egui/src/issue_report/ticket.rs:122-150`; `docs/problem-reports.md:61-69`                                                |
| Failed command + params      | Replay the one operation on the attached design                                                              | `crates/ui-egui/src/tickets/payload.rs:45-58`; `crates/ui-egui/src/issue_report/ticket.rs:155-194`; `docs/problem-reports.md:61-69` |
| Crash design + `replay.json` | Design before the command plus the command list; `solidcraft-cli crashes replay <id>`                        | `crates/engine/src/crash.rs:700-736`, `:1097-1142`; `docs/crash-reports.md:63-85`                                                   |
| Crash journal                | Last 512 KB of top-level commands with params/error, base design/autosave; 32 recent commands in reports     | `crates/engine/src/crash.rs:18-23`, `:759-850`, `:1097-1142`                                                                        |
| Undo snapshots               | Up to 200 `Snapshot`s with full `Document`                                                                   | `crates/engine/src/lib.rs:318-328`, `:803`, `:1996-2001`                                                                            |
| Command script export        | `timeline.export_script` writes a design's history as a command script; `solidcraft-cli run/eval` replays it | `crates/engine/src/cmd/script.rs:1-31`, `:186`; `docs/problem-reports.md:61-69`                                                     |
| Validation                   | SMLib full check after each step/feature; default `each_feature`; can be set by preference/command           | `crates/kernel/src/validation.rs:1-40`; `crates/ui-egui/src/prefs.rs:1274-1277`; `crates/engine/src/cmd/diagnostics.rs:20-45`       |
| Profiler                     | `diagnostics.profile` and `SOLIDCRAFT_PROFILE=1`; command/feature/import spans                               | `crates/engine/src/cmd/diagnostics.rs:47-70`                                                                                        |

**Gaps in reproducibility:** the report captures the design at **report-open** time, not failure time; there is no minimal operand extraction; non-fatal failures do not produce a replay automatically; the crash journal is not attached to non-crash reports; undo snapshots are not serialized into reports; error-site stack/context is absent.

---

## 8. Error-handling architecture (non-fatal failures)

### 8.1 Error types per layer

| Layer                             | Type             | What it carries                                                                                                                                                                |
| --------------------------------- | ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `crates/smlib`                    | `Error`          | SMLib status code, argument/empty/not-solid, pick mismatch, no-room, too-large, wall folds, corner refusals, unsupported, solidcore error, internal panic, unported, cancelled |
| `crates/kernel`                   | `KernelError`    | `Invalid`, `Failed`, `TooLarge`, `Internal`, `Lip`; structured fields live in the cause/`solidcore_ops` refusals                                                               |
| `crates/doc`                      | `DocError`       | expression/invalid/unknown, sketch, kernel, `Reselect`, `LostProjections`                                                                                                      |
| `crates/io`                       | `IoError`        | format/empty/invalid/kernel/doc                                                                                                                                                |
| `crates/engine`                   | `EngineError`    | panel/unknown/disabled/bad-params/other/plugin/refused/internal/cancelled                                                                                                      |
| `crates/sketch`, `dynamics`, etc. | their own errors | converted at their boundaries                                                                                                                                                  |

References: `crates/smlib/src/lib.rs:33-120`; `crates/kernel/src/lib.rs:254-315`; `crates/doc/src/lib.rs:120-150`; `crates/io/src/lib.rs:137-150`; `crates/engine/src/lib.rs:77-101`.

### 8.2 Where creation context is lost

1. **Kernel → engine.** `impl From<KernelError> for EngineError` preserves only `KernelError::Internal`; every other variant becomes `EngineError::Other(e.to_string())` **[V]** (`crates/engine/src/lib.rs:116-124`). The kernel's structured refusal (`TooLarge` with size/point, `NoRoom`, `WallFolds`, `TurnRefused`, `Refused(solidcore_ops::Error)`, etc.) is already turned into a display string. `crates/kernel/src/lib.rs:254-315` names the variants; `crates/kernel/src/lib.rs:419-434` catches panics.
2. **Doc/Sketch/Io → engine.** `DocError::Kernel` is forwarded, but every other `DocError` becomes `Other(e.to_string())`; `SketchError` and `IoError` likewise **[V]** (`crates/engine/src/lib.rs:103-113`, `:133-136`).
3. **Engine → UI.** `SolveApp::run` maps `EngineError` to `String` and returns it to the caller **[V]** (`crates/ui-egui/src/lib.rs:1462-1535`). `Session::execute` has the command id, params and `before` document, but it only records them for crash reporting; the returned error has no context **[V]** (`crates/engine/src/lib.rs:1850-2044`).
4. **Feature evaluation.** `FeatureResult` stores only `error: Option<String>` and `internal: bool` (plus warnings/import/repair/lost projections); the evaluator has the feature and state when it creates the error, but the result does not carry a creation-site record **[V]** (`crates/doc/src/eval.rs:1454-1475`, `:3800-3860`).
5. **Dialog refusal.** `Dialog::refuse` stores the commands/picks/revision in `Refused`, and the error string in `Dialog::error`; it does not store an error origin/code/stack **[V]** (`crates/ui-egui/src/dialogs.rs:805-817`, `:846-849`).
6. **Report-time reconstruction.** `draft_for` rebuilds the failed command from the dialog's _current_ inputs (`apply_commands`) and stores `failed_command` then; `failing_of` for a normal view failure sets `origin: None, code: None` **[V]** (`crates/ui-egui/src/issue_report.rs:469-538`; `crates/ui-egui/src/issue_report/ticket.rs:155-174`).

### 8.3 Where the user sees a failure

| Path                                              | What is shown                                                                                              | Report affordance                                                                                                                 |
| ------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| Open feature dialog / preview failure             | Red error row; guidance/refusal cases are shown plainly without a report button; otherwise `Create Issue…` | `crates/ui-egui/src/dialogs.rs:3628-3721`                                                                                         |
| Command run from toolbar/shortcut, error returned | Status toast; `Report…` only when `ticket::enabled`                                                        | `crates/ui-egui/src/viewport.rs:7094-7121`; `crates/ui-egui/src/lib.rs:3043-3047`                                                 |
| Timeline feature error after rebuild              | Error mark + tooltip with feature name/type/error; no report button                                        | `crates/ui-egui/src/timeline.rs:574-700`                                                                                          |
| Import warnings                                   | Warning mark/summary; no report button                                                                     | `crates/ui-egui/src/timeline.rs:670-750`                                                                                          |
| Background feature edit                           | Status message; `ended(...Failed)`                                                                         | `crates/ui-egui/src/feature_edit.rs:150-170`                                                                                      |
| Preview panic                                     | Generic `"the preview failed"` from `catch_unwind`; stack lost                                             | `crates/ui-egui/src/preview.rs:693-698`, `:741-742`                                                                               |
| Silent callers                                    | Errors discarded with `let _ = app.run(...)`; examples in timeline/tools/browser prompts                   | `crates/ui-egui/src/timeline.rs:39`, `:400`; `crates/ui-egui/src/tools.rs:981`; `crates/ui-egui/src/browser_prompts.rs:77`, `:81` |

### 8.4 What context exists at the failure site

| Context                 | Exists at creation?                                       | Survives to UI/report?                                                  |
| ----------------------- | --------------------------------------------------------- | ----------------------------------------------------------------------- |
| Command id              | `Session::execute` has `spec.id`                          | Yes, in dialog report if the dialog is open; no in generic error        |
| Command params          | `Session::execute` has `given`/`params`                   | Rebuilt from the dialog at report time; no creation-site copy           |
| Document before command | `Snapshot before` at `crates/engine/src/lib.rs:1876-1882` | No; restored on failure, not attached to report                         |
| Feature id/name/kind    | Evaluator has `f`; `FeatureResult` has id/name            | Id/name/error in report; kind only for selected/latest/failing-all list |
| Kernel error variant    | `KernelError` at creation; `smlib::Error` status          | Mostly lost to a string at engine boundary                              |
| Error code/origin       | `smlib::Error::Status`, kernel variant                    | Only crash reports derive `origin`/`code`; normal failures `None`       |
| Tolerances/topology     | Kernel/body records                                       | Not in `EngineError`/report                                             |
| Stack/creation site     | Panics: crash backtrace; ordinary errors: none            | Crash only                                                              |
| Recent commands         | Crash journal, 512 KB, top-level app design only          | Crash report only; not normal report                                    |
| Undo stack              | Up to 200 full snapshots                                  | Count in report; snapshots not attached                                 |

### 8.5 Diagnostics already present

- **Crash reports/backtraces:** panic hook, kernel guard, Rayon worker holding, native handlers; local only.
- **Crash journal:** top-level command journal with params (4 KB cap), error (300 chars), base; 32 recent commands in report.
- **Feature errors/warnings:** `FeatureResult` error/warning/import report/lost projections; tooltip and Messages.
- **Messages log:** 200 entries, 4,000 chars each; report takes last 8 in `draft_for` (and last 300 log lines in ticket evidence).
- **Profiler/validation:** command/feature/kernel spans; `diagnostics.profile`, `diagnostics.validation`.
- **No structured non-fatal error log:** no per-error file, ring buffer, or error-code table beyond enums/strings.

### 8.6 Cost of capture

- Crash backtrace is captured only on the first panic signature, not every error; journal is two appends per top-level command **[V]** (`crates/engine/src/crash.rs:18-23`, `:700-736`).
- A backtrace on every non-fatal error would be new cost in hot paths: booleans/blends/sweeps run in per-feature/per-step loops (`crates/kernel/src/validation.rs:1-40`; `crates/kernel/src/blend.rs`/`smlib_boolean.rs` validation steps). There is no measurement in this checkout for that cost **[U]**.
- The codebase does use errors for expected refusals (e.g., largest-fit searches on `TooLarge`) **[V]** (`crates/kernel/src/blend.rs:791-969`), so unconditional stack capture would be noisy and expensive there.

---

## 9. Failure-kind matrix (what the pipeline does today)

| Kind                            | Detection                                                                                | Local artifact                        | Report path                                                                | Main missing context                                                                                |
| ------------------------------- | ---------------------------------------------------------------------------------------- | ------------------------------------- | -------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| **Fatal crash**                 | Panic hook, kernel guard, unclean-exit journal, native Windows/macOS                     | `report.json`, design/replay/minidump | Crash Reports `Report…`; ticket can attach crash JSON; GitHub draft cannot | Creation-site non-panic context not applicable; native Linux not handled; `design.sc` may be absent |
| **Non-fatal operation failure** | `EngineError` returned by `Session::execute`                                             | None automatic                        | Dialog `Create Issue…`, status toast `Report…` (ticket only)               | Error origin/code, creation stack, exact failure-time inputs/document, operand bodies               |
| **Wrong result**                | SMLib validity check (`each_feature` default) can fail a feature; otherwise user notices | None automatic                        | User-initiated report; timeline mark has no button                         | No “succeeded but invalid” capture; no check output in report; no automatic flag                    |
| **Non-code**                    | None                                                                                     | N/A                                   | GitHub bug/feature forms or ticket form                                    | No code context expected; templates/bot handle                                                      |

---

## 10. Facts vs. unknowns / flags for Phase 2

- **Verified:** the ticket-server default, GitHub fallback, consent boxes, crash report content, error type conversions, and the absence of non-fatal creation-site stacks.
- **Inferred:** the web build has no ticket HTTP service and therefore uses the GitHub fallback; the desktop app installs the ticket service. The service wiring is visible only in the desktop app (`apps/solidcraft/src/main.rs:763`).
- **Unverified from this checkout:** the ticket server's storage, retention, access control, encryption, and deletion behavior; whether `hostname` is stored or displayed server-side; whether GitHub's blank-issue URL is the only app path in the web build.
- **Documentation inconsistency:** `docs/crash-reports.md` describes GitHub as the submission path and says desktop/web submit through GitHub; `docs/problem-reports.md` and the ticket code describe the ticket server default. `crates/ui-egui/src/issue_report.rs:1` says “never publishes or uploads a report” despite the upload path.
- **Coverage gaps to quantify in Phase 2:** how many issues are crash vs non-fatal vs wrong-result vs non-code; how many have error text, operation, inputs, model/replay; top normalized error messages; whether app templates/bot fields are filled or bypassed.

---

## Appendix: file index used

- Reporting UI: `crates/ui-egui/src/issue_report.rs`, `issue_report/form.rs`, `issue_report/ticket.rs`, `tickets.rs`, `tickets/api.rs`, `tickets/payload.rs`, `tickets/queue.rs`, `tickets/agent.rs`, `crash_ui.rs`, `help.rs`, `toolbar.rs`, `dialogs.rs`, `viewport.rs`, `timeline.rs`, `preview.rs`
- Engine/crash: `crates/engine/src/crash.rs`, `crates/engine/src/lib.rs`, `crates/engine/src/cmd/diagnostics.rs`, `crates/engine/src/report_issue.rs`
- Errors/kernel: `crates/smlib/src/lib.rs`, `crates/kernel/src/lib.rs`, `crates/kernel/src/validation.rs`, `crates/doc/src/lib.rs`, `crates/doc/src/eval.rs`, `crates/io/src/lib.rs`
- App/platform: `apps/solidcraft/src/main.rs`, `apps/solidcraft/src/native_crash.rs`, `apps/solidcraft/src/native_crash_macos.rs`, `apps/solidcraft-cli/src/crashes.rs`
- GitHub/triage: `.github/ISSUE_TEMPLATE/bug.yml`, `feature.yml`, `config.yml`, `.github/workflows/triage.yml`, `scripts/triage/triage.js`, `scripts/triage/needs-info.js`
- Docs: `docs/problem-reports.md`, `docs/crash-reports.md`, `docs/capabilities.md`, `docs/roadmap-detail.md`
