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
	Long: `contextspectre shows context usage for Claude Code conversations and
enables selective cleanup to extend conversation lifespan before compaction.

It reads JSONL conversation files from ~/.claude/projects/ and provides:
  - Context usage meter with compaction distance estimate
  - Selective message deletion with impact prediction
  - Image replacement to reclaim context space
  - Automatic backup before any modification`,
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
