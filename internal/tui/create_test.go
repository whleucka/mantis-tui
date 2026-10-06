package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
	"github.com/whleucka/mantis-tui/internal/meta"
	"github.com/whleucka/mantis-tui/internal/service"
)

func createHarness(t *testing.T) *harness {
	t.Helper()
	t.Setenv("TMPDIR", t.TempDir())
	hosts := testHosts()
	return newHarnessWith(t, &hosts[0], func(name string, f *mantistest.Fake) {
		seed(2)(name, f)
		f.ProjectList = []mantis.Project{
			{ID: 4, Name: "Soprano", Categories: []mantis.Category{{ID: 1, Name: "General"}, {ID: 2, Name: "Backend"}}},
			{ID: 8, Name: "Other", Categories: []mantis.Category{{ID: 9, Name: "Backend"}, {ID: 10, Name: "Docs"}}},
		}
		f.UsersByProj = map[int][]mantis.User{
			4: {{ID: 7, Name: "jane", RealName: "Jane Doe"}, {ID: 3, Name: "bob", RealName: "Bob"}},
			8: {{ID: 7, Name: "jane", RealName: "Jane Doe"}},
		}
	}, nil)
}

// field moves focus to the named form field.
func (h *harness) field(name string) {
	h.t.Helper()
	f := h.m.cur.create
	for i := 0; i < len(formFields); i++ {
		if formFields[f.focus] == name {
			return
		}
		h.keys("tab")
	}
	h.t.Fatalf("field %q not found", name)
}

func (h *harness) typeText(s string) {
	for _, r := range s {
		h.send(key(string(r)))
	}
}

func TestCreateFormOpensWithCursorProject(t *testing.T) {
	h := createHarness(t)
	h.keys("C")
	if h.m.cur.screen != screenCreate {
		t.Fatal("C should open the create form")
	}
	if got := h.m.cur.create.values["project"]; got != "Soprano" {
		t.Errorf("project preset = %q, want the cursor issue's project", got)
	}
	out := h.view()
	for _, want := range []string{"New issue", "Project", "Category", "Summary", "Priority", "Severity", "Reproducibility", "Assignee", "Description", "alt+enter"} {
		if !strings.Contains(out, want) {
			t.Errorf("form missing %q", want)
		}
	}
}

func TestCreateSubmitWithoutSummaryOrCategory(t *testing.T) {
	h := createHarness(t)
	h.keys("C", "alt+enter")
	if !strings.Contains(h.view(), "summary is required") && !strings.Contains(h.view(), "category is required") {
		t.Errorf("inline error missing:\n%s", h.view())
	}
	if len(h.fakes["alpha"].Created) != 0 {
		t.Error("nothing should be sent")
	}
	if h.m.cur.screen != screenCreate {
		t.Error("form should stay open")
	}
}

func TestCreateFullFlowMatchesCLIRequest(t *testing.T) {
	h := createHarness(t)
	stubEditor(h, "It crashes when saving.\n")
	h.keys("C")
	h.field("category")
	h.keys("enter", "b", "a", "c", "enter")
	h.field("summary")
	h.typeText("Crash on save")
	h.field("priority")
	h.keys("enter", "h", "i", "enter")
	h.field("assignee")
	h.keys("enter", "j", "a", "n", "enter")
	h.field("description")
	h.keys("e")
	if !strings.Contains(h.view(), "It crashes when saving.") {
		t.Fatalf("description should show after editing:\n%s", h.view())
	}
	h.keys("alt+enter")

	created := h.fakes["alpha"].Created
	if len(created) != 1 {
		t.Fatalf("created = %+v", created)
	}
	// Same input through the CLI's code path must give the same request.
	want, err := service.NewResolver(meta.New(h.fakes["alpha"])).NewIssue(context.Background(), service.CreateInput{
		Project: "Soprano", Category: "Backend", Summary: "Crash on save", Description: "It crashes when saving.",
		Priority: "high", Assignee: "jane",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(created[0], want) {
		t.Errorf("TUI request differs from CLI request:\n got %+v\nwant %+v", created[0], want)
	}
	if h.m.cur.screen != screenIssue || h.m.cur.issue.id != 1000 {
		t.Errorf("should open the new issue, screen %v", h.m.cur.screen)
	}
}

func TestCreateProjectChangeRevalidates(t *testing.T) {
	h := createHarness(t)
	h.keys("C")
	h.field("category")
	h.keys("enter", "b", "a", "c", "enter") // Backend exists in both projects
	h.field("assignee")
	h.keys("enter", "b", "o", "b", "enter") // bob is only in Soprano
	h.field("project")
	h.keys("enter", "o", "t", "h", "enter")

	v := h.m.cur.create.values
	if v["project"] != "Other" {
		t.Fatalf("project = %q", v["project"])
	}
	if v["category"] != "Backend" {
		t.Errorf("category valid in the new project should stay, got %q", v["category"])
	}
	if v["assignee"] != "" {
		t.Errorf("assignee not in the new project should be cleared, got %q", v["assignee"])
	}

	h.field("category")
	h.keys("enter")
	if !strings.Contains(h.view(), "Docs") || strings.Contains(h.view(), "General") {
		t.Errorf("category picker should list the new project's categories:\n%s", h.view())
	}
}

func TestCreateEscAsksWhenDirty(t *testing.T) {
	h := createHarness(t)
	h.keys("C", "esc")
	if h.m.cur.screen != screenList {
		t.Fatal("esc on an untouched form closes it")
	}
	h.keys("C")
	h.field("summary")
	h.typeText("draft")
	h.keys("esc")
	if !strings.Contains(h.view(), "Discard") {
		t.Fatalf("dirty form should confirm:\n%s", h.view())
	}
	h.keys("n")
	if h.m.cur.screen != screenCreate || h.m.cur.create.values["summary"] != "draft" {
		t.Error("n keeps the form")
	}
	h.keys("esc", "y")
	if h.m.cur.screen != screenList {
		t.Error("y discards the form")
	}
}

func TestCreateCtrlSAlsoSubmits(t *testing.T) {
	h := createHarness(t)
	stubEditor(h, "desc\n")
	h.keys("C")
	h.field("category")
	h.keys("enter", "g", "e", "n", "enter")
	h.field("summary")
	h.typeText("Via ctrl+s")
	h.field("description")
	h.keys("enter") // enter on description also opens the editor
	h.keys("ctrl+s")
	if len(h.fakes["alpha"].Created) != 1 {
		t.Error("ctrl+s should submit")
	}
}

func TestCreateServerErrorKeepsForm(t *testing.T) {
	h := createHarness(t)
	h.fakes["alpha"].Errs = map[string]error{"CreateIssue": errString("project is read-only")}
	stubEditor(h, "desc\n")
	h.keys("C")
	h.field("category")
	h.keys("enter", "g", "e", "n", "enter")
	h.field("summary")
	h.typeText("x")
	h.field("description")
	h.keys("e", "alt+enter")
	if h.m.cur.screen != screenCreate || !strings.Contains(h.view(), "read-only") {
		t.Errorf("error should show in the open form:\n%s", h.view())
	}
}
