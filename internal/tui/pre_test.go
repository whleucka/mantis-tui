package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestSplitPre(t *testing.T) {
	cases := []struct {
		in   string
		want []segment
	}{
		{"plain text", []segment{{text: "plain text", code: false}}},
		{
			"Before:\n\n<pre>\n| id | name |\n|  1 | a    |\n</pre>\n\nAfter",
			[]segment{{text: "Before:", code: false}, {text: "| id | name |\n|  1 | a    |", code: true}, {text: "After", code: false}},
		},
		{"<PRE class=\"sql\">select 1;</Pre>", []segment{{text: "select 1;", code: true, attrs: " class=\"sql\""}}},
		{"x\n<pre>\n  indented\n  still", []segment{{text: "x", code: false}, {text: "  indented\n  still", code: true}}},
		{"stray </pre> tag", []segment{{text: "stray </pre> tag", code: false}}},
		{"<pre>\r\nwin\r\n</pre>", []segment{{text: "win", code: true}}},
		{"<pre></pre>", nil},
	}
	for _, c := range cases {
		got := splitPre(c.in)
		if len(got) != len(c.want) {
			t.Errorf("splitPre(%q) = %+v, want %+v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitPre(%q)[%d] = %+v, want %+v", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestRenderBodyCodeBlock(t *testing.T) {
	body := "There are two:\n<pre>\n+----+\n| id |\n\tx\n" + strings.Repeat("a", 50) + "\n</pre>"
	out := ansi.Strip(renderBody(body, 30, nil))
	if strings.Contains(out, "<pre>") || strings.Contains(out, "</pre>") {
		t.Errorf("tags should be stripped:\n%s", out)
	}
	lines := strings.Split(out, "\n")
	want := []string{"There are two:", "", "│ +----+", "│ | id |", "│     x", "│ " + strings.Repeat("a", 26), "│ " + strings.Repeat("a", 24)}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(lines), len(want), out)
	}
	for i := range want {
		if strings.TrimRight(lines[i], " ") != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestStripPre(t *testing.T) {
	if got := stripPre("<pre>"); got != "" {
		t.Errorf("stripPre(<pre>) = %q", got)
	}
	if got := stripPre("<pre>select 1;</pre>"); got != "select 1;" {
		t.Errorf("stripPre = %q", got)
	}
}

func TestRenderBodySpacesBlocks(t *testing.T) {
	cases := map[string]string{
		"<pre>a</pre>\n<pre>b</pre>":     "│ a\n\n│ b",
		"x\n<pre>a</pre>\ny":             "x\n\n│ a\n\ny",
		"<pre>a</pre>":                   "│ a",
		"x\n\n\n<pre>\na\n</pre>\n\n\ny": "x\n\n│ a\n\ny",
	}
	for in, want := range cases {
		out := ansi.Strip(renderBody(in, 40, nil))
		var lines []string
		for l := range strings.SplitSeq(out, "\n") {
			lines = append(lines, strings.TrimRight(l, " "))
		}
		if got := strings.Join(lines, "\n"); got != want {
			t.Errorf("renderBody(%q) = %q, want %q", in, got, want)
		}
	}
}
