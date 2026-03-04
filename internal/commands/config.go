package commands

import (
	"fmt"
	"os"
	"strconv"

	"github.com/ppiankov/contextspectre/internal/analyzer"
	"github.com/ppiankov/contextspectre/internal/project"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage contextspectre configuration",
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigGet,
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration values",
	RunE:  runConfigList,
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	dir := resolveClaudeDir()
	cfg, err := project.Load(dir)
	if err != nil {
		return err
	}

	key, value := args[0], args[1]
	switch key {
	case "cost-alert":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid cost-alert value: %s (must be a number)", value)
		}
		if v < 0 {
			return fmt.Errorf("cost-alert must be >= 0 (0 disables)")
		}
		cfg.CostAlertThreshold = v
	default:
		return fmt.Errorf("unknown config key: %s (available: cost-alert)", key)
	}

	if err := project.Save(dir, cfg); err != nil {
		return err
	}

	if key == "cost-alert" && cfg.CostAlertThreshold == 0 {
		fmt.Println("Cost alert disabled.")
	} else {
		fmt.Printf("Set %s = %s\n", key, value)
	}
	return nil
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	dir := resolveClaudeDir()
	cfg, err := project.Load(dir)
	if err != nil {
		return err
	}

	key := args[0]
	switch key {
	case "cost-alert":
		if cfg.CostAlertThreshold == 0 {
			fmt.Println("cost-alert: disabled")
		} else {
			fmt.Printf("cost-alert: %s\n", analyzer.FormatCost(cfg.CostAlertThreshold))
		}
	default:
		return fmt.Errorf("unknown config key: %s (available: cost-alert)", key)
	}
	return nil
}

func runConfigList(cmd *cobra.Command, args []string) error {
	dir := resolveClaudeDir()
	cfg, err := project.Load(dir)
	if err != nil {
		return err
	}

	if isJSON() {
		return printJSON(buildConfigJSON(cfg))
	}

	fmt.Println("Configuration:")
	if cfg.CostAlertThreshold > 0 {
		fmt.Printf("  cost-alert: %s\n", analyzer.FormatCost(cfg.CostAlertThreshold))
	} else {
		fmt.Println("  cost-alert: disabled")
	}
	return nil
}

// ConfigJSON is the JSON output for the config list command.
type ConfigJSON struct {
	CostAlertThreshold float64 `json:"cost_alert_threshold"`
}

func buildConfigJSON(cfg *project.Config) *ConfigJSON {
	return &ConfigJSON{
		CostAlertThreshold: cfg.CostAlertThreshold,
	}
}

// loadCostAlertThreshold returns the configured cost alert threshold, or 0 if disabled.
func loadCostAlertThreshold() float64 {
	dir := resolveClaudeDir()
	cfg, err := project.Load(dir)
	if err != nil {
		return 0
	}
	return cfg.CostAlertThreshold
}

// printCostAlert prints a cost alert warning to stderr if cost exceeds threshold.
func printCostAlert(cost, threshold float64) {
	if threshold > 0 && cost >= threshold {
		fmt.Fprintf(os.Stderr, "!! Session cost: %s (threshold: %s)\n",
			analyzer.FormatCost(cost), analyzer.FormatCost(threshold))
	}
}

func init() {
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configListCmd)
	rootCmd.AddCommand(configCmd)
}
