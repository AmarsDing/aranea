// Package agent implements `aranea agent ...`. The command tree mirrors
// the /api/v1/agents endpoints: list, get, default. Mutating operations
// (create / update) intentionally live in the web UI for now because
// they require a much richer prompt-editing experience than a CLI can
// reasonably provide.
package agent

import (
	"context"
	"fmt"
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
		Use:   "agent",
		Short: "Inspect agents",
	}
	cmd.AddCommand(newListCmd(g), newGetCmd(g))
	return cmd
}

func newListCmd(g *apiclient.GlobalContext) *cobra.Command {
	var (
		keyword  string
		status   string
		provider string
		limit    int
		offset   int
	)
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List agents",
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if keyword != "" {
				q.Set("keyword", keyword)
			}
			if status != "" {
				q.Set("status", status)
			}
			if provider != "" {
				q.Set("provider", provider)
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			if offset > 0 {
				q.Set("offset", strconv.Itoa(offset))
			}
			var result domain.AgentListResult
			if err := g.Client().Get(cmd.Context(), "/api/v1/agents", q, &result); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), result)
			return nil
		},
	}
	cmd.Flags().StringVar(&keyword, "keyword", "", "Free-text filter")
	cmd.Flags().StringVar(&status, "status", "", "Status filter (active|paused|...)")
	cmd.Flags().StringVar(&provider, "provider", "", "Filter by provider code")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum rows to return")
	cmd.Flags().IntVar(&offset, "offset", 0, "Pagination offset")
	return cmd
}

func newGetCmd(g *apiclient.GlobalContext) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id-or-key>",
		Short: "Show a single agent by id or agent_key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			a, err := Resolve(cmd.Context(), g, args[0])
			if err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), a)
			return nil
		},
	}
}

// Resolve looks up an agent by either its UUID or its agent_key. It is
// exported so that other packages (the console launcher, the session
// command) can reuse the lookup without copying the heuristic.
func Resolve(ctx context.Context, g *apiclient.GlobalContext, idOrKey string) (domain.Agent, error) {
	var a domain.Agent
	if err := g.Client().Get(ctx, "/api/v1/agents/"+url.PathEscape(idOrKey), nil, &a); err == nil && a.ID != "" {
		return a, nil
	}
	q := url.Values{"keyword": {idOrKey}, "limit": {"5"}}
	var result domain.AgentListResult
	if err := g.Client().Get(ctx, "/api/v1/agents", q, &result); err != nil {
		return domain.Agent{}, err
	}
	for _, item := range result.Items {
		if item.AgentKey == idOrKey || item.ID == idOrKey {
			return item, nil
		}
	}
	return domain.Agent{}, fmt.Errorf("agent %q not found", idOrKey)
}
