package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
)

// newUICmd opens JobPilot's interface in the user's web browser, served by
// the daemon this CLI found or started (ADR-0035 §2).
func newUICmd(client *httpapi.Client) *cobra.Command {
	var noOpen bool
	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Open JobPilot in your web browser",
		Long: "Open JobPilot in your web browser, served by the local daemon.\n\n" +
			"The address changes whenever the daemon restarts, so run this again to reopen it.",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			address, err := client.InterfaceURL(context.Background())
			if err != nil {
				return err
			}
			fmt.Println(address)
			if noOpen {
				return nil
			}
			if err := openInBrowser(address); err != nil {
				return fmt.Errorf("could not open your browser, so open the address above yourself: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "print the address without opening a browser")
	return cmd
}

// openInBrowser opens address in the user's default browser. The address
// is the daemon's own, and is passed as an argument, never through a shell.
func openInBrowser(address string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", address)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
	default:
		cmd = exec.Command("xdg-open", address)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// The opener hands the address to the browser and exits; waiting
	// reaps it rather than leaving it behind.
	return cmd.Wait()
}
