package service

import "testing"

func TestIssueURL(t *testing.T) {
	if got := IssueURL("https://mantis.example.com", 33); got != "https://mantis.example.com/view.php?id=33" {
		t.Errorf("IssueURL = %q", got)
	}
}

func TestBrowserCommand(t *testing.T) {
	tests := map[string]string{"linux": "xdg-open", "freebsd": "xdg-open", "darwin": "open"}
	for goos, want := range tests {
		if got := BrowserCommand(goos); got != want {
			t.Errorf("BrowserCommand(%q) = %q, want %q", goos, got, want)
		}
	}
}
