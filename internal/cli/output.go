package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

// writeRaw prints the server's JSON verbatim.
func writeRaw(w io.Writer, raw json.RawMessage) error {
	_, err := fmt.Fprintf(w, "%s\n", raw)
	return err
}

func handlerName(is mantis.Issue) string {
	if is.Handler == nil || is.Handler.ID == 0 {
		return "-"
	}
	return is.Handler.Display()
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02")
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04")
}
