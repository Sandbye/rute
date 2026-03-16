package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "rute",
	Short: "Browse and export your API routes and Zod schemas from the terminal",
	Long: `rute reads your rute.yaml and renders API documentation directly in the terminal.

Run without a subcommand to launch the interactive TUI browser.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: launch TUI (Issue 8)
		fmt.Println("Interactive TUI coming in milestone 2. Try: rute list")
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "path to rute.yaml (default: ./rute.yaml)")
	rootCmd.AddCommand(completionCmd)
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion script",
	Long: `Generate a shell completion script for rute.

Bash:
  source <(rute completion bash)
  # Or persist it:
  rute completion bash > /etc/bash_completion.d/rute

Zsh:
  rute completion zsh > "${fpath[1]}/_rute"
  # Then reload: exec zsh

Fish:
  rute completion fish > ~/.config/fish/completions/rute.fish
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		default:
			return fmt.Errorf("unsupported shell: %s", args[0])
		}
	},
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
