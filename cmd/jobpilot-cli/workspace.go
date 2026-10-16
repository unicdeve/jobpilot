package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
)

func newWorkspaceCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "profiles", Short: "Manage isolated local profiles"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List local profiles",
		RunE: func(_ *cobra.Command, _ []string) error {
			workspaces, activeID, err := client.ListWorkspaces(context.Background())
			if err != nil {
				return fmt.Errorf("listing profiles: %w", err)
			}
			for _, workspace := range workspaces {
				marker := " "
				if workspace.ID == activeID {
					marker = "*"
				}
				fmt.Printf("%s %s\t%s\n", marker, workspace.ID, workspace.Name)
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "add <name>",
		Short: "Add an empty local profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			workspace, err := client.CreateWorkspace(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("adding profile: %w", err)
			}
			fmt.Printf("Created %s (%s).\n", workspace.Name, workspace.ID)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "use <id>",
		Short: "Make a profile active for clients without --workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			workspace, err := client.ActivateWorkspace(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("activating profile: %w", err)
			}
			fmt.Printf("Using %s.\n", workspace.Name)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "rename <id> <name>",
		Short: "Rename a local profile",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			workspace, err := client.RenameWorkspace(context.Background(), args[0], args[1])
			if err != nil {
				return fmt.Errorf("renaming profile: %w", err)
			}
			fmt.Printf("Renamed profile to %s.\n", workspace.Name)
			return nil
		},
	})
	return cmd
}
