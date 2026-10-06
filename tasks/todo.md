# mantis-tui: Task List

Spec: `SPEC.md` · Plan: `tasks/plan.md`

**Definition of done for every task:** `gofmt -l .` prints nothing,
`go vet ./...` is clean, `go test -race ./...` passes, `go build ./...`
succeeds, and no test writes to a real host.

---

## Phase 1: Foundation and tracer bullet

### Task 1: Scaffold the module, Makefile and lint config, with a cobra root that runs
**Description:** Create the Go module `github.com/whleucka/mantis-tui`, the
`cmd/mantis-tui/main.go` entrypoint, an `internal/cli` root command (with the
global flags `--host`, `--config`, `--json` and `--timeout`), a Makefile
(`build test race lint fmt it`), `.golangci.yml` and `.gitignore` (which
ignores `bin/`).
**Acceptance criteria:**
- [x] `go run ./cmd/mantis-tui --help` lists the global flags
- [x] `make build` produces `bin/mantis-tui`, and `make lint` passes
- [x] The Bubble Tea version decision is recorded in plan.md (v2, approved)
**Verification:** `make build && make lint && make test`
**Dependencies:** None
**Files:** `go.mod`, `cmd/mantis-tui/main.go`, `internal/cli/root.go`, `Makefile`, `.golangci.yml`
**Scope:** S

### Task 2: Config loading and host selection, plus `mantis-tui hosts`
**Description:** Load TOML from the XDG path or `--config`. Resolve each
token from `env` or `token`, drop hosts whose env var is unset (recording
why), warn when an inline token sits in a file others can read, and fall back
to auto-detecting `MANTIS_*` plus `MANTIS_*_URL`. Choose the host in the
order flag → `MANTIS_TUI_HOST` → `default` → the only host → last used (read
from the state file) → picker or error. Add the `hosts` subcommand.
**Acceptance criteria:**
- [x] Table tests cover every step of the selection order, dropped hosts, the permission warning and auto-detect
- [x] The state file read and write lives at `$XDG_STATE_HOME/mantis-tui/state.toml`, and a missing or stale host is ignored
- [x] `mantis-tui hosts` lists both real hosts as active (with your env set), and shows `chainlogic` as dropped when `MANTIS_CL` is unset
**Verification:** `go test ./internal/config/...` (coverage ≥ 85%). Manual: `MANTIS_CL= go run ./cmd/mantis-tui hosts`
**Dependencies:** 1
**Files:** `internal/config/config.go`, `internal/config/state.go`, `internal/config/config_test.go`, `internal/cli/hosts.go`
**Scope:** M

### Task 3: API client core and the read endpoints, with scrubbed fixtures
**Description:** Write `mantis.Client` with `do()` (sets the auth header,
doesn't follow redirects, maps errors to `APIError`, `ErrNotFound` and
`ErrUnauthorized`) and an `API` interface. Types: Issue, Note, History,
User, Project, Category, EnumValue. Read endpoints: `Me`, `ListIssues`
(page, page size, project, filter), `GetIssue`, `Projects`, `Project`,
`ProjectUsers`, `Config(options...)`. Add a `scripts/record-fixtures.sh` that
makes read-only calls against wh and scrubs emails and names.
**Acceptance criteria:**
- [x] `httptest` tests check the method, path, query and headers of every read endpoint, and the error mapping for 401, 403, 404, 500 and non-JSON bodies
- [x] A redirect response is returned as an error and is never followed, and a test asserts that `APIError.Error()` never contains the token
- [x] Fixtures in `testdata/` are from 2.27.0 and scrubbed, and a test fails if a real-looking email is present
**Verification:** `go test ./internal/mantis/...` (coverage ≥ 85%)
**Dependencies:** 1
**Files:** `internal/mantis/client.go`, `internal/mantis/types.go`, `internal/mantis/issues.go`, `internal/mantis/client_test.go`, `scripts/record-fixtures.sh`
**Scope:** M

### Task 4: CLI `list` and `show` with table and JSON output, and exit codes
**Description:** Wire config → client → output. `list` takes `--filter`,
`--project`, `--page` and `--page-size`. `show` takes `--notes` and
`--history`. The default output is an aligned table; `--json` prints the
server's JSON unchanged. Map errors to exit codes 1, 2, 3 and 4.
**Acceptance criteria:**
- [x] In-process cobra tests against `httptest` cover table output, `--json` passthrough and every exit code
- [x] `--project` accepts a project name or an id
- [x] Works against real wh: `mantis-tui list --host williamhleucka --filter assigned`
**Verification:** `go test ./internal/cli/...`. Manual read-only run against wh, plus a read-only `list --page-size 5` against chainlogic as a version smoke test
**Dependencies:** 2, 3
**Files:** `internal/cli/list.go`, `internal/cli/show.go`, `internal/cli/output.go`, `internal/cli/exit.go`, `internal/cli/cli_test.go`
**Scope:** M

### ✅ Checkpoint A: tracer bullet
- [x] All tests pass, `make lint` is clean
- [x] Real read path works on wh and chainlogic
- [x] Review with the user before Phase 2

---

## Phase 2: CLI write path and service layer

### Task 5: Metadata cache and name→id resolution
**Description:** `internal/meta` caches the enums (`status`, `priority`,
`severity`, `reproducibility` and `resolution` from `*_enum_string`),
`status_colors`, projects, categories per project and users per project,
for each host, with concurrent loads safe. `internal/service` resolves names
to refs case-insensitively. An invalid value is an error listing the valid
ones, and an ambiguous user name is an error listing the candidates.
**Acceptance criteria:**
- [x] Each metadata call is made once per host per session, verified with a counting fake
- [x] Users resolve by username, real name or id, and ambiguity is an error that lists the candidates
- [x] Enum resolution error messages list the server's values
**Verification:** `go test -race ./internal/meta/... ./internal/service/...`
**Dependencies:** 3
**Files:** `internal/meta/meta.go`, `internal/meta/meta_test.go`, `internal/service/resolve.go`, `internal/service/resolve_test.go`
**Scope:** M

### Task 6: Client write endpoints, including the unmonitor step
**Description:** Add `CreateIssue`, `UpdateIssue` (a typed `IssuePatch`
with pointer fields), `DeleteIssue`, `AddNote` (text, private flag,
`time_tracking.duration`), `DeleteNote` and `Monitor`. `service.Unmonitor`
does the GET of the monitor list and the PATCH back without the current user.
**Acceptance criteria:**
- [x] Every write endpoint's request body matches the Mantis REST shape, checked in tests
- [x] The unmonitor test proves other users' monitors are kept in the PATCH body
- [x] `IssuePatch` sends only the fields that were set (nil fields are left out)
**Verification:** `go test ./internal/mantis/... ./internal/service/...`
**Dependencies:** 3, 5
**Files:** `internal/mantis/write.go`, `internal/mantis/write_test.go`, `internal/service/monitor.go`, `internal/service/monitor_test.go`
**Scope:** M

### Task 7: CLI `update`, `assign`, `monitor`, `unmonitor` and `open`, plus the batch runner
**Description:** `service.Batch(ids, op)` runs at most 4 operations at once
and returns the result for each id. `update` and `assign` accept several
ids and use it. `open` builds `<url>/view.php?id=N` and runs `xdg-open`
(`open` on macOS).
**Acceptance criteria:**
- [x] A batch test with 10 ids and 1 injected failure gives 9 successes and 1 failure, never more than 4 in flight, and a non-zero exit code
- [x] `update --status resolved` resolves the name through meta, and an invalid name exits with code 2 and lists the valid values
- [x] `open` uses the right browser opener and has a testable injected opener
**Verification:** `go test -race ./internal/service/... ./internal/cli/...`
**Dependencies:** 4, 5, 6
**Files:** `internal/service/batch.go`, `internal/service/browser.go`, `internal/cli/update.go`, `internal/cli/monitor.go`, `internal/cli/open.go`
**Scope:** M

### Task 8: Editor package and CLI `note` / `note delete`
**Description:** `internal/editor` picks `$VISUAL`, then `$EDITOR`, then
`vi`, writes a 0600 temp file with `#` hint lines, strips the hints after
editing, aborts on empty or unchanged text, and keeps the file on failure.
It exposes a plain `Run` (for the CLI) and a `tea.ExecProcess`-ready
`Cmd`. The CLI `note` takes `-m`, `--edit` or `-` (stdin), plus `--time`
and `--private`. `note delete` has the TTY and `--yes` guard.
**Acceptance criteria:**
- [x] Editor tests use a fake `$EDITOR` script to cover stripping, the empty/unchanged abort and 0600 permissions
- [x] `note 1 --time 0:30` sends `time_tracking.duration = "0:30"`, and a malformed time exits with code 2
- [x] `note delete` with no TTY and no `--yes` refuses and exits with code 2
**Verification:** `go test ./internal/editor/... ./internal/cli/...`. Manual: `--edit` with nvim against an `httptest`-backed dev server, or against wh after approval
**Dependencies:** 6
**Files:** `internal/editor/editor.go`, `internal/editor/editor_test.go`, `internal/cli/note.go`, `internal/cli/confirm.go`
**Scope:** M

### Task 9: CLI `create` and `delete`, with the TTY and `--yes` guard
**Description:** `create` resolves the project, category, enums and
assignee, then takes the description from `-d` or `--edit`. `delete`
accepts several ids (it uses the batch runner) and reuses the confirmation
guard.
**Acceptance criteria:**
- [x] The `create` request body contains the resolved ids and refs, checked in a test
- [x] Without `--project` or `--summary`, it exits with code 2
- [x] `delete 1 2 3` on a TTY prompts once, showing the count
**Verification:** `go test ./internal/cli/...`
**Dependencies:** 7, 8
**Files:** `internal/cli/create.go`, `internal/cli/delete.go`, `internal/cli/create_test.go`
**Scope:** S

### ✅ Checkpoint B: CLI complete
- [x] `make race && make lint` are clean, and `config`, `mantis` and `service` coverage is ≥ 85%
- [x] **Ask the user** before doing one manual write round trip on wh: create → update → note → monitor/unmonitor → delete (approved for `ideas` and done; found time tracking disabled)
- [x] Review with the user before Phase 3 (`/build auto`: the only gate was the real-write approval)

---

## Phase 3: TUI read path

### Task 10: TUI root, keymap and chord helper, status bar, host picker and host switching
**Description:** The root model routes between views and owns the status
bar (host, filter, page, spinner, the last error). `keymap.go` holds all the
default bindings, which match mantis.nvim. The chord helper handles
multi-key sequences with a 1-second timeout. The host picker appears when
host selection reaches the picker step. `ctrl+h` switches host and keeps
per-host view state in memory. Selecting a host writes the state file.
Running with no subcommand launches the TUI.
**Acceptance criteria:**
- [x] Chord tests cover `gg`, `dn` and `bs` resolving, a lone `g` timing out, and an unknown second key falling through
- [x] Host picker → host chosen → state file written. On the next launch the last-used host is selected without the picker (when no host is marked `default`).
- [x] An error message from any view shows in the status bar and never contains the token
**Verification:** `go test ./internal/tui/...`. Manual: `go run ./cmd/mantis-tui`
**Dependencies:** 2, 5
**Files:** `internal/tui/app.go`, `internal/tui/keymap.go`, `internal/tui/chord.go`, `internal/tui/statusbar.go`, `internal/tui/hostpicker/model.go`
**Scope:** M

### Task 11: Issue list with filters, pagination, spinner, icons and status colours
**Description:** A table with priority icon, id, severity, status
(coloured from `status_colors`), category, summary (filling the remaining
width), updated and the monitor icon. `F` opens a filter picker, `L` / `H`
page, `r` refreshes, `enter` opens the issue. Icons can be changed in the
config.
**Acceptance criteria:**
- [x] Message tests: a filter change reloads page 1, paging past the last page is a no-op, and a load error goes to the status bar
- [x] Column widths recalculate on resize, and summary fills the remaining width (golden at 120×40 and 80×24)
- [x] The default filter and page size come from the config
**Verification:** `go test ./internal/tui/issuelist/...`. Manual against wh
**Dependencies:** 10
**Files:** `internal/tui/issuelist/model.go`, `internal/tui/issuelist/columns.go`, `internal/tui/issuelist/model_test.go`, `internal/tui/styles.go`
**Scope:** M

### Task 12: List extras: group by project and the `/` search
**Description:** `ctrl+g` groups the rows under project headers (cursor
movement skips the headers). `/` opens a fuzzy filter over id, summary,
category and handler on the loaded page, and `esc` clears it.
**Acceptance criteria:**
- [ ] When grouping is toggled, the cursor stays on the same issue id
- [ ] The search narrows the rows, and `esc` restores them with the cursor on the same issue
**Verification:** `go test ./internal/tui/issuelist/...`
**Dependencies:** 11
**Files:** `internal/tui/issuelist/group.go`, `internal/tui/issuelist/search.go`, `internal/tui/issuelist/extras_test.go`
**Scope:** S

### Task 13: Issue view with header, body, Notes/History tabs and scrolling
**Description:** A viewport showing the header fields, custom fields, tags,
relationships and attachment names, then the description, steps to
reproduce and additional info. `tab` switches between the Notes and History
tabs. The scrolling keys are `j`, `k`, `ctrl+d`, `ctrl+u`, `gg` and `G`,
and `q` / `esc` returns to the list.
**Acceptance criteria:**
- [ ] Goldens rendered from the scrubbed 2.27.0 fixtures, including an issue with notes, private notes, time tracking and history
- [ ] Missing optional fields (no handler, no tags) render with no gaps or panics
- [ ] Returning to the list puts the cursor back on the same issue
**Verification:** `go test ./internal/tui/issueview/...`. Manual against wh
**Dependencies:** 11
**Files:** `internal/tui/issueview/model.go`, `internal/tui/issueview/render.go`, `internal/tui/issueview/model_test.go`
**Scope:** M

### Task 14: Auto-refresh for the list and the issue view
**Description:** A `tea.Tick` per view, using the configured interval (`0`
disables it). The list keeps the cursor by issue id and keeps the
selection. The issue view keeps its scroll offset. A refresh is skipped
while a modal or `$EDITOR` is open, or while a request is in flight.
**Acceptance criteria:**
- [ ] A test where the rows are reordered between refreshes: the cursor follows the issue id
- [ ] A test where the selected issue disappears: the cursor clamps to a valid row and the selection drops that id
- [ ] With the interval set to `0`, no tick is ever scheduled
**Verification:** `go test ./internal/tui/...`
**Dependencies:** 12, 13
**Files:** `internal/tui/issuelist/refresh.go`, `internal/tui/issueview/refresh.go`, `internal/tui/refresh_test.go`
**Scope:** S

### ✅ Checkpoint C: TUI browse
- [ ] The full browse flow works against wh: pick a host, filter, page, search, group, open an issue, read notes and history, switch hosts, and let auto-refresh run
- [ ] Review with the user before Phase 4

---

## Phase 4: TUI write path

### Task 15: Picker and confirm modals, plus the single-issue actions in the list and the issue view
**Description:** A generic picker modal (with fuzzy filtering and options
loaded from meta) and a yes/no confirm modal. Wire up `s`, `p`, `V`, `c`,
`a`, `S` (text input), `m`, `o` and `D` (with confirmation) in the list,
and the same keys apart from `D` in the issue view. A successful update
patches the row in place, and a failure leaves the row unchanged and
reports the error.
**Acceptance criteria:**
- [ ] Each action sends the right service call, checked with the fake service
- [ ] Optimistic updates are not used: the row changes only after the server returns 2xx
- [ ] `D` → confirm → the issue is removed from the list, and the cursor moves to the next row
**Verification:** `go test ./internal/tui/...`. Manual against an `httptest` dev server
**Dependencies:** 14
**Files:** `internal/tui/picker/model.go`, `internal/tui/confirm/model.go`, `internal/tui/actions.go`, `internal/tui/actions_test.go`
**Scope:** M

### Task 16: Add and delete notes from the TUI, using `$EDITOR`
**Description:** `N` in the list or the issue view first asks for
optional time tracking and a private flag (a small form), then runs
`editor.Cmd` via `tea.ExecProcess`. `dn` in the issue view picks a note and
asks for confirmation.
**Acceptance criteria:**
- [ ] Using `$EDITOR`=nvim, the TUI suspends and resumes cleanly with no rendering glitches (manual check)
- [ ] Saving an empty buffer sends nothing and shows "note discarded"
- [ ] The time-tracking field is hidden when the server has time tracking disabled (`meta.TimeTrackingEnabled`)
- [ ] If the submit fails, the status bar shows the path of the saved temp file
**Verification:** `go test ./internal/tui/...`. Manual with nvim
**Dependencies:** 8, 15
**Files:** `internal/tui/notes.go`, `internal/tui/issueview/notes.go`, `internal/tui/notes_test.go`
**Scope:** S

### Task 17: Selection and batch operations
**Description:** `space` toggles selection, `ctrl+a` selects the whole
page and `ctrl+x` clears the selection. The chords `bs`, `bp`, `bv`, `bc`,
`ba` and `bD` apply to the selection (or to the cursor row when nothing is
selected) through `service.Batch`. Progress shows in the status bar. A
partial failure leaves only the failed issues selected.
**Acceptance criteria:**
- [ ] A batch with 10 issues and 1 injected failure reports 9 OK and 1 failed, and only the failed issue stays selected
- [ ] `bD` asks for confirmation and shows the count
- [ ] The selection survives paging and auto-refresh, because it's keyed by id
**Verification:** `go test -race ./internal/tui/...`
**Dependencies:** 15
**Files:** `internal/tui/issuelist/selection.go`, `internal/tui/batch.go`, `internal/tui/batch_test.go`
**Scope:** M

### Task 18: Create-issue form
**Description:** `C` opens a form for project, category (reloaded when
the project changes), summary, priority, severity, reproducibility and
assignee. `e` on the description field opens `$EDITOR`, `alt+enter`
submits, and `esc` cancels (asking first if there are unsaved changes).
A successful submit opens the new issue's view.
**Acceptance criteria:**
- [ ] Changing the project reloads the categories and users and clears any choices that are no longer valid
- [ ] Submitting without a summary or category shows an inline error and makes no request
- [ ] The request body matches the CLI `create` request body for the same inputs (both go through one shared service function)
**Verification:** `go test ./internal/tui/createform/...`. Manual against an `httptest` dev server
**Dependencies:** 15, 16
**Files:** `internal/tui/createform/model.go`, `internal/tui/createform/model_test.go`, `internal/service/create.go`
**Scope:** M

### ✅ Checkpoint D: parity
- [ ] Go through mantis.nvim's README keymap tables: every key has a working equivalent
- [ ] **Ask the user** before a manual TUI write session on wh
- [ ] Review with the user before Phase 5

---

## Phase 5: Polish

### Task 19: Help overlay, polished error display, README
**Description:** `?` shows a help overlay generated from `keymap.go` (so
it can never drift from the real bindings). Errors from the server show in
the status bar and stay until the next action. The
README covers install (`go install …@latest`), a sample config, the CLI
reference, the keymap tables and troubleshooting.
**Acceptance criteria:**
- [ ] The help overlay lists every binding in `keymap.go`, checked by a test
- [ ] The README's sample config parses (a test loads the README's sample config block)
**Verification:** `go test ./...`. Read through the README
**Dependencies:** 17, 18
**Files:** `internal/tui/help.go`, `internal/tui/statusbar.go`, `README.md`
**Scope:** S

### Task 20: Success-criteria audit: token-leak test, coverage targets, lint
**Description:** Add an end-to-end test that runs every CLI command against
`httptest` with a sentinel token, captures stdout, stderr and errors, and
asserts the token never appears. Raise coverage to the spec targets. Walk
through the 11 success criteria in SPEC.md and tick them off.
**Acceptance criteria:**
- [ ] The token-leak test passes across all commands and error paths
- [ ] Coverage: `config`, `mantis` and `service` at ≥ 85%, and `tui` at ≥ 60%
- [ ] Each success criterion in SPEC.md is ticked off with the evidence noted
**Verification:** `make race && make lint && go test -cover ./...`
**Dependencies:** 19
**Files:** `internal/cli/leak_test.go`, plus test files where coverage is missing
**Scope:** S

### ✅ Checkpoint E: complete
- [ ] All success criteria are met, the race detector and lint are clean, and the work is ready for review
