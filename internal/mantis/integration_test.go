//go:build integration

package mantis

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/whleucka/mantis-tui/internal/config"
)

// Read-only smoke test against a real host:
//
//	MANTIS_IT_HOST=williamhleucka [MANTIS_IT_CONFIG=path] go test -tags=integration ./internal/mantis/...
//
// Never add write calls here.
func TestIntegrationReadOnly(t *testing.T) {
	name := os.Getenv("MANTIS_IT_HOST")
	if name == "" {
		t.Skip("MANTIS_IT_HOST not set")
	}
	path, explicit := os.Getenv("MANTIS_IT_CONFIG"), true
	if path == "" {
		path, explicit = config.DefaultConfigPath(os.Getenv), false
	}
	cfg, err := config.Load(path, explicit)
	if err != nil {
		t.Fatal(err)
	}
	host, err := config.Select(config.Resolve(cfg, os.Environ()).Hosts, config.SelectInput{Flag: name})
	if err != nil {
		t.Fatal(err)
	}

	c := NewClient(host.URL, host.Token)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	me, err := c.Me(ctx)
	if err != nil {
		t.Fatalf("users/me: %v", err)
	}
	t.Logf("authenticated as user id %d", me.ID)

	projects, err := c.Projects(ctx)
	if err != nil {
		t.Fatalf("projects: %v", err)
	}
	t.Logf("%d projects", len(projects))

	list, err := c.ListIssues(ctx, ListOptions{PageSize: 5})
	if err != nil {
		t.Fatalf("issues: %v", err)
	}
	t.Logf("%d issues on first page", len(list.Issues))
	if len(list.Issues) > 0 {
		if _, err := c.GetIssue(ctx, list.Issues[0].ID); err != nil {
			t.Fatalf("issue %d: %v", list.Issues[0].ID, err)
		}
	}

	cfgs, err := c.Config(ctx, "status_enum_string", "priority_enum_string", "status_colors")
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if len(cfgs) != 3 {
		t.Errorf("config returned %d options, want 3", len(cfgs))
	}
}
