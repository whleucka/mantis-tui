package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPaneRunnerFor(t *testing.T) {
	found := func(string) (string, error) { return "/usr/bin/herdr", nil }
	inHerdr := func(k string) string { return map[string]string{"HERDR_ENV": "1"}[k] }
	var calls []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, "|"))
		if args[1] == "split" {
			return []byte(`{"id":"cli:pane:split","result":{"pane":{"pane_id":"w1:p7"}}}`), nil
		}
		return nil, nil
	}

	if paneRunnerFor(func(string) string { return "" }, found, "/w", run) != nil {
		t.Error("outside herdr there is no pane runner")
	}
	if paneRunnerFor(inHerdr, func(string) (string, error) { return "", errors.New("no") }, "/w", run) != nil {
		t.Error("without a herdr binary there is no pane runner")
	}

	runIn := paneRunnerFor(inHerdr, found, "/w", run)
	if err := runIn(context.Background(), "claude 'hi #5'", true); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/usr/bin/herdr pane|split|--current|--direction|right|--focus|--cwd|/w",
		"/usr/bin/herdr pane|run|w1:p7|claude 'hi #5'",
	}
	if strings.Join(calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestPaneRunnerReportsHerdrFailures(t *testing.T) {
	inHerdr := func(k string) string { return map[string]string{"HERDR_ENV": "1"}[k] }
	found := func(string) (string, error) { return "herdr", nil }
	for _, tt := range []struct {
		name string
		out  string
		err  error
		want string
	}{
		{"split fails", "", errors.New("exit status 1"), "herdr pane split: exit status 1"},
		{"no pane id", `{"result":{}}`, nil, "no pane id"},
	} {
		run := func(context.Context, string, ...string) ([]byte, error) { return []byte(tt.out), tt.err }
		err := paneRunnerFor(inHerdr, found, "", run)(context.Background(), "claude", false)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want %q", tt.name, err, tt.want)
		}
	}
}
