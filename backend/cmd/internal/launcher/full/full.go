// Package full assembles the launcher chain that powers the runtime
// half of the Aranea CLI. It mirrors google.golang.org/adk/cmd/launcher/full
// but only registers the SubLaunchers that make sense for our binary.
// The CLI ships with the console launcher today; web/A2A/etc are the
// natural extension points and can be added without changing main.go.
package full

import (
	"context"

	adklauncher "google.golang.org/adk/cmd/launcher"
	adkuniversal "google.golang.org/adk/cmd/launcher/universal"

	"arenea/backend/cmd/aranea/internal/apiclient"
	araneal "arenea/backend/cmd/aranea/internal/launcher"
	"arenea/backend/cmd/aranea/internal/launcher/console"
	"arenea/backend/cmd/aranea/internal/launcher/web"
)

// BuildConfig produces the Aranea launcher.Config consumed by the
// SubLaunchers. It performs the same configuration resolution as the
// Cobra path (so flags / environment variables / ~/.aranea/config.toml
// keep behaving identically) and then injects the resulting HTTP client
// into the launcher chain.
func BuildConfig(_ context.Context) (*araneal.Config, error) {
	g := apiclient.NewGlobalContext()
	if err := g.Resolve(); err != nil {
		return nil, err
	}
	return &araneal.Config{
		Client:  g,
		BaseURL: g.BaseURL,
		Token:   g.Token,
	}, nil
}

// NewLauncher returns the universal launcher with all Aranea
// SubLaunchers registered. The console launcher is registered first so
// that universal.NewLauncher uses it as the default when no keyword is
// provided on the command line; web is the embedded backend playground
// described in 前端/25 cli.md §1.4.
func NewLauncher(cfg *araneal.Config) adklauncher.Launcher {
	return adkuniversal.NewLauncher(
		console.NewLauncher(cfg),
		web.NewLauncher(cfg),
	)
}
