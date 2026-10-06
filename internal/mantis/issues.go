package mantis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ListOptions selects a page of issues.
type ListOptions struct {
	Filter    string // all | assigned | reported | monitored | unassigned
	Page      int
	PageSize  int
	ProjectID int
	Select    []string // limit returned fields (e.g. skip history on big pages)
}

// IssueList is a page of issues plus the server's raw JSON.
type IssueList struct {
	Issues []Issue
	Raw    json.RawMessage
}

// IssueResult is a single issue plus the server's raw JSON.
type IssueResult struct {
	Issue Issue
	Raw   json.RawMessage
}

// ListIssues fetches a page of issues.
func (c *Client) ListIssues(ctx context.Context, opts ListOptions) (*IssueList, error) {
	q := url.Values{}
	if opts.Filter != "" && opts.Filter != "all" {
		q.Set("filter_id", opts.Filter)
	}
	if opts.Page > 0 {
		q.Set("page", strconv.Itoa(opts.Page))
	}
	if opts.PageSize > 0 {
		q.Set("page_size", strconv.Itoa(opts.PageSize))
	}
	if opts.ProjectID > 0 {
		q.Set("project_id", strconv.Itoa(opts.ProjectID))
	}
	if len(opts.Select) > 0 {
		q.Set("select", strings.Join(opts.Select, ","))
	}

	var raw json.RawMessage
	if err := c.do(ctx, "GET", "issues", q, nil, &raw); err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}
	var env issuesEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("list issues: decode: %w", err)
	}
	return &IssueList{Issues: env.Issues, Raw: raw}, nil
}

// GetIssue fetches one issue with its notes and history.
func (c *Client) GetIssue(ctx context.Context, id int) (*IssueResult, error) {
	var raw json.RawMessage
	if err := c.do(ctx, "GET", fmt.Sprintf("issues/%d", id), nil, nil, &raw); err != nil {
		return nil, fmt.Errorf("get issue %d: %w", id, err)
	}
	var env issuesEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("get issue %d: decode: %w", id, err)
	}
	if len(env.Issues) == 0 {
		return nil, fmt.Errorf("get issue %d: %w", id, ErrNotFound)
	}
	return &IssueResult{Issue: env.Issues[0], Raw: raw}, nil
}

type issuesEnvelope struct {
	Issues []Issue `json:"issues"`
}
