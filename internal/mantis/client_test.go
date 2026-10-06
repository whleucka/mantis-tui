package mantis

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const testToken = "sentinel-token-3f9a"

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fakeServer serves a fixed status/body and records the last request.
type fakeServer struct {
	*httptest.Server
	last *http.Request
}

func newFakeServer(t *testing.T, status int, body []byte) *fakeServer {
	t.Helper()
	fs := &fakeServer{}
	fs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.last = r.Clone(context.Background())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(fs.Close)
	return fs
}

func newTestClient(url string) *Client {
	return NewClient(url, testToken)
}

func TestRequestsCarryAuthAndJSONHeaders(t *testing.T) {
	srv := newFakeServer(t, 200, fixture(t, "me"))

	if _, err := newTestClient(srv.URL).Me(context.Background()); err != nil {
		t.Fatal(err)
	}

	if got := srv.last.Header.Get("Authorization"); got != testToken {
		t.Errorf("Authorization = %q, want raw token", got)
	}
	if got := srv.last.Header.Get("Accept"); got != "application/json" {
		t.Errorf("Accept = %q", got)
	}
	if srv.last.URL.Path != "/api/rest/users/me" {
		t.Errorf("path = %q", srv.last.URL.Path)
	}
}

func TestBaseURLTrailingSlashIsTrimmed(t *testing.T) {
	srv := newFakeServer(t, 200, fixture(t, "me"))
	if _, err := newTestClient(srv.URL + "/").Me(context.Background()); err != nil {
		t.Fatal(err)
	}
	if srv.last.URL.Path != "/api/rest/users/me" {
		t.Errorf("path = %q", srv.last.URL.Path)
	}
}

func TestListIssuesBuildsQuery(t *testing.T) {
	srv := newFakeServer(t, 200, fixture(t, "issues_list"))

	res, err := newTestClient(srv.URL).ListIssues(context.Background(), ListOptions{
		Filter:    "assigned",
		Page:      2,
		PageSize:  25,
		ProjectID: 4,
		Select:    []string{"id", "summary"},
	})
	if err != nil {
		t.Fatal(err)
	}

	q := srv.last.URL.Query()
	want := map[string]string{"filter_id": "assigned", "page": "2", "page_size": "25", "project_id": "4", "select": "id,summary"}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("query %s = %q, want %q (full: %s)", k, q.Get(k), v, srv.last.URL.RawQuery)
		}
	}
	if srv.last.URL.Path != "/api/rest/issues" {
		t.Errorf("path = %q", srv.last.URL.Path)
	}
	if len(res.Issues) != 3 {
		t.Errorf("decoded %d issues, want 3", len(res.Issues))
	}
	if !json.Valid(res.Raw) || len(res.Raw) == 0 {
		t.Error("Raw should hold the server's JSON")
	}
}

func TestListIssuesAllFilterSendsNoFilterID(t *testing.T) {
	srv := newFakeServer(t, 200, fixture(t, "issues_list"))
	if _, err := newTestClient(srv.URL).ListIssues(context.Background(), ListOptions{Filter: "all"}); err != nil {
		t.Fatal(err)
	}
	if srv.last.URL.Query().Has("filter_id") {
		t.Errorf("filter 'all' must not send filter_id: %s", srv.last.URL.RawQuery)
	}
}

func TestGetIssueDecodesFixture(t *testing.T) {
	srv := newFakeServer(t, 200, fixture(t, "issue"))

	res, err := newTestClient(srv.URL).GetIssue(context.Background(), 33)
	if err != nil {
		t.Fatal(err)
	}
	if srv.last.URL.Path != "/api/rest/issues/33" {
		t.Errorf("path = %q", srv.last.URL.Path)
	}
	is := res.Issue
	if is.ID != 33 || is.Status.Name != "closed" || is.Status.Color == "" {
		t.Errorf("issue header = id %d status %+v", is.ID, is.Status)
	}
	if is.Project.ID == 0 || is.Category.Name == "" || is.Reporter.Name == "" {
		t.Errorf("refs not decoded: project %+v category %+v reporter %+v", is.Project, is.Category, is.Reporter)
	}
	if is.CreatedAt.IsZero() || is.UpdatedAt.IsZero() {
		t.Error("timestamps not decoded")
	}
	if len(is.Notes) == 0 || is.Notes[0].Text == "" {
		t.Errorf("notes not decoded: %+v", is.Notes)
	}
	if len(is.History) == 0 {
		t.Fatal("history not decoded")
	}
	var statusChange *HistoryEntry
	for i := range is.History {
		if is.History[i].Field != nil && is.History[i].Field.Name == "status" {
			statusChange = &is.History[i]
		}
	}
	if statusChange == nil || len(statusChange.NewValue) == 0 {
		t.Errorf("status history entry with raw new_value expected, got %+v", statusChange)
	}
	if !json.Valid(res.Raw) {
		t.Error("Raw should hold the server's JSON")
	}
}

func TestGetIssueEmptyEnvelopeIsNotFound(t *testing.T) {
	srv := newFakeServer(t, 200, []byte(`{"issues":[]}`))
	_, err := newTestClient(srv.URL).GetIssue(context.Background(), 9)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestMetadataEndpoints(t *testing.T) {
	ctx := context.Background()

	t.Run("me", func(t *testing.T) {
		srv := newFakeServer(t, 200, fixture(t, "me"))
		me, err := newTestClient(srv.URL).Me(ctx)
		if err != nil || me.ID == 0 || me.Name == "" {
			t.Fatalf("me = %+v, err %v", me, err)
		}
	})

	t.Run("projects", func(t *testing.T) {
		srv := newFakeServer(t, 200, fixture(t, "projects"))
		ps, err := newTestClient(srv.URL).Projects(ctx)
		if err != nil || len(ps) == 0 || ps[0].Name == "" {
			t.Fatalf("projects = %+v, err %v", ps, err)
		}
		if srv.last.URL.Path != "/api/rest/projects" {
			t.Errorf("path = %q", srv.last.URL.Path)
		}
	})

	t.Run("project with categories", func(t *testing.T) {
		srv := newFakeServer(t, 200, fixture(t, "project"))
		p, err := newTestClient(srv.URL).Project(ctx, 4)
		if err != nil || p.ID == 0 || len(p.Categories) == 0 {
			t.Fatalf("project = %+v, err %v", p, err)
		}
		if srv.last.URL.Path != "/api/rest/projects/4" {
			t.Errorf("path = %q", srv.last.URL.Path)
		}
	})

	t.Run("project users", func(t *testing.T) {
		srv := newFakeServer(t, 200, fixture(t, "project_users"))
		us, err := newTestClient(srv.URL).ProjectUsers(ctx, 4)
		if err != nil || len(us) == 0 || us[0].RealName == "" {
			t.Fatalf("users = %+v, err %v", us, err)
		}
		if srv.last.URL.Path != "/api/rest/projects/4/users" {
			t.Errorf("path = %q", srv.last.URL.Path)
		}
	})

	t.Run("config", func(t *testing.T) {
		srv := newFakeServer(t, 200, fixture(t, "config_enums"))
		cfg, err := newTestClient(srv.URL).Config(ctx, "status_enum_string", "status_colors")
		if err != nil {
			t.Fatal(err)
		}
		if got := srv.last.URL.Query()["option[]"]; len(got) != 2 || got[0] != "status_enum_string" {
			t.Errorf("option[] = %v", got)
		}
		var statuses []EnumValue
		if err := json.Unmarshal(cfg["status_enum_string"], &statuses); err != nil || len(statuses) == 0 {
			t.Errorf("status enum = %v, err %v", statuses, err)
		}
		if _, ok := cfg["status_colors"]; !ok {
			t.Error("status_colors missing")
		}
	})
}

func TestErrorMapping(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		sentinel error
		message  string
	}{
		{"not found", 404, `{"message":"Issue #9 not found","code":1100}`, ErrNotFound, "Issue #9 not found"},
		{"unauthorized", 401, `{"message":"API token not found"}`, ErrUnauthorized, "API token not found"},
		{"forbidden", 403, `{"message":"Access denied"}`, ErrUnauthorized, "Access denied"},
		{"server error json", 500, `{"message":"Database broke"}`, nil, "Database broke"},
		{"server error html", 502, `<html>Bad Gateway</html>`, nil, "<html>Bad Gateway</html>"},
		{"empty body", 500, ``, nil, "500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newFakeServer(t, tt.status, []byte(tt.body))
			_, err := newTestClient(srv.URL).Me(context.Background())

			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("err = %v, want *APIError", err)
			}
			if apiErr.Status != tt.status {
				t.Errorf("status = %d", apiErr.Status)
			}
			if tt.sentinel != nil && !errors.Is(err, tt.sentinel) {
				t.Errorf("errors.Is(%v, %v) = false", err, tt.sentinel)
			}
			if !strings.Contains(err.Error(), tt.message) {
				t.Errorf("error %q should contain %q", err, tt.message)
			}
		})
	}
}

func TestLongErrorBodyIsTruncated(t *testing.T) {
	srv := newFakeServer(t, 500, []byte(strings.Repeat("x", 5000)))
	_, err := newTestClient(srv.URL).Me(context.Background())
	if err == nil || len(err.Error()) > 400 {
		t.Errorf("error should be truncated, got %d chars", len(err.Error()))
	}
}

func TestRedirectIsNotFollowed(t *testing.T) {
	var leaked bool
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			leaked = true
		}
		_, _ = w.Write(fixture(t, "me"))
	}))
	defer elsewhere.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+r.URL.Path, http.StatusFound)
	}))
	defer origin.Close()

	_, err := newTestClient(origin.URL).Me(context.Background())
	if err == nil {
		t.Fatal("expected an error for a redirect")
	}
	if leaked {
		t.Fatal("token was sent to the redirect target")
	}
	if !strings.Contains(err.Error(), "redirect") {
		t.Errorf("error should mention the redirect: %v", err)
	}
}

func TestErrorsNeverContainToken(t *testing.T) {
	// A hostile or buggy server echoes the Authorization header back.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"message":"bad token ` + r.Header.Get("Authorization") + `"}`))
	}))
	defer srv.Close()

	_, err := newTestClient(srv.URL).Me(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("error leaks token: %v", err)
	}
}

func TestNetworkErrorNeverContainsToken(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // connection refused

	_, err := newTestClient(url).Me(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("error leaks token: %v", err)
	}
}

func TestInvalidJSONIsAnError(t *testing.T) {
	srv := newFakeServer(t, 200, []byte(`{not json`))
	if _, err := newTestClient(srv.URL).Me(context.Background()); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestContextCancellation(t *testing.T) {
	srv := newFakeServer(t, 200, fixture(t, "me"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := newTestClient(srv.URL).Me(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// Fixtures are recorded from a real server; make sure no personal data slipped through.
func TestFixturesAreScrubbed(t *testing.T) {
	email := regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]+`)
	files, err := filepath.Glob("testdata/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range email.FindAllString(string(b), -1) {
			if !strings.HasSuffix(m, "@example.test") {
				t.Errorf("%s contains a real-looking email: %s", f, m)
			}
		}
		if !json.Valid(b) {
			t.Errorf("%s is not valid JSON", f)
		}
	}
}

func TestEveryEndpointPropagatesErrors(t *testing.T) {
	srv := newFakeServer(t, 500, []byte(`{"message":"boom"}`))
	c := newTestClient(srv.URL)
	ctx := context.Background()

	calls := map[string]func() error{
		"Me":           func() error { _, err := c.Me(ctx); return err },
		"ListIssues":   func() error { _, err := c.ListIssues(ctx, ListOptions{}); return err },
		"GetIssue":     func() error { _, err := c.GetIssue(ctx, 1); return err },
		"Projects":     func() error { _, err := c.Projects(ctx); return err },
		"Project":      func() error { _, err := c.Project(ctx, 1); return err },
		"ProjectUsers": func() error { _, err := c.ProjectUsers(ctx, 1); return err },
		"Config":       func() error { _, err := c.Config(ctx, "x"); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call()
			var apiErr *APIError
			if !errors.As(err, &apiErr) || !strings.Contains(err.Error(), "boom") {
				t.Errorf("err = %v, want wrapped APIError with server message", err)
			}
		})
	}
}

func TestProjectEmptyEnvelopeIsNotFound(t *testing.T) {
	srv := newFakeServer(t, 200, []byte(`{"projects":[]}`))
	if _, err := newTestClient(srv.URL).Project(context.Background(), 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestUserDisplay(t *testing.T) {
	if got := (User{Name: "jdoe", RealName: "Jane Doe"}).Display(); got != "Jane Doe" {
		t.Errorf("Display = %q", got)
	}
	if got := (User{Name: "jdoe"}).Display(); got != "jdoe" {
		t.Errorf("Display without real name = %q", got)
	}
}

func TestNotePrivate(t *testing.T) {
	if !(Note{ViewState: EnumValue{Name: "private"}}).Private() {
		t.Error("private note not detected")
	}
	if (Note{ViewState: EnumValue{Name: "public"}}).Private() {
		t.Error("public note reported private")
	}
}
