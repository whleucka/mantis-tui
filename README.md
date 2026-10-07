# mantis-tui

A terminal UI and scriptable CLI for the [MantisBT](https://mantisbt.org) bug
tracker. It has feature parity with
[mantis.nvim](https://github.com/whleucka/mantis.nvim) and doesn't need Neovim.

- **TUI:** browse, filter, search and group issues across several Mantis
  hosts. Change status, priority, severity, category, summary and assignee.
  Write notes and descriptions in `$EDITOR`, and batch-edit a selection.
  Beyond mantis.nvim: a split view with a live preview, unread markers for
  issues that changed since you looked, a command palette that jumps to any
  issue by number, and mouse support.
- **CLI:** the same operations as one-shot commands, with `--json` output and
  stable exit codes for scripts and agents.

## Install

Requires Go 1.24 or newer and MantisBT 2.24 or newer, with the REST API
enabled.

```sh
go install github.com/whleucka/mantis-tui/cmd/mantis-tui@latest
```

## Configuration

Create `~/.config/mantis-tui/config.toml` (or `$XDG_CONFIG_HOME/mantis-tui/config.toml`,
or pass `--config <path>`):

```toml
[list]
default_filter = "assigned"   # all | assigned | reported | monitored | unassigned
page_size = 100               # issues per request
max_issues = 1000             # the TUI list loads the whole filter up to this
sort = "updated"              # updated | priority | severity | status | id | summary
auto_refresh = "120s"         # "0" disables
group_by_project = false
preview = true                # split view on terminals ≥ 140 columns

[issue]
auto_refresh = "120s"

[ui]
mouse = true                  # hold shift to select text with the mouse
notify = "auto"               # auto | herdr | terminal | off

[icons]
immediate = "🔥"
urgent    = "⚠️"
high      = "🔺"
normal    = "🔵"
low       = "🔻"
none      = "🟣"
monitor   = "👁️"

[[hosts]]
name = "work"
url  = "https://mantis.example.com"   # base URL, without /api/rest
env  = "MANTIS_WORK"                   # env var holding the API token

[[hosts]]
name = "personal"
url  = "https://bugs.example.org"
env  = "MANTIS_PERSONAL"
default = true
```

Every section except `[[hosts]]` is optional; the values above are the
defaults, apart from `default_filter`, which defaults to `all`.

**API tokens.** In Mantis, go to **My Account → API Tokens** and create a
token. Export it in your shell profile (for example `export MANTIS_WORK=…`).
A host can use `token = "…"` instead of `env`, but then keep the file private
(`chmod 600`). mantis-tui warns you if it isn't.

**Hosts.**
- A host whose env variable isn't set is **dropped**, the same as in
  mantis.nvim. `mantis-tui hosts` shows which hosts are active and why any
  were dropped.
- The host is chosen in this order: `--host <name>`, then `$MANTIS_TUI_HOST`,
  then the host with `default = true`, then the only host if there's just
  one, then the host you used last. If none of those settles it, the TUI
  shows a picker and the CLI asks you to pass `--host`.
- The last-used host is saved in `~/.local/state/mantis-tui/state.toml`.
  Read/unread state is kept next to it in `seen.json`.
- With no config file, mantis-tui looks for pairs of `MANTIS_<NAME>` (the
  token) and `MANTIS_<NAME>_URL` environment variables.

## TUI

Run `mantis-tui` with no arguments. Press `?` anywhere for the keys of the
current screen.

### Issue list

The list holds every issue the filter matches (up to `list.max_issues`). The
first chunk shows at once and the rest loads in the background; refreshes
swap the list in only when they finish, so it never shrinks mid-refresh.
Times within the last week show as relative ("3h ago"). On a wide list a
HANDLER column appears.

Actions that change issues (`s` `p` `v` `c` `a` `D`) apply to the selected
issues, or to the issue under the cursor when nothing is selected. Pickers
name their target ("Status for 3 issues").

| Key | Action |
|---|---|
| `j`/`k`, `up`/`down` | move |
| `ctrl+d`/`pgdown`, `ctrl+u`/`pgup` | half page down / up |
| `gg`/`home`, `G`/`end` | first / last issue |
| `enter`/`l` | view issue |
| `f` | filter (all, assigned, reported, monitored, unassigned) |
| `S` | sort by updated, priority, severity, status, id or summary (again to reverse) |
| `/` | search the list (id, summary, category, handler) |
| `esc` | clear the search, then the selection |
| `n` / `N` | next / previous unread issue |
| `u` | toggle read / unread |
| `space` | toggle selection |
| `ctrl+a` | select every shown issue |
| `s` / `p` / `v` / `c` | change status / priority / severity / category |
| `a` | assign |
| `e` | edit the summary |
| `r` | add a note (reply) |
| `m` | toggle monitoring (👁️ marks monitored issues) |
| `o` | open in browser |
| `y` | copy the issue URL (OSC 52, works over SSH) |
| `C` | create issue |
| `D` | delete issue(s) |
| `P` | toggle the preview pane |
| `J` / `K` | scroll the preview |
| `ctrl+g` | group by project |
| `R` | refresh |
| `H` | switch host |
| `1`–`9` | switch to the Nth host in the config |
| `:` / `ctrl+p` | command palette |
| `?` | toggle help |
| `q` | quit |

If a change to several issues partly fails, only the issues that failed stay
selected, so you can retry them.

**Split view.** On terminals at least 140 columns wide, the right side
shows a preview of the issue under the cursor: its details, description and
notes. It loads once the cursor rests on a row and is cached until the issue
changes. `P` hides or shows it, and `list.preview = false` starts with it
hidden.

**Unread issues.** An issue that changed since you last opened or previewed
it shows a `•` and a bold summary, and the status bar counts them. On the
first run with a host nothing is unread; from then on, other people's
changes are. Your own edits and notes never mark an issue unread.

**Command palette.** `:` or `ctrl+p` lists every command on the current
screen with its key, plus each filter, each other host and "Mark all read".
Type to narrow it down. Type a number (`1234` or `#1234`) to open that issue,
even if it isn't in the list.

**New-issue alerts.** While mantis-tui runs, every configured host's list
refreshes on `list.auto_refresh`, including hosts you aren't looking at.
When an issue appears in a list that wasn't there before, the status bar
says so ("2 new on work: #41 …") and a notification goes out:

- `notify = "auto"` uses [herdr](https://herdr.dev)'s `herdr notification
  show` inside herdr, and the terminal's own notification (OSC 9: iTerm2,
  WezTerm, Ghostty, kitty, Windows Terminal) elsewhere.
- `"herdr"` or `"terminal"` force one; `"off"` keeps only the status bar.

Issues you reported or just created don't count, and neither does the first
load or a filter change. The window title shows the unread count across all
hosts, e.g. `(3) mantis-tui`. Setting `list.auto_refresh = "0"` turns all of
this off.

**Mouse.** Click a row to move to it and click it again to open it. The
wheel scrolls the list, the preview, the issue view, help and pickers.
Clicking a picker option or a palette command chooses it, and clicking
Notes/History switches tabs. Mouse reporting stops the terminal's own text
selection, so hold `shift` to select text, or set `ui.mouse = false`.

### Issue view

| Key | Action |
|---|---|
| `j`/`k`, `up`/`down` | scroll |
| `ctrl+d`/`pgdown`, `ctrl+u`/`pgup` | half page down / up |
| `gg` / `G` | top / bottom |
| `]` / `[` | next / previous issue in the list |
| `tab` | switch between notes and history |
| `r` | add a note (reply) |
| `dn` | delete a note |
| `s` `p` `v` `c` `a` `e` `m` `o` `y` | same as in the list, for this issue |
| `R` | refresh |
| `u` | mark unread and go back to the list |
| `q`/`esc`/`h` | back to the list |
| `H`, `1`–`9`, `:`/`ctrl+p`, `?` | same as in the list |

### Notes and the create form

- **Notes:** `r` asks whether the note is private, and for the time spent
  (only if the server has time tracking enabled). It then opens
  `$VISUAL`/`$EDITOR` (falling back to `vi`). Lines starting with `# ` that
  mantis-tui inserted are removed. Saving an empty file cancels the note. If
  sending fails, your text is kept in a temp file and the status bar shows
  its path.
- **Create (`C`):** `tab`/arrow keys move between fields. `enter` picks a
  value. `e` writes the description in your editor. `alt+enter` (or `ctrl+s`)
  creates the issue, and `esc` cancels.

## CLI

```sh
mantis-tui hosts
mantis-tui list [--filter assigned] [--project <id|name>] [--page N] [--page-size N]
mantis-tui show <id> [--notes] [--history]
mantis-tui create --project P --category C --summary S [-d text | --edit]
                  [--priority P] [--severity S] [--reproducibility R] [--assign user]
mantis-tui update <id>... [--status S] [--priority P] [--severity S]
                  [--category C] [--summary S] [--resolution R]
mantis-tui assign <id>... <user>
mantis-tui note <id> [-m text | --edit | -] [--time H:MM] [--private]
mantis-tui note delete <id> <note-id> [--yes]
mantis-tui monitor <id>...   |   mantis-tui unmonitor <id>...
mantis-tui delete <id>... [--yes]
mantis-tui open <id>
```

These flags work with every command: `--host <name>`, `--config <path>`,
`--json` and `--timeout 30s`.

- Status, priority and other values are taken by **name or label**, in any
  case, as your server defines them. An unknown value is an error that lists
  the valid ones.
- A user can be given as a username, real name or numeric id.
- `note -` reads the note text from stdin. With no text source on a
  terminal, `note` opens your editor.
- `delete` and `note delete` ask for confirmation on a terminal. Without a
  terminal, they refuse unless you pass `--yes`.
- With `--json`, `list` and `show` print the server's JSON unchanged. Write
  commands print one result per issue:
  `[{"id":33,"ok":true}, {"id":7,"ok":false,"error":"…"}]`.

| Exit code | Meaning |
|---|---|
| 0 | ok |
| 1 | API or server error, including partly failed batches |
| 2 | usage or config error (bad flag or value, ambiguous host) |
| 3 | not found |
| 4 | authentication or permission failure (HTTP 401/403) |

```sh
mantis-tui show 1234 --json | jq '.issues[0].status.name'
git log -1 --format=%B | mantis-tui note 1234 -
mantis-tui update 1201 1202 1203 --status resolved
```

## Troubleshooting

- **"environment variable … is not set"**: export the token before you
  start mantis-tui. `mantis-tui hosts` shows which hosts are active.
- **"server answered with a redirect"**: the `url` is probably wrong
  (http instead of https, or a missing path). mantis-tui never follows
  redirects, so your token is never sent to another site.
- **"time tracking is disabled"**: the server doesn't accept time spent on
  notes. Drop `--time`. The TUI hides the field on such servers.
- **`alt+enter` doesn't submit**: some terminals intercept it. Use `ctrl+s`.
- **HTTP 403**: your token's account lacks permission for that action in
  that project.

## Development

```sh
make build      # bin/mantis-tui
make test       # go test ./...
make race       # race detector + coverage
make lint       # golangci-lint (pinned, run via go run)
make it         # read-only smoke test against a real host:
                #   MANTIS_IT_HOST=<host> [MANTIS_IT_CONFIG=path] make it
```

- `go run ./cmd/mantis-devserver` serves an in-memory Mantis seeded from the
  scrubbed test fixtures, so you can try write operations without touching a
  real host. Point a config host at `http://127.0.0.1:8989`, with any token.
- `scripts/record-fixtures.sh` re-records the fixtures from a real server
  using only GET requests, and scrubs personal data.
- `SPEC.md` is the specification, and `tasks/` holds the build plan.
