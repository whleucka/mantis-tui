package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

// setupUploadFake is the note fake with wh's limits, answering with as
// many attachments as kept.
func setupUploadFake(t *testing.T, kept int) (*fakeMantis, string) {
	t.Helper()
	fm, cfg := setupNoteFake(t)
	fm.on("GET", "/config", 200, []byte(`{"configs":[{"option":"max_file_size","value":100},{"option":"disallowed_files","value":"svg"}]}`))
	atts := strings.TrimSuffix(strings.Repeat(`{"id":9},`, kept), ",")
	fm.on("POST", "/issues/33/notes", 201, []byte(`{"note":{"id":62,"attachments":[`+atts+`]}}`))
	fm.on("POST", "/issues", 201, []byte(`{"issue":{"id":1234,"attachments":[`+atts+`]}}`))
	return fm, cfg
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func sentFiles(t *testing.T, body map[string]any) map[string]string {
	t.Helper()
	out := map[string]string{}
	files, _ := body["files"].([]any)
	for _, f := range files {
		m := f.(map[string]any)
		b, err := base64.StdEncoding.DecodeString(m["content"].(string))
		if err != nil {
			t.Fatal(err)
		}
		out[m["name"].(string)] = string(b)
	}
	return out
}

func clipboardWith(img *mantis.FileUpload) deps {
	d := testDeps()
	d.clipboard = func(context.Context) *mantis.FileUpload { return img }
	return d
}

// Files alone make a note with no text, and no editor opens even on a terminal.
func TestNoteWithOnlyFiles(t *testing.T) {
	fm, cfg := setupUploadFake(t, 2)
	d := clipboardWith(&mantis.FileUpload{Name: "screenshot-1.png", Content: []byte("PNG")})
	d.isTerminal = func() bool { return true }
	t.Setenv("EDITOR", "false") // would fail the run if it opened
	out, _, err := runCLIWith(t, d, "--config", cfg, "note", "33", "--file", writeFile(t, "app.log", "boom"), "--clipboard")
	if err != nil {
		t.Fatal(err)
	}
	body := noteBody(t, fm)
	if body["text"] != "" {
		t.Errorf("text = %q, want empty", body["text"])
	}
	if got := sentFiles(t, body); got["app.log"] != "boom" || got["screenshot-1.png"] != "PNG" || len(got) != 2 {
		t.Errorf("files = %v", got)
	}
	if !strings.Contains(out, "#33 note 62 added with 2 files") {
		t.Errorf("output = %q", out)
	}
}

func TestNoteTextAndFileJSON(t *testing.T) {
	fm, cfg := setupUploadFake(t, 1)
	out, _, err := runCLI(t, "--config", cfg, "--json", "note", "33", "-m", "see log", "--file", writeFile(t, "a.log", "x"))
	if err != nil {
		t.Fatal(err)
	}
	if noteBody(t, fm)["text"] != "see log" {
		t.Errorf("body = %v", noteBody(t, fm))
	}
	var res []map[string]any
	if err := json.Unmarshal([]byte(out), &res); err != nil || res[0]["files"] != float64(1) {
		t.Errorf("json = %s (%v)", out, err)
	}
}

func TestUploadsRefusedBeforeSending(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		d    deps
		want string
	}{
		"too big":      {[]string{"--file", writeFile(t, "big.log", strings.Repeat("x", 101))}, testDeps(), "big.log is 101 B; the server takes files up to 100 B"},
		"disallowed":   {[]string{"--file", writeFile(t, "logo.svg", "<svg/>")}, testDeps(), "doesn't accept .svg files"},
		"missing":      {[]string{"--file", "/nonexistent/x.png"}, testDeps(), "attach /nonexistent/x.png"},
		"no clipboard": {[]string{"--clipboard"}, clipboardWith(nil), "the clipboard holds no image"},
	} {
		t.Run(name, func(t *testing.T) {
			fm, cfg := setupUploadFake(t, 1)
			_, _, err := runCLIWith(t, tc.d, append([]string{"--config", cfg, "note", "33", "-m", "x"}, tc.args...)...)
			var ue *usageError
			if !errors.As(err, &ue) || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want a usage error with %q", err, tc.want)
			}
			if fm.lastRequest("POST", "/issues/33/notes") != nil {
				t.Error("nothing may be sent")
			}
		})
	}
}

func TestDroppedUploadIsAnError(t *testing.T) {
	_, cfg := setupUploadFake(t, 0)
	_, _, err := runCLI(t, "--config", cfg, "note", "33", "--file", writeFile(t, "a.log", "x"))
	if err == nil || !strings.Contains(err.Error(), "the server kept 0 of 1 files") {
		t.Errorf("err = %v", err)
	}
}

func TestCreateWithClipboard(t *testing.T) {
	fm, cfg := setupUploadFake(t, 1)
	fm.on("GET", "/projects", 200, mantisFixture(t, "projects"))
	fm.on("GET", "/projects/4", 200, mantisFixture(t, "project"))
	d := clipboardWith(&mantis.FileUpload{Name: "screenshot-2.png", Content: []byte("PNG")})
	out, _, err := runCLIWith(t, d, "--config", cfg, "create", "--project", "4", "--category", "General", "--summary", "s", "-d", "d", "--clipboard")
	if err != nil {
		t.Fatal(err)
	}
	r := fm.lastRequest("POST", "/issues")
	var body map[string]any
	if r == nil || json.Unmarshal(r.Body, &body) != nil {
		t.Fatal("no create POST")
	}
	if got := sentFiles(t, body); got["screenshot-2.png"] != "PNG" {
		t.Errorf("files = %v", got)
	}
	if !strings.Contains(out, "#1234 created with 1 file:") {
		t.Errorf("output = %q", out)
	}
}
