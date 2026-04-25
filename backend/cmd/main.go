// Aranea CLI entrypoint.
//
// The binary follows the dual-layer command model documented in
// 前端/25 cli.md §1, mirroring the layout of google/adk-go's `adkgo` and
// `adk` binaries:
//
//   * Cobra owns administrative sub-commands (agent / skill / tool / ...)
//     that map 1:1 to existing Aranea REST endpoints under /api/v1/*.
//   * ADK launcher.Launcher / launcher.SubLauncher own runtime modes
//     (`console` REPL, `web`, ...). When the first positional argument is a
//     launcher keyword the Cobra layer is bypassed and the request is
//     handed off to the launcher chain so it can drive the agent runtime
//     using ADK's own session / runner / agent abstractions.
//
// `aranea` with no arguments defaults to the `console` launcher, matching
// the expectation of users who simply want to chat with the system admin
// agent (`__system_admin__`).
package main

import (
	"context"
	"fmt"
	"os"

	"arenea/backend/cmd/aranea/internal/launcher/full"
	cliroot "arenea/backend/cmd/aranea/internal/root"
)

// launcherKeywords are the first-argument tokens that should be routed to
// the ADK launcher chain instead of Cobra. They mirror the keywords
// registered by the SubLauncher implementations under
// internal/launcher/{console,web,...}.
var launcherKeywords = map[string]struct{}{
	"console": {},
	"web":     {},
}

func main() {
	args := os.Args[1:]

	// Route to the ADK launcher chain when the first argument is a known
	// launcher keyword, OR when no arguments were supplied (default to the
	// interactive console).
	if shouldRouteToLauncher(args) {
		if err := runLauncher(args); err != nil {
			fmt.Fprintln(os.Stderr, "aranea:", err)
			os.Exit(1)
		}
		return
	}

	// Otherwise let Cobra handle the request. cobra.Command.Execute writes
	// its own error message and we simply propagate the exit code.
	if err := cliroot.Execute(); err != nil {
		os.Exit(1)
	}
}

func shouldRouteToLauncher(args []string) bool {
	if len(args) == 0 {
		return true
	}
	first := args[0]
	_, ok := launcherKeywords[first]
	return ok
}

func runLauncher(args []string) error {
	ctx := context.Background()
	cfg, err := full.BuildConfig(ctx)
	if err != nil {
		return fmt.Errorf("build launcher config: %w", err)
	}
	if len(args) == 0 {
		args = []string{"console"}
	}
	l := full.NewLauncher(cfg)
	return l.Execute(ctx, cfg.ADK(), args)
}
