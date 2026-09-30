package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
	"github.com/unicdev/jobpilot/internal/core/domain"
)

// newCompanyCmd is "company": the companies whose hiring pages JobPilot
// reads (docs/02_features/17_company_watchlists.md).
func newCompanyCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "company",
		Short: "Watch companies' own hiring pages on Greenhouse, Lever and Ashby",
		Long: "A watched company's openings are found by your saved searches, like any job source's.\n" +
			"Watching one does not import its jobs: a search that matches them finds them.",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "watch [address]",
		Short: "Watch a company, from the address of its hiring page or any posting on it",
		Long: "Supported: boards.greenhouse.io/…, job-boards.greenhouse.io/…, jobs.lever.co/… and jobs.ashbyhq.com/…,\n" +
			"and their EU forms. 'company suggestions' lists companies to start from, with their addresses.",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			company, created, err := client.WatchCompany(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("watching %s: %w", args[0], err)
			}
			if !created {
				fmt.Printf("You already watch %s (%s).\n", company.Name, company.URL)
				return nil
			}
			fmt.Printf("✅ Watching %s on %s: %s.\n", company.Name, company.Board.Provider.ProviderName(), openRoles(company.OpenRoles))
			fmt.Printf("   ID: %s. Rename it with 'company rename %s <name>'.\n", company.ID, company.ID)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List the companies you watch, and how each was last checked",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			companies, err := client.ListCompanies(context.Background())
			if err != nil {
				return fmt.Errorf("listing watched companies: %w", err)
			}
			if len(companies) == 0 {
				fmt.Println("You watch no companies yet. Add one with 'company watch <address>', or see 'company suggestions'.")
				return nil
			}
			for _, company := range companies {
				fmt.Printf("%s — %s\n    %s\n    ID: %s\n", company.Name, company.Board.Provider.ProviderName(), describeCheck(company), company.ID)
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "rename [id] [name]",
		Short: "Change what a watched company is called",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			company, err := client.RenameCompany(context.Background(), args[0], args[1])
			if err != nil {
				return fmt.Errorf("renaming: %w", err)
			}
			fmt.Printf("✅ Renamed to %s.\n", company.Name)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "unwatch [id]",
		Short: "Stop watching a company; the jobs already found from it stay",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := client.UnwatchCompany(context.Background(), args[0]); err != nil {
				return fmt.Errorf("unwatching: %w", err)
			}
			fmt.Println("✅ No longer watching it. Jobs already found from it are kept.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "suggestions",
		Short: "List companies to start from, with what was measured about their open roles",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			suggestions, err := client.CompanySuggestions(context.Background())
			if err != nil {
				return fmt.Errorf("listing suggestions: %w", err)
			}
			if len(suggestions) == 0 {
				fmt.Println("No suggestions left: you watch every company on the starter list.")
				return nil
			}
			for _, suggestion := range suggestions {
				fmt.Printf("%s — %s\n    %s\n    company watch %s\n", suggestion.Name,
					suggestion.Board.Provider.ProviderName(), describeMeasurement(suggestion), suggestion.URL)
			}
			fmt.Println("\nThese are what their boards held when measured, not a claim about the companies.")
			return nil
		},
	})

	return cmd
}

// openRoles is a count of open roles as a sentence says it.
func openRoles(count int) string {
	if count == 1 {
		return "1 open role"
	}
	return fmt.Sprintf("%d open roles", count)
}

// describeCheck says how a company's page was last read.
func describeCheck(company *domain.WatchedCompany) string {
	if company.CheckedAt == nil {
		return "Not checked yet."
	}
	when := company.CheckedAt.Local().Format("2006-01-02 15:04")
	switch company.Check {
	case domain.BoardCheckOK:
		return fmt.Sprintf("%s when checked %s.", capitalize(openRoles(company.OpenRoles)), when)
	case domain.BoardCheckGone:
		return fmt.Sprintf("Its page was gone when checked %s: the company may have moved provider. Its address: %s", when, company.URL)
	case domain.BoardCheckUnreachable:
		return fmt.Sprintf("Couldn't be reached when checked %s.", when)
	}
	return fmt.Sprintf("Answered with an error when checked %s.", when)
}

// describeMeasurement is what earned a suggestion its place, as measured
// (docs/02_features/17_company_watchlists.md D2).
func describeMeasurement(suggestion domain.StarterCompany) string {
	return fmt.Sprintf("%d of %d open roles could be done from anywhere or from more than one continent, measured %s.",
		suggestion.Qualifying, suggestion.OpenRoles, suggestion.MeasuredOn)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
