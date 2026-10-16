// Command jobpilot-cli is the JobPilot command-line interface: onboarding,
// profile/resume management, job discovery, and match analysis. It is a
// thin client of the jobpilot daemon (see cmd/jobpilot), which owns the
// database and every core service; this binary talks to it over the
// loopback HTTP API in internal/adapters/primary/httpapi, starting it if
// it isn't already running.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/llm"
	"github.com/unicdev/jobpilot/internal/version"
)

func main() {
	// rootCmd.Execute() is the single place errors from any command are
	// printed and turned into a process exit code (below); silence Cobra's
	// own error/usage printing so each error is reported exactly once.
	var rootCmd = &cobra.Command{Use: "jobpilot-cli", Version: version.Version, SilenceUsage: true, SilenceErrors: true}
	rootCmd.SetVersionTemplate("jobpilot-cli {{.Version}}\n")
	var workspaceID string
	rootCmd.PersistentFlags().StringVar(&workspaceID, "workspace", "", "use this workspace ID for this command")

	// Answered before the daemon is reached, which it does not need.
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("jobpilot-cli " + version.Version)
		return
	}

	ctx := context.Background()

	client, err := ensureDaemon(ctx)
	if err != nil {
		log.Fatalf("Error connecting to jobpilot daemon: %v", err)
	}
	workspaceID = workspaceIDFromArgs(os.Args[1:])
	if workspaceID != "" {
		client = client.WithWorkspace(workspaceID)
	}

	llmSettings, err := client.GetLLMSettings(ctx)
	if err != nil {
		log.Fatalf("Error loading LLM settings: %v", err)
	}
	resolvedLLMSettings := llm.ResolveSettings(*llmSettings)

	rootCmd.AddCommand(newOnboardCmd(client, resolvedLLMSettings))
	rootCmd.AddCommand(newProfileCmd(client))
	rootCmd.AddCommand(newResumeCmd(client))
	rootCmd.AddCommand(newSearchCmd(client))
	rootCmd.AddCommand(newMatchCmd(client))
	rootCmd.AddCommand(newSettingsCmd(client))
	rootCmd.AddCommand(newApplicationCmd(client))
	rootCmd.AddCommand(newScoutCmd(client))
	rootCmd.AddCommand(newCredentialCmd(client))
	rootCmd.AddCommand(newActivityCmd(client))
	rootCmd.AddCommand(newJobCmd(client))
	rootCmd.AddCommand(newDataCmd(client))
	rootCmd.AddCommand(newCompanyCmd(client))
	rootCmd.AddCommand(newNotificationsCmd(client))
	rootCmd.AddCommand(newWorkspaceCmd(client))

	// This is the only place a command error is printed and turned into a
	// non-zero exit code, via RunE (rather than log.Fatalf/os.Exit inside a
	// command's handler).
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func workspaceIDFromArgs(args []string) string {
	for i, arg := range args {
		if value, ok := strings.CutPrefix(arg, "--workspace="); ok {
			return value
		}
		if arg == "--workspace" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// plural picks the right word for n, so a count of one does not read as
// "1 searches".
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// joinNames lists names as a sentence does: "A", "A and B", "A, B and C".
func joinNames(names []string) string {
	if len(names) <= 1 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
