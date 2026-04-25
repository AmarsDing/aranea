// Package config owns the on-disk CLI configuration at
// ~/.aranea/config.toml. It deliberately uses a hand-rolled TOML reader
// for the small subset of syntax the CLI cares about (key = "value"
// pairs grouped under [profile.<name>] sections) so that the binary
// stays free of an extra third-party dependency.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"arenea/backend/cmd/aranea/internal/output"
)

// Config models the contents of ~/.aranea/config.toml. The file uses
// TOML by convention but this package only needs the small subset
// described in 前端/25 cli.md §10. Anything more elaborate (lists,
// inline tables, nested arrays) is intentionally rejected so the parser
// stays tiny and predictable.
type Config struct {
	Default  string              `toml:"default"`
	Profiles map[string]*Profile `toml:"profile"`
}

// Profile is a single named configuration block.
type Profile struct {
	BaseURL     string `toml:"base_url"`
	Token       string `toml:"token"`
	Output      string `toml:"output"`
	StreamMode  string `toml:"stream_mode"`
	DefaultMode string `toml:"default_mode"`
}

// Profile returns the named profile or nil if it doesn't exist.
func (c *Config) Profile(name string) *Profile {
	if c == nil || c.Profiles == nil {
		return nil
	}
	return c.Profiles[name]
}

// Path returns the resolved absolute path to the config file. It honors
// $ARANEA_CONFIG and otherwise defaults to ~/.aranea/config.toml.
func Path() (string, error) {
	if env := os.Getenv("ARANEA_CONFIG"); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".aranea", "config.toml"), nil
}

// Load reads the config file from disk. A missing file is treated as an
// empty configuration rather than an error so first-run usage works
// without setup.
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	cfg := &Config{Profiles: map[string]*Profile{}}
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, err
	}
	defer f.Close()
	if err := decode(f, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", p, err)
	}
	return cfg, nil
}

// Save persists cfg to disk, creating ~/.aranea if it does not exist.
func Save(cfg *Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp := p + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := encode(f, cfg); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func decode(r io.Reader, cfg *Config) error {
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]*Profile{}
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	currentSection := ""
	for lineNo, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			return fmt.Errorf("line %d: expected key = value", lineNo+1)
		}
		key := strings.TrimSpace(line[:eq])
		value := unquote(strings.TrimSpace(line[eq+1:]))

		switch {
		case currentSection == "":
			if key == "default" {
				cfg.Default = value
			}
		case strings.HasPrefix(currentSection, "profile."):
			name := strings.TrimPrefix(currentSection, "profile.")
			prof, ok := cfg.Profiles[name]
			if !ok {
				prof = &Profile{}
				cfg.Profiles[name] = prof
			}
			switch key {
			case "base_url":
				prof.BaseURL = value
			case "token":
				prof.Token = value
			case "output":
				prof.Output = value
			case "stream_mode":
				prof.StreamMode = value
			case "default_mode":
				prof.DefaultMode = value
			}
		}
	}
	return nil
}

func encode(w io.Writer, cfg *Config) error {
	if cfg.Default != "" {
		if _, err := fmt.Fprintf(w, "default = %q\n\n", cfg.Default); err != nil {
			return err
		}
	}
	names := make([]string, 0, len(cfg.Profiles))
	for k := range cfg.Profiles {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		prof := cfg.Profiles[name]
		if _, err := fmt.Fprintf(w, "[profile.%s]\n", name); err != nil {
			return err
		}
		writePair := func(k, v string) error {
			if v == "" {
				return nil
			}
			_, err := fmt.Fprintf(w, "%s = %q\n", k, v)
			return err
		}
		for _, kv := range []struct{ k, v string }{
			{"base_url", prof.BaseURL},
			{"token", prof.Token},
			{"output", prof.Output},
			{"stream_mode", prof.StreamMode},
			{"default_mode", prof.DefaultMode},
		} {
			if err := writePair(kv.k, kv.v); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	return nil
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}

// NewCommand builds the `aranea config` Cobra sub-tree (get/set/path/show).
// We intentionally keep the surface area tiny — heavier ergonomics belong
// in the web UI, not the CLI.
func NewCommand(_ any) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage local CLI configuration (~/.aranea/config.toml)",
	}
	cmd.AddCommand(newPathCmd(), newShowCmd(), newSetCmd(), newGetCmd())
	return cmd
}

func newPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the absolute path of the config file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := Path()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), p)
			return nil
		},
	}
}

func newShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the resolved configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := Load()
			if err != nil {
				return err
			}
			if output.Format() == "json" {
				output.Render(cmd.OutOrStdout(), cfg)
				return nil
			}
			return encode(cmd.OutOrStdout(), cfg)
		},
	}
}

func newSetCmd() *cobra.Command {
	var profile string
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value (key may be base_url|token|output|stream_mode|default_mode)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := Load()
			if err != nil {
				return err
			}
			if profile == "" {
				profile = cfg.Default
			}
			if profile == "" {
				profile = "default"
			}
			prof, ok := cfg.Profiles[profile]
			if !ok {
				prof = &Profile{}
				cfg.Profiles[profile] = prof
			}
			switch args[0] {
			case "base_url":
				prof.BaseURL = args[1]
			case "token":
				prof.Token = args[1]
			case "output":
				prof.Output = args[1]
			case "stream_mode":
				prof.StreamMode = args[1]
			case "default_mode":
				prof.DefaultMode = args[1]
			case "default":
				cfg.Default = args[1]
			default:
				return fmt.Errorf("unknown key: %q", args[0])
			}
			if cfg.Default == "" {
				cfg.Default = profile
			}
			return Save(cfg)
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile to update (defaults to active profile)")
	return cmd
}

func newGetCmd() *cobra.Command {
	var profile string
	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Print a single configuration value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := Load()
			if err != nil {
				return err
			}
			if profile == "" {
				profile = cfg.Default
			}
			if profile == "" {
				profile = "default"
			}
			prof := cfg.Profile(profile)
			if prof == nil {
				return fmt.Errorf("unknown profile: %q", profile)
			}
			switch args[0] {
			case "base_url":
				fmt.Fprintln(cmd.OutOrStdout(), prof.BaseURL)
			case "token":
				fmt.Fprintln(cmd.OutOrStdout(), prof.Token)
			case "output":
				fmt.Fprintln(cmd.OutOrStdout(), prof.Output)
			case "stream_mode":
				fmt.Fprintln(cmd.OutOrStdout(), prof.StreamMode)
			case "default_mode":
				fmt.Fprintln(cmd.OutOrStdout(), prof.DefaultMode)
			case "default":
				fmt.Fprintln(cmd.OutOrStdout(), cfg.Default)
			default:
				return fmt.Errorf("unknown key: %q", args[0])
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&profile, "profile", "", "Profile to read")
	return cmd
}
