// Package root 组装 Aranea CLI 管理侧半边的 Cobra 命令树（见 前端/25 cli.md §1.3）。
package root

import (
	"github.com/spf13/cobra"

	"arenea/backend/cmd/internal/agent"
	"arenea/backend/cmd/internal/apiclient"
	"arenea/backend/cmd/internal/channel"
	clicompletion "arenea/backend/cmd/internal/completion"
	cliconfig "arenea/backend/cmd/internal/config"
	"arenea/backend/cmd/internal/cron"
	"arenea/backend/cmd/internal/login"
	"arenea/backend/cmd/internal/mcp"
	"arenea/backend/cmd/internal/monitor"
	"arenea/backend/cmd/internal/output"
	"arenea/backend/cmd/internal/plugin"
	"arenea/backend/cmd/internal/session"
	"arenea/backend/cmd/internal/skill"
	"arenea/backend/cmd/internal/system"
	"arenea/backend/cmd/internal/tool"
	"arenea/backend/cmd/internal/version"
)

// Execute 是 main.go 使用的公开入口：构建根命令并派发到用户请求的子命令。
func Execute() error {
	return New().Execute()
}

// New 构造带齐全部子命令的根 *cobra.Command，并将全局标志接入共享 CLI 上下文。
func New() *cobra.Command {
	gctx := apiclient.NewGlobalContext()

	root := &cobra.Command{
		Use:   "aranea",
		Short: "Aranea control plane CLI",
		Long: `Aranea CLI lets you operate the agent platform from the terminal.

It speaks to the Aranea backend exclusively through the public REST API
under /api/v1/*, so every administrative action goes through the same
authentication, authorization and audit pipeline as the web UI. Run
"aranea" with no arguments to drop into the interactive console where
the system administrator agent can perform tasks for you in natural
language.`,
		SilenceUsage:  true,
		SilenceErrors: false,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := gctx.Resolve(); err != nil {
				return err
			}
			output.Configure(gctx.Output, gctx.Quiet, gctx.NoColor)
			return nil
		},
	}

	// 各子命令共享的全局标志，与 前端/25 cli.md §2 中表格一致。
	pf := root.PersistentFlags()
	pf.StringVar(&gctx.BaseURL, "base-url", "", "Aranea backend base URL (overrides ARANEA_BASE_URL and config)")
	pf.StringVar(&gctx.Token, "token", "", "Bearer token for authenticated remote backends")
	pf.StringVarP(&gctx.Output, "output", "o", "", "Output format: text|json|table (default text)")
	pf.BoolVarP(&gctx.Quiet, "quiet", "q", false, "Suppress decorative output, only print primary value")
	pf.BoolVarP(&gctx.Yes, "yes", "y", false, "Assume yes for confirmation prompts")
	pf.StringVar(&gctx.Profile, "profile", "", "Configuration profile to load from ~/.aranea/config.toml")
	pf.BoolVar(&gctx.NoColor, "no-color", false, "Disable ANSI colors regardless of TTY detection")
	pf.DurationVar(&gctx.Timeout, "timeout", 0, "HTTP timeout for a single API request (e.g. 30s)")

	root.AddCommand(version.NewCommand(gctx))
	root.AddCommand(cliconfig.NewCommand(gctx))
	root.AddCommand(login.NewCommand(gctx))
	root.AddCommand(agent.NewCommand(gctx))
	root.AddCommand(skill.NewCommand(gctx))
	root.AddCommand(tool.NewCommand(gctx))
	root.AddCommand(plugin.NewCommand(gctx))
	root.AddCommand(mcp.NewCommand(gctx))
	root.AddCommand(cron.NewCommand(gctx))
	root.AddCommand(channel.NewCommand(gctx))
	root.AddCommand(monitor.NewCommand(gctx))
	root.AddCommand(session.NewCommand(gctx))
	root.AddCommand(system.NewCommand(gctx))
	root.AddCommand(clicompletion.NewCommand())
	return root
}
