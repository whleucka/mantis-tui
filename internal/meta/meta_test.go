package meta

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/whleucka/mantis-tui/internal/mantis"
	"github.com/whleucka/mantis-tui/internal/mantis/mantistest"
)

func newFake() *mantistest.Fake {
	return &mantistest.Fake{
		CurrentUser:  mantis.User{ID: 2, Name: "me"},
		ConfigValues: mantistest.StandardConfig(),
		ProjectList: []mantis.Project{
			{ID: 1, Name: "Alpha", Categories: []mantis.Category{{ID: 1, Name: "General"}}, SubProjects: []mantis.Project{
				{ID: 3, Name: "Alpha Sub", Categories: []mantis.Category{{ID: 5, Name: "UI"}}},
			}},
			{ID: 2, Name: "Beta"},
		},
		UsersByProj: map[int][]mantis.User{1: {{ID: 2, Name: "me"}, {ID: 3, Name: "bob"}}},
	}
}

func TestEnumsLoadedOnceForAllKinds(t *testing.T) {
	fake := newFake()
	c := New(fake)
	ctx := context.Background()

	for _, kind := range Kinds {
		vals, err := c.Enum(ctx, kind)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if len(vals) == 0 {
			t.Errorf("%s: no values", kind)
		}
	}
	colors, err := c.StatusColors(ctx)
	if err != nil || colors["new"] != "#fcbdbd" {
		t.Errorf("status colors = %v, err %v", colors, err)
	}
	if n := fake.Calls("Config"); n != 1 {
		t.Errorf("Config called %d times, want 1", n)
	}
}

func TestUnknownEnumKind(t *testing.T) {
	if _, err := New(newFake()).Enum(context.Background(), "colour"); err == nil {
		t.Fatal("expected error for unknown kind")
	}
}

func TestProjectsFlattenSubProjectsAndCacheOnce(t *testing.T) {
	fake := newFake()
	c := New(fake)
	ctx := context.Background()

	for range 3 {
		ps, err := c.Projects(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(ps) != 3 {
			t.Fatalf("projects = %+v, want Alpha, Alpha Sub, Beta flattened", ps)
		}
	}
	if fake.Calls("Projects") != 1 {
		t.Errorf("Projects called %d times", fake.Calls("Projects"))
	}
}

func TestCategoriesAndUsersCachedPerProject(t *testing.T) {
	fake := newFake()
	c := New(fake)
	ctx := context.Background()

	for range 2 {
		cats, err := c.Categories(ctx, 1)
		if err != nil || len(cats) != 1 || cats[0].Name != "General" {
			t.Fatalf("categories = %+v, err %v", cats, err)
		}
		users, err := c.Users(ctx, 1)
		if err != nil || len(users) != 2 {
			t.Fatalf("users = %+v, err %v", users, err)
		}
	}
	if _, err := c.Categories(ctx, 2); err != nil {
		t.Fatal(err)
	}
	if fake.Calls("Project") != 2 || fake.Calls("ProjectUsers") != 1 {
		t.Errorf("Project calls = %d (want 2: one per project), ProjectUsers = %d (want 1)",
			fake.Calls("Project"), fake.Calls("ProjectUsers"))
	}
}

func TestMeCached(t *testing.T) {
	fake := newFake()
	c := New(fake)
	for range 2 {
		me, err := c.Me(context.Background())
		if err != nil || me.ID != 2 {
			t.Fatalf("me = %+v, err %v", me, err)
		}
	}
	if fake.Calls("Me") != 1 {
		t.Errorf("Me called %d times", fake.Calls("Me"))
	}
}

func TestErrorsAreNotCached(t *testing.T) {
	fake := newFake()
	fake.Errs = map[string]error{"Config": errors.New("offline")}
	c := New(fake)
	ctx := context.Background()

	if _, err := c.Enum(ctx, Status); err == nil {
		t.Fatal("expected error")
	}
	fake.Errs = nil
	if _, err := c.Enum(ctx, Status); err != nil {
		t.Fatalf("retry after error should succeed: %v", err)
	}
}

func TestConcurrentLoadsCallOnce(t *testing.T) {
	fake := newFake()
	c := New(fake)
	ctx := context.Background()

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Enum(ctx, Priority)
			_, _ = c.Projects(ctx)
			_, _ = c.Users(ctx, 1)
		}()
	}
	wg.Wait()
	if fake.Calls("Config") != 1 || fake.Calls("Projects") != 1 || fake.Calls("ProjectUsers") != 1 {
		t.Errorf("calls: Config %d Projects %d ProjectUsers %d, want 1 each",
			fake.Calls("Config"), fake.Calls("Projects"), fake.Calls("ProjectUsers"))
	}
}

func TestTimeTrackingEnabled(t *testing.T) {
	for raw, want := range map[string]bool{"1": true, "0": false, `"ON"`: true, `"OFF"`: false} {
		fake := newFake()
		fake.ConfigValues["time_tracking_enabled"] = []byte(raw)
		got, err := New(fake).TimeTrackingEnabled(context.Background())
		if err != nil || got != want {
			t.Errorf("time_tracking_enabled=%s: got %v, err %v; want %v", raw, got, err, want)
		}
		if fake.Calls("Config") != 1 {
			t.Errorf("time tracking should share the enums /config call")
		}
	}
	fake := newFake() // option missing from the response
	if got, _ := New(fake).TimeTrackingEnabled(context.Background()); got {
		t.Error("missing option should mean disabled")
	}
}

func TestUploads(t *testing.T) {
	fake := newFake()
	fake.ConfigValues["max_file_size"] = []byte("5242880")
	fake.ConfigValues["allowed_files"] = []byte(`""`)
	fake.ConfigValues["disallowed_files"] = []byte(`"svg, .PHP ,"`)
	got, err := New(fake).Uploads(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := UploadLimits{MaxFileSize: 5242880, Disallowed: []string{"svg", "php"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("limits = %+v, want %+v", got, want)
	}
	if fake.Calls("Config") != 1 {
		t.Error("upload limits should share the enums /config call")
	}

	got, _ = New(newFake()).Uploads(context.Background()) // options missing
	if got.MaxFileSize != DefaultMaxFileSize || got.Allowed != nil || got.Disallowed != nil {
		t.Errorf("defaults = %+v", got)
	}
}
