package mantis

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func ptr[T any](v T) *T { return &v }

// bodyJSON decodes the last request body into a generic map for comparison.
func bodyJSON(t *testing.T, srv *fakeServer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(srv.lastBody, &m); err != nil {
		t.Fatalf("request body is not JSON: %v (%q)", err, srv.lastBody)
	}
	return m
}

func jsonEqual(t *testing.T, got map[string]any, want string) {
	t.Helper()
	var w map[string]any
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatal(err)
	}
	gb, _ := json.Marshal(got)
	wb, _ := json.Marshal(w)
	if string(gb) != string(wb) {
		t.Errorf("body = %s\nwant   %s", gb, wb)
	}
}

func TestCreateIssue(t *testing.T) {
	srv := newFakeServer(t, 201, []byte(`{"issue":{"id":101,"summary":"Crash on save"}}`))

	is, err := newTestClient(srv.URL).CreateIssue(context.Background(), NewIssue{
		Summary:         "Crash on save",
		Description:     "It crashes.",
		Project:         Ref{ID: 4},
		Category:        Ref{Name: "General"},
		Priority:        &Ref{Name: "high"},
		Severity:        &Ref{Name: "crash"},
		Reproducibility: &Ref{Name: "always"},
		Handler:         &Ref{ID: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	if srv.last.Method != "POST" || srv.last.URL.Path != "/api/rest/issues" {
		t.Errorf("%s %s", srv.last.Method, srv.last.URL.Path)
	}
	if srv.last.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", srv.last.Header.Get("Content-Type"))
	}
	jsonEqual(t, bodyJSON(t, srv), `{
		"summary":"Crash on save","description":"It crashes.",
		"project":{"id":4},"category":{"name":"General"},
		"priority":{"name":"high"},"severity":{"name":"crash"},
		"reproducibility":{"name":"always"},"handler":{"id":3}}`)
	if is.ID != 101 {
		t.Errorf("created id = %d", is.ID)
	}
}

func TestCreateIssueOmitsUnsetOptionalFields(t *testing.T) {
	srv := newFakeServer(t, 201, []byte(`{"issue":{"id":102}}`))
	_, err := newTestClient(srv.URL).CreateIssue(context.Background(), NewIssue{
		Summary: "s", Description: "d", Project: Ref{ID: 1}, Category: Ref{ID: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	jsonEqual(t, bodyJSON(t, srv), `{"summary":"s","description":"d","project":{"id":1},"category":{"id":2}}`)
}

func TestUpdateIssueSendsOnlySetFields(t *testing.T) {
	srv := newFakeServer(t, 200, []byte(`{"issues":[{"id":33,"status":{"id":80,"name":"resolved"}}]}`))

	is, err := newTestClient(srv.URL).UpdateIssue(context.Background(), 33, IssuePatch{
		Status:  &Ref{Name: "resolved"},
		Summary: ptr("New title"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if srv.last.Method != "PATCH" || srv.last.URL.Path != "/api/rest/issues/33" {
		t.Errorf("%s %s", srv.last.Method, srv.last.URL.Path)
	}
	jsonEqual(t, bodyJSON(t, srv), `{"status":{"name":"resolved"},"summary":"New title"}`)
	if is.Status.Name != "resolved" {
		t.Errorf("returned issue status = %+v", is.Status)
	}
}

func TestUpdateIssueAcceptsSingularEnvelope(t *testing.T) {
	srv := newFakeServer(t, 200, []byte(`{"issue":{"id":33,"summary":"x"}}`))
	is, err := newTestClient(srv.URL).UpdateIssue(context.Background(), 33, IssuePatch{Summary: ptr("x")})
	if err != nil || is.ID != 33 {
		t.Fatalf("issue = %+v, err %v", is, err)
	}
}

func TestUpdateIssueLongFields(t *testing.T) {
	srv := newFakeServer(t, 200, []byte(`{"issues":[{"id":5}]}`))
	_, err := newTestClient(srv.URL).UpdateIssue(context.Background(), 5, IssuePatch{
		StepsToReproduce:      ptr("1. open it"),
		AdditionalInformation: ptr("seen on 2.27"),
	})
	if err != nil {
		t.Fatal(err)
	}
	jsonEqual(t, bodyJSON(t, srv), `{"steps_to_reproduce":"1. open it","additional_information":"seen on 2.27"}`)
}

func TestUpdateIssueEmptyMonitorsIsSent(t *testing.T) {
	srv := newFakeServer(t, 200, []byte(`{"issues":[{"id":5}]}`))
	empty := []Ref{}
	if _, err := newTestClient(srv.URL).UpdateIssue(context.Background(), 5, IssuePatch{Monitors: &empty}); err != nil {
		t.Fatal(err)
	}
	jsonEqual(t, bodyJSON(t, srv), `{"monitors":[]}`)
}

func TestDeleteIssue(t *testing.T) {
	srv := newFakeServer(t, 204, nil)
	if err := newTestClient(srv.URL).DeleteIssue(context.Background(), 7); err != nil {
		t.Fatal(err)
	}
	if srv.last.Method != "DELETE" || srv.last.URL.Path != "/api/rest/issues/7" {
		t.Errorf("%s %s", srv.last.Method, srv.last.URL.Path)
	}
}

func TestAddNote(t *testing.T) {
	srv := newFakeServer(t, 201, []byte(`{"note":{"id":61,"text":"done","view_state":{"name":"private"},"time_tracking":{"duration":"00:30"}}}`))

	n, err := newTestClient(srv.URL).AddNote(context.Background(), 33, NewNote{
		Text:         "done",
		Private:      true,
		TimeTracking: "0:30",
	})
	if err != nil {
		t.Fatal(err)
	}
	if srv.last.Method != "POST" || srv.last.URL.Path != "/api/rest/issues/33/notes" {
		t.Errorf("%s %s", srv.last.Method, srv.last.URL.Path)
	}
	jsonEqual(t, bodyJSON(t, srv), `{"text":"done","view_state":{"name":"private"},"time_tracking":{"duration":"0:30"}}`)
	if n.ID != 61 || !n.Private() {
		t.Errorf("note = %+v", n)
	}
}

func TestAddPublicNoteWithoutTime(t *testing.T) {
	srv := newFakeServer(t, 201, []byte(`{"note":{"id":62}}`))
	if _, err := newTestClient(srv.URL).AddNote(context.Background(), 33, NewNote{Text: "hi"}); err != nil {
		t.Fatal(err)
	}
	jsonEqual(t, bodyJSON(t, srv), `{"text":"hi","view_state":{"name":"public"}}`)
}

func TestDeleteNote(t *testing.T) {
	srv := newFakeServer(t, 200, []byte(`{"issue":{"id":33}}`))
	if err := newTestClient(srv.URL).DeleteNote(context.Background(), 33, 61); err != nil {
		t.Fatal(err)
	}
	if srv.last.Method != "DELETE" || srv.last.URL.Path != "/api/rest/issues/33/notes/61" {
		t.Errorf("%s %s", srv.last.Method, srv.last.URL.Path)
	}
}

func TestMonitor(t *testing.T) {
	srv := newFakeServer(t, 201, []byte(`{"issues":[{"id":33}]}`))
	if err := newTestClient(srv.URL).Monitor(context.Background(), 33); err != nil {
		t.Fatal(err)
	}
	if srv.last.Method != "POST" || srv.last.URL.Path != "/api/rest/issues/33/monitors" {
		t.Errorf("%s %s", srv.last.Method, srv.last.URL.Path)
	}
}

func TestWriteEndpointsPropagateErrors(t *testing.T) {
	srv := newFakeServer(t, 403, []byte(`{"message":"Access denied"}`))
	c := newTestClient(srv.URL)
	ctx := context.Background()
	calls := map[string]func() error{
		"CreateIssue": func() error { _, err := c.CreateIssue(ctx, NewIssue{}); return err },
		"UpdateIssue": func() error { _, err := c.UpdateIssue(ctx, 1, IssuePatch{}); return err },
		"DeleteIssue": func() error { return c.DeleteIssue(ctx, 1) },
		"AddNote":     func() error { _, err := c.AddNote(ctx, 1, NewNote{Text: "x"}); return err },
		"DeleteNote":  func() error { return c.DeleteNote(ctx, 1, 2) },
		"Monitor":     func() error { return c.Monitor(ctx, 1) },
	}
	for name, call := range calls {
		if err := call(); err == nil || !errorsIsUnauthorized(err) {
			t.Errorf("%s: err = %v, want ErrUnauthorized", name, err)
		}
	}
}

func errorsIsUnauthorized(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Is(ErrUnauthorized)
}
