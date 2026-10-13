# Roadmap: candidates after v1.8

Phases 1–13 in `plan.md` are done (Phase 13's boxes were never ticked; its
code landed in `f9d5ed8`). This is a menu, not a plan. Pick items, and each
one gets a spec and tasks as usual.

Size: **S** ≈ an evening, **M** ≈ a few sessions, **L** ≈ a phase of its own.
★ marks my picks.

---

## 0. Before v0.0.2 (release hygiene)

| # | Item | Size |
|---|---|---|
| 0.1 ✅ | **`make lint` is broken.** golangci-lint v2.13.2 can't read Go 1.27 export data. Bump the pin to a release built for 1.27. | S |
| 0.2 ✅ | **Versioning.** There are no git tags and no `mantis-tui --version`. Stamp the version with `-ldflags -X` from `git describe` in the Makefile, add `version` to the CLI and the help footer, then tag `v0.0.2`. | S |
| 0.3 ✅ | **Stale docs.** `SPEC.md` is gone but still referenced 9 times. Tick Phase 13 after the two live checks (CLI upload against the dev server, a clipboard thumbnail from the TUI in herdr). | S |
| 0.4 | **Release build.** A `make release` (or goreleaser) that builds linux/amd64 and darwin/arm64 with `-trimpath -s -w`. The binary is 19.5 MB now; stripping should save ~30%. | S |

---

## 1. Performance

| # | Item | Why | Size |
|---|---|---|---|
| 1.1 ✅ | **Incremental refresh.** Each tick refetches the whole filter (up to `max_issues`/`page_size` = 10 requests) on *every* host. Instead, fetch page 1 sorted by last-updated, merge it, and stop paging once a page has nothing newer than what's held. Run a full sweep every Nth tick (or on `R`) so issues that left the filter or were deleted drop out. | ~10× less background traffic and server load; refreshes feel instant. **Check first:** does 2.27/2.28 `GET /issues` order by `last_updated desc` by default? | M |
| 1.2 ✅ | **Instant cold start.** Write each host+filter list (and the preview cache) to `$XDG_CACHE_HOME/mantis-tui/` on quit, and show it immediately on launch, marked stale, while the real load runs (stale-while-revalidate). | The list shows at once instead of after N round trips. Pairs with 1.1: launch becomes one incremental refresh. | M |
| 1.3 ✅ | **Memoize `rows()`.** It re-runs the fuzzy search (and the grouping sort) over every issue, several times per View and per keypress (`list.go` calls it 9 times). Cache it, invalidated by issues/search/group/sort. Add a benchmark at 1000 and 5000 issues. | `j` held down on a big grouped and searched list stays smooth. Cheap, measurable. | S |
| 1.4 | **Adaptive polling.** Use Bubble Tea v2 focus reporting: poll at the configured rate while focused, back off (e.g. ×4) when unfocused, back off exponentially on network errors, and show an "offline" badge instead of an error each tick. | Fewer requests while it idles in a pane all day; laptop sleep/VPN drops don't spam errors. | S |
| 1.5 | **Concurrent chunk loading.** Once page 1 returns, fetch the remaining pages 3–4 at a time instead of one by one. Only matters for full sweeps once 1.1 lands. | Full loads finish in about a quarter of the time. | S |
| 1.6 | **Drop `Raw` from list chunks.** `IssueList.Raw` keeps every page's JSON alive for `--json` passthrough, which the TUI never needs. | Less memory per host with 4 hosts × 1000 issues watched. Small, but free. | S |

---

## 2. Gnarly features

### Workflow

| # | Item | Size |
|---|---|---|
| 2.1 ★ | **Edit the selection as text (oil.nvim for tickets).** Select issues, press a key, and `$EDITOR` opens a table of `id  status  priority  handler  summary`. Edit cells, save, and mantis-tui diffs the result and shows a confirm ("3 status changes, 1 reassign, 2 renames"), then applies it through the batch runner. Unknown values fail per line with the valid list. Deleting a line does nothing (no surprise deletes). This is the "triage 40 tickets in 2 minutes" feature. | M |
| 2.2 ★ | **"What changed since I last looked."** Unread tracking already knows *when* you last saw an issue. In the issue view, mark notes and history entries newer than that (gutter bar or "new" badge), open scrolled to the first new note, and give `n`/`N` a meaning there (next/previous new note). The preview pane says "2 new notes, status → resolved" at the top. | M |
| 2.3 | **Kanban view.** `B` shows the current list as columns by status (or by handler). `h`/`l` moves between columns, `H`/`L` moves the issue, which is a status PATCH, with the same no-optimistic-update rule. Columns scroll on their own. | L |
| 2.4 | **Undo the last change.** Keep the previous field values of the last batch. For ~10s, `ctrl+z` (or `U`) patches them back. Notes and deletes can't be undone; the status bar says so. | S |
| 2.5 | **Resolve/close flow.** `x` asks for a resolution (fixed, won't fix, duplicate → asks for the issue), opens the editor for a closing note, then does the PATCH and the note together. A failure halfway reports what happened. Optional `--resolve` on the CLI `note`. | S |
| 2.6 | **Note and issue templates.** `[templates]` in the config: named bodies with `{id}`, `{summary}`, `{branch}` and `{commit}` placeholders. `r` offers them when any are defined; `create --template bug`. | S |
| 2.7 | **Time tracking timer.** `T` starts a timer on an issue (persisted in state, shown in the status bar across restarts). `T` again stops it and opens the note form with the duration filled in. Hidden on hosts without time tracking. | S |

### Finding things

| # | Item | Size |
|---|---|---|
| 2.8 ★ | **Query language for `/` and saved views.** `/status:new,feedback prio:>=high @me -cat:docs age:<7d updated:<2d login` parses into field filters plus fuzzy words; the plain fuzzy search still works. Then `[[views]]` in the config (name, host(s), filter, query, sort, group) shows as tabs or `1`–`9`-style switching and appears in the palette. This brings project filtering, which the TUI lacks today (only the CLI has `--project`). | M |
| 2.9 | **Server-side saved filters.** List the user's Mantis saved filters and use them as `filter_id`, so filters built in the web UI show up in `f`. **Check first:** whether 2.27's REST exposes the list of saved filters. | S–M |
| 2.10 | **Local full-text search.** Every issue the preview or issue view has fetched (description, notes) is already in memory, so index it (cached on disk with 1.2) and let `?/` or the palette search note bodies across hosts. Results say which issues haven't been indexed yet. | M |
| 2.11 | **All-hosts list.** Every host is already refreshed in the background. Add a merged view (`0`) with a HOST column, where actions route to the right host. | M |

### Editing what's missing

| # | Item | Size |
|---|---|---|
| 2.12 | **Tags.** Show them in the list (as a column or chips) and add/remove with `t` (picker with create-new). REST: `POST/DELETE issues/{id}/tags`. | S |
| 2.13 | **Relationships.** `gr` already reads them; `L` adds a link (related, duplicate of, parent/child) by picking an issue from the palette's jump-to-number. REST: `POST issues/{id}/relationships`. | S |
| 2.14 | **Render Markdown/BBCode.** If the server has the Markdown option on, render descriptions and notes (headings, lists, links, bold) instead of showing raw `**`. `<pre>` handling stays. | M |

---

## 3. Integrations

| # | Item | Size |
|---|---|---|
| 3.1 ★ | **`mantis-tui mcp`: a built-in MCP server.** Expose list/show/files/download/note/update/assign/create as MCP tools over stdio, reusing `service`. Writes are flagged so Claude Code asks first. This replaces the CLI-plus-curl approach in `docs/mantis-agent.md`, gives Claude structured results and errors, and `A` (ask Claude) can launch with it pre-wired. Images from `download` can be returned as image content, so Claude can *see* screenshots. | M |
| 3.2 ★ | **Git awareness.** Find the issue id in the current branch (`fix/19112-login`) or the last commit. `mantis-tui note -` with no id uses it; `mantis-tui branch <id>` makes `<id>-<slug-of-summary>`; and `mantis-tui hook install` adds a post-commit hook (opt-in) that posts `Commit abc123: subject` as a note on the branch's issue. In the TUI, the issue view shows "branch `19112-login` exists locally" when it finds one. | M |
| 3.3 | **Headless watcher.** `mantis-tui watch` runs only the new-issue/unread notifier (no UI), for a systemd user unit or a herdr background pane, with `--once` for cron and a JSON line per event for scripting. | S |
| 3.4 | **Daily digest.** `mantis-tui digest [--since 24h] [--json]`: what changed on your assigned, reported and monitored issues (new notes, status changes, new issues), grouped by issue. Good standup input, and good input for Claude via 3.1. | S |

---

## Done for v0.0.2 (2026-10-13)

- **0.1** golangci-lint pinned to v2.14.0.
- **0.2** `internal/version`, stamped by `make build` from `git describe`; `mantis-tui --version`; shown in the help overlay's footer.
- **0.3** `plan.md`/`todo.md` point at `SPEC.md` in git history. Phase 13 is ticked, the thumbnail check confirmed by eye.
- **1.3** `rows()` is cached. `BenchmarkListMove` (grouped and searched, j plus a redraw): 1000 issues 28.5 → 2.4 ms, 5000 issues 145 → 2.8 ms.
- **1.1** Confirmed on both real hosts (2.27 and 2.28.4) that `GET /issues` comes newest-updated first. Auto-refresh now pages only until it reaches older issues (sticky ones don't count), and does a full sweep every 10 minutes, on `R`, and on a filter change. Live on the dev server with one issue per page: a full load took 4 requests, a quiet tick 2, a tick after an outside change 3.
- **1.2** Complete loads are saved to `$XDG_CACHE_HOME/mantis-tui/lists/<host>.<filter>.json` (0600). They show at once on start and on a filter change, marked "saved list, refreshing…", and stay with their age when the server is down. `--issue` panes don't use them.

## Suggested cut for v0.0.2

The release, plus the cheapest big wins:

1. 0.1–0.3 (lint, version, docs/ticks), then tag.
2. 1.3 (memoize rows), the S-sized warm-up.
3. 1.1 + 1.2 (incremental refresh + instant start), the biggest change you'll feel day to day.

Next after that: 2.1 (edit the selection as text) or 3.1 (MCP server), whichever you'd use more.
