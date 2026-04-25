// Package cron implements `aranea cron ls/runs`. Cron tasks are stored
// under /api/v1/cron-tasks (platform resource) and runs are exposed via
// /api/v1/cron-task-runs.
package cron

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
		Use:   "cron",
		Short: "Inspect cron tasks and recent runs",
	}
	cmd.AddCommand(newListCmd(g), newRunsCmd(g))
	return cmd
}

func newListCmd(g *apiclient.GlobalContext) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List cron tasks",
		RunE: func(cmd *cobra.Command, _ []string) error {
			var resp struct {
				Items []domain.PlatformResource `json:"items"`
			}
			if err := g.Client().Get(cmd.Context(), "/api/v1/cron-tasks", nil, &resp); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), resp)
			return nil
		},
	}
}

func newRunsCmd(g *apiclient.GlobalContext) *cobra.Command {
	var (
		taskID string
		status string
		limit  int
	)
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "List recent cron runs",
		RunE: func(cmd *cobra.Command, _ []string) error {
			q := url.Values{}
			if taskID != "" {
				q.Set("cron_task_id", taskID)
			}
			if status != "" {
				q.Set("status", status)
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}
			var resp struct {
				Items []domain.CronTaskRun `json:"items"`
			}
			if err := g.Client().Get(cmd.Context(), "/api/v1/cron-task-runs", q, &resp); err != nil {
				return err
			}
			output.Render(cmd.OutOrStdout(), resp)
			return nil
		},
	}
	cmd.Flags().StringVar(&taskID, "task", "", "Filter by cron task id")
	cmd.Flags().StringVar(&status, "status", "", "Filter by status")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum rows")
	return cmd
}
