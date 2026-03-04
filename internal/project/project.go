package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Config holds the contextspectre configuration file.
type Config struct {
	Version int              `json:"version"`
	Aliases map[string]Alias `json:"aliases"`
}

// Alias maps a logical project name to one or more filesystem paths.
type Alias struct {
	Paths []string `json:"paths"`
}

// ConfigPath returns the path to contextspectre.json within the given claude dir.
func ConfigPath(claudeDir string) string {
	return filepath.Join(claudeDir, "contextspectre.json")
}

// Load reads the config from disk. Returns empty config if file does not exist.
func Load(claudeDir string) (*Config, error) {
	data, err := os.ReadFile(ConfigPath(claudeDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{Version: 1, Aliases: make(map[string]Alias)}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Aliases == nil {
		cfg.Aliases = make(map[string]Alias)
	}
	return &cfg, nil
}

// Save writes the config to disk atomically (temp file + rename).
func Save(claudeDir string, cfg *Config) error {
	if cfg.Version == 0 {
		cfg.Version = 1
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')

	path := ConfigPath(claudeDir)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename config: %w", err)
	}
	return nil
}

// SetAlias adds or updates an alias. Validates name format and that paths exist.
func (c *Config) SetAlias(name string, paths []string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("invalid alias name %q: must be lowercase alphanumeric with hyphens", name)
	}
	if len(paths) == 0 {
		return fmt.Errorf("alias requires at least one path")
	}

	for _, p := range paths {
		if !filepath.IsAbs(p) {
			return fmt.Errorf("path must be absolute: %s", p)
		}
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("path does not exist: %s", p)
		}
	}

	c.Aliases[name] = Alias{Paths: paths}
	return nil
}

// RemoveAlias removes an alias by name.
func (c *Config) RemoveAlias(name string) error {
	if _, ok := c.Aliases[name]; !ok {
		return fmt.Errorf("alias %q not found", name)
	}
	delete(c.Aliases, name)
	return nil
}

// Resolve returns the paths for a given alias name. Returns nil if not found.
func (c *Config) Resolve(name string) []string {
	a, ok := c.Aliases[name]
	if !ok {
		return nil
	}
	return a.Paths
}

// SortedNames returns alias names in sorted order.
func (c *Config) SortedNames() []string {
	names := make([]string, 0, len(c.Aliases))
	for name := range c.Aliases {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
