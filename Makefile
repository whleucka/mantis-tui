BIN := bin/mantis-tui
GOLANGCI_LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2

.PHONY: build test race lint fmt it

build:
	go build -o $(BIN) ./cmd/mantis-tui

test:
	go test ./...

race:
	go test -race -cover ./...

lint:
	$(GOLANGCI_LINT) run ./...

fmt:
	gofmt -l -w .
	go vet ./...

# Read-only smoke tests against a real host, e.g. MANTIS_IT_HOST=williamhleucka make it
it:
	go test -tags=integration ./internal/mantis/...
