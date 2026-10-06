package service

import (
	"fmt"
	"os/exec"
	"runtime"
)

// IssueURL is the web UI link for an issue.
func IssueURL(baseURL string, id int) string {
	return fmt.Sprintf("%s/view.php?id=%d", baseURL, id)
}

// BrowserCommand is the program that opens URLs on goos.
func BrowserCommand(goos string) string {
	if goos == "darwin" {
		return "open"
	}
	return "xdg-open"
}

// OpenBrowser opens url in the user's browser without waiting for it.
func OpenBrowser(url string) error {
	cmd := exec.Command(BrowserCommand(runtime.GOOS), url)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	go func() { _ = cmd.Wait() }() // reap the child
	return nil
}
