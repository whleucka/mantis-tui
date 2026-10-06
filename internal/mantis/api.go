package mantis

import (
	"context"
	"encoding/json"
)

// API is the subset of the Mantis REST API that mantis-tui uses. *Client
// implements it; tests in other packages substitute fakes.
type API interface {
	BaseURL() string
	Me(ctx context.Context) (*User, error)
	ListIssues(ctx context.Context, opts ListOptions) (*IssueList, error)
	GetIssue(ctx context.Context, id int) (*IssueResult, error)
	Projects(ctx context.Context) ([]Project, error)
	Project(ctx context.Context, id int) (*Project, error)
	ProjectUsers(ctx context.Context, projectID int) ([]User, error)
	Config(ctx context.Context, options ...string) (map[string]json.RawMessage, error)

	CreateIssue(ctx context.Context, in NewIssue) (*Issue, error)
	UpdateIssue(ctx context.Context, id int, patch IssuePatch) (*Issue, error)
	DeleteIssue(ctx context.Context, id int) error
	AddNote(ctx context.Context, issueID int, n NewNote) (*Note, error)
	DeleteNote(ctx context.Context, issueID, noteID int) error
	Monitor(ctx context.Context, issueID int) error
}

var _ API = (*Client)(nil)
