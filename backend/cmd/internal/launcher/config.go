// Package launcher hosts the Aranea-side glue around ADK's
// launcher.Config. The console / web sub-launchers live in dedicated
// child packages and consume *launcher.Config (the ADK type) for
// compatibility with universal.NewLauncher.
//
// Aranea's design (前端/25 cli.md §1.4) extends the ADK config with the
// fields a remote CLI needs: backend base URL, bearer token and an
// optional in-process embedding flag. Launchers receive the extended
// Config through closures rather than mutating the ADK struct.
package launcher

import (
	adklauncher "google.golang.org/adk/cmd/launcher"

	"arenea/backend/cmd/internal/apiclient"
)

// Config carries everything the Aranea launcher chain needs to run.
// Most of it is the ADK launcher.Config that universal/console expect;
// the Aranea-specific fields are additive.
type Config struct {
	Adk      adklauncher.Config
	Client   *apiclient.GlobalContext
	BaseURL  string
	Token    string
	Embedded bool
}

// ADK returns the embedded ADK config so callers can hand it to
// adklauncher.Launcher.Execute without leaking Aranea types into ADK.
func (c *Config) ADK() *adklauncher.Config { return &c.Adk }
