package tui

import (
	"fmt"
	"log/slog"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/editor"
	"github.com/ppiankov/contextspectre/internal/jsonl"
	"github.com/ppiankov/contextspectre/internal/project"
	"github.com/ppiankov/contextspectre/internal/session"
)

type viewState int

const (
	viewSessions viewState = iota
	viewBranches
	viewDetail
	viewMessages
	viewConfirm
	viewCommitPoint
	viewEpochs
	viewVector
)

// AppModel is the top-level Bubble Tea model.
type AppModel struct {
	currentView   viewState
	sessions      sessionsModel
	branches      branchesModel
	detail        detailModel
	messages      messagesModel
	confirm       confirmModel
	commitPoint   commitPointModel
	epochs        epochsModel
	vector        vectorModel
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

	aliasLookup := buildAliasLookup(claudeDir)
	costThreshold := loadCostThreshold(claudeDir)

	return AppModel{
		currentView: viewSessions,
		sessions:    newSessionsModel(sessions, aliasLookup, costThreshold),
		claudeDir:   claudeDir,
		version:     version,
	}
}

// buildAliasLookup builds a reverse lookup from encoded path prefix to alias name.
func buildAliasLookup(claudeDir string) map[string]string {
	cfg, err := project.Load(claudeDir)
	if err != nil || len(cfg.Aliases) == 0 {
		return nil
	}
	lookup := make(map[string]string)
	for name, alias := range cfg.Aliases {
		for _, p := range alias.Paths {
			encoded := session.EncodePath(p)
			lookup[encoded] = name
		}
	}
	return lookup
}

// loadCostThreshold loads the cost alert threshold from config.
func loadCostThreshold(claudeDir string) float64 {
	cfg, err := project.Load(claudeDir)
	if err != nil {
		return 0
	}
	return cfg.CostAlertThreshold
}

// resolveAliasName returns the alias name for a session, or empty string.
func resolveAliasName(fullPath string, lookup map[string]string) string {
	for encoded, name := range lookup {
		if strings.Contains(fullPath, encoded) {
			return name
		}
	}
	return ""
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
		m.detail.width = msg.Width
		m.detail.height = msg.Height
		m.detail.messages.width = msg.Width
		m.detail.messages.height = msg.Height - 3 // tab bar overhead
		m.messages.width = msg.Width
		m.messages.height = msg.Height
		m.confirm.width = msg.Width
		m.confirm.height = msg.Height
		m.commitPoint.width = msg.Width
		m.commitPoint.height = msg.Height
		m.epochs.width = msg.Width
		m.epochs.height = msg.Height
		m.vector.width = msg.Width
		m.vector.height = msg.Height
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
			m.detail = newDetailModel(msg.info)
			m.detail.width = m.width
			m.detail.height = m.height
			m.detail.messages.width = m.width
			m.detail.messages.height = m.height - 3
			m.currentView = viewDetail
		}
		return m, nil

	case drillIntoBranchMsg:
		m.detail = newDetailModel(msg.info)
		m.detail.messages.cursor = msg.startIdx
		m.detail.messages.scrollOffset = msg.startIdx
		m.detail.branchOrigin = true
		m.detail.activePanel = panelMessages
		m.detail.width = m.width
		m.detail.height = m.height
		m.detail.messages.width = m.width
		m.detail.messages.height = m.height - 3
		m.currentView = viewDetail
		return m, nil

	case backFromBranchesMsg:
		m.currentView = viewSessions
		return m, nil

	case backFromDetailMsg:
		if msg.branchOrigin {
			m.currentView = viewBranches
		} else {
			m.currentView = viewSessions
		}
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
		// Route to detail.messages if we came from detail view
		msgs := &m.messages
		if m.detail.activePanel == panelMessages {
			msgs = &m.detail.messages
		}
		result, err := editor.Delete(msgs.session.FullPath, msg.selected)
		if err != nil {
			msgs.statusMsg = fmt.Sprintf("Delete error: %v", err)
		} else {
			msgs.statusMsg = fmt.Sprintf("Deleted %d entries, %d chain repairs",
				result.EntriesRemoved, result.ChainRepairs)
			*msgs = msgs.reload()
		}
		if m.detail.activePanel == panelMessages {
			m.currentView = viewDetail
		} else {
			m.currentView = viewMessages
		}
		return m, nil

	case cancelDeleteMsg:
		if m.detail.activePanel == panelMessages {
			m.currentView = viewDetail
		} else {
			m.currentView = viewMessages
		}
		return m, nil

	case showCommitPointMsg:
		m.commitPoint = newCommitPointModel(msg.commitPoint, msg.cursorIdx)
		m.commitPoint.width = m.width
		m.commitPoint.height = m.height
		m.currentView = viewCommitPoint
		return m, nil

	case confirmCommitPointMsg:
		msgs := &m.messages
		if m.detail.activePanel == panelMessages {
			msgs = &m.detail.messages
		}
		msgs.markers.AddCommitPoint(msg.commitPoint)
		for i := 0; i < msg.cursorIdx; i++ {
			uuid := msgs.entries[i].UUID
			if uuid != "" && !msgs.markers.IsKeep(uuid) {
				msgs.markers.Set(uuid, editor.MarkerCandidate)
			}
		}
		_ = editor.SaveMarkers(msgs.session.FullPath, msgs.markers)
		msgs.statusMsg = fmt.Sprintf("Commit point set — %d entries marked CANDIDATE", msg.cursorIdx)
		if m.detail.activePanel == panelMessages {
			m.currentView = viewDetail
		} else {
			m.currentView = viewMessages
		}
		return m, nil

	case cancelCommitPointMsg:
		if m.detail.activePanel == panelMessages {
			m.currentView = viewDetail
		} else {
			m.currentView = viewMessages
		}
		return m, nil

	case openEpochsMsg:
		m.epochs = newEpochsModel(msg.epochs, msg.info)
		m.epochs.driftResult = msg.drift
		m.epochs.width = m.width
		m.epochs.height = m.height
		m.currentView = viewEpochs
		return m, nil

	case backFromEpochsMsg:
		if m.detail.activePanel == panelMessages {
			m.currentView = viewDetail
		} else {
			m.currentView = viewMessages
		}
		return m, nil

	case openVectorMsg:
		m.vector = newVectorModel(msg.info, m.claudeDir)
		m.vector.width = m.width
		m.vector.height = m.height
		m.currentView = viewVector
		return m, nil

	case backFromVectorMsg:
		m.currentView = viewSessions
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
	case viewDetail:
		m.detail, cmd = m.detail.Update(msg)
	case viewMessages:
		m.messages, cmd = m.messages.Update(msg)
	case viewConfirm:
		m.confirm, cmd = m.confirm.Update(msg)
	case viewCommitPoint:
		m.commitPoint, cmd = m.commitPoint.Update(msg)
	case viewEpochs:
		m.epochs, cmd = m.epochs.Update(msg)
	case viewVector:
		m.vector, cmd = m.vector.Update(msg)
	}

	return m, cmd
}

func (m AppModel) View() string {
	switch m.currentView {
	case viewBranches:
		return m.branches.View()
	case viewDetail:
		return m.detail.View()
	case viewMessages:
		return m.messages.View()
	case viewConfirm:
		return m.confirm.View()
	case viewCommitPoint:
		return m.commitPoint.View()
	case viewEpochs:
		return m.epochs.View()
	case viewVector:
		return m.vector.View()
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
