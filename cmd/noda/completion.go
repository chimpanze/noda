package main

import (
	"os"

	"github.com/spf13/cobra"
)

func newCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish]",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for bash, zsh, or fish.

To load completions:

Bash:
  $ source <(noda completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ noda completion bash > /etc/bash_completion.d/noda
  # macOS:
  $ noda completion bash > $(brew --prefix)/etc/bash_completion.d/noda

Zsh:
  $ source <(noda completion zsh)

  # To load completions for each session, execute once:
  $ noda completion zsh > "${fpath[1]}/_noda"

Fish:
  $ noda completion fish | source

  # To load completions for each session, execute once:
  $ noda completion fish > ~/.config/fish/completions/noda.fish
`,
		ValidArgs:             []string{"bash", "zsh", "fish"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(os.Stdout)
			case "zsh":
				return cmd.Root().GenZshCompletion(os.Stdout)
			case "fish":
				return cmd.Root().GenFishCompletion(os.Stdout, true)
			}
			return nil
		},
	}
	return cmd
}
