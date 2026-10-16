package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
)

func newMatchCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "match", Short: "Analyze job fit"}

	cmd.AddCommand(&cobra.Command{
		Use:   "analyze [job-id]",
		Short: "Analyze how well a job matches the candidate",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			jobID := args[0]

			profile, err := client.GetProfile(ctx)
			if err != nil {
				return fmt.Errorf("no candidate profile found, run 'onboard' first: %w", err)
			}

			match, err := client.AnalyzeMatch(ctx, profile.ID, jobID)
			if err != nil {
				return fmt.Errorf("analyzing job: %w", err)
			}

			fmt.Printf("✅ Analysis complete!\nScore: %d/100 (Confidence: %.2f)\n", match.Score, match.Confidence)
			fmt.Printf("Rationale: %s\n", match.Rationale)
			fmt.Println("\nStrengths:")
			for _, s := range match.Strengths {
				fmt.Printf("- %s\n", s)
			}
			fmt.Println("\nGaps:")
			for _, g := range match.Gaps {
				fmt.Printf("- %s\n", g)
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all match analyses",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()
			profile, err := client.GetProfile(ctx)
			if err != nil {
				return fmt.Errorf("no candidate profile found: %w", err)
			}

			matches, err := client.ListMatches(ctx, profile.ID)
			if err != nil {
				return fmt.Errorf("listing matches: %w", err)
			}

			if len(matches) == 0 {
				fmt.Println("No matches analyzed yet.")
				return nil
			}

			fmt.Println("Match Analyses:")
			for i, m := range matches {
				fmt.Printf("%d. Job ID: %s | Score: %d/100 | Confidence: %.2f\n", i+1, m.JobID, m.Score, m.Confidence)
			}
			return nil
		},
	})

	return cmd
}
