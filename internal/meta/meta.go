// Package meta caches per-host Mantis metadata (enums, projects, categories,
// users) for the lifetime of a session.
package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

// Enum kinds, named as in the server's "<kind>_enum_string" config options.
const (
	Status          = "status"
	Priority        = "priority"
	Severity        = "severity"
	Reproducibility = "reproducibility"
	Resolution      = "resolution"
)

// Kinds lists every enum kind the cache loads.
var Kinds = []string{Status, Priority, Severity, Reproducibility, Resolution}

const (
	statusColorsOption = "status_colors"
	timeTrackingOption = "time_tracking_enabled"
)

// Cache lazily loads and caches metadata for one host. It is safe for
// concurrent use; each item is fetched at most once unless the fetch fails.
type Cache struct {
	api mantis.API

	enums    lazy[enumSet]
	projects lazy[[]mantis.Project]
	me       lazy[*mantis.User]

	mu         sync.Mutex
	categories map[int]*lazy[[]mantis.Category]
	users      map[int]*lazy[[]mantis.User]
}

type enumSet struct {
	values       map[string][]mantis.EnumValue
	colors       map[string]string
	timeTracking bool
}

// New returns an empty cache backed by api.
func New(api mantis.API) *Cache {
	return &Cache{
		api:        api,
		categories: map[int]*lazy[[]mantis.Category]{},
		users:      map[int]*lazy[[]mantis.User]{},
	}
}

// API returns the client the cache reads from.
func (c *Cache) API() mantis.API { return c.api }

// Enum returns the values of an enum kind (see Kinds), in server order.
func (c *Cache) Enum(ctx context.Context, kind string) ([]mantis.EnumValue, error) {
	if !slices.Contains(Kinds, kind) {
		return nil, fmt.Errorf("unknown enum kind %q", kind)
	}
	set, err := c.loadEnums(ctx)
	if err != nil {
		return nil, err
	}
	return set.values[kind], nil
}

// StatusColors maps status names to the server's hex colors.
func (c *Cache) StatusColors(ctx context.Context) (map[string]string, error) {
	set, err := c.loadEnums(ctx)
	if err != nil {
		return nil, err
	}
	return set.colors, nil
}

// TimeTrackingEnabled reports whether the server accepts time tracking on
// notes (Mantis rejects it with 403 when disabled).
func (c *Cache) TimeTrackingEnabled(ctx context.Context) (bool, error) {
	set, err := c.loadEnums(ctx)
	if err != nil {
		return false, err
	}
	return set.timeTracking, nil
}

// truthy decodes Mantis ON/OFF config values, which arrive as 0/1 or "ON"/"OFF".
func truthy(raw json.RawMessage) bool {
	var n int
	if json.Unmarshal(raw, &n) == nil {
		return n != 0
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s == "1" || strings.EqualFold(s, "on")
	}
	return false
}

func (c *Cache) loadEnums(ctx context.Context) (enumSet, error) {
	return c.enums.get(func() (enumSet, error) {
		options := make([]string, 0, len(Kinds)+1)
		for _, k := range Kinds {
			options = append(options, k+"_enum_string")
		}
		raw, err := c.api.Config(ctx, append(options, statusColorsOption, timeTrackingOption)...)
		if err != nil {
			return enumSet{}, err
		}
		set := enumSet{values: map[string][]mantis.EnumValue{}, colors: map[string]string{}}
		for _, k := range Kinds {
			var vals []mantis.EnumValue
			if v, ok := raw[k+"_enum_string"]; ok {
				if err := json.Unmarshal(v, &vals); err != nil {
					return enumSet{}, fmt.Errorf("decode %s enum: %w", k, err)
				}
			}
			set.values[k] = vals
		}
		if v, ok := raw[statusColorsOption]; ok {
			_ = json.Unmarshal(v, &set.colors) // colors are cosmetic; ignore odd shapes
		}
		set.timeTracking = truthy(raw[timeTrackingOption])
		return set, nil
	})
}

// Projects returns every accessible project, with sub-projects flattened in
// after their parent.
func (c *Cache) Projects(ctx context.Context) ([]mantis.Project, error) {
	return c.projects.get(func() ([]mantis.Project, error) {
		ps, err := c.api.Projects(ctx)
		if err != nil {
			return nil, err
		}
		return flatten(ps), nil
	})
}

func flatten(ps []mantis.Project) []mantis.Project {
	var out []mantis.Project
	for _, p := range ps {
		subs := p.SubProjects
		p.SubProjects = nil
		out = append(out, p)
		out = append(out, flatten(subs)...)
	}
	return out
}

// Categories returns a project's categories.
func (c *Cache) Categories(ctx context.Context, projectID int) ([]mantis.Category, error) {
	return entry(c, c.categories, projectID).get(func() ([]mantis.Category, error) {
		p, err := c.api.Project(ctx, projectID)
		if err != nil {
			return nil, err
		}
		return p.Categories, nil
	})
}

// Users returns the users with access to a project.
func (c *Cache) Users(ctx context.Context, projectID int) ([]mantis.User, error) {
	return entry(c, c.users, projectID).get(func() ([]mantis.User, error) {
		return c.api.ProjectUsers(ctx, projectID)
	})
}

// Me returns the user that owns the API token.
func (c *Cache) Me(ctx context.Context) (*mantis.User, error) {
	return c.me.get(func() (*mantis.User, error) { return c.api.Me(ctx) })
}

func entry[T any](c *Cache, m map[int]*lazy[T], key int) *lazy[T] {
	c.mu.Lock()
	defer c.mu.Unlock()
	l, ok := m[key]
	if !ok {
		l = &lazy[T]{}
		m[key] = l
	}
	return l
}

// lazy holds a value loaded on first successful get. Failed loads are retried.
type lazy[T any] struct {
	mu sync.Mutex
	ok bool
	v  T
}

func (l *lazy[T]) get(load func() (T, error)) (T, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ok {
		return l.v, nil
	}
	v, err := load()
	if err != nil {
		var zero T
		return zero, err
	}
	l.v, l.ok = v, true
	return v, nil
}
