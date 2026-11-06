package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
	"github.com/unicdev/jobpilot/internal/core/domain"
)

// newPromptSettingsCmd shows and changes the guidance the user gives the
// prompts they may change: matching, cover letters and written answers
// (ADR-0031 §8).
func newPromptSettingsCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prompts",
		Short: "Show or change the guidance JobPilot's prompts are written under",
		Long: `Lists the prompts you can give guidance to: matching, cover letters and
written answers. Guidance is how JobPilot judges a fit or writes for you.
The rules that keep it to what you have told it, and the shape of its reply,
are not guidance and cannot be changed.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			prompts, err := client.ListPromptGuidance(context.Background())
			if err != nil {
				return fmt.Errorf("loading prompts: %w", err)
			}
			for _, prompt := range prompts {
				fmt.Printf("%s (%s): %s\n", prompt.Title, prompt.Key, guidanceState(prompt))
				fmt.Printf("  %s\n", prompt.UsedFor)
			}
			fmt.Println("\nShow one with 'jobpilot-cli settings prompts show <key>'.")
			return nil
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "show [key]",
		Short: "Show a prompt's guidance, and its default",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			prompt, err := promptGuidance(context.Background(), client, domain.PromptKey(args[0]))
			if err != nil {
				return err
			}
			printPromptGuidance(prompt)
			return nil
		},
	})

	var fromFile string
	editCmd := &cobra.Command{
		Use:   "edit [key]",
		Short: "Replace a prompt's guidance with your own",
		Long: `Opens the prompt's guidance in $VISUAL or $EDITOR, starting from what it is
now, and saves what you leave there. --file reads it from a file instead,
or from standard input when the file is -.

To go back to the default, use 'jobpilot-cli settings prompts reset <key>'.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			prompt, err := promptGuidance(ctx, client, domain.PromptKey(args[0]))
			if err != nil {
				return err
			}

			var text string
			if fromFile != "" {
				text, err = readGuidanceFile(fromFile)
			} else {
				text, err = editInEditor(prompt.Text())
			}
			if err != nil {
				return err
			}
			if strings.TrimSpace(text) == strings.TrimSpace(prompt.Text()) {
				fmt.Println("No change.")
				return nil
			}

			saved, err := client.SavePromptGuidance(ctx, prompt.Key, text)
			if err != nil {
				return fmt.Errorf("saving the guidance: %w", err)
			}
			fmt.Printf("✅ Saved your guidance for %s. Nothing already scored or written changes.\n", strings.ToLower(saved.Title))
			return nil
		},
	}
	editCmd.Flags().StringVar(&fromFile, "file", "", "read the guidance from this file, or - for standard input")
	cmd.AddCommand(editCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "reset [key]",
		Short: "Put a prompt back on its default guidance",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			prompt, err := client.ResetPromptGuidance(context.Background(), domain.PromptKey(args[0]))
			if err != nil {
				return fmt.Errorf("resetting the guidance: %w", err)
			}
			fmt.Printf("✅ Back to the default for %s.\n", strings.ToLower(prompt.Title))
			return nil
		},
	})

	return cmd
}

// promptGuidance returns the prompt with the given key, or an error that
// lists the keys there are.
func promptGuidance(ctx context.Context, client *httpapi.Client, key domain.PromptKey) (domain.PromptGuidance, error) {
	prompts, err := client.ListPromptGuidance(ctx)
	if err != nil {
		return domain.PromptGuidance{}, fmt.Errorf("loading prompts: %w", err)
	}
	keys := make([]string, 0, len(prompts))
	for _, prompt := range prompts {
		if prompt.Key == key {
			return prompt, nil
		}
		keys = append(keys, string(prompt.Key))
	}
	return domain.PromptGuidance{}, fmt.Errorf("no prompt is called %q; the prompts are %s", key, strings.Join(keys, ", "))
}

func guidanceState(prompt domain.PromptGuidance) string {
	switch {
	case prompt.IsDefault():
		return "the default"
	case prompt.DefaultChanged:
		return "your own; the default has changed since you edited it"
	default:
		return "your own"
	}
}

func printPromptGuidance(prompt domain.PromptGuidance) {
	fmt.Printf("%s (%s)\n%s\n", prompt.Title, prompt.Key, prompt.UsedFor)
	fmt.Printf("\nGuidance, %s:\n%s\n", guidanceState(prompt), indent(prompt.Text()))
	if !prompt.IsDefault() {
		fmt.Printf("\nThe default:\n%s\n", indent(prompt.Default))
	}
}

func indent(text string) string {
	return "  " + strings.ReplaceAll(text, "\n", "\n  ")
}

func readGuidanceFile(path string) (string, error) {
	var content []byte
	var err error
	if path == "-" {
		content, err = io.ReadAll(os.Stdin)
	} else {
		content, err = os.ReadFile(path)
	}
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	return string(content), nil
}

// editInEditor opens text in the user's editor and returns what they left
// there.
func editInEditor(text string) (string, error) {
	file, err := os.CreateTemp("", "jobpilot-guidance-*.txt")
	if err != nil {
		return "", fmt.Errorf("creating a file to edit: %w", err)
	}
	path := file.Name()
	defer func() { _ = os.Remove(path) }()
	if _, err := file.WriteString(text + "\n"); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("writing the guidance to edit: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("writing the guidance to edit: %w", err)
	}

	// An editor setting can carry arguments, such as "code --wait".
	editor := strings.Fields(preferredEditor())
	command := exec.Command(editor[0], append(editor[1:], path)...) //nolint:gosec // the user's own editor setting
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("running %s: %w", editor[0], err)
	}

	edited, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading the edited guidance: %w", err)
	}
	return string(edited), nil
}

func preferredEditor() string {
	for _, name := range []string{"VISUAL", "EDITOR"} {
		if editor := strings.TrimSpace(os.Getenv(name)); editor != "" {
			return editor
		}
	}
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	return "vi"
}
