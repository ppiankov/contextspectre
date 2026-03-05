package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/ppiankov/contextspectre/internal/analyzer"
)

var (
	colorGreen  = lipgloss.Color("#00CC66")
	colorYellow = lipgloss.Color("#FFCC00")
	colorRed    = lipgloss.Color("#FF4444")
	colorAmber  = lipgloss.Color("#FF8C00")
	colorMuted  = lipgloss.Color("#888888")
	colorAccent = lipgloss.Color("#7B68EE")
	colorWhite  = lipgloss.Color("#FFFFFF")
	colorDim    = lipgloss.Color("#555555")
	colorCyan   = lipgloss.Color("#00CCCC")
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

	styleCompacted = lipgloss.NewStyle().
			Foreground(colorAmber)

	styleGhost = lipgloss.NewStyle().
			Foreground(colorDim)

	styleWarning = lipgloss.NewStyle().
			Foreground(colorRed).
			Bold(true)

	styleCommitPoint = lipgloss.NewStyle().
				Foreground(colorCyan).
				Bold(true)

	stylePhaseExplore = lipgloss.NewStyle().
				Foreground(colorDim)

	stylePhaseDecision = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true)

	stylePhaseOperational = lipgloss.NewStyle().
				Foreground(colorGreen)

	styleAmputate = lipgloss.NewStyle().
			Background(lipgloss.Color("#3B0000")).
			Foreground(colorRed)
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

// contextColorCompacted returns amber for post-compaction sessions at low usage,
// falling back to normal color coding at higher usage levels.
func contextColorCompacted(pct float64) lipgloss.Color {
	if pct < 60 {
		return colorAmber
	}
	return contextColor(pct)
}

// gradeStyle returns a lipgloss style colored by health grade.
func gradeStyle(grade string) lipgloss.Style {
	switch grade {
	case "A", "B":
		return lipgloss.NewStyle().Foreground(colorGreen)
	case "C":
		return lipgloss.NewStyle().Foreground(colorYellow)
	default: // D, F
		return lipgloss.NewStyle().Foreground(colorRed)
	}
}

func entropyStyle(level string) lipgloss.Style {
	switch level {
	case "LOW":
		return lipgloss.NewStyle().Foreground(colorGreen)
	case "MEDIUM":
		return lipgloss.NewStyle().Foreground(colorYellow)
	case "HIGH":
		return lipgloss.NewStyle().Foreground(colorAmber)
	case "CRITICAL":
		return lipgloss.NewStyle().Foreground(colorRed)
	default:
		return lipgloss.NewStyle().Foreground(colorMuted)
	}
}

// gaugeStateColor returns a color for the vector gauge state.
func gaugeStateColor(state analyzer.VectorState) lipgloss.Color {
	switch state {
	case analyzer.VectorEmergency:
		return colorRed
	case analyzer.VectorUnstable:
		return colorAmber
	case analyzer.VectorDegrading:
		return colorYellow
	default:
		return colorGreen
	}
}
