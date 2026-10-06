// Command mantis-tui is a terminal UI and CLI for the MantisBT bug tracker.
package main

import (
	"fmt"
	"os"

	"github.com/whleucka/mantis-tui/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "mantis-tui:", err)
		os.Exit(1)
	}
}
