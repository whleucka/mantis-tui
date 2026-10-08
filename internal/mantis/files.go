package mantis

import (
	"context"
	"fmt"
)

// File is an attachment with its content.
type File struct {
	Attachment
	Content []byte `json:"content"` // base64 in the JSON
}

// GetFile downloads one attachment of an issue. It works for files attached
// to notes too, which on 2.27+ is where pasted screenshots end up.
func (c *Client) GetFile(ctx context.Context, issueID, fileID int) (*File, error) {
	var env struct {
		Files []File `json:"files"`
	}
	if err := c.do(ctx, "GET", fmt.Sprintf("issues/%d/files/%d", issueID, fileID), nil, nil, &env); err != nil {
		return nil, fmt.Errorf("get file %d of issue %d: %w", fileID, issueID, err)
	}
	if len(env.Files) == 0 {
		return nil, fmt.Errorf("get file %d of issue %d: %w", fileID, issueID, ErrNotFound)
	}
	return &env.Files[0], nil
}
