package cli

import (
	"errors"
	"fmt"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/service"
)

// Exit codes, as documented in SPEC.md.
const (
	exitOK       = 0
	exitAPI      = 1
	exitUsage    = 2
	exitNotFound = 3
	exitAuth     = 4
)

// usageError marks bad flags, arguments or config (exit code 2).
type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

func usageErrorf(format string, args ...any) error {
	return &usageError{fmt.Errorf(format, args...)}
}

func asUsage(err error) error {
	if err == nil {
		return nil
	}
	return &usageError{err}
}

// ExitCode maps an error returned by the command tree to a process exit code.
func ExitCode(err error) int {
	var usage *usageError
	var invalid *service.InvalidValueError
	var ambiguous *service.AmbiguousError
	var missing *service.MissingFieldError
	switch {
	case err == nil:
		return exitOK
	case errors.As(err, &usage), errors.As(err, &invalid), errors.As(err, &ambiguous), errors.As(err, &missing):
		return exitUsage
	case errors.Is(err, mantis.ErrUnauthorized):
		return exitAuth
	case errors.Is(err, mantis.ErrNotFound):
		return exitNotFound
	default:
		return exitAPI
	}
}
