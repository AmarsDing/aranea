// Package tool implements `aranea tool ls/get/enable/disable`. The
// command surface is intentionally narrow because tools are managed by
// admins through the web UI; the CLI is for inspection and scripted
// toggles.
package tool

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"arenea/backend/cmd/internal/apiclient"
	"arenea/backend/cmd/internal/output"
	"arenea/backend/internal/domain"
)

// NewCommand returns the parent command.
func NewCommand(g *apiclient.GlobalContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tool",
		Short: "Inspect and toggle tools",
	}
	cmd.AddCommand(newListCmd(g), newGetCmd(g), newToggleCmd(g, true), newToggleCmd(g, false))
	return cmd
}

func newListCmd(g *apiclient.GlobalContext) *cobra.Command {
	var (
		search    string
		category  string
		source    string
		riskLevel string
		enabled   string
		limit     int
		offset    int
	)
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List tools",
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			for k, v := range map[string]string{
				"search": search, "category": category, "source": source,
				"risk_level": riskLevel, "enabled": enabled,
			} {
				if v != "" {
					q.Set(k, v)
				}
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			if offset > 0 {
				q.Set("offset", strconv.Itoa(offset))
			}
			var result domain.ToolListResult
			if err := g.Client().Get(cmd.Context(), "/api/v1/tools", q, &result); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().StringVar(&search, "search", "", "Free-text search")
	cmd.Flags().StringVar(&category, "category", "", "Filter by category")
	cmd.Flags().StringVar(&source, "source", "", "Filter by source (builtin|skill|mcp|...)")
	cmd.Flags().StringVar(&riskLevel, "risk", "", "Filter by risk level")
	cmd.Flags().StringVar(&enabled, "enabled", "", "Filter by enabled flag (true|false)")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum rows")
	cmd.Flags().IntVar(&offset, "offset", 0, "Pagination offset")
	return cmd
}

func newGetCmd(g *apiclient.GlobalContext) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id-or-key>",
		Short: "Show a single tool",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var t domain.Tool
			if err := g.Client().Get(cmd.Context(), "/api/v1/tools/"+url.PathEscape(args[0]), nil, &t); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), t)
			return nil
		},
	}
}

func newToggleCmd(g *apiclient.GlobalContext, enabled bool) *cobra.Command {
	use := "enable"
	short := "Enable a tool"
	if !enabled {
		use = "disable"
		short = "Disable a tool"
	}
	return &cobra.Command{
		Use:   use + " <id-or-key>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]bool{"enabled": enabled}
			var updated domain.Tool
			if err := g.Client().Patch(cmd.Context(), "/api/v1/tools/"+url.PathEscape(args[0])+"/enabled", body, &updated); err != nil {
				return err
			}
			output.Success(cmd.OutOrStdout(), updated.Key)
			return nil
		},
	}
}
