// Package mcp implements `aranea mcp ls/get`. MCP servers live under
// /api/v1/mcp-servers in the backend's platform-resources router.
package mcp

import (
	"net/url"

	"github.com/spf13/cobra"

	"arenea/backend/cmd/aranea/internal/apiclient"
	"arenea/backend/cmd/aranea/internal/output"
	"arenea/backend/internal/domain"
)

// NewCommand returns the parent command.
func NewCommand(g *apiclient.GlobalContext) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Inspect MCP servers",
	}
	cmd.AddCommand(newListCmd(g), newGetCmd(g))
	return cmd
}

func newListCmd(g *apiclient.GlobalContext) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List MCP servers",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var resp struct {
				Items []domain.PlatformResource `json:"items"`
			}
			if err := g.Client().Get(cmd.Context(), "/api/v1/mcp-servers", nil, &resp); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), resp)
			return nil
		},
	}
}

func newGetCmd(g *apiclient.GlobalContext) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show a single MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var p domain.PlatformResource
			if err := g.Client().Get(cmd.Context(), "/api/v1/mcp-servers/"+url.PathEscape(args[0]), nil, &p); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), p)
			return nil
		},
	}
}
