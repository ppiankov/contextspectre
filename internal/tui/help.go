package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// helpEntry represents a single keybinding in the help overlay.
type helpEntry struct {
	key  string
	desc string
}

// helpModel manages the help overlay display.
type helpModel struct {
	visible bool
	title   string
	entries []helpEntry
	width   int
	height  int
}

// toggle flips help visibility. When showing, sets the title and entries.
func (h *helpModel) toggle(title string, entries []helpEntry) {
	if h.visible {
		h.visible = false
		return
	}
	h.visible = true
	h.title = title
	h.entries = entries
}

// dismiss hides the help overlay.
func (h *helpModel) dismiss() {
	h.visible = false
}

var (
	helpBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorAccent)).
			Padding(1, 2)
	helpTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorAccent)).
			Bold(true)
	helpKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorWhite)).
			Bold(true).
			Width(12)
	helpDescStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorMuted))
)

// View renders the help overlay as a centered box.
func (h helpModel) View() string {
	if !h.visible || len(h.entries) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", helpTitleStyle.Render(h.title))

	for _, e := range h.entries {
		fmt.Fprintf(&b, "%s %s\n", helpKeyStyle.Render(e.key), helpDescStyle.Render(e.desc))
	}
	b.WriteString("\nPress ? or Esc to close")

	content := helpBorderStyle.Render(b.String())

	// Center horizontally and vertically.
	contentWidth := lipgloss.Width(content)
	contentHeight := lipgloss.Height(content)

	padLeft := (h.width - contentWidth) / 2
	if padLeft < 0 {
		padLeft = 0
	}
	padTop := (h.height - contentHeight) / 2
	if padTop < 0 {
		padTop = 0
	}

	var out strings.Builder
	for range padTop {
		out.WriteString("\n")
	}
	for _, line := range strings.Split(content, "\n") {
		fmt.Fprintf(&out, "%s%s\n", strings.Repeat(" ", padLeft), line)
	}
	return out.String()
}

// sessionsHelp returns help entries for the session browser.
func sessionsHelp() []helpEntry {
	return []helpEntry{
		{"j/↓", "Move down"},
		{"k/↑", "Move up"},
		{"G/End", "Jump to last entry"},
		{"gg/Home", "Jump to first entry"},
		{"Ctrl+d", "Half page down"},
		{"Ctrl+u", "Half page up"},
		{"Space", "Full page down"},
		{"Ctrl+f", "Full page down"},
		{"Ctrl+b", "Full page up"},
		{"H", "Top of screen"},
		{"M", "Middle of screen"},
		{"L", "Bottom of screen"},
		{"Enter", "Open session"},
		{"/", "Search sessions"},
		{"n/N", "Next/prev search match"},
		{"s", "Cycle sort column"},
		{"v", "Vector control panel"},
		{"q", "Quit"},
		{"?", "Toggle this help"},
	}
}

// messagesHelp returns help entries for the messages panel.
func messagesHelp() []helpEntry {
	return []helpEntry{
		{"j/↓", "Move down"},
		{"k/↑", "Move up"},
		{"G/End", "Jump to last entry"},
		{"Home", "Jump to first entry"},
		{"Ctrl+d", "Half page down"},
		{"Ctrl+u", "Half page up"},
		{"Ctrl+f", "Full page down"},
		{"Ctrl+b", "Full page up"},
		{"H", "Top of screen"},
		{"M", "Middle of screen"},
		{"L", "Bottom of screen"},
		{"/", "Search messages"},
		{"n/N", "Next/prev search match"},
		{"Space", "Toggle selection"},
		{"d", "Delete selected"},
		{"u", "Undo last change"},
		{"K", "Toggle keep marker"},
		{"N*", "Toggle noise marker"},
		{"p", "Commit point"},
		{"!", "Amputate range"},
		{"e", "Epoch timeline"},
		{"q/Esc", "Back"},
		{"?", "Toggle this help"},
	}
}

// overviewHelp returns help entries for the overview panel.
func overviewHelp() []helpEntry {
	return []helpEntry{
		{"j/↓", "Scroll down"},
		{"k/↑", "Scroll up"},
		{"G/End", "Jump to bottom"},
		{"gg/Home", "Jump to top"},
		{"Ctrl+d", "Half page down"},
		{"Ctrl+u", "Half page up"},
		{"Space", "Full page down"},
		{"Tab", "Next panel"},
		{"Shift+Tab", "Previous panel"},
		{"1-4", "Jump to panel"},
		{"Enter", "Go to Messages"},
		{"Esc", "Back to sessions"},
		{"?", "Toggle this help"},
	}
}

// branchesHelp returns help entries for the branch navigator.
func branchesHelp() []helpEntry {
	return []helpEntry{
		{"j/↓", "Move down"},
		{"k/↑", "Move up"},
		{"G/End", "Jump to last branch"},
		{"gg/Home", "Jump to first branch"},
		{"Ctrl+d", "Half page down"},
		{"Ctrl+u", "Half page up"},
		{"Space", "Toggle selection"},
		{"Enter", "Drill into branch"},
		{"e", "Export selected"},
		{"W", "Export + wipe"},
		{"q/Esc", "Back"},
		{"?", "Toggle this help"},
	}
}

// vectorHelp returns help entries for the vector control panel.
func vectorHelp() []helpEntry {
	return []helpEntry{
		{"j/\u2193", "Scroll down"},
		{"k/\u2191", "Scroll up"},
		{"G/End", "Jump to bottom"},
		{"gg/Home", "Jump to top"},
		{"Ctrl+d", "Half page down"},
		{"Ctrl+u", "Half page up"},
		{"Space", "Full page down"},
		{"C", "Clean session"},
		{"S", "Split tangent range"},
		{"E", "Export decisions (TBD)"},
		{"q/Esc", "Back to sessions"},
		{"?", "Toggle this help"},
	}
}

// epochsHelp returns help entries for the epoch timeline.
func epochsHelp() []helpEntry {
	return []helpEntry{
		{"j/↓", "Move down"},
		{"k/↑", "Move up"},
		{"G/End", "Jump to last epoch"},
		{"gg/Home", "Jump to first epoch"},
		{"Ctrl+d", "Half page down"},
		{"Ctrl+u", "Half page up"},
		{"Enter", "Toggle epoch details"},
		{"q/Esc", "Back"},
		{"?", "Toggle this help"},
	}
}
