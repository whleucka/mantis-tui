package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// issueStub is a minimal issue body for GET /issues/{id}.
func issueStub(id, projectID int) []byte {
	b, _ := json.Marshal(map[string]any{"issues": []any{map[string]any{
		"id": id, "summary": "s", "project": map[string]any{"id": projectID, "name": "p"},
		"monitors": []any{map[string]any{"id": 2, "name": "user2"}, map[string]any{"id": 5, "name": "user5"}},
	}}})
	return b
}

func setupWriteFake(t *testing.T) (*fakeMantis, string) {
	fm := newFakeMantis(t)
	fm.on("GET", "/config", 200, mantisFixture(t, "config_enums"))
	fm.on("GET", "/projects/4", 200, mantisFixture(t, "project"))
	fm.on("GET", "/projects/4/users", 200, mantisFixture(t, "project_users"))
	fm.on("GET", "/users/me", 200, mantisFixture(t, "me"))
	for _, id := range []int{33, 34} {
		fm.on("GET", "/issues/"+itoa(id), 200, issueStub(id, 4))
		fm.on("PATCH", "/issues/"+itoa(id), 200, issueStub(id, 4))
		fm.on("POST", "/issues/"+itoa(id)+"/monitors", 201, issueStub(id, 4))
	}
	return fm, fakeHostConfig(t, fm, "")
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

func patchBody(t *testing.T, fm *fakeMantis, id int) map[string]any {
	t.Helper()
	r := fm.lastRequest("PATCH", "/issues/"+itoa(id))
	if r == nil {
		t.Fatalf("no PATCH for issue %d", id)
	}
	var m map[string]any
	if err := json.Unmarshal(r.Body, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestUpdateResolvesNamesAndPatchesEachIssue(t *testing.T) {
	fm, cfg := setupWriteFake(t)

	out, _, err := runCLI(t, "--config", cfg, "update", "33", "34", "--status", "Resolved", "--priority", "high", "--severity", "minor")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{33, 34} {
		body := patchBody(t, fm, id)
		b, _ := json.Marshal(body)
		if !strings.Contains(string(b), `"status":{"id":80,"name":"resolved"}`) ||
			!strings.Contains(string(b), `"priority":{"id":40,"name":"high"}`) ||
			!strings.Contains(string(b), `"severity":{"id":50,"name":"minor"}`) {
			t.Errorf("issue %d patch = %s", id, b)
		}
		if _, ok := body["category"]; ok {
			t.Error("category must not be sent when not requested")
		}
	}
	if !strings.Contains(out, "#33 updated") || !strings.Contains(out, "#34 updated") {
		t.Errorf("output = %q", out)
	}
}

func TestUpdateCategoryResolvedInIssueProject(t *testing.T) {
	fm, cfg := setupWriteFake(t)
	if _, _, err := runCLI(t, "--config", cfg, "update", "33", "--category", "general"); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(patchBody(t, fm, 33)["category"])
	if !strings.Contains(string(b), `"name":"General"`) {
		t.Errorf("category = %s", b)
	}
}

func TestUpdateSummary(t *testing.T) {
	fm, cfg := setupWriteFake(t)
	if _, _, err := runCLI(t, "--config", cfg, "update", "33", "--summary", "New title"); err != nil {
		t.Fatal(err)
	}
	if got := patchBody(t, fm, 33)["summary"]; got != "New title" {
		t.Errorf("summary = %v", got)
	}
}

func TestUpdateInvalidStatusIsUsageErrorAndSendsNothing(t *testing.T) {
	fm, cfg := setupWriteFake(t)
	_, _, err := runCLI(t, "--config", cfg, "update", "33", "--status", "done")
	if ExitCode(err) != 2 || !strings.Contains(err.Error(), "resolved") {
		t.Fatalf("exit %d err %v, want 2 listing valid statuses", ExitCode(err), err)
	}
	if fm.lastRequest("PATCH", "/issues/33") != nil {
		t.Error("nothing should be sent for an invalid value")
	}
}

func TestUpdateWithoutFieldsIsUsageError(t *testing.T) {
	_, cfg := setupWriteFake(t)
	_, _, err := runCLI(t, "--config", cfg, "update", "33")
	if ExitCode(err) != 2 {
		t.Fatalf("exit %d err %v, want 2", ExitCode(err), err)
	}
}

func TestUpdatePartialFailure(t *testing.T) {
	fm, cfg := setupWriteFake(t)
	fm.on("PATCH", "/issues/34", 403, []byte(`{"message":"Access denied"}`))

	out, _, err := runCLI(t, "--config", cfg, "--json", "update", "33", "34", "--status", "closed")
	if err == nil || !strings.Contains(err.Error(), "1 of 2") {
		t.Fatalf("err = %v, want a '1 of 2 failed' error", err)
	}
	if ExitCode(err) == 0 {
		t.Error("partial failure must exit non-zero")
	}
	var results []struct {
		ID    int    `json:"id"`
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &results); err != nil {
		t.Fatalf("--json output: %v\n%s", err, out)
	}
	if len(results) != 2 || !results[0].OK || results[1].OK || !strings.Contains(results[1].Error, "Access denied") {
		t.Errorf("results = %+v", results)
	}
}

func TestAssign(t *testing.T) {
	fm, cfg := setupWriteFake(t)
	// Fixture users are scrubbed to user<id>; pick user1.
	if _, _, err := runCLI(t, "--config", cfg, "assign", "33", "34", "user1"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{33, 34} {
		b, _ := json.Marshal(patchBody(t, fm, id)["handler"])
		if !strings.Contains(string(b), `"id":1`) {
			t.Errorf("issue %d handler = %s", id, b)
		}
	}
}

func TestAssignNeedsIDAndUser(t *testing.T) {
	_, cfg := setupWriteFake(t)
	if _, _, err := runCLI(t, "--config", cfg, "assign", "33"); ExitCode(err) != 2 {
		t.Errorf("exit %d err %v, want 2", ExitCode(err), err)
	}
}

func TestMonitorAndUnmonitor(t *testing.T) {
	fm, cfg := setupWriteFake(t)

	if _, _, err := runCLI(t, "--config", cfg, "monitor", "33"); err != nil {
		t.Fatal(err)
	}
	if fm.lastRequest("POST", "/issues/33/monitors") == nil {
		t.Error("monitor should POST /issues/33/monitors")
	}

	// me.json is user 2; the stub issue is monitored by users 2 and 5.
	if _, _, err := runCLI(t, "--config", cfg, "unmonitor", "33"); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(patchBody(t, fm, 33)["monitors"])
	if string(b) != `[{"id":5}]` {
		t.Errorf("unmonitor PATCH monitors = %s, want [{\"id\":5}]", b)
	}
}

func TestOpenUsesBrowserOpener(t *testing.T) {
	_, cfg := setupWriteFake(t)
	var opened string
	out, _, err := runCLIWith(t, deps{openURL: func(u string) error { opened = u; return nil }}, "--config", cfg, "open", "33")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(opened, "/view.php?id=33") {
		t.Errorf("opened %q", opened)
	}
	if !strings.Contains(out, "/view.php?id=33") {
		t.Errorf("open should print the URL, got %q", out)
	}
}
