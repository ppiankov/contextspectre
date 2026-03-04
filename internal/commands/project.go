package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/ppiankov/contextspectre/internal/project"
	"github.com/ppiankov/contextspectre/internal/session"
	"github.com/spf13/cobra"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage project aliases for federated session grouping",
}

var projectAliasCmd = &cobra.Command{
	Use:   "alias <name> <path1> [path2...]",
	Short: "Define a project alias mapping to one or more directories",
	Args:  cobra.MinimumNArgs(2),
	RunE:  runProjectAlias,
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all project aliases with session counts",
	RunE:  runProjectList,
}

var projectRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a project alias",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectRemove,
}

func runProjectAlias(cmd *cobra.Command, args []string) error {
	dir := resolveClaudeDir()
	name := args[0]

	// Resolve paths to absolute
	paths := make([]string, len(args)-1)
	for i, p := range args[1:] {
		abs, err := filepath.Abs(p)
		if err != nil {
			return fmt.Errorf("resolve path %s: %w", p, err)
		}
		paths[i] = abs
	}

	cfg, err := project.Load(dir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := cfg.SetAlias(name, paths); err != nil {
		return err
	}

	if err := project.Save(dir, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	if isJSON() {
		return printJSON(map[string]any{
			"name":  name,
			"paths": paths,
		})
	}

	fmt.Printf("Alias %q set to %d path(s)\n", name, len(paths))
	for _, p := range paths {
		fmt.Printf("  %s\n", p)
	}
	return nil
}

func runProjectList(cmd *cobra.Command, args []string) error {
	dir := resolveClaudeDir()

	cfg, err := project.Load(dir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if len(cfg.Aliases) == 0 {
		if isJSON() {
			return printJSON(ProjectListOutput{Aliases: []ProjectAliasJSON{}})
		}
		fmt.Println("No project aliases defined.")
		fmt.Println("Use: contextspectre project alias <name> <path1> [path2...]")
		return nil
	}

	// Discover all sessions once
	d := &session.Discoverer{ClaudeDir: dir}
	allSessions, _ := d.ListAllSessions()

	names := cfg.SortedNames()

	if isJSON() {
		out := ProjectListOutput{}
		for _, name := range names {
			alias := cfg.Aliases[name]
			count := countAliasedSessions(allSessions, alias.Paths)
			out.Aliases = append(out.Aliases, ProjectAliasJSON{
				Name:     name,
				Paths:    alias.Paths,
				Sessions: count,
			})
		}
		return printJSON(out)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tPATHS\tSESSIONS")
	fmt.Fprintln(w, "────\t─────\t────────")
	for _, name := range names {
		alias := cfg.Aliases[name]
		count := countAliasedSessions(allSessions, alias.Paths)
		for i, p := range alias.Paths {
			label := name
			countStr := fmt.Sprintf("%d", count)
			if i > 0 {
				label = ""
				countStr = ""
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", label, p, countStr)
		}
	}
	return w.Flush()
}

func runProjectRemove(cmd *cobra.Command, args []string) error {
	dir := resolveClaudeDir()
	name := args[0]

	cfg, err := project.Load(dir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := cfg.RemoveAlias(name); err != nil {
		return err
	}

	if err := project.Save(dir, cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	if isJSON() {
		return printJSON(map[string]any{"removed": name})
	}

	fmt.Printf("Alias %q removed.\n", name)
	return nil
}

// countAliasedSessions counts sessions matching any of the given paths.
func countAliasedSessions(sessions []session.Info, paths []string) int {
	count := 0
	for _, s := range sessions {
		for _, p := range paths {
			if strings.Contains(s.FullPath, session.EncodePath(p)) {
				count++
				break
			}
		}
	}
	return count
}

func init() {
	projectCmd.AddCommand(projectAliasCmd)
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectRemoveCmd)
	rootCmd.AddCommand(projectCmd)
}
