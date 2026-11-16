# SolidCraft issue-reporting pipeline: what exists and what to change

**Scope:** `tangency-labs/solidcraft` on `main`, read-only. External ticket server treated as in scope. Open issues only in the backlog analysis.
**Artifacts:** this report, `reports/phase1-findings.md`, `analysis/phase2-evidence.txt`, `analysis/phase2-operation-areas.txt`, `analysis/open-failure-kinds.tsv`.
**Method:** source and docs read only; no builds/tests in the project; no GitHub writes. Backlog numbers are from the local triage DB (6,477 open issues), classified by deterministic rules and calibrated on a stratified hand-read sample. See Method notes.

---

## 1. Executive summary

- The desktop app now defaults to a **private ticket server**, not GitHub. GitHub is an opt-in/fallback draft. Crash reports remain local-only until shared.
- **Non-fatal operation failures are the main gap:** 65% of open issues, but only 60% carry an error text, 66% name the operation, 64% carry parameters, 7% attach a model, and **0.4% carry a stack trace**. The error's creation site is lost at layer boundaries.
- Crash reporting already has what non-fatal failures need: a bounded journal, command/params, design-before-command, replay, backtrace, and signature grouping. It is not reused for ordinary failures.
- The report dialog's evidence is strong for system/params/screenshot/log, but it captures the **current** design at report-open time, not the failure-time operands or document state.
- The biggest privacy defect: the ticket request sends the machine **hostname** outside the visible consent record, and only the home folder is redacted from logs/errors. The offline queue writes attachments to disk with ordinary file permissions.
- Wrong-result reports are sparse and poorly evidenced: only 4% carry an error text, 2% a model, and under 1% an error code. Most "unexpected result" reports leave the app's placeholder text unfilled.

---

## 2. What exists today

### 2.1 Entry points

| Entry                                | What opens                                                                | Notes                                                                                                  |
| ------------------------------------ | ------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| Title bar / app bar **Report a bug** | In-app ticket form; GitHub draft if ticket service is absent or preferred | `crates/ui-egui/src/toolbar.rs:256-263`; `crates/ui-egui/src/issue_report.rs:667-673`, `:966`          |
| Help › **Report an Issue…**          | Same form; captures the open dialog's error and preview error             | `crates/ui-egui/src/help.rs:81-84`, `:152`                                                             |
| Dialog **Create Issue…**             | Same form, pre-filled for a failed dialog operation                       | `crates/ui-egui/src/dialogs.rs:3706-3709`                                                              |
| Status toast **Report…**             | Same form, with the toast's error                                         | `crates/ui-egui/src/viewport.rs:7094-7121` — only when `ticket::enabled` (`:7095`)                     |
| Crash Reports **Report…**            | Crash-origin form; can attach crash JSON                                  | `crates/ui-egui/src/crash_ui.rs:158-162`                                                               |
| Agent `diagnostics.report_issue`     | App-side report dialog or direct draft; requires the app                  | `crates/engine/src/cmd/diagnostics.rs:24-45`, `:237-265`; `crates/ui-egui/src/tickets/agent.rs:56-140` |
| GitHub bug/feature forms             | Public issue, labels `bug` / `enhancement`                                | `.github/ISSUE_TEMPLATE/bug.yml:1-43`; `feature.yml`; `config.yml:1-3`                                 |
| `solidcraft-cli crashes`             | Lists/reads/replays local crashes; does not file                          | `apps/solidcraft-cli/src/crashes.rs:1-24`; `docs/crash-reports.md:63-85`                               |

**Non-obvious:** timeline feature errors and import warnings have **no report action** (`crates/ui-egui/src/timeline.rs:574-700`); the toast's report action exists only on the ticket path; crash installation is desktop-only (`crates/engine/src/crash.rs:18-20`; `apps/solidcraft/src/main.rs:1287-1305`), and native handlers are Windows/macOS only. Many UI calls discard `app.run` errors with `let _ =` (examples: `crates/ui-egui/src/timeline.rs:39,400`; `crates/ui-egui/src/tools.rs:981`; `crates/ui-egui/src/browser_prompts.rs:77,81`).

### 2.2 What is captured

The in-app ticket defaults (`crates/ui-egui/src/tickets/payload.rs:27-43`): **system, repro, log, screenshot on; design and crash off**.

| Evidence         | Contents                                                                                                        | Source                                                             |
| ---------------- | --------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| System           | version, commit, build date, target, OS/arch, renderer, GPU, graphics API, adapters, window system/size, locale | `crates/ui-egui/src/issue_report/ticket.rs:39-83`                  |
| Failed operation | command id, params as the API takes them, error string, origin, code                                            | `crates/ui-egui/src/tickets/payload.rs:45-58`; `ticket.rs:155-175` |
| Repro            | session start, design SHA-256, format version, undo/redo counts, commit                                         | `crates/ui-egui/src/issue_report/ticket.rs:122-150`                |
| Log              | last 300 lines of command log + Messages; home folder → `~`                                                     | `ticket.rs:86-103`                                                 |
| Screenshot       | current 3D view PNG, previewed, local only                                                                      | `ticket.rs:196-215`; `issue_report/form.rs:31-115`                 |
| Design           | full `.sc` with history; off by default                                                                         | `ticket.rs:120-150`, `:284-296`; `payload.rs:235-239`              |
| Crash            | crash report id + full JSON; off by default                                                                     | `issue_report.rs:543-614`; `ticket.rs:331-334`                     |
| Consent          | ticked kinds, time, client, `via`, `confirmed_by`                                                               | `payload.rs:176-204`                                               |

**The key limitation:** for a failed dialog, the command is rebuilt from the dialog's _current_ inputs at report-open time (`issue_report.rs:469-538`). It is not captured where the error was created. For a timeline failure, only the feature name/type/error is shown in the tooltip; the report has the currently open design, not the failure-time state.

### 2.3 Transport

- **Desktop default:** `POST /api/drafts` on `https://tickets.solidcraft.io`, resumable uploads with SHA-256, then open the draft page in the browser. No app account or token. Offline reports wait in `<data>/report-queue` (20 reports / 1 GB). See `crates/ui-egui/src/tickets/api.rs:9-25`, `:150-212`; `crates/ui-egui/src/tickets/queue.rs:1-70`; `docs/problem-reports.md:28-44`.
- **GitHub fallback:** `https://github.com/tangency-labs/solidcraft/issues/new?title=…&body=…`; title 160, body 24,000, URL 6,000, sign-in URL 7,000. If too long, only the title goes in the URL and the body is copied to the clipboard. No design bytes; screenshot is manual. `crates/ui-egui/src/issue_report.rs:20-30`, `:655-665`; `docs/crash-reports.md:9-23`.
- **Crash reports:** folder `crashes/<time>-<signature>/` with `report.json`, optional `design.sc`, `replay.json`, minidump. Local only. `crates/engine/src/crash.rs:1-34`, `:1150-1230`; `docs/crash-reports.md:1-7`.
- No direct GitHub API, email, or server upload from the app; no telemetry.

### 2.4 Templates and triage

- GitHub bug form requires “What happened”, “Steps to reproduce”, and version/OS; feature form requires goal/idea; blank issues stay enabled for the app's URL. `.github/ISSUE_TEMPLATE/bug.yml:1-43`; `feature.yml`; `config.yml:1-3`.
- The triage bot adds `bug` + `priority:P1` for crash titles, `bug` for app operation failures, `enhancement` for feature titles, coarse `area:*` from title/command, and `needs-triage` without a priority. `.github/workflows/triage.yml:1-86`; `scripts/triage/triage.js:14-117`, `:156-185`.
- `needs-info` asks short hand-written bug reports once for steps/version, exempts app reports with `Settings and selection`/`Backtrace`/`Recent commands`, and closes after 14 days. `scripts/triage/needs-info.js:1-90`, `:104-180`.
- `docs/problem-reports.md` describes the ticket flow; `docs/crash-reports.md:5-7`, `:20-23` is stale (still says GitHub is the submission path).

### 2.5 Privacy and consent

- Ticket is private by default (reporter/triagers/admins); GitHub is public. `docs/problem-reports.md:4-6`; `crates/ui-egui/src/issue_report/form.rs:173-178`.
- Design and crash JSON are off until ticked; system/repro/log/screenshot are on. Consent records only the ticked kinds. `payload.rs:27-43`, `:176-204`.
- Redaction: home folder → `~` in logs/messages/backtraces; design filename keeps only the basename. `ticket.rs:86-103`; `issue_report.rs:402-413`; `crash.rs:300-335`; `ticket.rs:106-118`.
- **Gaps:** `tickets/api.rs:135-158` adds the machine hostname to the draft request outside `consent.kinds` and outside the visible system preview. Other absolute paths/usernames/project names can remain. The queue writes `report.json` and attachments with ordinary filesystem permissions (`queue.rs:37-70`). If the design is queued, it sits on disk unencrypted until sent or removed.
- No telemetry; network only on send/retry or agent request. `docs/problem-reports.md:71-74`.

### 2.6 Reproducibility already available

| Asset                        | Use                                                                            | Source                                                          |
| ---------------------------- | ------------------------------------------------------------------------------ | --------------------------------------------------------------- |
| `.sc` design                 | full history; SHA-256 in report                                                | `ticket.rs:122-150`; `docs/problem-reports.md:61-69`            |
| Failed command + params      | replay on the design                                                           | `payload.rs:45-58`; `ticket.rs:155-194`                         |
| Crash design + `replay.json` | `solidcraft-cli crashes replay <id>`                                           | `crash.rs:700-736`, `:1097-1142`; `docs/crash-reports.md:63-85` |
| Crash journal                | 512 KB of top-level commands, params/error, base; 32 recent commands in report | `crash.rs:18-23`, `:759-850`, `:1097-1142`                      |
| Undo                         | up to 200 full `Document` snapshots                                            | `engine/src/lib.rs:318-328`, `:803`                             |
| Script export                | `timeline.export_script`; `solidcraft-cli run/eval`                            | `engine/src/cmd/script.rs:1-31`, `:186`                         |
| Validation                   | SMLib full check each step/feature; default `each_feature`                     | `kernel/src/validation.rs:1-40`; `prefs.rs:1274-1277`           |
| Profiler                     | `diagnostics.profile`, `SOLIDCRAFT_PROFILE=1`                                  | `engine/src/cmd/diagnostics.rs:47-70`                           |

Not reused for non-fatal reports: the crash journal, the design-before-command snapshot, `replay.json`, or a creation-site trace.

### 2.7 Error architecture and where context is lost

| Layer    | Error         | What survives                                                                            |
| -------- | ------------- | ---------------------------------------------------------------------------------------- |
| `smlib`  | `Error`       | SMLib status, typed refusals, internal panic                                             |
| `kernel` | `KernelError` | `Invalid`, `Failed`, `TooLarge`, `Internal`, `Lip`; structured causes in `solidcore_ops` |
| `doc`    | `DocError`    | expression/sketch/kernel, `Reselect`, `LostProjections`                                  |
| `io`     | `IoError`     | format/empty/invalid/kernel/doc                                                          |
| `engine` | `EngineError` | panel/unknown/disabled/bad-params/other/plugin/refused/internal/cancelled                |

`crates/engine/src/lib.rs:77-136`: `From<KernelError>` preserves only `Internal`; every other variant becomes `Other(e.to_string())`. `From<DocError>`, `From<SketchError>` and `From<IoError>` do the same. `FeatureResult` stores `error: Option<String>` and `internal: bool` only (`doc/eval.rs:1454-1475`), so a rebuilt feature loses the creation context. `SolveApp::run` maps the error to `String` for the UI (`ui-egui/src/lib.rs:1462-1535`). `Session::execute` has the command id, params and the pre-command snapshot, but only writes them to the **crash** journal (`engine/src/lib.rs:1850-2044`, `:2037-2042`).

The user-visible surfaces are dialog errors with `Create Issue…` (`dialogs.rs:3628-3721`), status toasts with `Report…` only on the ticket path (`viewport.rs:7094-7121`), and timeline marks with a tooltip but no report action (`timeline.rs:574-700`). Preview panics become the generic `"the preview failed"` (`preview.rs:693-698`, `:741-742`); many `let _ = app.run(...)` calls swallow errors.

---

## 3. Evidence from the open issue backlog

**Sample:** all **6,477 open issues** in the local DB (6,431 SolidCraft, 46 solidcore). All failure-kind tags are written to the DB as `failure/*`.
**Classification:** deterministic rules over title/body/sections/labels, calibrated on a stratified hand-read sample. Wrong-result is a **lower bound**; the unknown bucket is mostly internal tasks, empty/placeholder reports, and vague non-English titles.

### 3.1 Failure kinds

| Kind                             | Issues | Share |
| -------------------------------- | -----: | ----: |
| Non-fatal operation failure      |  4,230 | 65.3% |
| Non-code (request/docs/UX/build) |    823 | 12.7% |
| Unknown/insufficient             |    750 | 11.6% |
| Fatal crash                      |    422 |  6.5% |
| Wrong result                     |    252 |  3.9% |

### 3.2 Report format

| Format                                                                          | Issues | Share |
| ------------------------------------------------------------------------------- | -----: | ----: |
| Current app ticket shape (`## Operation context` / `## Settings and selection`) |  2,914 | 45.0% |
| Freeform GitHub issue                                                           |  2,665 | 41.1% |
| Crash draft (`## Backtrace` / `## Recent commands`)                             |    426 |  6.6% |
| GitHub issue form (`### What happened` …)                                       |    363 |  5.6% |
| Older app shape (`## Actual behavior` / `## Reproduction`)                      |    109 |  1.7% |

### 3.3 What is present, by kind

| Evidence                                 | Non-fatal (4,230) | Wrong result (252) | Fatal (422) |
| ---------------------------------------- | ----------------: | -----------------: | ----------: |
| Error text                               |             59.8% |               4.0% |        5.2% |
| Error code/type                          |              4.2% |               0.8% |        9.7% |
| Operation named                          |             66.1% |              31.7% |       37.2% |
| Structured params                        |             64.1% |              59.5% |       75.4% |
| Attached model/minimal file              |              6.6% |               2.0% |       13.7% |
| Reproduction support (steps/script/file) |             47.5% |              65.1% |       42.4% |
| Stack trace                              |              0.4% |               0.0% |        9.0% |
| Log excerpt                              |              0.4% |               0.0% |       74.4% |
| Version                                  |             73.5% |              67.1% |       96.7% |
| OS                                       |             67.7% |              54.4% |       77.7% |

**Most often missing for non-fatal failures:** a model/minimal repro (93.4% missing), a creation-site stack (99.6% missing), an error text (40.2% missing), a reproduction (52.5% missing), and structured params (35.9% missing). The app captures params far better than error or trace.

**App reports specifically (n=2,914):** params 99%, operation 78%, error text 68%, repro support 64%, model 5%, stack 0%. **GitHub forms (n=363):** repro 93%, version 75%, OS 76%, model 16%, error text 33%. **Old-app reports (n=109):** error 16%, operation 19%, params 0%, repro 77%. **Crash (n=426):** log 77%, stack 12%, model 15%, version 96%.

### 3.4 Template fields: filled, ignored, or deleted

- **GitHub form fields are mostly used:** 93% have reproduction, 75% version, 76% OS, 16% a model; no placeholder text remains. The missing 25% version/OS is the clearest compliance gap.
- **App reports leave fields unfilled:** 21.1% still contain the reproduction placeholder (“Describe the steps or a small public-safe example”); 32% have no command error (“No command error was recorded”); 95% have no model; 100% have no stack.
- **Old-app reports are the weakest:** 16% error text, 19% operation, 0% params. Those are legacy reports, but they dominate the “no context” tail.
- No evidence of users deleting template fields wholesale; the GitHub required fields survive. The app's own technical form is the one with unfilled sections.

### 3.5 Repetition and error messages

Grouped by the first clause of the displayed error, the top non-fatal messages are:

| Count | Normalized message                                                                    |
| ----: | ------------------------------------------------------------------------------------- |
|   254 | `boolean Cut: the operation failed: no result`                                        |
|   183 | `boolean Union: the operation failed: no result`                                      |
|    74 | `not supported yet: the blend is larger than the neighbouring faces`                  |
|    55 | `Create Sketch: this face can't keep an attachment: …`                                |
|    30 | `invalid input: mesh conversion: this mesh is not a supported constant-section prism` |
|    22 | `lumps: shell boundaries intersect or touch`                                          |
|    22 | `offset: a corner whose faces don't meet the same way after the move`                 |
|    22 | `boolean Intersect: the operation failed: no result`                                  |
|    19 | `offset: only bodies with planes, cylinders, cones, spheres and tori`                 |
|    19 | `invalid input: a boolean needs a solid body`                                         |

These match the app/kernel wording and the checked-in regression fixtures; users generally transcribe the displayed message accurately. Messages with `;` are composites of several causes, so raw-text duplicate detection splits the same root cause. Normalized error signatures are needed.

### 3.6 `needs-info` back-and-forth

- **219 open issues (3.4%) currently carry `needs-info`.**
- **464 open issues** have the bot's ask; **331 (71.3%)** got a later comment.
- Median time since the ask on still-open issues: **13.5 days** (max 29.1). The 14-day sweep will close many if they do not reply.
- By kind: non-fatal 287 asks, 187 replies, median 13.1 days; unknown 149 asks, 117 replies, 13.4 days; wrong-result 15 asks, 15 replies, 25.2 days; non-code 13 asks, 12 replies, 14.6 days.

### 3.7 Which areas have the worst report quality

Coarse project labels:

| Label         |     n | Non-fatal | Error text | Operation | Params | Model | Repro | Unknown |
| ------------- | ----: | --------: | ---------: | --------: | -----: | ----: | ----: | ------: |
| `area:kernel` | 3,184 |     84.5% |      65.0% |     77.2% |  64.3% |  7.0% | 49.1% |    6.2% |
| `area:sketch` |   497 |     64.4% |      32.0% |     40.8% |  40.4% |  7.8% | 38.4% |   15.5% |
| `area:ui`     |   230 |     33.9% |       7.8% |     16.5% |  21.7% |  3.0% | 33.9% |   10.9% |
| `area:io`     |   197 |     65.5% |      14.7% |     19.3% |  21.8% | 12.2% | 33.5% |   17.3% |
| `area:doc`    |   196 |     40.8% |       9.2% |     32.7% |  26.5% |  8.7% | 27.0% |   12.8% |
| `area:render` |   126 |     49.2% |      15.1% |     21.4% |  27.0% |  7.9% | 32.5% |   10.3% |

Among operation-failure issues (non-fatal + wrong) with >=20 issues, the weakest fine areas are:

| Area                            |   n | Error | Operation | Params | Model | Repro |
| ------------------------------- | --: | ----: | --------: | -----: | ----: | ----: |
| `kernel/surfaces`               |  90 | 34.4% |     54.4% |  54.4% | 11.1% | 47.8% |
| `kernel/threads-holes`          | 220 | 53.2% |     65.0% |  63.2% |  4.5% | 49.1% |
| `kernel/topology`               |  46 | 21.7% |     30.4% |  21.7% | 17.4% | 34.8% |
| `doc/parameters-configurations` |  42 | 14.3% |     19.0% |  21.4% |  2.4% | 23.8% |
| `doc/naming-format`             |  37 |  5.4% |     10.8% |  13.5% | 10.8% | 24.3% |
| `kernel/patterns-split`         | 245 | 61.6% |     76.3% |  67.8% |  5.7% | 53.5% |

The kernel/sweeps, blends and shell/offset areas are better on error/operation text (78–85% error, 87–91% operation), but model attachment is still only 4–6%. **The missing model and missing creation-site trace are universal, not area-specific.**

---

## 4. Recommendations

Effort: **S** ≤ 2 days, **M** ~1 week, **L** > 1 week (one experienced engineer). Risk is implementation/regression risk, not user risk.

### Must improve

#### M1. Put structured error context on the error at creation, and keep it through propagation

**Addresses:** non-fatal, wrong result.
**What:** an `ErrorContext` record created where the error is raised: operation/command id, stable error kind/code, creation-site stack (bounded), structured parameters (size-capped, redacted), feature/body/face/edge ids, document revision, and the pre-command document snapshot. Attach it as `Arc<ErrorContext>` only on error. Preserve it through `KernelError` → `DocError` → `EngineError` → `FeatureResult` → UI.
**Why:** 40% of non-fatal issues have no error text, 34% no operation, 36% no params, 99.6% no trace. The creation site is lost at `From<KernelError> for EngineError` and `FeatureResult`.
**Impact:** The single largest triage improvement: exact failure site, operation and inputs; makes duplicate grouping and minimal repro possible.
**Effort:** L. **Risk:** high: `KernelError` is an enum with many construction sites and pattern matches; a wrapper struct or a context field is an API change. A staged version can put the context at `Session::execute` first (M) and add creation-site capture in `guard`/`KernelError::smlib` second.
**Where:** `crates/kernel/src/lib.rs:254-315`, `:419-434`; `crates/engine/src/lib.rs:77-136`, `:1850-2044`; `crates/doc/src/eval.rs:1454-1475`; `crates/ui-egui/src/dialogs.rs:805-849`.
**Depends on:** error-code design and a measured stack-capture policy.

#### M2. Preserve the failure-time document, not the report-open document

**Addresses:** non-fatal, wrong result.
**What:** attach the engine's `before` snapshot (or a referenced local snapshot) when a failure is recorded. Do not serialize a new design at report-open time.
**Why:** `Session::execute` restores `before` on failure (`engine/src/lib.rs:2021-2030`) but the report reads the current design (`issue_report/ticket.rs:128`). A user who changes the model before reporting sends the wrong state.
**Impact:** Makes replay and model attachment trustworthy.
**Effort:** M. **Risk:** medium: lifetimes/memory; serialize lazily on report/save, keep `Arc<Document>` bounded.
**Where:** `crates/engine/src/lib.rs:1876-1882`, `:2021-2030`; `crates/ui-egui/src/issue_report/ticket.rs:122-150`.
**Depends on:** M1.

#### M3. A report action on every user-visible failure surface

**Addresses:** non-fatal, wrong result.
**What:** one `Report this failure` action on dialog errors, status toasts, timeline feature errors and import warnings, independent of ticket vs GitHub transport. The global Help/title-bar entry stays for reports without a specific error.
**Why:** timeline marks have no action (`timeline.rs:574-700`); toast action exists only on tickets (`viewport.rs:7095`); many issues are filed via the generic title-bar button with no failed operation.
**Impact:** More reports carry the actual command/error; fewer "operation failed" issues with no context.
**Effort:** S/M. **Risk:** low.
**Where:** `crates/ui-egui/src/timeline.rs:574-700`; `viewport.rs:7094-7121`; `dialogs.rs:3706-3709`; `issue_report.rs:667-673`.
**Depends on:** M1/M2 for the best payload; the action can ship first with the current error string.

#### M4. Privacy: hostname consent, path redaction, and queue permissions

**Addresses:** all.
**What:** remove the hostname from `POST /api/drafts`, or show it and include it in `consent`; add a path/username scrubber beyond the home folder; write queue files with user-only permissions (0600) and delete them promptly; offer encryption for queued designs/crashes.
**Why:** `tickets/api.rs:135-158` sends hostname outside the visible consent record; logs can contain other absolute paths; `queue.rs:37-70` writes attachments unencrypted with ordinary permissions.
**Impact:** Removes a legal/privacy exposure and a trust risk; no triage downside.
**Effort:** S/M. **Risk:** low/medium.
**Where:** `crates/ui-egui/src/tickets/api.rs:135-158`; `tickets/payload.rs:176-204`; `issue_report.rs:402-413`; `tickets/queue.rs:37-70`.
**Depends on:** none.

#### M5. Never upload a full model automatically or to a public issue

**Addresses:** all.
**What:** keep the existing per-report design tick, private ticket attachments, and no design in the GitHub draft. Add a private attachment reference if a public issue needs to name one.
**Why:** the design is proprietary IP; the current default is right and must not be weakened by a "better repro" change.
**Impact:** preserves trust; minimal repro should be a deliberate, smaller attachment.
**Effort:** S. **Risk:** low.
**Where:** `tickets/payload.rs:27-43`, `:235-239`; `issue_report/ticket.rs:284-296`; `docs/problem-reports.md:13-16`.

### Should improve

#### S1. Minimal-operand capture for the failing operation

**What:** serialize the operand bodies/features and their dependency closure (sketches, parameters, datum geometry) into a small lossless design/replay, not the whole document.
**Why:** 93% of non-fatal issues have no model; the full `.sc` is often too much to share.
**Impact:** High; turns "cannot reproduce" into a small file.
**Effort:** L. **Risk:** high: topology/provenance/lineage; the `.sc` reader expects a complete document.
**Where:** `crates/io/src/lib.rs:1284` (`write_design`), `crates/io/src/design.rs:171`; `crates/kernel/src/body.rs:517` (`record`); `crates/engine/src/cmd/script.rs:1-31` (history script).
**Depends on:** M1/M2 and a document-subset serializer.

#### S2. Deterministic non-fatal replay file

**What:** write `replay.json` + the before-design for an ordinary failed command, reusing the crash replay shape; `solidcraft-cli run replay.json --in design.sc` is the regression test.
**Why:** the crash path already proves this works; non-fatal failures are 65% of the backlog and have no replay.
**Impact:** One command reproduces the failure; regression tests can be generated from reports.
**Effort:** M. **Risk:** medium: failure-time snapshot; command ids/params; design-format compatibility.
**Where:** `crates/engine/src/crash.rs:700-736`, `:1097-1142`; `engine/src/lib.rs:2037-2042`; `crates/engine/src/cmd/script.rs`.
**Depends on:** M1/M2.

#### S3. Bounded recent-command ring buffer for every report

**What:** attach the last N top-level commands (id, params, ok/error) to non-fatal reports; the crash journal already has the data.
**Why:** 74% of fatal reports have a log; only 0.4% of non-fatal reports do. Users cannot reconstruct "what I did before it failed."
**Impact:** Medium/high; cheap; no continuous disk logging.
**Effort:** S/M. **Risk:** low: reuse the existing journal, keep the 512 KB/4 KB caps.
**Where:** `crates/engine/src/crash.rs:759-850`, `:1097-1142`; `crates/ui-egui/src/issue_report/ticket.rs:86-103`.
**Depends on:** none beyond exposing the journal to the report dialog.

#### S4. Client-side error signature

**What:** hash stable error kind + operation + normalized creation-site frames (or a stable code) for client-side duplicate grouping; keep it advisory, never auto-close.
**Why:** top messages repeat (boolean Cut ~254, Union ~183); raw text with numbers/paths splits duplicates; 2,529 non-fatal issues carry an error text.
**Impact:** High for triage; makes the "fix one, close many" view possible.
**Effort:** M. **Risk:** medium: inlining, symbols, line tables, platform/build differences; use a stable code as the primary key and frames as a secondary signal.
**Where:** `crates/engine/src/crash.rs:221` (existing signature); `crates/kernel/src/lib.rs:254-315`; `crates/engine/src/lib.rs:77-136`.
**Depends on:** M1 and stable error codes.

#### S5. Mark “succeeded but invalid” results

**What:** run the existing kernel validity check on results (it already runs `each_feature` by default), and when a body is kept but the check refuses it or reports a suspicious invariant, add a structured warning/context to the feature and offer the check output in the report.
**Why:** wrong-result issues have only 4% error text and 2% model; the kernel already has the checker.
**Impact:** Captures a class of failures users currently report only by describing the shape.
**Effort:** M/L. **Risk:** medium: false positives/performance; the check is already on by default, so reuse its result rather than adding a second pass.
**Where:** `crates/kernel/src/validation.rs:1-40`; `crates/doc/src/eval.rs:3800-3860`; `crates/engine/src/cmd/diagnostics.rs:20-45`; `crates/ui-egui/src/timeline.rs:574-700`.
**Depends on:** performance measurement; M1.

#### S6. Detect and surface silent failures

**What:** make `SolveApp::run`/command call sites report errors centrally; audit `let _ = app.run(...)`; distinguish user-visible no-ops from background state changes.
**Why:** many failure paths are swallowed (`timeline.rs:39,400`, `tools.rs:981`, `browser_prompts.rs:77,81`); the user sees nothing and cannot report.
**Impact:** Medium; turns silent failures into actionable reports.
**Effort:** M. **Risk:** medium: noisy status messages; only report real operation failures, not optional state updates.
**Where:** `crates/ui-egui/src/lib.rs:1462-1535`; `timeline.rs`; `tools.rs`; `browser_prompts.rs`.
**Depends on:** M3.

#### S7. Local “save report” option

**What:** save the reviewed report (ticket JSON + attachments) to a chosen folder, and load it later.
**Why:** offline/enterprise users cannot use the ticket server; a local file is the fallback; the queue already serializes the same shape.
**Impact:** Medium; no server dependency.
**Effort:** S. **Risk:** low: privacy of the local file; use safe writes.
**Where:** `crates/ui-egui/src/issue_report/form.rs` actions; `tickets/payload.rs:99-106`; `tickets/queue.rs:37-77`.
**Depends on:** M4.

#### S8. Align templates and triage with failure kind

**What:** add failure-kind fields/labels to GitHub forms and map the app's sections to a `failure_kind` label; keep the app's detailed template as the preferred path.
**Why:** the GitHub form is 5.6% of open issues; the triage bot only knows coarse `area:*`; 11.6% are unclassified.
**Impact:** Medium; better routing without changing user workflow.
**Effort:** S. **Risk:** low.
**Where:** `.github/ISSUE_TEMPLATE/*.yml`; `scripts/triage/triage.js:51-117`; `scripts/triage/needs-info.js:20-49`.
**Depends on:** M1/S4 if labels should use stable codes.

### Could improve

- **C1. Strip-metadata option** for screenshots and attached designs (PNG metadata, design metadata, names/paths). Effort S; risk low. Hook `ticket.rs` attachment collection and `queue.rs`.
- **C2. Report queue management UI** (list, inspect, delete, export before send). Effort S/M; low risk. Hook `tickets/queue.rs`, `issue_report/ticket.rs`.
- **C3. Error-code reference table** generated from `KernelError`/`solidcore_ops::Error`, shown in reports and docs. Effort S; low risk.
- **C4. Optional screenshot default off** for users who work on proprietary models. Effort S; low risk; would reduce visual evidence, so only as a preference.
- **C5. Attachment retention/expiry controls** on the ticket server. Effort server-side; ask owner.

### Should not change or build

- **Automatic upload of full models.** It is the fastest way to lose a user's IP and trust; keep per-report opt-in and private storage.
- **Full model in a public GitHub issue.** No.
- **Raw strings, paths, environment variables, or command lines in reports.** Redact/allowlist; capture structured values instead.
- **A backtrace on every error in hot kernel loops.** It would slow booleans/blends/sweeps and produce noisy control-flow errors. Measure first; capture at creation only where it is actionable, with sampling/opt-in if needed.
- **Continuous logging/telemetry.** The crash journal is bounded and disable-able; do not expand it into background telemetry.
- **Auto-filing issues without review.** The current reviewed draft is correct; agents already need a person for design/crash.
- **Replacing the GitHub fallback or disabling blank issues.** It is the only path for some builds/users; keep it.
- **Treating raw error text as a stable duplicate key.** Numbers, paths, and composites make it unstable; use a stable code/signature.
- **Sending reports to a third-party LLM/cloud without explicit consent.** Keep report content local until the user sends it.

---

## 5. Suggested sequencing

1. **Privacy first (M4, M5):** hostname consent/preview, path scrub, queue permissions, no public model. Independent, low risk, removes legal exposure.
2. **Make every failure reportable (M3, S3, S7):** timeline/status actions, recent-command attachment, local save. Uses the existing evidence and immediately improves new reports.
3. **Failure-time snapshot + replay (M2, S2):** attach the before-document and a replay; this makes existing reports correct.
4. **Error context design (M1):** start with a stable `ErrorContext` at the engine boundary (command/params/before doc), then add creation-site capture in `guard`/`KernelError::smlib`. This is the largest change; do the type/API design before touching every call site.
5. **Stable codes + signature (S4):** after M1, build the client-side duplicate key and the triage grouping.
6. **Minimal operand capture (S1):** after M1/M2 and a document-subset serializer; do not attempt by slicing `.sc`.
7. **Wrong-result validation (S5):** after measuring the capture budget; reuse the existing `diagnostics.validation` result.
8. **Templates/triage (S8):** can run in parallel from step 1; label `failure_kind` and route by it.
9. **Silent-failure audit (S6):** broaden once the report action exists, so surfaced errors are useful rather than noisy.

---

## 6. Open questions for the owner

1. **Performance budget for creation-site capture.** How much release-build overhead is acceptable on errors? Capture a stack on every error, sample, or only on typed refusals/operations above a cost threshold? This decides whether M1 is a small engine change or a kernel-wide instrumentation project.
2. **Can the error type change project-wide?** `KernelError` is a public enum used across kernel/doc/engine; changing it to a context-carrying struct is invasive. Is a breaking internal API acceptable, or must it be additive behind a wrapper?
3. **Are error codes stable public API?** Should `solidcore_ops`/kernel error kinds become a documented, stable code table? Raw SMLib status codes are not suitable for signatures. This decides S4's stability.
4. **Ticket server policy.** Who can see a ticket, how long it is retained, whether attachments are encrypted at rest, and whether the hostname should be stored at all. The server code is not in this checkout; this is a policy decision.
5. **Model/export-control stance.** Are design files from enterprise/regulated users allowed to leave the machine at all? If not, the "local save report" path and a private on-prem server become requirements, not options.
6. **Public issue attachments.** Should a public GitHub issue ever carry a model reference? If yes, where is the private object stored and who can access it?
7. **Wrong-result expectation.** Is a "succeeded but invalid" result a bug report, a warning, or a feature? That decides whether S5 is a safety net or a user-facing workflow.

---

## 7. Appendix: method and files

**Backlog method:** local SQLite DB from the triage task (`triage` CLI, Podman volume), 6,477 open issues. Rules classify `fatal-crash`, `nonfatal-operation`, `wrong-result`, `non-code`, `unknown`; a `failure/*` tag was written to the DB for every open issue. `triage area kernel --tag failure/nonfatal-operation` works. Counts are rule-based, not ground truth; wrong-result is a lower bound and the unknown bucket contains empty templates and internal tasks. The hand-read calibration sample is `analysis/classification-sample2.md`.

**Key files read:** `crates/ui-egui/src/issue_report.rs`, `issue_report/form.rs`, `issue_report/ticket.rs`, `tickets.rs`, `tickets/{api,payload,queue,agent}.rs`, `crash_ui.rs`, `help.rs`, `toolbar.rs`, `dialogs.rs`, `viewport.rs`, `timeline.rs`, `preview.rs`; `crates/engine/src/{crash,lib,report_issue}.rs`, `cmd/diagnostics.rs`; `crates/kernel/src/{lib,validation}.rs`; `crates/smlib/src/lib.rs`; `crates/doc/src/{lib,eval}.rs`; `crates/io/src/{lib,design}.rs`; `apps/solidcraft/src/main.rs`; `apps/solidcraft-cli/src/crashes.rs`; `.github/ISSUE_TEMPLATE/*`, `.github/workflows/triage.yml`, `scripts/triage/*`; `docs/problem-reports.md`, `docs/crash-reports.md`.

**Artifacts:** `reports/phase1-findings.md`; `analysis/phase2-evidence.txt`; `analysis/phase2-operation-areas.txt`; `analysis/phase2-metrics.json`; `analysis/open-failure-kinds.tsv`; `analysis/classification-sample2.md`.
