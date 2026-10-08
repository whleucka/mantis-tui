// Package devserver is a tiny in-memory MantisBT REST server seeded from the
// scrubbed fixtures, for trying mantis-tui's write paths without touching a
// real host. It is a development tool, not a faithful Mantis implementation.
package devserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

// Server holds the in-memory issues and canned metadata responses.
type Server struct {
	mu      sync.Mutex
	issues  map[int]mantis.Issue
	static  map[string][]byte // path → body for read-only endpoints
	enums   map[string][]mantis.EnumValue
	me      mantis.User
	nextID  int
	nextNID int
	logger  *log.Logger

	// Delay is added to every response.
	Delay time.Duration
}

// New loads fixtures from dir (internal/mantis/testdata).
func New(dir string, logger *log.Logger) (*Server, error) {
	read := func(name string) ([]byte, error) { return os.ReadFile(filepath.Join(dir, name+".json")) }
	s := &Server{issues: map[int]mantis.Issue{}, static: map[string][]byte{}, enums: map[string][]mantis.EnumValue{}, logger: logger}

	for path, name := range map[string]string{
		"/users/me": "me", "/projects": "projects", "/config": "config_enums",
	} {
		b, err := read(name)
		if err != nil {
			return nil, err
		}
		s.static[path] = b
	}
	project, err := read("project")
	if err != nil {
		return nil, err
	}
	users, err := read("project_users")
	if err != nil {
		return nil, err
	}
	var p struct{ Projects []mantis.Project }
	_ = json.Unmarshal(project, &p)
	var allProjects struct{ Projects []mantis.Project }
	_ = json.Unmarshal(s.static["/projects"], &allProjects)
	for _, pr := range allProjects.Projects {
		// Every project shares the fixture project's categories and users.
		single, _ := json.Marshal(map[string]any{"projects": []any{withCategories(pr, p.Projects)}})
		s.static[fmt.Sprintf("/projects/%d", pr.ID)] = single
		s.static[fmt.Sprintf("/projects/%d/users", pr.ID)] = users
	}
	_ = json.Unmarshal(s.static["/users/me"], &s.me)

	// Advertise time tracking so the TUI's time field can be exercised.
	var cfgEnv map[string][]json.RawMessage
	if json.Unmarshal(s.static["/config"], &cfgEnv) == nil {
		cfgEnv["configs"] = append(cfgEnv["configs"], json.RawMessage(`{"option":"time_tracking_enabled","value":1}`))
		s.static["/config"], _ = json.Marshal(cfgEnv)
	}

	var cfg struct {
		Configs []struct {
			Option string
			Value  json.RawMessage
		}
	}
	_ = json.Unmarshal(s.static["/config"], &cfg)
	for _, c := range cfg.Configs {
		var vals []mantis.EnumValue
		if json.Unmarshal(c.Value, &vals) == nil {
			s.enums[strings.TrimSuffix(c.Option, "_enum_string")] = vals
		}
	}

	for _, name := range []string{"issues_list", "issue"} {
		b, err := read(name)
		if err != nil {
			return nil, err
		}
		var env struct{ Issues []mantis.Issue }
		if err := json.Unmarshal(b, &env); err != nil {
			return nil, err
		}
		for _, is := range env.Issues {
			s.issues[is.ID] = is
			s.nextID = max(s.nextID, is.ID)
			for _, n := range is.Notes {
				s.nextNID = max(s.nextNID, n.ID)
			}
		}
	}
	return s, nil
}

func withCategories(p mantis.Project, fixture []mantis.Project) mantis.Project {
	if len(fixture) > 0 {
		p.Categories = fixture[0].Categories
	}
	return p
}

var (
	reIssue    = regexp.MustCompile(`^/issues/(\d+)$`)
	reNotes    = regexp.MustCompile(`^/issues/(\d+)/notes$`)
	reNote     = regexp.MustCompile(`^/issues/(\d+)/notes/(\d+)$`)
	reMonitors = regexp.MustCompile(`^/issues/(\d+)/monitors$`)
	reFile     = regexp.MustCompile(`^/issues/(\d+)/files/(\d+)$`)
)

// ServeHTTP implements http.Handler for /api/rest/*.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/rest")
	if r.Header.Get("Authorization") == "" {
		writeJSON(w, 401, map[string]string{"message": "API token required"})
		return
	}
	body, _ := io.ReadAll(r.Body)
	if r.Method != "GET" {
		s.logger.Printf("%s %s %s", r.Method, path, body)
	}
	time.Sleep(s.Delay) // make spinners visible during manual checks

	s.mu.Lock()
	defer s.mu.Unlock()

	switch {
	case r.Method == "GET" && s.static[path] != nil:
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(s.static[path])
		return
	case r.Method == "GET" && path == "/issues":
		s.list(w, r)
		return
	case r.Method == "POST" && path == "/issues":
		s.create(w, body)
		return
	}

	if m := reIssue.FindStringSubmatch(path); m != nil {
		id, _ := strconv.Atoi(m[1])
		is, ok := s.issues[id]
		if !ok {
			writeJSON(w, 404, map[string]string{"message": fmt.Sprintf("Issue #%d not found", id)})
			return
		}
		switch r.Method {
		case "GET":
			writeJSON(w, 200, map[string]any{"issues": []mantis.Issue{is}})
		case "PATCH":
			if err := s.patch(&is, body); err != nil {
				writeJSON(w, 400, map[string]string{"message": err.Error()})
				return
			}
			is.UpdatedAt = time.Now()
			s.issues[id] = is
			writeJSON(w, 200, map[string]any{"issues": []mantis.Issue{is}})
		case "DELETE":
			delete(s.issues, id)
			w.WriteHeader(204)
		}
		return
	}
	if m := reFile.FindStringSubmatch(path); m != nil && r.Method == "GET" {
		s.file(w, atoi(m[1]), atoi(m[2]))
		return
	}
	if m := reNotes.FindStringSubmatch(path); m != nil && r.Method == "POST" {
		s.addNote(w, atoi(m[1]), body)
		return
	}
	if m := reNote.FindStringSubmatch(path); m != nil && r.Method == "DELETE" {
		id, nid := atoi(m[1]), atoi(m[2])
		is := s.issues[id]
		kept := is.Notes[:0]
		for _, n := range is.Notes {
			if n.ID != nid {
				kept = append(kept, n)
			}
		}
		is.Notes = kept
		s.issues[id] = is
		writeJSON(w, 200, map[string]any{"issue": is})
		return
	}
	if m := reMonitors.FindStringSubmatch(path); m != nil && r.Method == "POST" {
		id := atoi(m[1])
		is := s.issues[id]
		is.Monitors = append(is.Monitors, s.me)
		s.issues[id] = is
		writeJSON(w, 201, map[string]any{"issues": []mantis.Issue{is}})
		return
	}
	writeJSON(w, 404, map[string]string{"message": "devserver: no route " + r.Method + " " + path})
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var out []mantis.Issue
	for _, is := range s.issues {
		switch q.Get("filter_id") {
		case "assigned":
			if is.Handler == nil || is.Handler.ID != s.me.ID {
				continue
			}
		case "reported":
			if is.Reporter.ID != s.me.ID {
				continue
			}
		case "monitored":
			if !monitoredBy(is, s.me.ID) {
				continue
			}
		case "unassigned":
			if is.Handler != nil && is.Handler.ID != 0 {
				continue
			}
		}
		if pid := atoi(q.Get("project_id")); pid > 0 && is.Project.ID != pid {
			continue
		}
		is.History, is.Notes = nil, nil
		out = append(out, is)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	size, page := atoi(q.Get("page_size")), atoi(q.Get("page"))
	if size < 1 {
		size = 50
	}
	if page < 1 {
		page = 1
	}
	start := min((page-1)*size, len(out))
	end := min(start+size, len(out))
	writeJSON(w, 200, map[string]any{"issues": out[start:end]})
}

func monitoredBy(is mantis.Issue, id int) bool {
	for _, m := range is.Monitors {
		if m.ID == id {
			return true
		}
	}
	return false
}

func (s *Server) enumRef(kind string, ref *mantis.Ref) (mantis.EnumValue, error) {
	for _, v := range s.enums[kind] {
		if (ref.ID != 0 && v.ID == ref.ID) || (ref.Name != "" && strings.EqualFold(v.Name, ref.Name)) {
			return v, nil
		}
	}
	return mantis.EnumValue{}, fmt.Errorf("invalid %s %+v", kind, *ref)
}

func (s *Server) patch(is *mantis.Issue, body []byte) error {
	var p mantis.IssuePatch
	if err := json.Unmarshal(body, &p); err != nil {
		return err
	}
	for kind, pair := range map[string]struct {
		ref *mantis.Ref
		dst *mantis.EnumValue
	}{
		"status": {p.Status, &is.Status}, "priority": {p.Priority, &is.Priority},
		"severity": {p.Severity, &is.Severity}, "resolution": {p.Resolution, &is.Resolution},
		"reproducibility": {p.Reproducibility, &is.Reproducibility},
	} {
		if pair.ref == nil {
			continue
		}
		v, err := s.enumRef(kind, pair.ref)
		if err != nil {
			return err
		}
		*pair.dst = v
	}
	if p.Summary != nil {
		is.Summary = *p.Summary
	}
	if p.Description != nil {
		is.Description = *p.Description
	}
	if p.StepsToReproduce != nil {
		is.StepsToReproduce = *p.StepsToReproduce
	}
	if p.AdditionalInformation != nil {
		is.AdditionalInformation = *p.AdditionalInformation
	}
	if p.Category != nil {
		is.Category = *p.Category
	}
	if p.Handler != nil {
		is.Handler = &mantis.User{ID: p.Handler.ID, Name: fmt.Sprintf("user%d", p.Handler.ID), RealName: fmt.Sprintf("User %d", p.Handler.ID)}
	}
	if p.Monitors != nil {
		is.Monitors = nil
		for _, m := range *p.Monitors {
			is.Monitors = append(is.Monitors, mantis.User{ID: m.ID, Name: fmt.Sprintf("user%d", m.ID)})
		}
	}
	return nil
}

func (s *Server) create(w http.ResponseWriter, body []byte) {
	var in mantis.NewIssue
	if err := json.Unmarshal(body, &in); err != nil || in.Summary == "" {
		writeJSON(w, 400, map[string]string{"message": "summary is required"})
		return
	}
	s.nextID++
	now := time.Now()
	is := mantis.Issue{
		ID: s.nextID, Summary: in.Summary, Description: in.Description,
		Project: in.Project, Category: in.Category, Reporter: s.me,
		CreatedAt: now, UpdatedAt: now,
	}
	is.Status, _ = s.enumRef("status", &mantis.Ref{Name: "new"})
	for kind, pair := range map[string]struct {
		ref *mantis.Ref
		dst *mantis.EnumValue
	}{"priority": {in.Priority, &is.Priority}, "severity": {in.Severity, &is.Severity}, "reproducibility": {in.Reproducibility, &is.Reproducibility}} {
		if pair.ref != nil {
			*pair.dst, _ = s.enumRef(kind, pair.ref)
		}
	}
	if in.Handler != nil {
		is.Handler = &mantis.User{ID: in.Handler.ID, Name: fmt.Sprintf("user%d", in.Handler.ID)}
		is.Status, _ = s.enumRef("status", &mantis.Ref{Name: "assigned"})
	}
	s.issues[is.ID] = is
	writeJSON(w, 201, map[string]any{"issue": is})
}

func (s *Server) addNote(w http.ResponseWriter, id int, body []byte) {
	is, ok := s.issues[id]
	if !ok {
		writeJSON(w, 404, map[string]string{"message": "Issue not found"})
		return
	}
	var in struct {
		Text         string               `json:"text"`
		ViewState    mantis.Ref           `json:"view_state"`
		TimeTracking *mantis.TimeTracking `json:"time_tracking"`
	}
	if err := json.Unmarshal(body, &in); err != nil || strings.TrimSpace(in.Text) == "" {
		writeJSON(w, 400, map[string]string{"message": "note text is required"})
		return
	}
	s.nextNID++
	n := mantis.Note{
		ID: s.nextNID, Reporter: s.me, Text: in.Text, TimeTracking: in.TimeTracking,
		ViewState: mantis.EnumValue{Name: in.ViewState.Name, Label: in.ViewState.Name},
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	is.Notes = append(is.Notes, n)
	is.UpdatedAt = time.Now()
	s.issues[id] = is
	writeJSON(w, 201, map[string]any{"note": n, "issue": is})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// file serves an attachment of an issue or its notes. The fixtures carry no
// content, so images get a generated picture and everything else a line of
// text.
func (s *Server) file(w http.ResponseWriter, issueID, fileID int) {
	is := s.issues[issueID]
	all := append([]mantis.Attachment{}, is.Attachments...)
	for _, n := range is.Notes {
		all = append(all, n.Attachments...)
	}
	for _, a := range all {
		if a.ID != fileID {
			continue
		}
		content := []byte(fmt.Sprintf("Contents of %s (file %d on issue #%d).\n", a.Filename, a.ID, issueID))
		if strings.HasPrefix(a.ContentType, "image/") {
			content = samplePNG(fileID)
		}
		a.Size = int64(len(content))
		writeJSON(w, 200, map[string]any{"files": []mantis.File{{Attachment: a, Content: content}}})
		return
	}
	writeJSON(w, 404, map[string]string{"message": fmt.Sprintf("File %d not found", fileID)})
}

// samplePNG draws a 480x270 gradient whose hue depends on seed.
func samplePNG(seed int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 480, 270))
	for y := range 270 {
		for x := range 480 {
			img.Set(x, y, color.RGBA{R: uint8(x * 255 / 480), G: uint8(y * 255 / 270), B: uint8(seed * 70), A: 255})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}
