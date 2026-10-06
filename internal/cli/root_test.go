package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootHelpListsGlobalFlags(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute --help: %v", err)
	}

	for _, flag := range []string{"--host", "--config", "--json", "--timeout"} {
		if !strings.Contains(out.String(), flag) {
			t.Errorf("help output missing %s\n%s", flag, out.String())
		}
	}
}
