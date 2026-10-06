// Package mantis is a minimal client for the MantisBT REST API.
package mantis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	// ErrNotFound is matched by errors.Is for 404 responses and empty results.
	ErrNotFound = errors.New("not found")
	// ErrUnauthorized is matched by errors.Is for 401 and 403 responses.
	ErrUnauthorized = errors.New("unauthorized")
)

const maxErrorMessage = 300

// APIError is a non-2xx response from the Mantis server.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("mantis: HTTP %d", e.Status)
	}
	return fmt.Sprintf("mantis: HTTP %d: %s", e.Status, e.Message)
}

// Is lets errors.Is match ErrNotFound and ErrUnauthorized.
func (e *APIError) Is(target error) bool {
	switch target {
	case ErrNotFound:
		return e.Status == http.StatusNotFound
	case ErrUnauthorized:
		return e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden
	}
	return false
}

// Client talks to one Mantis host.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient returns a client for the Mantis instance at baseURL (without
// /api/rest). Redirects are never followed so the token cannot leak to
// another origin.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http: &http.Client{
			Timeout: 60 * time.Second, // backstop; callers pass contexts with tighter deadlines
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// BaseURL is the Mantis base URL this client talks to.
func (c *Client) BaseURL() string { return c.baseURL }

// do sends a request to /api/rest/<path> and decodes a JSON response into out
// (if non-nil). A *json.RawMessage out receives the body verbatim.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, in, out any) error {
	u := c.baseURL + "/api/rest/" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", c.token)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return c.redact(err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", c.redact(err))
	}

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		loc, _ := url.Parse(resp.Header.Get("Location"))
		target := "unknown location"
		if loc != nil && loc.Host != "" {
			target = loc.Scheme + "://" + loc.Host
		}
		return &APIError{Status: resp.StatusCode, Message: fmt.Sprintf(
			"server answered with a redirect to %s (not followed); check the host url", target)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{Status: resp.StatusCode, Message: c.errorMessage(data)}
	}

	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode %s response: %w", path, err)
	}
	return nil
}

// errorMessage extracts {"message": ...} from an error body, falling back to
// the (truncated) raw body.
func (c *Client) errorMessage(body []byte) string {
	var payload struct {
		Message string `json:"message"`
	}
	msg := strings.TrimSpace(string(body))
	if json.Unmarshal(body, &payload) == nil && payload.Message != "" {
		msg = payload.Message
	}
	msg = c.scrub(msg)
	if len(msg) > maxErrorMessage {
		msg = msg[:maxErrorMessage] + "…"
	}
	return msg
}

func (c *Client) scrub(s string) string {
	if c.token == "" {
		return s
	}
	return strings.ReplaceAll(s, c.token, "<redacted>")
}

// redact wraps transport errors so the token can never appear in them, while
// keeping errors.Is working (e.g. context.Canceled).
func (c *Client) redact(err error) error {
	msg := err.Error()
	if clean := c.scrub(msg); clean != msg {
		return &redactedError{msg: clean, err: err}
	}
	return err
}

type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }
