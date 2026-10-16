package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
	"github.com/unicdev/jobpilot/internal/core/domain/geo"
)

// newSearchTemplateCmd exposes the ready-made searches. The same catalog
// backs the desktop application, served by the daemon, so the two cannot
// offer different templates under the same name.
func newSearchTemplateCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "templates",
		Short: "Ready-made searches you can adopt and edit",
		Long: "Search criteria are matched strictly: every word you set must appear, and\n" +
			"keywords are matched against the whole job description rather than the title.\n" +
			"A search that reads well can therefore return almost nothing. These templates\n" +
			"are criteria that have been measured against live postings from every source.",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "Show the available templates",
		RunE: func(_ *cobra.Command, _ []string) error {
			groups, err := client.ListSearchTemplates(context.Background())
			if err != nil {
				return fmt.Errorf("listing search templates: %w", err)
			}

			for _, group := range groups {
				fmt.Printf("\n%s\n  %s\n\n", group.Name, group.Summary)
				for _, template := range group.Templates {
					fmt.Printf("  %-32s %s\n", template.ID, template.Name)
					fmt.Printf("  %-32s %s\n", "", describeCriteria(template.Criteria))
					fmt.Printf("  %-32s %s\n\n", "", template.Summary)
				}
			}
			fmt.Println("Adopt some with 'search templates import <id> [<id>...]'.")
			return nil
		},
	})

	importCmd := &cobra.Command{
		Use:   "import [id...]",
		Short: "Create a saved search from each named template",
		Long: "The searches created are ordinary saved searches. Edit, rename or delete them\n" +
			"as you like — nothing links them back to the template they came from.\n" +
			"Each starts with your profile's country as the one you work from, unless --work-from says otherwise.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			workFrom, err := cmd.Flags().GetString("work-from")
			if err != nil {
				return err
			}
			code := ""
			if strings.TrimSpace(workFrom) != "" {
				var ok bool
				if code, ok = geo.ResolveCountry(workFrom); !ok {
					return fmt.Errorf("--work-from %q is not a country JobPilot knows; try its name or its two-letter code", workFrom)
				}
			} else if countries, err := client.ListCountries(ctx); err == nil {
				code = countries.ProfileCountry
			}

			created, err := client.ImportSearchTemplates(ctx, args, code)
			if err != nil {
				return fmt.Errorf("importing search templates: %w", err)
			}

			for _, search := range created {
				fmt.Printf("✅ %s\n    %s\n", search.Name, describeCriteria(search.Criteria))
			}
			fmt.Printf("\nCreated %d saved %s. Run one with 'search run --saved \"%s\"'.\n",
				len(created), plural(len(created), "search", "searches"), created[0].Name)
			return nil
		},
	}
	importCmd.Flags().String("work-from", "", "The country you work from (default: your profile's)")
	cmd.AddCommand(importCmd)

	return cmd
}
