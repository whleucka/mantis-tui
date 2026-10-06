package cli

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestListPrintsTable(t *testing.T) {
	fm := newFakeMantis(t)
	fm.on("GET", "/issues", 200, mantisFixture(t, "issues_list"))
	cfg := fakeHostConfig(t, fm, "")

	out, _, err := runCLI(t, "--config", cfg, "list")
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("want header + 3 rows, got:\n%s", out)
	}
	for _, col := range []string{"ID", "STATUS", "PRIORITY", "SEVERITY", "CATEGORY", "HANDLER", "UPDATED", "SUMMARY"} {
		if !strings.Contains(lines[0], col) {
			t.Errorf("header missing %s: %q", col, lines[0])
		}
	}
	if !strings.Contains(out, "Sample summary") {
		t.Errorf("rows should include summaries:\n%s", out)
	}
}

func TestListUsesConfigDefaultsAndSelect(t *testing.T) {
	fm := newFakeMantis(t)
	fm.on("GET", "/issues", 200, mantisFixture(t, "issues_list"))
	cfg := fakeHostConfig(t, fm, "[list]\ndefault_filter = \"assigned\"\npage_size = 500\n")

	if _, _, err := runCLI(t, "--config", cfg, "list"); err != nil {
		t.Fatal(err)
	}

	q, _ := url.ParseQuery(fm.lastRequest("GET", "/issues").Query)
	if q.Get("filter_id") != "assigned" || q.Get("page_size") != "500" || q.Get("page") != "1" {
		t.Errorf("query = %v", q)
	}
	if sel := q.Get("select"); !strings.Contains(sel, "summary") || strings.Contains(sel, "history") {
		t.Errorf("list should select summary fields and skip history, select=%q", sel)
	}
}

func TestListFlagsOverrideDefaults(t *testing.T) {
	fm := newFakeMantis(t)
	fm.on("GET", "/issues", 200, mantisFixture(t, "issues_list"))
	cfg := fakeHostConfig(t, fm, "")

	if _, _, err := runCLI(t, "--config", cfg, "list", "--filter", "reported", "--page", "3", "--page-size", "10", "--project", "7"); err != nil {
		t.Fatal(err)
	}
	q, _ := url.ParseQuery(fm.lastRequest("GET", "/issues").Query)
	if q.Get("filter_id") != "reported" || q.Get("page") != "3" || q.Get("page_size") != "10" || q.Get("project_id") != "7" {
		t.Errorf("query = %v", q)
	}
}

func TestListProjectByName(t *testing.T) {
	fm := newFakeMantis(t)
	fm.on("GET", "/issues", 200, mantisFixture(t, "issues_list"))
	fm.on("GET", "/projects", 200, mantisFixture(t, "projects"))
	cfg := fakeHostConfig(t, fm, "")

	// Fixture project names are scrubbed to project-<id>.
	if _, _, err := runCLI(t, "--config", cfg, "list", "--project", "PROJECT-4"); err != nil {
		t.Fatal(err)
	}
	q, _ := url.ParseQuery(fm.lastRequest("GET", "/issues").Query)
	if q.Get("project_id") != "4" {
		t.Errorf("project_id = %q, want 4", q.Get("project_id"))
	}
}

func TestListUnknownProjectIsUsageError(t *testing.T) {
	fm := newFakeMantis(t)
	fm.on("GET", "/projects", 200, mantisFixture(t, "projects"))
	cfg := fakeHostConfig(t, fm, "")

	_, _, err := runCLI(t, "--config", cfg, "list", "--project", "nope")
	if ExitCode(err) != 2 {
		t.Fatalf("exit code = %d (err %v), want 2", ExitCode(err), err)
	}
	if !strings.Contains(err.Error(), "project-4") {
		t.Errorf("error should list valid projects: %v", err)
	}
}

func TestListInvalidFilterIsUsageError(t *testing.T) {
	fm := newFakeMantis(t)
	cfg := fakeHostConfig(t, fm, "")
	_, _, err := runCLI(t, "--config", cfg, "list", "--filter", "mine")
	if ExitCode(err) != 2 {
		t.Fatalf("exit code = %d (err %v), want 2", ExitCode(err), err)
	}
}

func TestListJSONIsServerJSON(t *testing.T) {
	fm := newFakeMantis(t)
	fixture := mantisFixture(t, "issues_list")
	fm.on("GET", "/issues", 200, fixture)
	cfg := fakeHostConfig(t, fm, "")

	out, _, err := runCLI(t, "--config", cfg, "--json", "list")
	if err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	_ = json.Unmarshal(fixture, &want)
	gb, _ := json.Marshal(got)
	wb, _ := json.Marshal(want)
	if string(gb) != string(wb) {
		t.Error("--json output differs from the server's JSON")
	}
}
