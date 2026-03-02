package tui

import (
	"fmt"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/session"
)

type viewState int

const (
	viewSessions viewState = iota
	viewMessages
	viewConfirm
)

// AppModel is the top-level Bubble Tea model.
type AppModel struct {
	currentView   viewState
	sessions      sessionsModel
	messages      messagesModel
	confirm       confirmModel
	claudeDir     string
	version       string
	width, height int
	_             error // reserved for future error display
}

// NewApp creates a new app model with session discovery.
func NewApp(claudeDir, version string) AppModel {
	d := &session.Discoverer{ClaudeDir: claudeDir}
	sessions, err := d.ListAllSessions()
	if err != nil {
		slog.Warn("Failed to list sessions", "error", err)
	}

	return AppModel{
		currentView: viewSessions,
		sessions:    newSessionsModel(sessions),
		claudeDir:   claudeDir,
		version:     version,
	}
}

func (m AppModel) Init() tea.Cmd {
	return nil
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.sessions.width = msg.Width
		m.sessions.height = msg.Height
		m.messages.width = msg.Width
		m.messages.height = msg.Height
		m.confirm.width = msg.Width
		m.confirm.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case openSessionMsg:
		m.messages = newMessagesModel(msg.info)
		m.messages.width = m.width
		m.messages.height = m.height
		m.currentView = viewMessages
		return m, nil

	case backToSessionsMsg:
		m.currentView = viewSessions
		return m, nil

	case showConfirmMsg:
		m.confirm = newConfirmModel(msg.selected, msg.impact)
		m.confirm.width = m.width
		m.confirm.height = m.height
		m.currentView = viewConfirm
		return m, nil

	case confirmDeleteMsg:
		result, err := editor.Delete(m.messages.session.FullPath, msg.selected)
		if err != nil {
			m.messages.statusMsg = fmt.Sprintf("Delete error: %v", err)
		} else {
			m.messages.statusMsg = fmt.Sprintf("Deleted %d entries, %d chain repairs",
				result.EntriesRemoved, result.ChainRepairs)
			m.messages = m.messages.reload()
		}
		m.currentView = viewMessages
		return m, nil

	case cancelDeleteMsg:
		m.currentView = viewMessages
		return m, nil
	}

	// Delegate to current view
	var cmd tea.Cmd
	switch m.currentView {
	case viewSessions:
		if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "q" {
			return m, tea.Quit
		}
		m.sessions, cmd = m.sessions.Update(msg)
	case viewMessages:
		m.messages, cmd = m.messages.Update(msg)
	case viewConfirm:
		m.confirm, cmd = m.confirm.Update(msg)
	}

	return m, cmd
}

func (m AppModel) View() string {
	switch m.currentView {
	case viewMessages:
		return m.messages.View()
	case viewConfirm:
		return m.confirm.View()
	default:
		return m.sessions.View()
	}
}

// Run starts the TUI application.
func Run(claudeDir, version string) error {
	model := NewApp(claudeDir, version)
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
