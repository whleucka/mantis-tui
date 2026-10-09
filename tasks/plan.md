# Implementation Plan: mantis-tui

## Overview

This builds the Go and Bubble Tea MantisBT client described in `SPEC.md`: a
full-screen TUI and a non-interactive CLI that share one config, API client
and service layer. Work is sliced vertically. The first slice is a tracer
bullet that runs config → API client → `mantis-tui list` against the real wh
host. After that, the CLI write path is completed so the service layer is
proven before any TUI depends on it. Then the TUI read path, the TUI write
path, and finally polish.

## Architecture Decisions

- **The service layer is shared by CLI and TUI.** Name→id resolution, the
  batch runner, the unmonitor step (GET the monitor list, PATCH it back) and
  browser URLs live in `internal/service`, so the CLI and TUI can't drift
  apart in behaviour. The CLI is built first because it's the cheapest way to
  test the service layer end to end.
- **The client sits behind an interface.** `service` depends on a small
  `mantis.API` interface rather than `*mantis.Client`, so service, CLI and TUI
  tests use a fake and never touch a network.
- **Fixtures are recorded from wh, then scrubbed.** Real response shapes for
  2.27.0 come from read-only calls. Emails, real names and tokens are
  replaced before anything is committed under `testdata/`. The wh history
  entries include emails, so scrubbing is mandatory.
- **Bubble Tea v2.** v2 was stable (v2.0.10) at Task 1, so the user approved
  using v2 (bubbletea, bubbles and lipgloss from the `charm.land/*` import
  paths) instead of v1. SPEC.md's Tech Stack table has been updated.
- **Manual checks run in herdr panes.** The user works inside herdr, a
  terminal multiplexer for agents. Each "Manual" verification step launches
  `mantis-tui` in a sibling pane (`herdr pane split --current --no-focus`),
  drives it with `herdr pane send-keys` / `run`, and reads the screen with
  `herdr pane read`. This replaces asking the user to try things by hand,
  and is especially useful for the nvim `$EDITOR` round trips.
- **The TUI is one flat `internal/tui` package** with a file per view,
  instead of the planned sub-packages. Views have to send actions to the
  root (open issue, act on the selection), and sub-packages would need an
  extra shared-messages package just for that.
- **The issue list is rendered by hand** rather than with the bubbles
  `table`. The table can't color cells per row, show selection markers or
  draw group headers, and its default keys clash with `space`, `ctrl+a`
  and `g`.
- **Chords are handled at the TUI root.** One small chord state machine
  (with a 1-second timeout) sits in front of the view routing, so `gg`, `dn`
  and `b*` behave the same in every view.

## Task List

The full task details are in `tasks/todo.md`.

### Phase 1: Foundation and tracer bullet
- [x] Task 1: Scaffold the module, Makefile and lint config, with a cobra root that runs
- [x] Task 2: Config loading and host selection, plus `mantis-tui hosts`
- [x] Task 3: API client core and the read endpoints, with scrubbed fixtures
- [x] Task 4: CLI `list` and `show` with table and JSON output, and exit codes

### Checkpoint A: tracer bullet
- [x] `mantis-tui list --host williamhleucka` and `show <id> --json` work against the real wh host (and chainlogic, read-only)

### Phase 2: CLI write path and service layer
- [x] Task 5: Metadata cache and name→id resolution
- [x] Task 6: Client write endpoints, including the unmonitor step
- [x] Task 7: CLI `update`, `assign`, `monitor`, `unmonitor` and `open`, plus the batch runner
- [x] Task 8: Editor package and CLI `note` / `note delete`
- [x] Task 9: CLI `create` and `delete`, with the TTY and `--yes` guard

### Checkpoint B: CLI complete
- [x] All of the CLI surface in the spec works. Tests are green and the race detector is clean.
- [x] One manual write round trip on wh (create, update, note, delete), **only after the user approves it** (approved; done in `ideas`)

### Phase 3: TUI read path
- [x] Task 10: TUI root, keymap and chord helper, status bar, host picker and host switching
- [x] Task 11: Issue list with filters, pagination, spinner, icons and status colours
- [x] Task 12: List extras: group by project and the `/` search
- [x] Task 13: Issue view with header, body, Notes/History tabs and scrolling
- [x] Task 14: Auto-refresh for the list and the issue view

### Checkpoint C: TUI browse
- [x] I can launch the TUI, pick a host, filter and page the list, open an issue, read notes and history, switch hosts, and auto-refresh keeps the cursor in place

### Phase 4: TUI write path
- [x] Task 15: Picker and confirm modals, plus the single-issue actions in the list and the issue view
- [x] Task 16: Add and delete notes from the TUI, using `$EDITOR`
- [x] Task 17: Selection and batch operations
- [x] Task 18: Create-issue form

### Checkpoint D: parity
- [x] Every key in mantis.nvim's README keymap tables has a working equivalent (spec success criterion 3). The exceptions are `?`, which lands in Task 19, and nvim's `<C-s>` layout toggle, which doesn't apply.

### Phase 5: Polish
- [x] Task 19: Help overlay, polished error display, README
- [x] Task 20: Success-criteria audit: token-leak test, coverage targets, lint

### Checkpoint E: complete
- [x] All 11 success criteria in SPEC.md are met. Ready for review. Evidence is in `tasks/todo.md`.

### Phase 6: Beyond parity (v1.1)
- [x] Task 21: Split view with a preview pane
- [x] Task 22: Unread tracking
- [x] Task 23: Command palette and jump to issue
- [x] Task 24: Mouse support
- [x] Task 25: Docs and a live check

### Phase 7: Keymap redesign (v1.2)
- [x] Task 26: Keys that suit a terminal app

### Phase 8: Whole-filter list, sorting, display (v1.3)
- [x] Task 27: The list holds the whole filter
- [x] Task 28: Sorting
- [x] Task 29: Relative times, status colours, handler column

### Phase 9: New-issue notifications (v1.4)
- [x] Task 30: Watch every host and notify about new issues

### Phase 10: Long fields and related issues (v1.5)
- [x] Task 31: Edit the description, steps and additional information
- [x] Task 32: Go to a related issue, with a back trail

### Phase 11: Attachments and images (v1.6)
- [x] Task 33: Download attachments: API, service, CLI `files` and `download`
- [x] Task 34: Open attachments in the TUI, images with kitten icat

### Phase 12: Inline images (v1.7)
- [x] Task 35: `internal/graphics`: fit, scale, kitty transmit and placeholder text
- [x] Task 36: Thumbnails in the issue view: detection, background loading, cleanup

## Findings During Build

- On 2.27, `GET /issues` includes the full `history` for every issue in a
  list. List views should pass `select=` to skip history, especially with
  `page_size = 500`.
- History entries carry a plain-text `change` string and file-added
  `message`s that can contain personal data. The fixture scrubber handles
  both.

- wh has **time tracking disabled**, and Mantis answers a note with
  `time_tracking` with `403 time tracking disabled`. `meta` now reads
  `time_tracking_enabled` in the same `/config` call. The CLI rejects
  `--time` up front with a usage error. The TUI note form (Task 16) must
  hide the time field when it's disabled.
- Checkpoint B real writes (approved, `ideas` project on wh): create,
  update, private note, monitor, unmonitor and delete all worked. Test
  issues #34 and #35 were deleted afterwards.

- On 2.27, attachments are attached to **notes**, not the issue. `Note`
  has an `Attachments` field, and the issue view lists them under their
  note.

## Parallelization

After Task 5, the CLI tasks (6–9) and the TUI read path (10–14) touch
separate packages and could run in parallel. The rest is sequential.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| chainlogic runs a different MantisBT version from wh: confirmed 2.28.4 vs 2.27.0. The read-only smoke test passes on both. | Low | Lenient decoding that ignores unknown fields. A read-only smoke test against chainlogic at Checkpoint A (reads are allowed; writes are not). |
| Fixtures leak personal data (emails in history and users) | High | A scrub step in the fixture script, plus a test that fails if `testdata/` contains `@` outside known placeholder domains |
| `teatest` is an experimental API (`x/exp`) and may change | Low | Pin the version. Keep most TUI tests as plain `Update()` message tests and use teatest only for a few goldens. |
| Chord keys clash with bubbles' default table bindings (`space`, `ctrl+a`, `g`) | Med | Build the chord helper in Task 10 with tests before any views exist, and override the table keymap explicitly |
| `tea.ExecProcess` with nvim leaves the terminal in a bad state | Med | Test this early in Task 8 (CLI) and again in Task 16 (TUI) with real nvim |
| Writes against real hosts during development | High | Writes are tested only against `httptest`. Manual writes happen only at Checkpoint B, on wh, with approval. chainlogic is never written to. |

## Open Questions

None. The spec's open questions are resolved.
