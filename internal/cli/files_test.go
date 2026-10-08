package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesListsNoteAttachments(t *testing.T) {
	fm := newFakeMantis(t)
	fm.on("GET", "/issues/33", 200, mantisFixture(t, "issue"))
	cfg := fakeHostConfig(t, fm, "")

	out, _, err := runCLI(t, "--config", cfg, "files", "33")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ID", "ATTACHED TO", "6", "64.3 KB", "note 62 by User 2", "file-6.txt"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	out, _, err = runCLI(t, "--config", cfg, "--json", "files", "33")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		ID     int    `json:"id"`
		Name   string `json:"filename"`
		NoteID int    `json:"note_id"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 1 || rows[0].ID != 6 || rows[0].NoteID != 62 {
		t.Fatalf("json = %s (err %v)", out, err)
	}
}

func TestShowNotesNamesFileIDs(t *testing.T) {
	fm := newFakeMantis(t)
	fm.on("GET", "/issues/33", 200, mantisFixture(t, "issue"))
	cfg := fakeHostConfig(t, fm, "")
	out, _, err := runCLI(t, "--config", cfg, "show", "33", "--notes")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "attachment: file-6.txt (file 6, 64.3 KB)") {
		t.Errorf("note attachment missing:\n%s", out)
	}
}

func TestDownloadSavesFiles(t *testing.T) {
	fm := newFakeMantis(t)
	fm.on("GET", "/issues/33", 200, mantisFixture(t, "issue"))
	fm.on("GET", "/issues/33/files/6", 200, []byte(`{"files":[{"id":6,"filename":"file-6.txt","size":5,"content":"aGVsbG8="}]}`))
	cfg := fakeHostConfig(t, fm, "")
	dir := filepath.Join(t.TempDir(), "out")

	out, _, err := runCLI(t, "--config", cfg, "download", "33", "6", "-o", dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "6-file-6.txt")
	if strings.TrimSpace(out) != path {
		t.Errorf("printed %q, want %q", out, path)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "hello" {
		t.Errorf("content = %q, err %v", b, err)
	}

	if _, _, err := runCLI(t, "--config", cfg, "download", "33", "99", "-o", dir); err == nil || !strings.Contains(err.Error(), "has no file 99") {
		t.Errorf("unknown file id: err = %v", err)
	}
}
