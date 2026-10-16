package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
)

// newCredentialCmd manages API keys for job sources.
//
// There is no "show" or "get": a credential goes in and never comes out
// (ADR-0019 §4). The daemon has no authentication, so a route that returned
// a secret would make it readable by any local process.
func newCredentialCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "credentials",
		Short: "API keys for job sources that need one",
		Long: "Keys are kept in your operating system's keychain, never in JobPilot's\n" +
			"database. Once stored, a key cannot be read back — not by the desktop app,\n" +
			"not by this CLI, and not by anything else on your machine talking to the\n" +
			"daemon. Replace one by setting it again.",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "Show which credentials are configured, without their values",
		RunE: func(_ *cobra.Command, _ []string) error {
			refs, err := client.ListCredentials(context.Background())
			if err != nil {
				return fmt.Errorf("listing credentials: %w", err)
			}
			if len(refs) == 0 {
				fmt.Println("No credentials configured. Add one with 'credentials set <source>'.")
				return nil
			}

			for _, ref := range refs {
				fmt.Printf("  %s\n", ref.String())
			}
			fmt.Println("\nValues are never shown. Replace one by setting it again.")
			return nil
		},
	})

	setCmd := &cobra.Command{
		Use:   "set [source]",
		Short: "Store an API key for a job source",
		Long: "The key is read from a prompt that does not echo it, or from standard\n" +
			"input when that is a pipe. It is never taken as a flag, because a flag\n" +
			"would put it in your shell history and in the process list.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			label, err := cmd.Flags().GetString("label")
			if err != nil {
				return err
			}

			secret, err := readSecret("API key")
			if err != nil {
				return err
			}

			ref, err := client.SetCredential(context.Background(), args[0], label, secret)
			if err != nil {
				return fmt.Errorf("storing credential: %w", err)
			}
			fmt.Printf("✅ Stored %s in your keychain. It cannot be shown again.\n", ref.String())
			return nil
		},
	}
	setCmd.Flags().String("label", "", "Name this key, to hold more than one for a source (default: \"default\")")
	cmd.AddCommand(setCmd)

	deleteCmd := &cobra.Command{
		Use:   "delete [source]",
		Short: "Remove a stored API key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			label, err := cmd.Flags().GetString("label")
			if err != nil {
				return err
			}
			if err := client.DeleteCredential(context.Background(), args[0], label); err != nil {
				return fmt.Errorf("deleting credential: %w", err)
			}
			fmt.Println("✅ Removed.")
			return nil
		},
	}
	deleteCmd.Flags().String("label", "", "Which key to remove (default: \"default\")")
	cmd.AddCommand(deleteCmd)

	return cmd
}

// readSecret reads a secret without echoing it, or from standard input when
// that is a pipe, so a key can be stored from a script without appearing on
// a command line.
func readSecret(label string) (string, error) {
	info, err := os.Stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("reading standard input: %w", err)
	}

	if info.Mode()&os.ModeCharDevice == 0 {
		piped, readErr := io.ReadAll(os.Stdin)
		if readErr != nil {
			return "", fmt.Errorf("reading the key from standard input: %w", readErr)
		}
		secret := strings.TrimRight(string(piped), "\r\n")
		if secret == "" {
			return "", fmt.Errorf("nothing was piped in")
		}
		return secret, nil
	}

	prompt := promptui.Prompt{Label: label, Mask: '*'}
	secret, err := prompt.Run()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(secret) == "" {
		return "", fmt.Errorf("an empty key cannot be stored")
	}
	return secret, nil
}
