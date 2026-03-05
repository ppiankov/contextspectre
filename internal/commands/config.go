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
	case "weekly-budget":
		fallthrough
	case "weekly-limit":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid %s value: %s (must be a number)", key, value)
		}
		if v < 0 {
			return fmt.Errorf("%s must be >= 0 (0 disables)", key)
		}
		cfg.WeeklyLimit = v
		cfg.WeeklyBudgetLimit = v
	case "billing-week-start":
		v, err := validateBillingWeekStart(value)
		if err != nil {
			return err
		}
		cfg.BillingWeekStart = v
	case "expert-mode":
		switch value {
		case "true", "1", "on":
			cfg.ExpertMode = true
		case "false", "0", "off":
			cfg.ExpertMode = false
		default:
			return fmt.Errorf("invalid expert-mode value: %s (use true/false)", value)
		}
	case "health-context-warn":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid health-context-warn value: %s (must be a number)", value)
		}
		if v < 0 || v > 100 {
			return fmt.Errorf("health-context-warn must be 0-100 (0 resets to default)")
		}
		cfg.HealthContextWarn = v
	case "health-cpd-warn":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid health-cpd-warn value: %s (must be a number)", value)
		}
		if v < 0 {
			return fmt.Errorf("health-cpd-warn must be >= 0 (0 resets to default)")
		}
		cfg.HealthCPDWarn = v
	case "health-ttc-warn":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid health-ttc-warn value: %s (must be an integer)", value)
		}
		if v < 0 {
			return fmt.Errorf("health-ttc-warn must be >= 0 (0 resets to default)")
		}
		cfg.HealthTTCWarn = v
	case "health-cdr-warn":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("invalid health-cdr-warn value: %s (must be a number)", value)
		}
		if v < 0 || v > 1 {
			return fmt.Errorf("health-cdr-warn must be 0-1 (0 resets to default)")
		}
		cfg.HealthCDRWarn = v
	default:
		return fmt.Errorf("unknown config key: %s (available: cost-alert, weekly-budget, weekly-limit, billing-week-start, expert-mode, health-context-warn, health-cpd-warn, health-ttc-warn, health-cdr-warn)", key)
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
	case "weekly-budget":
		fallthrough
	case "weekly-limit":
		limit := cfg.WeeklyLimit
		if limit == 0 {
			limit = cfg.WeeklyBudgetLimit
		}
		if limit == 0 {
			fmt.Printf("%s: disabled\n", key)
		} else {
			fmt.Printf("%s: %s/week\n", key, analyzer.FormatCost(limit))
		}
	case "billing-week-start":
		if cfg.BillingWeekStart == "" {
			fmt.Println("billing-week-start: monday")
		} else {
			fmt.Printf("billing-week-start: %s\n", cfg.BillingWeekStart)
		}
	case "expert-mode":
		if cfg.ExpertMode {
			fmt.Println("expert-mode: enabled")
		} else {
			fmt.Println("expert-mode: disabled")
		}
	case "health-context-warn":
		if cfg.HealthContextWarn > 0 {
			fmt.Printf("health-context-warn: %.1f%%\n", cfg.HealthContextWarn)
		} else {
			fmt.Printf("health-context-warn: default (%.1f%%)\n", analyzer.DefaultGaugeThresholds.ContextWarn)
		}
	case "health-cpd-warn":
		if cfg.HealthCPDWarn > 0 {
			fmt.Printf("health-cpd-warn: %s\n", analyzer.FormatCost(cfg.HealthCPDWarn))
		} else {
			fmt.Printf("health-cpd-warn: default (%s)\n", analyzer.FormatCost(analyzer.DefaultGaugeThresholds.CPDWarn))
		}
	case "health-ttc-warn":
		if cfg.HealthTTCWarn > 0 {
			fmt.Printf("health-ttc-warn: %d turns\n", cfg.HealthTTCWarn)
		} else {
			fmt.Printf("health-ttc-warn: default (%d turns)\n", analyzer.DefaultGaugeThresholds.TTCWarn)
		}
	case "health-cdr-warn":
		if cfg.HealthCDRWarn > 0 {
			fmt.Printf("health-cdr-warn: %.2f\n", cfg.HealthCDRWarn)
		} else {
			fmt.Printf("health-cdr-warn: default (%.2f)\n", analyzer.DefaultGaugeThresholds.CDRWarn)
		}
	default:
		return fmt.Errorf("unknown config key: %s (available: cost-alert, weekly-budget, weekly-limit, billing-week-start, expert-mode, health-context-warn, health-cpd-warn, health-ttc-warn, health-cdr-warn)", key)
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
	limit := cfg.WeeklyLimit
	if limit == 0 {
		limit = cfg.WeeklyBudgetLimit
	}
	if limit > 0 {
		fmt.Printf("  weekly-limit: %s/week\n", analyzer.FormatCost(limit))
	} else {
		fmt.Println("  weekly-limit: disabled")
	}
	if cfg.BillingWeekStart == "" {
		fmt.Println("  billing-week-start: monday")
	} else {
		fmt.Printf("  billing-week-start: %s\n", cfg.BillingWeekStart)
	}
	if cfg.ExpertMode {
		fmt.Println("  expert-mode: enabled")
	} else {
		fmt.Println("  expert-mode: disabled")
	}
	printThreshold := func(key string, val, defVal float64, suffix string) {
		if val > 0 {
			fmt.Printf("  %s: %.4g%s\n", key, val, suffix)
		} else {
			fmt.Printf("  %s: default (%.4g%s)\n", key, defVal, suffix)
		}
	}
	printThreshold("health-context-warn", cfg.HealthContextWarn, analyzer.DefaultGaugeThresholds.ContextWarn, "%")
	printThreshold("health-cpd-warn", cfg.HealthCPDWarn, analyzer.DefaultGaugeThresholds.CPDWarn, "")
	if cfg.HealthTTCWarn > 0 {
		fmt.Printf("  health-ttc-warn: %d turns\n", cfg.HealthTTCWarn)
	} else {
		fmt.Printf("  health-ttc-warn: default (%d turns)\n", analyzer.DefaultGaugeThresholds.TTCWarn)
	}
	printThreshold("health-cdr-warn", cfg.HealthCDRWarn, analyzer.DefaultGaugeThresholds.CDRWarn, "")
	return nil
}

// ConfigJSON is the JSON output for the config list command.
type ConfigJSON struct {
	CostAlertThreshold float64 `json:"cost_alert_threshold"`
	WeeklyBudgetLimit  float64 `json:"weekly_budget_limit"`
	WeeklyLimit        float64 `json:"weekly_limit"`
	BillingWeekStart   string  `json:"billing_week_start"`
	ExpertMode         bool    `json:"expert_mode"`
	HealthContextWarn  float64 `json:"health_context_warn"`
	HealthCPDWarn      float64 `json:"health_cpd_warn"`
	HealthTTCWarn      int     `json:"health_ttc_warn"`
	HealthCDRWarn      float64 `json:"health_cdr_warn"`
}

func buildConfigJSON(cfg *project.Config) *ConfigJSON {
	t := loadGaugeThresholds()
	limit := cfg.WeeklyLimit
	if limit == 0 {
		limit = cfg.WeeklyBudgetLimit
	}
	weekStart := cfg.BillingWeekStart
	if weekStart == "" {
		weekStart = "monday"
	}
	return &ConfigJSON{
		CostAlertThreshold: cfg.CostAlertThreshold,
		WeeklyBudgetLimit:  cfg.WeeklyBudgetLimit,
		WeeklyLimit:        limit,
		BillingWeekStart:   weekStart,
		ExpertMode:         cfg.ExpertMode,
		HealthContextWarn:  t.ContextWarn,
		HealthCPDWarn:      t.CPDWarn,
		HealthTTCWarn:      t.TTCWarn,
		HealthCDRWarn:      t.CDRWarn,
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

// loadGaugeThresholds returns gauge thresholds from config, falling back to defaults.
func loadGaugeThresholds() analyzer.GaugeThresholds {
	dir := resolveClaudeDir()
	cfg, err := project.Load(dir)
	if err != nil {
		return analyzer.DefaultGaugeThresholds
	}
	t := analyzer.DefaultGaugeThresholds
	if cfg.HealthContextWarn > 0 {
		t.ContextWarn = cfg.HealthContextWarn
	}
	if cfg.HealthCPDWarn > 0 {
		t.CPDWarn = cfg.HealthCPDWarn
	}
	if cfg.HealthTTCWarn > 0 {
		t.TTCWarn = cfg.HealthTTCWarn
	}
	if cfg.HealthCDRWarn > 0 {
		t.CDRWarn = cfg.HealthCDRWarn
	}
	return t
}

func init() {
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configListCmd)
	rootCmd.AddCommand(configCmd)
}
