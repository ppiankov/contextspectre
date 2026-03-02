package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorGreen  = lipgloss.Color("#00CC66")
	colorYellow = lipgloss.Color("#FFCC00")
	colorRed    = lipgloss.Color("#FF4444")
	colorMuted  = lipgloss.Color("#888888")
	colorAccent = lipgloss.Color("#7B68EE")
	colorWhite  = lipgloss.Color("#FFFFFF")
	colorDim    = lipgloss.Color("#555555")
)

var (
	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorWhite)

	styleMuted = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleSelected = lipgloss.NewStyle().
			Background(colorAccent).
			Foreground(colorWhite)

	styleMarked = lipgloss.NewStyle().
			Foreground(colorRed).
			Bold(true)

	styleActive = lipgloss.NewStyle().
			Foreground(colorYellow).
			Bold(true)

	styleFooter = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleBar = lipgloss.NewStyle()
)

func contextColor(pct float64) lipgloss.Color {
	switch {
	case pct >= 80:
		return colorRed
	case pct >= 60:
		return colorYellow
	default:
		return colorGreen
	}
}
