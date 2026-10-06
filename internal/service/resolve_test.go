package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
	"github.com/whleucka/mantis-tui/internal/meta"
)

func newResolver() (*Resolver, *mantistest.Fake) {
	fake := &mantistest.Fake{
		ConfigValues: mantistest.StandardConfig(),
		ProjectList: []mantis.Project{
			{ID: 4, Name: "Soprano", Categories: []mantis.Category{{ID: 1, Name: "General"}, {ID: 2, Name: "Backend"}}},
			{ID: 7, Name: "Other"},
		},
		UsersByProj: map[int][]mantis.User{4: {
			{ID: 2, Name: "will", RealName: "Will H"},
			{ID: 3, Name: "jsmith", RealName: "John Smith"},
			{ID: 4, Name: "jsmith2", RealName: "John Smith"},
			{ID: 5, Name: "bot", RealName: "Bot"},
		}},
	}
	return NewResolver(meta.New(fake)), fake
}

func TestResolveEnum(t *testing.T) {
	r, _ := newResolver()
	ctx := context.Background()

	tests := []struct {
		kind, in string
		wantID   int
	}{
		{meta.Status, "resolved", 80},
		{meta.Status, "RESOLVED", 80},
		{meta.Status, "80", 80},
		{meta.Priority, "Urgent", 50},
		{meta.Reproducibility, "n/a", 100},
		{meta.Resolution, "won't fix", 90},
	}
	for _, tt := range tests {
		got, err := r.Enum(ctx, tt.kind, tt.in)
		if err != nil {
			t.Errorf("%s %q: %v", tt.kind, tt.in, err)
			continue
		}
		if got.ID != tt.wantID {
			t.Errorf("%s %q = %d, want %d", tt.kind, tt.in, got.ID, tt.wantID)
		}
	}
}

func TestResolveEnumInvalidListsValidValues(t *testing.T) {
	r, _ := newResolver()
	_, err := r.Enum(context.Background(), meta.Status, "done")
	var inv *InvalidValueError
	if !errors.As(err, &inv) {
		t.Fatalf("err = %v, want InvalidValueError", err)
	}
	if inv.Kind != "status" || !strings.Contains(err.Error(), "resolved") || !strings.Contains(err.Error(), "closed") {
		t.Errorf("error should list server values: %v", err)
	}
}

func TestResolveProject(t *testing.T) {
	r, fake := newResolver()
	ctx := context.Background()

	if p, err := r.Project(ctx, "soprano"); err != nil || p.ID != 4 {
		t.Errorf("by name: %+v, %v", p, err)
	}
	if p, err := r.Project(ctx, "7"); err != nil || p.ID != 7 {
		t.Errorf("by id: %+v, %v", p, err)
	}
	var inv *InvalidValueError
	if _, err := r.Project(ctx, "nope"); !errors.As(err, &inv) || !strings.Contains(err.Error(), "Soprano") {
		t.Errorf("unknown project: %v", err)
	}
	if _, err := r.Project(ctx, "999"); !errors.As(err, &inv) {
		t.Errorf("unknown project id should be InvalidValueError: %v", err)
	}
	if fake.Calls("Projects") != 1 {
		t.Errorf("projects fetched %d times, want 1 (cached)", fake.Calls("Projects"))
	}
}

func TestResolveCategory(t *testing.T) {
	r, _ := newResolver()
	ctx := context.Background()
	if c, err := r.Category(ctx, 4, "backend"); err != nil || c.ID != 2 {
		t.Errorf("category = %+v, %v", c, err)
	}
	var inv *InvalidValueError
	if _, err := r.Category(ctx, 4, "Frontend"); !errors.As(err, &inv) || !strings.Contains(err.Error(), "General") {
		t.Errorf("unknown category: %v", err)
	}
}

func TestResolveUser(t *testing.T) {
	r, _ := newResolver()
	ctx := context.Background()

	tests := []struct {
		in     string
		wantID int
	}{
		{"will", 2},
		{"WILL", 2},
		{"Will H", 2},
		{"5", 5},
		{"42", 42}, // ids are accepted even if not listed for the project
		{"jsmith", 3},
	}
	for _, tt := range tests {
		u, err := r.User(ctx, 4, tt.in)
		if err != nil {
			t.Errorf("%q: %v", tt.in, err)
			continue
		}
		if u.ID != tt.wantID {
			t.Errorf("%q = %d, want %d", tt.in, u.ID, tt.wantID)
		}
	}
}

func TestResolveUserAmbiguousRealName(t *testing.T) {
	r, _ := newResolver()
	_, err := r.User(context.Background(), 4, "john smith")
	var amb *AmbiguousError
	if !errors.As(err, &amb) {
		t.Fatalf("err = %v, want AmbiguousError", err)
	}
	if !strings.Contains(err.Error(), "jsmith") || !strings.Contains(err.Error(), "jsmith2") {
		t.Errorf("error should list candidates: %v", err)
	}
}

func TestResolveUserUnknown(t *testing.T) {
	r, _ := newResolver()
	var inv *InvalidValueError
	if _, err := r.User(context.Background(), 4, "nobody"); !errors.As(err, &inv) {
		t.Fatalf("err = %v, want InvalidValueError", err)
	}
}
