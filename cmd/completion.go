package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newCompletionCmd(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion",
		Short: "Set up shell tab completion",
		Long: `Set up tab completion so that theia commands complete on <Tab>.

Running without a subcommand detects your shell and prompts to install.
To print the script to stdout instead (for manual setup or scripting), pass the shell name:

  theia completion bash
  theia completion zsh
  theia completion fish`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInteractiveCompletion(cmd, root)
		},
	}

	cmd.AddCommand(newCompletionBashCmd(root))
	cmd.AddCommand(newCompletionZshCmd(root))
	cmd.AddCommand(newCompletionFishCmd(root))

	return cmd
}

func detectShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return ""
	}
	return filepath.Base(shell)
}

func completionTargetPath(shell string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch shell {
	case "bash":
		return filepath.Join(home, ".local", "share", "bash-completion", "completions", "theia"), nil
	case "zsh":
		return filepath.Join(home, ".zsh", "completions", "_theia"), nil
	case "fish":
		return filepath.Join(home, ".config", "fish", "completions", "theia.fish"), nil
	default:
		return "", fmt.Errorf("unsupported shell: %s", shell)
	}
}

func writeCompletionScript(root *cobra.Command, shell, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // path derived from completionTargetPath
	if err != nil {
		return err
	}
	var genErr error
	switch shell {
	case "bash":
		genErr = root.GenBashCompletionV2(f, true)
	case "zsh":
		genErr = root.GenZshCompletion(f)
	case "fish":
		genErr = root.GenFishCompletion(f, true)
	default:
		genErr = fmt.Errorf("unsupported shell: %s", shell)
	}
	if closeErr := f.Close(); closeErr != nil && genErr == nil {
		return closeErr
	}
	return genErr
}

// confirmYesNo prints a yes/no prompt to cmd's output and reads one line of
// response from its input. An empty response (bare Enter) resolves to false.
func confirmYesNo(cmd *cobra.Command, prompt string) bool {
	cmd.Printf("%s [y/N] ", prompt)
	line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}

func runInteractiveCompletion(cmd *cobra.Command, root *cobra.Command) error {
	shell := detectShell()
	if shell == "" {
		cmd.PrintErrln("  could not detect shell; run 'theia completion bash|zsh|fish' to print the script manually")
		return nil
	}
	if shell != "bash" && shell != "zsh" && shell != "fish" {
		cmd.PrintErrf("  shell %q not supported; run 'theia completion bash|zsh|fish' to print the script manually\n", shell)
		return nil
	}

	targetPath, err := completionTargetPath(shell)
	if err != nil {
		return err
	}

	cmd.Printf("\n  Detected shell: %s\n\n", shell)

	if !confirmYesNo(cmd, fmt.Sprintf("Install tab completion for %s?", shell)) {
		cmd.Printf("\n  Skipped. Run 'theia completion %s' to print the script manually.\n\n", shell)
		return nil
	}

	if err := writeCompletionScript(root, shell, targetPath); err != nil {
		return fmt.Errorf("writing completion script: %w", err)
	}

	cmd.Printf("\n  installed -> %s\n", targetPath)

	if shell == "zsh" {
		if patched, patchErr := patchZshrc(filepath.Dir(targetPath)); patchErr != nil {
			cmd.Printf("  could not patch ~/.zshrc: %s\n", patchErr.Error())
		} else if patched {
			cmd.Printf("  patched -> ~/.zshrc\n")
		} else {
			cmd.Printf("  ~/.zshrc already has fpath entry — no change\n")
		}
	}
	cmd.Printf("  reload shell: exec $SHELL\n\n")

	return nil
}

func patchZshrc(completionDir string) (patched bool, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	zshrc := filepath.Join(home, ".zshrc")

	existing, err := os.ReadFile(zshrc) //nolint:gosec // path derived from os.UserHomeDir
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}

	fpathLine := fmt.Sprintf("fpath=(%s $fpath)", completionDir)
	if strings.Contains(string(existing), completionDir) {
		return false, nil
	}

	f, err := os.OpenFile(zshrc, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644) //nolint:gosec // user home dir
	if err != nil {
		return false, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	prefix := "\n"
	if len(existing) == 0 {
		prefix = ""
	}
	_, err = fmt.Fprintf(f, "%s# theia tab completion\n%s\nautoload -Uz compinit && compinit\n", prefix, fpathLine)
	if err != nil {
		return false, err
	}
	return true, nil
}

func newCompletionBashCmd(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "bash",
		Short: "Print bash completion script to stdout",
		Long: `Print the bash completion script to stdout.

Install system-wide (requires sudo):
  sudo theia completion bash > /etc/bash_completion.d/theia

Install for current user (no sudo):
  theia completion bash > ~/.local/share/bash-completion/completions/theia`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return root.GenBashCompletionV2(cmd.OutOrStdout(), true)
		},
	}
}

func newCompletionZshCmd(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "zsh",
		Short: "Print zsh completion script to stdout",
		Long: `Print the zsh completion script to stdout.

Install:
  theia completion zsh > "${fpath[1]}/_theia"

Then reload: exec $SHELL`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return root.GenZshCompletion(cmd.OutOrStdout())
		},
	}
}

func newCompletionFishCmd(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "fish",
		Short: "Print fish completion script to stdout",
		Long: `Print the fish completion script to stdout.

Install:
  theia completion fish > ~/.config/fish/completions/theia.fish`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return root.GenFishCompletion(cmd.OutOrStdout(), true)
		},
	}
}
