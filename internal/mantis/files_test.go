package mantis

import (
	"context"
	"errors"
	"testing"
)

func TestGetFile(t *testing.T) {
	// "iVBORw0K" is the base64 of a PNG signature's first bytes.
	srv := newFakeServer(t, 200, []byte(`{"files":[{"id":17,"filename":"image.png","size":6,"content_type":"image/png; charset=binary","content":"iVBORw0K"}]}`))
	f, err := newTestClient(srv.URL).GetFile(context.Background(), 5, 17)
	if err != nil {
		t.Fatal(err)
	}
	if srv.last.Method != "GET" || srv.last.URL.Path != "/api/rest/issues/5/files/17" {
		t.Errorf("%s %s", srv.last.Method, srv.last.URL.Path)
	}
	if f.ID != 17 || f.Filename != "image.png" || string(f.Content) != "\x89PNG\r\n" {
		t.Errorf("file = %+v", f)
	}
}

func TestGetFileEmptyIsNotFound(t *testing.T) {
	srv := newFakeServer(t, 200, []byte(`{"files":[]}`))
	if _, err := newTestClient(srv.URL).GetFile(context.Background(), 5, 17); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v", err)
	}
}
