package tui

import (
	"encoding/json"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// Themes the "auto" code theme picks between.
const (
	codeThemeDark  = "catppuccin-mocha"
	codeThemeLight = "catppuccin-latte"
)

// codeStyle resolves a code_theme setting: "none" turns highlighting off and
// "auto" follows the terminal background.
func codeStyle(theme string, light bool) *chroma.Style {
	switch theme {
	case "none":
		return nil
	case "", "auto":
		if light {
			theme = codeThemeLight
		} else {
			theme = codeThemeDark
		}
	}
	return styles.Get(theme)
}

var (
	// sqlPrompt is a mysql/mariadb client prompt or continuation, with the
	// statement after it.
	sqlPrompt = regexp.MustCompile(`^(\s*(?:mysql|MariaDB \[[^\]]*\]|\s*->)>?\s)(.*)$`)
	// sqlStart opens a statement typed without a prompt.
	sqlStart = regexp.MustCompile(`(?i)^\s*(select|insert|update|delete|with|create|alter|drop|show|describe|explain)\s`)
	// tableRule is a line of a client's bordered result table.
	tableRule = regexp.MustCompile(`^\s*[+|]`)
	// sqlOutcome is a client's summary line after a statement.
	sqlOutcome = regexp.MustCompile(`^(\d+ rows?|Empty set)\b|^Query OK`)
	// langClass is the language named in a <pre> tag's class attribute.
	langClass = regexp.MustCompile(`(?i)\bclass\s*=\s*["']?(?:lang(?:uage)?-)?([\w+#-]+)`)
)

// highlight colours one <pre> block, returning one string per line. A nil
// style, or a block whose language can't be told, comes back plain apart
// from dimmed table borders.
func highlight(code, attrs string, st *chroma.Style) []string {
	lines := strings.Split(code, "\n")
	if st == nil {
		return lines
	}
	if !isConsole(lines) {
		if lexer := lexerFor(code, attrs); lexer != nil {
			return colorize(lexer, code, st)
		}
		return lines
	}
	sql := lexers.Get("mysql")
	out := make([]string, 0, len(lines))
	var stmt []string // statement lines awaiting a lex, so a query can span several
	flush := func() {
		if len(stmt) > 0 {
			out = append(out, colorize(sql, strings.Join(stmt, "\n"), st)...)
			stmt = nil
		}
	}
	for _, line := range lines {
		switch m := sqlPrompt.FindStringSubmatch(line); {
		case m != nil:
			flush()
			out = append(out, styleMuted.Render(m[1])+colorize(sql, m[2], st)[0])
		case tableRule.MatchString(line):
			flush()
			out = append(out, dimBorders(line))
		case sqlStart.MatchString(line) || len(stmt) > 0 && !isOutcome(line):
			stmt = append(stmt, line)
		default:
			flush()
			out = append(out, line)
		}
	}
	flush()
	return out
}

// isConsole reports whether lines look like a SQL client session or its
// result tables rather than source code.
func isConsole(lines []string) bool {
	for _, l := range lines {
		if sqlPrompt.MatchString(l) || strings.HasPrefix(strings.TrimSpace(l), "+-") {
			return true
		}
	}
	return false
}

// isOutcome is a client's summary after a statement, e.g. "1 row in set".
func isOutcome(line string) bool {
	l := strings.TrimSpace(line)
	return l == "" || sqlOutcome.MatchString(l)
}

// lexerFor picks the block's language: the tag's class, then a few cheap
// tells, then chroma's guess. Nil means leave it plain.
func lexerFor(code, attrs string) chroma.Lexer {
	if m := langClass.FindStringSubmatch(attrs); m != nil {
		if l := lexers.Get(m[1]); l != nil {
			return l
		}
	}
	trimmed := strings.TrimSpace(code)
	switch {
	case strings.HasPrefix(trimmed, "<?php"):
		return lexers.Get("php")
	case (strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")) && json.Valid([]byte(trimmed)):
		return lexers.Get("json")
	case sqlStart.MatchString(trimmed):
		return lexers.Get("mysql")
	}
	return lexers.Analyse(code)
}

// colorize lexes code and styles each token with st's foreground, bold and
// italic. Text in the theme's base colour keeps the terminal's own.
func colorize(lexer chroma.Lexer, code string, st *chroma.Style) []string {
	it, err := chroma.Coalesce(lexer).Tokenise(nil, code)
	if err != nil {
		return strings.Split(code, "\n")
	}
	base := st.Get(chroma.Background).Colour
	cache := map[chroma.TokenType]lipgloss.Style{}
	var out []string
	for _, line := range chroma.SplitTokensIntoLines(it.Tokens()) {
		var b strings.Builder
		for _, tok := range line {
			s, ok := cache[tok.Type]
			if !ok {
				e := st.Get(tok.Type)
				s = lipgloss.NewStyle().Bold(e.Bold == chroma.Yes).Italic(e.Italic == chroma.Yes)
				if e.Colour.IsSet() && e.Colour != base {
					s = s.Foreground(lipgloss.Color(e.Colour.String()))
				}
				cache[tok.Type] = s
			}
			b.WriteString(s.Render(strings.TrimSuffix(tok.Value, "\n")))
		}
		out = append(out, b.String())
	}
	return out
}

// dimBorders mutes a result table's borders so the values stand out: all of
// a +---+ rule, and the | between cells.
func dimBorders(line string) string {
	if strings.HasPrefix(strings.TrimSpace(line), "+") {
		return styleMuted.Render(line)
	}
	return strings.ReplaceAll(line, "|", styleMuted.Render("|"))
}
