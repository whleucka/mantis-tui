package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/whleucka/mantis-tui/internal/mantis"
)

// benchList is a loaded list of n issues across 20 projects, grouped and
// searched, the slowest case for rows().
func benchList(b *testing.B, n int) (*harness, *listModel) {
	b.Helper()
	h := listHarness(&testing.T{}, 0, nil)
	l := h.m.cur.list
	issues := make([]mantis.Issue, n)
	for i := range issues {
		issues[i] = mantis.Issue{
			ID: i + 1, Summary: fmt.Sprintf("Issue number %d about the login form", i+1),
			Project:   mantis.Ref{ID: i % 20, Name: fmt.Sprintf("Project %02d", i%20)},
			Category:  mantis.Ref{Name: "General"},
			Status:    enum("assigned"),
			Priority:  enum("normal"),
			UpdatedAt: time.Date(2026, 9, 1, 0, 0, i, 0, time.UTC),
		}
	}
	l.setIssues(h.m, issues, 0)
	l.grouped = true
	l.setSearch(h.m, "log")
	return h, l
}

// BenchmarkListMove is one j press and one redraw.
func BenchmarkListMove(b *testing.B) {
	for _, n := range []int{1000, 5000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			h, l := benchList(b, n)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				l.move(h.m, 1)
				_ = h.m.View()
			}
		})
	}
}
