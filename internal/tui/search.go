package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// searchAction represents the result of processing a key during search mode.
type searchAction int

const (
	searchNone    searchAction = iota
	searchConfirm              // Enter — jump to current match
	searchCancel               // Esc — cancel search, restore position
	searchNext                 // n — next match
	searchPrev                 // N — previous match
)

// searchModel provides vim-style / search with n/N match cycling.
type searchModel struct {
	active     bool
	query      string
	matches    []int // indices of matching items
	currentIdx int   // index into matches slice
	prevCursor int   // cursor position before search started (for restore on cancel)
}

// activate enters search mode and records the current cursor for restore.
func (s *searchModel) activate(currentCursor int) {
	s.active = true
	s.query = ""
	s.matches = nil
	s.currentIdx = 0
	s.prevCursor = currentCursor
}

// handleSearchKey processes a key during search input mode.
// Returns whether the key was handled and the resulting action.
func (s *searchModel) handleSearchKey(msg tea.KeyMsg) (bool, searchAction) {
	switch msg.Type {
	case tea.KeyEsc:
		s.active = false
		return true, searchCancel
	case tea.KeyEnter:
		s.active = false
		return true, searchConfirm
	case tea.KeyBackspace:
		if len(s.query) > 0 {
			s.query = s.query[:len(s.query)-1]
		}
		return true, searchNone
	case tea.KeyRunes:
		s.query += string(msg.Runes)
		return true, searchNone
	}
	return false, searchNone
}

// handleMatchNav processes n/N for match cycling (called outside search input mode).
func (s *searchModel) handleMatchNav(msg tea.KeyMsg) searchAction {
	if len(s.matches) == 0 {
		return searchNone
	}
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		switch msg.Runes[0] {
		case 'n':
			s.next()
			return searchNext
		case 'N':
			s.prev()
			return searchPrev
		}
	}
	return searchNone
}

// findMatches scans total items using matchFn to populate the match list.
// matchFn(i) should return true if item i matches the current query.
func (s *searchModel) findMatches(total int, matchFn func(i int) bool) {
	s.matches = nil
	s.currentIdx = 0
	if s.query == "" {
		return
	}
	for i := 0; i < total; i++ {
		if matchFn(i) {
			s.matches = append(s.matches, i)
		}
	}
}

func (s *searchModel) next() {
	if len(s.matches) == 0 {
		return
	}
	s.currentIdx = (s.currentIdx + 1) % len(s.matches)
}

func (s *searchModel) prev() {
	if len(s.matches) == 0 {
		return
	}
	s.currentIdx = (s.currentIdx - 1 + len(s.matches)) % len(s.matches)
}

// currentMatch returns the item index of the current match, or -1 if no matches.
func (s *searchModel) currentMatch() int {
	if len(s.matches) == 0 {
		return -1
	}
	return s.matches[s.currentIdx]
}

// isMatch returns true if item index i is in the match set.
func (s *searchModel) isMatch(i int) bool {
	for _, m := range s.matches {
		if m == i {
			return true
		}
	}
	return false
}

// hasQuery returns true if there's a non-empty query (even after search input is closed).
func (s *searchModel) hasQuery() bool {
	return s.query != ""
}

var styleSearchBar = lipgloss.NewStyle().Foreground(lipgloss.Color(colorAccent))

// renderBar returns the search bar string for display at the bottom of a panel.
func (s *searchModel) renderBar(width int) string {
	if !s.active && !s.hasQuery() {
		return ""
	}

	var bar strings.Builder
	fmt.Fprintf(&bar, "/%s", s.query)
	if s.active {
		bar.WriteString("█") // cursor indicator
	}

	if len(s.matches) > 0 {
		fmt.Fprintf(&bar, "  [%d/%d]", s.currentIdx+1, len(s.matches))
	} else if s.hasQuery() {
		bar.WriteString("  [0 matches]")
	}

	line := bar.String()
	if len(line) > width {
		line = line[:width]
	}
	return styleSearchBar.Render(line)
}
