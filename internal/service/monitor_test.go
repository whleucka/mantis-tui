package service

import (
	"context"
	"errors"
	"testing"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
	"github.com/whleucka/mantis-tui/internal/meta"
)

func TestUnmonitorKeepsOtherMonitors(t *testing.T) {
	fake := &mantistest.Fake{
		CurrentUser: mantis.User{ID: 2, Name: "me"},
		Issues: map[int]mantis.Issue{33: {ID: 33, Monitors: []mantis.User{
			{ID: 2, Name: "me"}, {ID: 3, Name: "bob"}, {ID: 4, Name: "eve"},
		}}},
	}

	if err := Unmonitor(context.Background(), meta.New(fake), 33); err != nil {
		t.Fatal(err)
	}

	if len(fake.Patches) != 1 {
		t.Fatalf("patches = %+v, want exactly one", fake.Patches)
	}
	p := fake.Patches[0]
	if p.ID != 33 || p.Patch.Monitors == nil {
		t.Fatalf("patch = %+v", p)
	}
	got := *p.Patch.Monitors
	if len(got) != 2 || got[0].ID != 3 || got[1].ID != 4 {
		t.Errorf("monitors sent = %+v, want bob(3) and eve(4) only", got)
	}
	if p.Patch.Status != nil || p.Patch.Summary != nil {
		t.Error("unmonitor patch must only touch monitors")
	}
}

func TestUnmonitorWhenOnlyMonitorSendsEmptyList(t *testing.T) {
	fake := &mantistest.Fake{
		CurrentUser: mantis.User{ID: 2},
		Issues:      map[int]mantis.Issue{5: {ID: 5, Monitors: []mantis.User{{ID: 2}}}},
	}
	if err := Unmonitor(context.Background(), meta.New(fake), 5); err != nil {
		t.Fatal(err)
	}
	if m := fake.Patches[0].Patch.Monitors; m == nil || len(*m) != 0 {
		t.Errorf("want empty (non-nil) monitors list, got %+v", m)
	}
}

func TestUnmonitorNotMonitoringIsNoop(t *testing.T) {
	fake := &mantistest.Fake{
		CurrentUser: mantis.User{ID: 2},
		Issues:      map[int]mantis.Issue{5: {ID: 5, Monitors: []mantis.User{{ID: 9}}}},
	}
	if err := Unmonitor(context.Background(), meta.New(fake), 5); err != nil {
		t.Fatal(err)
	}
	if len(fake.Patches) != 0 {
		t.Errorf("no PATCH expected when not monitoring, got %+v", fake.Patches)
	}
}

func TestUnmonitorPropagatesErrors(t *testing.T) {
	fake := &mantistest.Fake{Errs: map[string]error{"Me": errors.New("offline")}}
	if err := Unmonitor(context.Background(), meta.New(fake), 5); err == nil {
		t.Error("expected error when current user cannot be loaded")
	}
	fake = &mantistest.Fake{CurrentUser: mantis.User{ID: 2}}
	if err := Unmonitor(context.Background(), meta.New(fake), 404); !errors.Is(err, mantis.ErrNotFound) {
		t.Errorf("missing issue: err = %v, want ErrNotFound", err)
	}
}

func TestIsMonitoring(t *testing.T) {
	is := mantis.Issue{Monitors: []mantis.User{{ID: 3}, {ID: 2}}}
	if !IsMonitoring(is, 2) || IsMonitoring(is, 7) {
		t.Error("IsMonitoring wrong")
	}
}
