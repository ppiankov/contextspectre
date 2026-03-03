package tui

import (
	"fmt"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/session"
)

type viewState int

const (
	viewSessions viewState = iota
	viewBranches
	viewMessages
	viewConfirm
	viewEpochs
)

// AppModel is the top-level Bubble Tea model.
type AppModel struct {
	currentView   viewState
	sessions      sessionsModel
	branches      branchesModel
	messages      messagesModel
	confirm       confirmModel
	epochs        epochsModel
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
		m.branches.width = msg.Width
		m.branches.height = msg.Height
		m.messages.width = msg.Width
		m.messages.height = msg.Height
		m.confirm.width = msg.Width
		m.confirm.height = msg.Height
		m.epochs.width = msg.Width
		m.epochs.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case openSessionMsg:
		entries, err := jsonl.Parse(msg.info.FullPath)
		if err != nil {
			m.messages = messagesModel{session: msg.info, statusMsg: fmt.Sprintf("Error: %v", err)}
			m.messages.width = m.width
			m.messages.height = m.height
			m.currentView = viewMessages
			return m, nil
		}
		stats := analyzer.Analyze(entries)
		branches := analyzer.FindBranches(entries, stats.Compactions)
		if len(branches) > 1 {
			m.branches = newBranchesModel(branches, msg.info)
			m.branches.width = m.width
			m.branches.height = m.height
			m.currentView = viewBranches
		} else {
			m.messages = newMessagesModel(msg.info)
			m.messages.width = m.width
			m.messages.height = m.height
			m.currentView = viewMessages
		}
		return m, nil

	case drillIntoBranchMsg:
		m.messages = newMessagesModel(msg.info)
		m.messages.cursor = msg.startIdx
		m.messages.scrollOffset = msg.startIdx
		m.messages.branchOrigin = true
		m.messages.width = m.width
		m.messages.height = m.height
		m.currentView = viewMessages
		return m, nil

	case backFromBranchesMsg:
		m.currentView = viewSessions
		return m, nil

	case backToSessionsMsg:
		if m.messages.branchOrigin {
			m.currentView = viewBranches
		} else {
			m.currentView = viewSessions
		}
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

	case openEpochsMsg:
		m.epochs = newEpochsModel(msg.epochs, msg.info)
		m.epochs.driftResult = msg.drift
		m.epochs.width = m.width
		m.epochs.height = m.height
		m.currentView = viewEpochs
		return m, nil

	case backFromEpochsMsg:
		m.currentView = viewMessages
		return m, nil
	}

	// Delegate to current view
	var cmd tea.Cmd
	switch m.currentView {
	case viewSessions:
		if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "q" && !m.sessions.searching {
			return m, tea.Quit
		}
		m.sessions, cmd = m.sessions.Update(msg)
	case viewBranches:
		m.branches, cmd = m.branches.Update(msg)
	case viewMessages:
		m.messages, cmd = m.messages.Update(msg)
	case viewConfirm:
		m.confirm, cmd = m.confirm.Update(msg)
	case viewEpochs:
		m.epochs, cmd = m.epochs.Update(msg)
	}

	return m, cmd
}

func (m AppModel) View() string {
	switch m.currentView {
	case viewBranches:
		return m.branches.View()
	case viewMessages:
		return m.messages.View()
	case viewConfirm:
		return m.confirm.View()
	case viewEpochs:
		return m.epochs.View()
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
