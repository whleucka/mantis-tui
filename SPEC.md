# Spec: mantis-tui

A terminal UI (plus scriptable CLI) for the [MantisBT](https://mantisbt.org) bug
tracker, built to reach feature parity with
[mantis.nvim](https://github.com/whleucka/mantis.nvim) without needing Neovim.

## Assumptions

1. Target servers run MantisBT **2.24+** with the REST API enabled. The plugin's
   behaviour was verified on 2.24.4, and the `williamhleucka` host runs 2.27.0.
2. Auth is a Mantis API token sent raw in the `Authorization` header, the same
   way mantis.nvim sends it.
3. The primary user is a single developer (Will) working across two hosts:
   `chainlogic` (mantis.chainlogic.it, `MANTIS_CL`) and `williamhleucka`
   (mantis.williamhleucka.com, `MANTIS_WH`).
4. It runs on Linux first. macOS should work but isn't a v1 test target.
   Windows is out of scope.
5. The binary is named `mantis-tui` and the Go module is
   `github.com/whleucka/mantis-tui`. It's installed with
   `go install github.com/whleucka/mantis-tui/cmd/mantis-tui@latest`; there are
   no release binaries or packages in v1.

## Objective

**What:** a single static binary. Run with no arguments, it opens a full-screen
TUI for browsing and editing Mantis issues across several hosts. Run with a
subcommand, it does one operation without the TUI, for scripts and quick
one-liners.

**Why:** mantis.nvim is only useful inside Neovim. The tracker should also be
usable from any terminal, over SSH, and from shell scripts and agents. The CLI
mode matters because it gives tools like Claude Code a stable interface to the
tracker.

**User stories**

- As a dev with two Mantis accounts, I switch hosts in one keystroke and never
  paste a token into a config file.
- As a dev triaging, I see my assigned issues on launch, sorted and paged, and
  I change status, priority, severity or category without leaving the list.
- As a dev working a ticket, I open it, read the description, notes and
  history, and add a note (with optional time tracking) in `$EDITOR`.
- As a dev filing a bug, I create an issue: pick a project and category, set
  priority, severity and reproducibility, assign someone, and write the
  description in `$EDITOR`.
- As a dev doing cleanup, I select several issues and change their status,
  assignee or category, or delete them, in one action.
- As a script or agent, I run `mantis-tui show 1234 --json` or
  `mantis-tui note 1234 -m "fixed in abc123"` and get a reliable exit code.

## Tech Stack

| Concern | Choice |
|---|---|
| Language | Go ≥ 1.24 (`go.mod`: `go 1.24`) |
| TUI runtime | `charm.land/bubbletea/v2` |
| Components | `charm.land/bubbles/v2` (table, textinput, viewport, spinner, help, key) |
| Styling | `charm.land/lipgloss/v2` |
| CLI | `github.com/spf13/cobra` |
| Config | `github.com/BurntSushi/toml` |
| HTTP | stdlib `net/http` (no client library) |
| TUI tests | `github.com/charmbracelet/x/exp/teatest/v2` |

Adding any dependency not in this table requires asking first.

## Configuration

Path: `$XDG_CONFIG_HOME/mantis-tui/config.toml`, falling back to
`~/.config/mantis-tui/config.toml`. A `--config <path>` flag overrides it.

```toml
# Optional global defaults
[list]
default_filter = "assigned"   # all | assigned | reported | monitored | unassigned
page_size = 50
auto_refresh = "120s"         # Go duration; "0" disables
group_by_project = false

[issue]
auto_refresh = "120s"

[icons]                       # parity with mantis.nvim priority_emojis
immediate = "🔥"
urgent    = "⚠️"
high      = "🔺"
normal    = "🔵"
low       = "🔻"
none      = "🟣"
monitor   = "👁️"

[[hosts]]
name = "chainlogic"
url  = "https://mantis.chainlogic.it"   # without /api/rest
env  = "MANTIS_CL"

[[hosts]]
name = "williamhleucka"
url  = "https://mantis.williamhleucka.com"
env  = "MANTIS_WH"
default = true
```

**Host rules (same as mantis.nvim):**

- Each host needs a `url` and exactly one of `env` or `token`.
- A host whose `env` variable is unset or empty is **dropped silently** and
  doesn't appear in the picker. `mantis-tui hosts` lists dropped hosts and
  the reason.
- Selection order: `--host <name>` flag → `MANTIS_TUI_HOST` env → the host
  with `default = true` → the only remaining host → the **last-used host** →
  interactive picker (in CLI mode, an error listing the host names instead of
  a picker).
- The last-used host is saved to `$XDG_STATE_HOME/mantis-tui/state.toml`
  (falling back to `~/.local/state/mantis-tui/state.toml`) whenever the TUI
  selects a host. Only the TUI writes this file; the CLI reads it but never
  writes it. If the saved host no longer exists or was dropped, it's ignored.
- This config is owned by mantis-tui only. mantis.nvim keeps its own Lua config.
- If a host uses an inline `token` and the config file can be read by group or
  others, print a warning to stderr.
- If no config file exists, fall back to **auto-detection**: for each
  `MANTIS_*` env var, look for a matching `MANTIS_*_URL`. If none are found,
  exit with a message that shows a sample config.

## Feature Scope (v1 = mantis.nvim parity)

(The keys below are v1's. "v1.2: Keymap redesign" further down replaces
several of them.)

### Hosts
- Host picker at startup, following the selection rules above.
- `ctrl+h` switches host at runtime. Each host's view state (filter, page,
  cursor) is cached in memory so switching back restores it.
- The active host name always shows in the status bar.

### Issue list
- Columns: priority icon, id, severity, status, category, summary (fills the
  remaining width), updated. Status cells are coloured using the server's
  `status_colors` option from `/config` (a map of status name to hex colour,
  confirmed on the wh host).
- Filters: all, assigned, reported, monitored, unassigned (`F`).
- Pagination: `L` / `H`, with page size from config.
- Group-by-project toggle (`ctrl+g`).
- Monitored issues show the monitor icon.
- Auto-refresh on an interval. It keeps the cursor on the same **issue id**
  (not the same row) and keeps the selection.
- `/` does a client-side fuzzy filter on the loaded page by id, summary,
  category and handler.
- Single-issue actions: view (`enter`), open in browser (`o`), create (`C`),
  delete (`D`, with confirmation), assign (`a`), change status (`s`), priority
  (`p`), severity (`V`), category (`c`), summary (`S`), add note (`N`), toggle
  monitor (`m`), refresh (`r`), help (`?`), quit (`q`).
- Selection: toggle (`space`), select the whole page (`ctrl+a`), clear
  (`ctrl+x`).
- Batch: status (`bs`), priority (`bp`), severity (`bv`), category (`bc`),
  assign (`ba`), delete (`bD`, with confirmation that shows the count). Batch
  requests run with at most 4 in flight. Successes and failures are reported
  per issue, and a partial failure leaves the failed issues selected.

### Issue view
- Header: id, project, summary, status, resolution, priority, severity,
  reproducibility, reporter, handler, category, created and updated dates,
  tags, relationships, attachments (names only), custom fields.
- Body: description, steps to reproduce, additional info.
- Tabs: **Notes** and **History**. Both come back in the standard
  `GET issues/{id}` response, so no extra request is needed (confirmed on
  2.27.0).
- Notes: add (`N`, in `$EDITOR`), with optional time tracking (`HH:MM`) and a
  private flag. Delete a note (`dn`, with confirmation).
- The scrolling keys `j` / `k`, `ctrl+d` / `ctrl+u` and `gg` / `G` work.
- `s`, `p`, `V`, `c`, `a`, `m` and `o` work here too.
- Auto-refresh keeps the scroll position.

### Create issue
- A form with: project, category (loaded for the selected project), summary,
  priority, severity, reproducibility, and assignee (from the project's users).
- The description opens in `$EDITOR` (`e` on the field). `alt+enter` submits
  and `esc` cancels; cancelling with unsaved input asks for confirmation.
- After a successful submit, the TUI opens the new issue's view.

### `$EDITOR` integration
- Uses `$VISUAL`, then `$EDITOR`, then `vi`. It runs through
  `tea.ExecProcess` so the TUI suspends and resumes cleanly.
- The temp file is created in `os.TempDir()` with mode `0600` and named
  `mantis-<host>-<issue>-note-*.md`. Lines starting with `#` are template
  hints and are stripped.
- If the file is empty or unchanged after stripping, the action is aborted
  with nothing sent.
- If a submit fails, the text is kept and the temp-file path is shown so the
  text isn't lost.

### Enum/metadata loading
- Status, priority, severity, reproducibility and resolution options come from
  `GET /config?option[]=status_enum_string&option[]=…`, which returns
  `[{id, name, label}]`. Status colours come from `status_colors`. They're cached per
  host for the session and never hardcoded.
- Projects come from `GET /projects`, categories from `GET /projects/{id}`
  and users from `GET /projects/{id}/users`, all cached per host and project.

### Non-interactive CLI
Global flags: `--host <name>`, `--config <path>`, `--json`, `--timeout 30s`.

```
mantis-tui                                   # launch TUI
mantis-tui hosts                             # list configured hosts + status
mantis-tui list [--filter assigned] [--project <id|name>] [--page N] [--page-size N]
mantis-tui show <id> [--notes] [--history]
mantis-tui create --project <p> --category <c> --summary <s>
                  [--priority P] [--severity S] [--reproducibility R]
                  [--assign <user>] [-d <text> | --edit]
mantis-tui update <id>... [--status S] [--priority P] [--severity S]
                  [--category C] [--summary S] [--resolution R]
mantis-tui assign <id>... <user>
mantis-tui note <id> [-m <text> | --edit | -] [--time HH:MM] [--private]
mantis-tui note delete <id> <note-id> [--yes]
mantis-tui monitor <id>  |  mantis-tui unmonitor <id>
mantis-tui delete <id>... [--yes]
mantis-tui open <id>
```

- Enum values (status names and so on) are accepted by **name** and are
  case-insensitive. A name that isn't valid is an error that lists the valid
  values.
- Users can be given as a username, real name or numeric id. If a name
  matches more than one user, that's an error listing the candidates.
- Output is an aligned plain-text table by default. With `--json`, read
  commands (`list`, `show`) print the Mantis API's JSON unchanged. Write
  commands print a JSON array of per-issue results,
  `[{"id":33,"ok":true}, {"id":7,"ok":false,"error":"…"}]`, because they
  can touch several issues.
- `delete` and `note delete` ask for confirmation on a TTY. They refuse to run
  without `--yes` when stdin isn't a TTY.
- **Exit codes:** `0` ok, `1` API or server error, `2` usage or config error,
  `3` not found, `4` auth failure (401 or 403).
- `note -` reads the note text from stdin.

### Out of scope for v1
Uploading attachments, editing notes, adding or removing tags and
relationships, editing custom fields, saved server-side filters,
configurable keymaps, multiple panes or split view, Windows support.

(Split view was moved into v1.1, below.)

## v1.1: Beyond parity

v1 copied mantis.nvim closely. v1.1 adds things a full-screen terminal app can
do that a Neovim buffer can't do as well. Parity stays: every v1 key keeps
its meaning.

### Config additions

```toml
[list]
preview = true      # show the preview pane when the terminal is wide enough

[ui]
mouse = true        # mouse support; hold shift to select text with the mouse
```

Both default to `true`.

### Split view (preview pane)
- When `list.preview` is on and the terminal is at least **140 columns**
  wide, the list takes the left part of the screen and a preview of the
  issue under the cursor fills the right part (45% of the width, clamped to
  50–100 columns), separated by a vertical rule. Below 140 columns the list
  is full width, as in v1.
- `P` toggles the preview for the session.
- The preview shows what the issue view shows (header, description, notes),
  always on the Notes tab. `ctrl+d` / `ctrl+u` scroll it from the list.
  `enter` still opens the full issue view.
- The issue is fetched with `GET issues/{id}` once the cursor rests on it for
  150ms, so holding `j` doesn't fire a request per row. Fetched issues are
  cached per host and reused while the list row's `updated_at` matches, so
  moving back and forth costs nothing, and an auto-refresh that shows a newer
  `updated_at` refetches the preview.

### Vim-style open and back
- `l` opens the issue under the cursor (like `enter`), and `h` goes back from
  the issue view to the list (like `q` / `esc`). Paging stays on `L` / `H`.

### Unread tracking
- The TUI remembers, per host, the `updated_at` of each issue when you last
  saw it, in `$XDG_STATE_HOME/mantis-tui/seen.json` (mode `0600`, written
  atomically). The CLI never reads or writes it.
- An issue is **unread** when its `updated_at` is newer than the time you last
  saw it. Issues you have never opened count as seen at the moment the host
  was first used (the host's *baseline*), so the first run doesn't mark
  everything unread.
- "Seen" means: its issue view or preview finished loading. Your own changes
  from the TUI (field edits, notes, monitoring) also count as seen, so they
  never mark an issue unread.
- Unread rows show an accent `•` in the gutter and a bold summary. The status
  bar counts the unread issues on the page.
- `n` moves to the next unread issue (wrapping). `u` toggles read/unread for
  the selection, or the issue under the cursor. In the issue view, `u` marks
  the issue unread and goes back to the list.
- After adding a note, the TUI re-reads the issue to learn its new
  `updated_at`, because the note's timestamp and the issue's can differ.
- Each host keeps at most 5000 entries; the oldest are dropped first.

### Command palette
- `:` or `ctrl+p` opens a palette in the list and the issue view. Typing
  fuzzy-matches commands; `enter` runs the highlighted one, `esc` closes.
- Commands are every keybinding of the current screen (with its key shown),
  plus commands that have no key: each list filter, each other host, toggle
  preview and "mark page read".
- Typing a number (with or without `#`) offers **Open issue #N** first,
  which opens that issue even if it isn't in the loaded list.

### Mouse
- When `ui.mouse` is on, the TUI enables mouse reporting (click, release and
  wheel; no motion events).
- List: clicking a row moves the cursor there; clicking the row under the
  cursor again within 400ms opens it. The wheel moves the cursor three rows
  (over the list) or scrolls the preview (over the preview).
- Issue view: the wheel scrolls; clicking the Notes/History tab label
  switches tabs.
- Pickers, the palette and help: the wheel moves the cursor or scrolls, and
  clicking a picker option chooses it.

## v1.2: Keymap redesign

v1 copied mantis.nvim's keys. v1.2 picks keys that suit a terminal app,
without compatibility aliases for the old ones. Every v1 action still has a
key, which keeps success criterion 3.

- **Actions follow the selection.** In the list, `s` `p` `v` `c` `a` `D`
  act on the selected issues, or on the issue under the cursor when nothing is
  selected. The `b` chords (`bs` `bp` `bv` `bc` `ba` `bD`) are gone. Pickers
  name their target ("Status for 3 issues") and the status bar counts the
  selection.
- **`ctrl+h` is gone**, because many terminals send it for Backspace.
  `H` opens the host picker, and `1`–`9` switch straight to the Nth
  configured host.

| Action | v1 | v1.2 |
|---|---|---|
| switch host | `ctrl+h` | `H`; `1`–`9` for host N |
| next / previous page | `L` / `H` | `]` / `[` |
| half page down / up (list) | none | `ctrl+d` / `ctrl+u`, `pgdown` / `pgup` |
| scroll preview | `ctrl+d` / `ctrl+u` | `J` / `K` |
| next / previous issue (issue view) | none | `]` / `[` |
| next / previous unread | `n` / none | `n` / `N` |
| add note | `N` | `r` |
| refresh | `r` | `R` |
| severity | `V` | `v` |
| change summary | `S` | `e` |
| filter | `F` | `f` |
| clear selection | `ctrl+x` | `esc` (clears the search first, then the selection) |
| batch actions | `b` + key | the plain key, applied to the selection |
| copy issue URL | none | `y` (OSC 52, so it works over SSH) |

Unchanged: `j` `k` `gg` `G` `home` `end` `enter` `l` `q` `esc` `h` `/`
`space` `ctrl+a` `s` `p` `c` `a` `m` `o` `u` `D` `C` `P` `ctrl+g` `tab` `dn`
`?` `:` `ctrl+p`.

- In the issue view, `]` / `[` open the next or previous issue in the list's
  order, and move the list cursor with them, so going back lands on the issue
  you were reading.

## API Client Contract (`internal/mantis`)

- `Client{BaseURL, Token, HTTP *http.Client}`. Every method takes a
  `context.Context`, and the timeout defaults to 30s.
- Requests go to `<url>/api/rest/<endpoint>` with `Authorization: <token>`
  and `Content-Type: application/json`. Redirects are **not** followed, to
  avoid leaking the token cross-origin.
- A non-2xx response becomes `*APIError{Status, Message}`. `Message` is
  the `message` field from the JSON body if there is one, otherwise the raw
  body, truncated. Sentinels `ErrNotFound` and `ErrUnauthorized` are matched
  with `errors.Is`.
- Typed structs cover Issue, Note, User, Project, Category, Enum value and
  History entry. Unknown JSON fields are ignored.
- Endpoints (from mantis.nvim `api.lua`):

| Method | Endpoint | Use |
|---|---|---|
| GET | `issues?page_size&page&project_id&filter_id` | list/filter |
| GET/PATCH/DELETE | `issues/{id}` | show/update/delete |
| POST | `issues` | create |
| POST | `issues/{id}/notes` | add note (`text`, `view_state`, `time_tracking.duration`) |
| DELETE | `issues/{id}/notes/{nid}` | delete note |
| POST | `issues/{id}/monitors` | monitor (self) |
| PATCH | `issues/{id}` `{monitors:[…]}` | unmonitor: GET the monitor list, PATCH it back without self (2.24 has no DELETE route) |
| GET | `users/me` | current user, for monitor state and self-assign |
| GET | `projects`, `projects/{id}`, `projects/{id}/users` | metadata |
| GET | `config?option[]=…` | enums (`*_enum_string`), `status_colors` |

## Commands

```
Build:      go build -o bin/mantis-tui ./cmd/mantis-tui
Run:        go run ./cmd/mantis-tui [--host williamhleucka]
Test:       go test ./...
Test+race:  go test -race -cover ./...
Integration (read-only, real host):
            MANTIS_IT_HOST=williamhleucka go test -tags=integration ./internal/mantis/...
Lint:       golangci-lint run ./...
Format:     gofmt -l -w . && go vet ./...
Update TUI goldens: go test ./internal/tui/... -update
```

A `Makefile` wraps these as `make build | test | race | lint | fmt | it`.

## Project Structure

```
cmd/mantis-tui/main.go     → entrypoint; wires config → cobra root
internal/config/           → TOML load, env-token resolution, host selection, auto-detect,
                             last-used host state file
internal/mantis/           → REST client, types, errors (no TUI/CLI imports)
internal/meta/             → per-host cache of enums/projects/categories/users
internal/service/          → operations shared by CLI + TUI (name→id resolution,
                             batch runner, unmonitor dance, open-in-browser URL)
internal/cli/              → cobra commands, table/JSON output, exit codes
internal/tui/              → root model, routing between views, status bar, keymap
internal/tui/hostpicker/
internal/tui/issuelist/
internal/tui/issueview/
internal/tui/createform/
internal/tui/picker/       → generic single-select modal (status, user, category…)
internal/tui/confirm/      → yes/no modal
internal/editor/           → $EDITOR temp-file round trip
testdata/                  → recorded Mantis JSON fixtures, TUI golden files
SPEC.md, README.md, Makefile
```

Dependency direction: `cli`, `tui` → `service` → `meta` → `mantis`;
`config` is a leaf. `mantis` never imports anything internal.

## Code Style

`gofmt` and `goimports` formatting, golangci-lint defaults plus `errcheck`,
`revive`, `gocritic` and `misspell`. Errors are wrapped with `%w` and context.
There are no package-level mutable globals. Contexts are passed explicitly.
Bubble Tea models stay pure: all I/O goes through `tea.Cmd`, and results come
back as typed messages.

```go
// UpdateIssue PATCHes the given fields onto an issue.
func (c *Client) UpdateIssue(ctx context.Context, id int, patch IssuePatch) (*Issue, error) {
	var out issuesEnvelope
	if err := c.do(ctx, http.MethodPatch, fmt.Sprintf("issues/%d", id), patch, &out); err != nil {
		return nil, fmt.Errorf("update issue %d: %w", id, err)
	}
	if len(out.Issues) == 0 {
		return nil, fmt.Errorf("update issue %d: %w", id, ErrNotFound)
	}
	return &out.Issues[0], nil
}

// In a TUI view: side effects as commands, results as messages.
func (m Model) changeStatus(id int, status mantis.Ref) tea.Cmd {
	return func() tea.Msg {
		issue, err := m.svc.UpdateIssue(m.ctx, id, mantis.IssuePatch{Status: &status})
		return issueUpdatedMsg{issue: issue, err: err}
	}
}
```

Naming: exported types are nouns (`Issue`, `HostConfig`) and messages end in
`Msg`. Key bindings are declared once in `internal/tui/keymap.go` using
`bubbles/key`. The defaults started as the mantis.nvim keymap and were
redesigned in v1.2. A chord
helper handles multi-key sequences (`gg`, `dn`, `bs`…) with a 1-second
timeout.

## Testing Strategy

| Level | What | How |
|---|---|---|
| Unit: `config` | parsing, env resolution, dropped hosts, selection order, permission warning, auto-detect | table tests with `t.Setenv`, temp dirs |
| Unit: `mantis` | every endpoint's method, path, query, headers, body; error mapping; no redirect follow | `httptest.Server` serving fixtures from `testdata/` |
| Unit: `service` | name→id resolution and ambiguity, batch runner partial failure, unmonitor PATCH body | fake client interface |
| Unit: `editor` | template stripping, empty/unchanged abort, 0600 perms | fake editor = small shell script via `$EDITOR` |
| TUI | key → message → state transitions for each view; cursor-by-id preservation on refresh; chord handling | `teatest` against a fake service, golden snapshots at 120×40 |
| CLI | each subcommand's output, `--json`, exit codes, `--yes` / non-TTY refusal | cobra executed in-process against `httptest` |
| Integration | **read-only** smoke: `users/me`, `projects`, `issues?page_size=5`, `config` | `-tags=integration`, real host chosen via `MANTIS_IT_HOST` |

Coverage targets: `config`, `mantis` and `service` at **≥ 85%**, and
`internal/tui/...` at ≥ 60%. `go test -race ./...` must pass.

## Boundaries

- **Always:** run `gofmt`, `go vet` and `go test ./...` before calling a
  task done. Confirm destructive actions (delete issue or note, batch
  anything) in both TUI and CLI. Pass contexts with timeouts. Keep the TUI
  responsive by doing all network I/O inside `tea.Cmd`. Load enums from the
  server.
- **Ask first:** adding a dependency not in the Tech Stack table, changing
  the config schema, adding a feature listed as out of scope, and running any
  **write** request against a real host (`chainlogic` especially), including
  manual testing.
- **Never:** log, print or include API tokens in errors, debug output, panics
  or test fixtures. Follow redirects with the auth header. Commit a config
  containing tokens. Run automated tests that write to a real Mantis host.
  Skip or delete a failing test to get green.

## Success Criteria

1. With the two hosts configured and both env vars set, `mantis-tui` shows a
   host picker. With `default = true` on one host it goes straight to that
   host's list. Unsetting `MANTIS_CL` removes `chainlogic` from the picker.
2. The list loads the configured default filter in under 2s on a normal
   connection, shows a spinner while loading, and pages with `L` / `H`.
3. Every single-issue and batch keybinding in mantis.nvim's README has a
   working equivalent, checked against the keymap table in the README.
4. Changing status, priority, severity, category or summary from the list
   updates the row without a full reload. A failed request shows the server's
   error message in the status bar and leaves the row unchanged.
5. Adding a note in `$EDITOR` (nvim) round-trips correctly. Saving an empty
   buffer sends nothing. Time tracking `0:30` shows up on the server.
6. Creating an issue from the TUI and from `mantis-tui create` both produce
   an issue with the requested project, category, priority, severity,
   reproducibility and handler.
7. A batch status change on 10 issues where 1 is rejected reports 9
   successes and 1 failure, and only the failed issue stays selected.
8. Auto-refresh happens every interval without moving the cursor off the
   current issue id, even when rows reorder.
9. `mantis-tui show <id> --json | jq .issues[0].id` works, and the CLI exit
   codes match the table above (verified by tests).
10. `grep -r` across logs, debug output and test fixtures finds no token
    values, and a test asserts that `APIError.Error()` never contains the
    token.
11. `go test -race ./...` and `golangci-lint run` are clean, and the
    coverage targets are met.

## Resolved Decisions

1. **Binary name:** `mantis-tui`.
2. **Distribution:** `go install` only for v1.
3. **Last-used host:** remembered in the XDG state file. It sits below
   `default = true` in the selection order.
4. **Shared config with mantis.nvim:** no. Each tool keeps its own config.
5. **History:** included in the standard `GET issues/{id}` response
   (`created_at`, `user`, `type{id,name}`, `message`), verified with a
   read-only call against the wh host (MantisBT 2.27.0).

## Open Questions

None at this point.
