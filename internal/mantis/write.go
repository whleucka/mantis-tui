package mantis

import (
	"context"
	"fmt"
)

// NewIssue is the body of a create-issue request.
type NewIssue struct {
	Summary               string       `json:"summary"`
	Description           string       `json:"description"`
	StepsToReproduce      string       `json:"steps_to_reproduce,omitempty"`
	AdditionalInformation string       `json:"additional_information,omitempty"`
	Project               Ref          `json:"project"`
	Category              Ref          `json:"category"`
	Priority              *Ref         `json:"priority,omitempty"`
	Severity              *Ref         `json:"severity,omitempty"`
	Reproducibility       *Ref         `json:"reproducibility,omitempty"`
	Handler               *Ref         `json:"handler,omitempty"`
	Files                 []FileUpload `json:"files,omitempty"`
}

// FileUpload is a file sent with a new issue or note. Content goes over
// the wire in base64, which encoding/json does for a []byte.
type FileUpload struct {
	Name    string `json:"name"`
	Content []byte `json:"content"`
}

// IssuePatch is a partial issue update; nil fields are left unchanged.
type IssuePatch struct {
	Summary               *string `json:"summary,omitempty"`
	Description           *string `json:"description,omitempty"`
	StepsToReproduce      *string `json:"steps_to_reproduce,omitempty"`
	AdditionalInformation *string `json:"additional_information,omitempty"`
	Status                *Ref    `json:"status,omitempty"`
	Resolution            *Ref    `json:"resolution,omitempty"`
	Priority              *Ref    `json:"priority,omitempty"`
	Severity              *Ref    `json:"severity,omitempty"`
	Reproducibility       *Ref    `json:"reproducibility,omitempty"`
	Category              *Ref    `json:"category,omitempty"`
	Handler               *Ref    `json:"handler,omitempty"`
	// Monitors replaces the whole monitor list; a pointer to an empty slice
	// clears it.
	Monitors *[]Ref `json:"monitors,omitempty"`
}

// NewNote is a note to add to an issue.
type NewNote struct {
	Text         string
	Private      bool
	TimeTracking string // "H:MM"; empty for none
	Files        []FileUpload
}

// CreateIssue files a new issue and returns it.
func (c *Client) CreateIssue(ctx context.Context, in NewIssue) (*Issue, error) {
	var env issueEnvelope
	if err := c.do(ctx, "POST", "issues", nil, in, &env); err != nil {
		return nil, fmt.Errorf("create issue: %w", err)
	}
	is := env.first()
	if err := checkKept(len(in.Files), len(is.Attachments)); err != nil {
		return is, fmt.Errorf("issue %d created, but %w", is.ID, err)
	}
	return is, nil
}

// checkKept fails when the server kept fewer files than were sent: an
// oversized request can be answered with success and nothing attached.
func checkKept(sent, kept int) error {
	if kept < sent {
		return fmt.Errorf("the server kept %d of %d files", kept, sent)
	}
	return nil
}

// UpdateIssue applies a partial update and returns the updated issue.
func (c *Client) UpdateIssue(ctx context.Context, id int, patch IssuePatch) (*Issue, error) {
	var env issueEnvelope
	if err := c.do(ctx, "PATCH", fmt.Sprintf("issues/%d", id), nil, patch, &env); err != nil {
		return nil, fmt.Errorf("update issue %d: %w", id, err)
	}
	return env.first(), nil
}

// DeleteIssue deletes an issue.
func (c *Client) DeleteIssue(ctx context.Context, id int) error {
	if err := c.do(ctx, "DELETE", fmt.Sprintf("issues/%d", id), nil, nil, nil); err != nil {
		return fmt.Errorf("delete issue %d: %w", id, err)
	}
	return nil
}

// AddNote adds a note to an issue and returns it.
func (c *Client) AddNote(ctx context.Context, issueID int, n NewNote) (*Note, error) {
	type timeTracking struct {
		Duration string `json:"duration"`
	}
	body := struct {
		Text         string        `json:"text"`
		ViewState    Ref           `json:"view_state"`
		TimeTracking *timeTracking `json:"time_tracking,omitempty"`
		Files        []FileUpload  `json:"files,omitempty"`
	}{Text: n.Text, ViewState: Ref{Name: "public"}, Files: n.Files}
	if n.Private {
		body.ViewState.Name = "private"
	}
	if n.TimeTracking != "" {
		body.TimeTracking = &timeTracking{Duration: n.TimeTracking}
	}

	var env struct {
		Note Note `json:"note"`
	}
	if err := c.do(ctx, "POST", fmt.Sprintf("issues/%d/notes", issueID), nil, body, &env); err != nil {
		return nil, fmt.Errorf("add note to issue %d: %w", issueID, err)
	}
	if err := checkKept(len(n.Files), len(env.Note.Attachments)); err != nil {
		return &env.Note, fmt.Errorf("note %d added to issue %d, but %w", env.Note.ID, issueID, err)
	}
	return &env.Note, nil
}

// DeleteNote deletes a note from an issue.
func (c *Client) DeleteNote(ctx context.Context, issueID, noteID int) error {
	if err := c.do(ctx, "DELETE", fmt.Sprintf("issues/%d/notes/%d", issueID, noteID), nil, nil, nil); err != nil {
		return fmt.Errorf("delete note %d of issue %d: %w", noteID, issueID, err)
	}
	return nil
}

// Monitor adds the current user to an issue's monitors.
func (c *Client) Monitor(ctx context.Context, issueID int) error {
	if err := c.do(ctx, "POST", fmt.Sprintf("issues/%d/monitors", issueID), nil, struct{}{}, nil); err != nil {
		return fmt.Errorf("monitor issue %d: %w", issueID, err)
	}
	return nil
}

// issueEnvelope accepts both {"issue": {...}} and {"issues": [{...}]}, which
// Mantis uses inconsistently across write endpoints.
type issueEnvelope struct {
	Issue  *Issue  `json:"issue"`
	Issues []Issue `json:"issues"`
}

func (e issueEnvelope) first() *Issue {
	if e.Issue != nil {
		return e.Issue
	}
	if len(e.Issues) > 0 {
		return &e.Issues[0]
	}
	return &Issue{}
}
