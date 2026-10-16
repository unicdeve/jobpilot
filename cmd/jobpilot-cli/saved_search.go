package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
	"github.com/unicdev/jobpilot/internal/core/domain"
)

// resolveSavedSearch finds the saved search a command should use: the one
// named (by name or ID), or the default when nothing was named. It reports
// the absence of any saved search as an actionable message rather than an
// empty result, since that is the state a new installation starts in and
// discovery cannot run in it.
func resolveSavedSearch(ctx context.Context, client *httpapi.Client, nameOrID string) (*domain.SavedSearch, error) {
	searches, err := client.ListSavedSearches(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing saved searches: %w", err)
	}
	if len(searches) == 0 {
		return nil, fmt.Errorf("no saved searches yet — create one with 'search saved add', or run 'onboard' to set up your first")
	}

	if nameOrID == "" {
		for _, search := range searches {
			if search.IsDefault {
				return search, nil
			}
		}
		return nil, fmt.Errorf("no default saved search — pick one with 'search saved default <name>' or pass --saved")
	}

	for _, search := range searches {
		if strings.EqualFold(search.Name, nameOrID) || search.ID == nameOrID {
			return search, nil
		}
	}
	return nil, fmt.Errorf("no saved search named %q — 'search saved list' shows what you have", nameOrID)
}

func newSavedSearchCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "saved", Short: "Manage saved searches"}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List saved searches",
		RunE: func(_ *cobra.Command, _ []string) error {
			searches, err := client.ListSavedSearches(context.Background())
			if err != nil {
				return fmt.Errorf("listing saved searches: %w", err)
			}
			if len(searches) == 0 {
				fmt.Println("No saved searches yet. Add one with 'search saved add <name> --keywords ...'.")
				return nil
			}

			for _, search := range searches {
				marker := " "
				if search.IsDefault {
					marker = "*"
				}
				fmt.Printf("%s %s\n    %s\n    ID: %s\n", marker, search.Name, describeCriteria(search.Criteria), search.ID)
			}
			fmt.Println("\n* = default, used by 'search run' when no search is named.")
			return nil
		},
	})

	addCmd := &cobra.Command{
		Use:   "add [name]",
		Short: "Save a new named search",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			criteria, err := criteriaFromFlags(cmd)
			if err != nil {
				return err
			}

			search, err := client.CreateSavedSearch(context.Background(), args[0], criteria)
			if err != nil {
				return fmt.Errorf("saving search: %w", err)
			}

			fmt.Printf("✅ Saved search %q: %s\n", search.Name, describeCriteria(search.Criteria))
			if search.IsDefault {
				fmt.Println("   It is your default, so 'search run' will use it.")
			}
			return nil
		},
	}
	criteriaFlags(addCmd)
	cmd.AddCommand(addCmd)

	editCmd := &cobra.Command{
		Use:   "edit [name-or-id]",
		Short: "Replace a saved search's criteria",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			existing, err := resolveSavedSearch(ctx, client, args[0])
			if err != nil {
				return err
			}

			criteria, err := criteriaFromFlags(cmd)
			if err != nil {
				return err
			}
			name, err := cmd.Flags().GetString("name")
			if err != nil {
				return err
			}
			if name == "" {
				name = existing.Name
			}

			updated, err := client.UpdateSavedSearch(ctx, existing.ID, name, criteria)
			if err != nil {
				return fmt.Errorf("updating saved search: %w", err)
			}
			fmt.Printf("✅ Updated %q: %s\n", updated.Name, describeCriteria(updated.Criteria))
			return nil
		},
	}
	criteriaFlags(editCmd)
	editCmd.Flags().String("name", "", "Rename the saved search")
	cmd.AddCommand(editCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "delete [name-or-id]",
		Short: "Delete a saved search",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			search, err := resolveSavedSearch(ctx, client, args[0])
			if err != nil {
				return err
			}
			if err := client.DeleteSavedSearch(ctx, search.ID); err != nil {
				return fmt.Errorf("deleting saved search: %w", err)
			}
			fmt.Printf("✅ Deleted saved search %q.\n", search.Name)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "default [name-or-id]",
		Short: "Choose the search 'search run' uses by default",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			search, err := resolveSavedSearch(ctx, client, args[0])
			if err != nil {
				return err
			}
			updated, err := client.SetDefaultSavedSearch(ctx, search.ID)
			if err != nil {
				return fmt.Errorf("setting default saved search: %w", err)
			}
			fmt.Printf("✅ %q is now your default search.\n", updated.Name)
			return nil
		},
	})

	return cmd
}
