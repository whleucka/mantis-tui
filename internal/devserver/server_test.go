package devserver

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http/httptest"
	"testing"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

func TestDevServerRoundTrip(t *testing.T) {
	srv, err := New("../mantis/testdata", log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(srv)
	defer hs.Close()
	c := mantis.NewClient(hs.URL, "any-token")
	ctx := context.Background()

	list, err := c.ListIssues(ctx, mantis.ListOptions{PageSize: 2})
	if err != nil || len(list.Issues) != 2 {
		t.Fatalf("list: %v, %d issues", err, len(list.Issues))
	}
	id := list.Issues[0].ID

	is, err := c.UpdateIssue(ctx, id, mantis.IssuePatch{Status: &mantis.Ref{Name: "resolved"}})
	if err != nil || is.Status.Name != "resolved" {
		t.Fatalf("update: %v, status %+v", err, is.Status)
	}
	if _, err := c.AddNote(ctx, id, mantis.NewNote{Text: "hi", TimeTracking: "0:30"}); err != nil {
		t.Fatal(err)
	}
	got, err := c.GetIssue(ctx, id)
	if err != nil || got.Issue.Notes[len(got.Issue.Notes)-1].Text != "hi" {
		t.Fatalf("note not stored: %v", err)
	}
	created, err := c.CreateIssue(ctx, mantis.NewIssue{Summary: "new", Project: mantis.Ref{ID: 4}, Category: mantis.Ref{Name: "General"}})
	if err != nil || created.ID == 0 {
		t.Fatalf("create: %v", err)
	}
	if err := c.DeleteIssue(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetIssue(ctx, created.ID); err == nil {
		t.Error("deleted issue still readable")
	}
	if _, err := c.Project(ctx, 4); err != nil {
		t.Errorf("project: %v", err)
	}
	if _, err := c.Config(ctx, "status_enum_string"); err != nil {
		t.Errorf("config: %v", err)
	}
}

func TestDevServerServesNoteAttachments(t *testing.T) {
	srv, err := New("../mantis/testdata", log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	hs := httptest.NewServer(srv)
	defer hs.Close()
	f, err := mantis.NewClient(hs.URL, "any-token").GetFile(context.Background(), 33, 6)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(f.Content, []byte("\x89PNG")) || f.Size != int64(len(f.Content)) {
		t.Errorf("file 6 should be a PNG of its stated size: %d bytes, size %d", len(f.Content), f.Size)
	}
}
