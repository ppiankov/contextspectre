package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// navAction represents a vim-style navigation action.
type navAction int

const (
	navNone      navAction = iota
	navTop                 // gg, Home
	navBottom              // G, End
	navHalfDown            // Ctrl+d
	navHalfUp              // Ctrl+u
	navPageDown            // Space, Ctrl+f
	navPageUp              // Ctrl+b
	navScreenTop           // H
	navScreenMid           // M
	navScreenBot           // L
)

const ggTimeout = 500 * time.Millisecond

// navState tracks state for vim-style navigation (gg double-tap detection).
type navState struct {
	lastGTime time.Time
}

// handleVimNav checks if a key is a vim navigation key and returns the action.
// Set ggEnabled=false for panels where 'g' has another binding (e.g. tangents in messages).
func (ns *navState) handleVimNav(msg tea.KeyMsg, ggEnabled bool) navAction {
	switch msg.Type {
	case tea.KeyHome:
		return navTop
	case tea.KeyEnd:
		return navBottom
	case tea.KeyCtrlD:
		return navHalfDown
	case tea.KeyCtrlU:
		return navHalfUp
	case tea.KeyCtrlF:
		return navPageDown
	case tea.KeyCtrlB:
		return navPageUp
	case tea.KeyRunes:
		if len(msg.Runes) != 1 {
			return navNone
		}
		switch msg.Runes[0] {
		case 'G':
			return navBottom
		case 'g':
			if !ggEnabled {
				return navNone
			}
			now := time.Now()
			if !ns.lastGTime.IsZero() && now.Sub(ns.lastGTime) < ggTimeout {
				ns.lastGTime = time.Time{}
				return navTop
			}
			ns.lastGTime = now
			return navNone
		case 'H':
			return navScreenTop
		case 'M':
			return navScreenMid
		case 'L':
			return navScreenBot
		}
	}
	return navNone
}

// applyNavAction computes new cursor and scrollOffset for a given navigation action.
// total is the number of items, visible is the number of visible rows.
func applyNavAction(action navAction, cursor, scrollOffset, total, visible int) (int, int) {
	if total == 0 {
		return 0, 0
	}
	maxCursor := total - 1

	switch action {
	case navTop:
		return 0, 0

	case navBottom:
		scroll := total - visible
		if scroll < 0 {
			scroll = 0
		}
		return maxCursor, scroll

	case navHalfDown:
		cursor += visible / 2
		if cursor > maxCursor {
			cursor = maxCursor
		}

	case navHalfUp:
		cursor -= visible / 2
		if cursor < 0 {
			cursor = 0
		}

	case navPageDown:
		cursor += visible
		if cursor > maxCursor {
			cursor = maxCursor
		}

	case navPageUp:
		cursor -= visible
		if cursor < 0 {
			cursor = 0
		}

	case navScreenTop:
		cursor = scrollOffset
		if cursor > maxCursor {
			cursor = maxCursor
		}

	case navScreenMid:
		cursor = scrollOffset + visible/2
		if cursor > maxCursor {
			cursor = maxCursor
		}

	case navScreenBot:
		cursor = scrollOffset + visible - 1
		if cursor > maxCursor {
			cursor = maxCursor
		}
	}

	// Adjust scroll to keep cursor visible.
	if cursor < scrollOffset {
		scrollOffset = cursor
	}
	if cursor >= scrollOffset+visible {
		scrollOffset = cursor - visible + 1
	}
	if scrollOffset < 0 {
		scrollOffset = 0
	}
	return cursor, scrollOffset
}
