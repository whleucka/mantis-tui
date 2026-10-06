package mantis

import (
	"encoding/json"
	"time"
)

// Ref identifies an object by id or name; used in requests and as the
// minimal shape of nested objects in responses.
type Ref struct {
	ID   int    `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// EnumValue is one value of a Mantis enum (status, priority, ...).
type EnumValue struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Label string `json:"label"`
	Color string `json:"color,omitempty"`
}

// User is a Mantis account.
type User struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	RealName string `json:"real_name,omitempty"`
	Email    string `json:"email,omitempty"`
}

// Display returns the real name if set, else the username.
func (u User) Display() string {
	if u.RealName != "" {
		return u.RealName
	}
	return u.Name
}

// Category is a project issue category.
type Category struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Project is a Mantis project.
type Project struct {
	ID          int        `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Enabled     bool       `json:"enabled"`
	Status      EnumValue  `json:"status"`
	Categories  []Category `json:"categories,omitempty"`
	SubProjects []Project  `json:"subProjects,omitempty"`
}

// Issue is a Mantis issue. Optional collections are omitted by the server
// when empty.
type Issue struct {
	ID                    int                `json:"id"`
	Summary               string             `json:"summary"`
	Description           string             `json:"description"`
	StepsToReproduce      string             `json:"steps_to_reproduce,omitempty"`
	AdditionalInformation string             `json:"additional_information,omitempty"`
	Project               Ref                `json:"project"`
	Category              Ref                `json:"category"`
	Reporter              User               `json:"reporter"`
	Handler               *User              `json:"handler,omitempty"`
	Status                EnumValue          `json:"status"`
	Resolution            EnumValue          `json:"resolution"`
	Priority              EnumValue          `json:"priority"`
	Severity              EnumValue          `json:"severity"`
	Reproducibility       EnumValue          `json:"reproducibility"`
	ViewState             EnumValue          `json:"view_state"`
	Sticky                bool               `json:"sticky"`
	CreatedAt             time.Time          `json:"created_at"`
	UpdatedAt             time.Time          `json:"updated_at"`
	Notes                 []Note             `json:"notes,omitempty"`
	History               []HistoryEntry     `json:"history,omitempty"`
	Monitors              []User             `json:"monitors,omitempty"`
	Tags                  []Ref              `json:"tags,omitempty"`
	Relationships         []Relationship     `json:"relationships,omitempty"`
	Attachments           []Attachment       `json:"attachments,omitempty"`
	CustomFields          []CustomFieldValue `json:"custom_fields,omitempty"`
}

// Note is a comment on an issue.
type Note struct {
	ID           int           `json:"id"`
	Reporter     User          `json:"reporter"`
	Text         string        `json:"text"`
	ViewState    EnumValue     `json:"view_state"`
	Type         string        `json:"type,omitempty"`
	TimeTracking *TimeTracking `json:"time_tracking,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// Private reports whether the note is only visible to developers.
func (n Note) Private() bool { return n.ViewState.Name == "private" }

// TimeTracking is time logged against a note, as "HH:MM".
type TimeTracking struct {
	Duration string `json:"duration"`
}

// HistoryEntry is one change in an issue's history. Old and new values vary
// in shape (objects for enums/users, strings for text) so they stay raw.
type HistoryEntry struct {
	CreatedAt time.Time       `json:"created_at"`
	User      User            `json:"user"`
	Type      Ref             `json:"type"`
	Field     *HistoryField   `json:"field,omitempty"`
	OldValue  json.RawMessage `json:"old_value,omitempty"`
	NewValue  json.RawMessage `json:"new_value,omitempty"`
	Message   string          `json:"message"`
	Change    string          `json:"change,omitempty"`
}

// HistoryField names the field a history entry changed.
type HistoryField struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

// Relationship links an issue to another issue.
type Relationship struct {
	ID    int       `json:"id"`
	Type  EnumValue `json:"type"`
	Issue struct {
		ID      int       `json:"id"`
		Summary string    `json:"summary,omitempty"`
		Status  EnumValue `json:"status"`
	} `json:"issue"`
}

// Attachment is a file attached to an issue.
type Attachment struct {
	ID          int       `json:"id"`
	Filename    string    `json:"filename"`
	Size        int64     `json:"size"`
	ContentType string    `json:"content_type,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// CustomFieldValue is a custom field's value on an issue.
type CustomFieldValue struct {
	Field Ref    `json:"field"`
	Value string `json:"value"`
}
