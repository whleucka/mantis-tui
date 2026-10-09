# AGENTS.md

A Go terminal UI and CLI for MantisBT. User docs are in `README.md` and
`docs/usage.md`; this file covers what isn't obvious from the code.

## Commands

```sh
make build   # bin/mantis-tui
make test    # go test ./...
make lint    # golangci-lint, pinned, via go run
make fmt     # gofmt + go vet
```

`make it` runs smoke tests against a real Mantis host. Don't run it without
asking.

## Layout

- `internal/mantis`: REST client (`mantis.API`); `mantistest` is its in-memory fake for tests.
- `internal/service`: operations shared by the CLI and the TUI.
- `internal/cli` (cobra) and `internal/tui` (Bubble Tea v2): the two front ends.
  A new action usually needs both.
- `internal/config`: config file, defaults, validation, read state.
- `cmd/mantis-devserver`: fake Mantis on `http://127.0.0.1:8989` (any token)
  for trying the TUI or CLI without a real tracker.

## Gotchas

- Tests check the docs:
  - Every key binding must appear in `docs/usage.md` (`TestUsageDocumentsEveryKey`).
  - The README's toml sample must load.
- The config loader rejects unknown keys. A new setting needs a default in
  `config.Defaults`, validation in `validate`, and a line in the config sample
  in `docs/usage.md`.
- TUI golden files live in `internal/tui/testdata`. After an intended UI
  change, run `go test ./internal/tui -update` and review the diff.
- Never put real ticket data (names, hosts, IDs, query output) in tests or
  fixtures. Use made-up data. `scripts/record-fixtures.sh` scrubs recorded
  fixtures.
