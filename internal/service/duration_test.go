package service

import "testing"

func TestValidDuration(t *testing.T) {
	for s, want := range map[string]bool{"0:30": true, "12:05": true, "100:00": true, "1:75": false, "30m": false, "": false, "1:5": false} {
		if got := ValidDuration(s); got != want {
			t.Errorf("ValidDuration(%q) = %v", s, got)
		}
	}
}
