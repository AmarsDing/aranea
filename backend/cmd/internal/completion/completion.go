// Package completion exposes Cobra's built-in shell completion script
// generator under the `aranea completion <shell>` command tree.
package completion

import (
	"os"

	"github.com/spf13/cobra"
)

// NewCommand returns the parent completion command. The four standard
// targets supported by Cobra are wired up explicitly so users can
// discover them via tab-completion of the command itself.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion",
		Short: "Generate shell completion scripts (bash|zsh|fish|powershell)",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "bash",
		Short: "Generate bash completion script",
		RunE: func(c *cobra.Command, _ []string) error {
			return c.Root().GenBashCompletionV2(os.Stdout, true)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "zsh",
		Short: "Generate zsh completion script",
		RunE: func(c *cobra.Command, _ []string) error {
			return c.Root().GenZshCompletion(os.Stdout)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "fish",
		Short: "Generate fish completion script",
		RunE: func(c *cobra.Command, _ []string) error {
			return c.Root().GenFishCompletion(os.Stdout, true)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "powershell",
		Short: "Generate PowerShell completion script",
		RunE: func(c *cobra.Command, _ []string) error {
			return c.Root().GenPowerShellCompletionWithDesc(os.Stdout)
		},
	})
	return cmd
}
