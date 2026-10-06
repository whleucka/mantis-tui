package mantis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// Me returns the user that owns the API token.
func (c *Client) Me(ctx context.Context) (*User, error) {
	var u User
	if err := c.do(ctx, "GET", "users/me", nil, nil, &u); err != nil {
		return nil, fmt.Errorf("get current user: %w", err)
	}
	return &u, nil
}

// Projects lists the projects the user can access.
func (c *Client) Projects(ctx context.Context) ([]Project, error) {
	var env struct {
		Projects []Project `json:"projects"`
	}
	if err := c.do(ctx, "GET", "projects", nil, nil, &env); err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return env.Projects, nil
}

// Project fetches one project, including its categories.
func (c *Client) Project(ctx context.Context, id int) (*Project, error) {
	var env struct {
		Projects []Project `json:"projects"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("projects/%d", id), nil, nil, &env); err != nil {
		return nil, fmt.Errorf("get project %d: %w", id, err)
	}
	if len(env.Projects) == 0 {
		return nil, fmt.Errorf("get project %d: %w", id, ErrNotFound)
	}
	return &env.Projects[0], nil
}

// ProjectUsers lists users with access to a project.
func (c *Client) ProjectUsers(ctx context.Context, projectID int) ([]User, error) {
	var env struct {
		Users []User `json:"users"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("projects/%d/users", projectID), nil, nil, &env); err != nil {
		return nil, fmt.Errorf("list users of project %d: %w", projectID, err)
	}
	return env.Users, nil
}

// Config fetches server config options, keyed by option name.
func (c *Client) Config(ctx context.Context, options ...string) (map[string]json.RawMessage, error) {
	q := url.Values{"option[]": options}
	var env struct {
		Configs []struct {
			Option string          `json:"option"`
			Value  json.RawMessage `json:"value"`
		} `json:"configs"`
	}
	if err := c.do(ctx, "GET", "config", q, nil, &env); err != nil {
		return nil, fmt.Errorf("get config: %w", err)
	}
	out := make(map[string]json.RawMessage, len(env.Configs))
	for _, cfg := range env.Configs {
		out[cfg.Option] = cfg.Value
	}
	return out, nil
}
