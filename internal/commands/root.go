package commands

import (
	"github.com/ppiankov/contextspectre/internal/logging"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/ppiankov/contextspectre/internal/tui"
	"github.com/spf13/cobra"
)

var (
	verbose   bool
	claudeDir string
	version   string
	commit    string
	date      string
)

var rootCmd = &cobra.Command{
	Use:   "contextspectre",
	Short: "contextspectre — Claude Code conversation context manager",
	Long: `contextspectre gives you visibility and control over Claude Code session context.

Quick start:
  contextspectre status          One-screen summary (auto-detects from CWD)
  contextspectre active          Show active sessions with signal grades
  contextspectre quick-clean     One-command cleanup of most recent session
  contextspectre                 Launch interactive TUI

Aliases: "status" = summary, "list" = sessions

See https://github.com/ppiankov/contextspectre for full documentation.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logging.Init(verbose)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := claudeDir
		if dir == "" {
			dir = session.DefaultClaudeDir()
		}
		return tui.Run(dir, version)
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command with injected build info.
func Execute(v, c, d string) error {
	version = v
	commit = c
	date = d
	return rootCmd.Execute()
}

// ClaudeDir returns the configured claude directory path.
func ClaudeDir() string {
	return claudeDir
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Enable verbose logging")
	rootCmd.PersistentFlags().StringVar(&claudeDir, "claude-dir", "", "Override ~/.claude directory path")
	rootCmd.PersistentFlags().StringVar(&outputFormat, "format", "", "Output format (json for machine-readable)")

	rootCmd.AddCommand(versionCmd)
}
