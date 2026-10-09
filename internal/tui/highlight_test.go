package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestLexerFor(t *testing.T) {
	cases := []struct{ code, attrs, want string }{
		{"x = 1", ` class="language-python"`, "Python"},
		{"x = 1", ` class=lang-ruby`, "Ruby"},
		{"<?php echo 1;", "", "PHP"},
		{`{"a": [1, 2]}`, "", "JSON"},
		{"SELECT * FROM t;", "", "MySQL"},
		{"select id\nfrom t", ` class="nope"`, "MySQL"},
		{"2026-10-09 ERROR update failed\nmore", "", ""},
	}
	for _, c := range cases {
		got := ""
		if l := lexerFor(c.code, c.attrs); l != nil {
			got = l.Config().Name
		}
		if got != c.want {
			t.Errorf("lexerFor(%q, %q) = %q, want %q", c.code, c.attrs, got, c.want)
		}
	}
}

func TestHighlightKeepsText(t *testing.T) {
	st := codeStyle("auto", false)
	for _, code := range []string{
		"MariaDB [db]> select id from t\n    -> where id = 24;\n+----+\n| id |\n+----+\n| 24 |\n+----+\n1 row in set (0.000 sec)",
		"<?php\n// note\n$x = \"a\";\n\nreturn $x;",
		"plain log line\nanother",
	} {
		lines := highlight(code, "", st)
		if got := ansi.Strip(strings.Join(lines, "\n")); got != code {
			t.Errorf("highlight changed the text:\n got %q\nwant %q", got, code)
		}
	}
}

func TestHighlightConsole(t *testing.T) {
	st := codeStyle("auto", false)
	lines := highlight("mysql> SELECT name FROM t;\n+------+\n| a-b  |\n+------+\n1 row in set", "", st)
	if !strings.Contains(lines[0], styleMuted.Render("mysql> ")) {
		t.Errorf("prompt should be muted: %q", lines[0])
	}
	if strings.Contains(lines[0], " SELECT ") || !strings.Contains(ansi.Strip(lines[0]), "SELECT") {
		t.Errorf("query should be highlighted: %q", lines[0])
	}
	if lines[1] != styleMuted.Render("+------+") {
		t.Errorf("rule should be muted: %q", lines[1])
	}
	if !strings.Contains(lines[2], "a-b") {
		t.Errorf("only the | of a row should be muted, not the values: %q", lines[2])
	}
	if lines[4] != "1 row in set" {
		t.Errorf("summary should stay plain: %q", lines[4])
	}
}

func TestHighlightPlain(t *testing.T) {
	code := "SELECT 1;\n| x |"
	if got := highlight(code, "", nil); strings.Join(got, "\n") != code {
		t.Errorf("a nil style should leave the block alone: %q", got)
	}
	if got := highlight("plain log\nmore", "", codeStyle("auto", false)); got[0] != "plain log" {
		t.Errorf("an unknown language should stay plain: %q", got)
	}
}

func TestCodeStyle(t *testing.T) {
	if codeStyle("none", false) != nil {
		t.Error("none should turn highlighting off")
	}
	if got := codeStyle("auto", false).Name; got != codeThemeDark {
		t.Errorf("auto on dark = %q", got)
	}
	if got := codeStyle("", true).Name; got != codeThemeLight {
		t.Errorf("auto on light = %q", got)
	}
	if got := codeStyle("monokai", true).Name; got != "monokai" {
		t.Errorf("named theme = %q", got)
	}
}
