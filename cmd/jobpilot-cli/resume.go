package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
	"github.com/unicdev/jobpilot/internal/core/domain"
)

func newResumeCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "resume", Short: "Manage resumes"}

	cmd.AddCommand(&cobra.Command{
		Use:   "import [path]",
		Short: "Import a resume file",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			path := args[0]

			profile, err := client.GetProfile(ctx)
			if err != nil {
				return fmt.Errorf("no candidate profile found, run 'onboard' first: %w", err)
			}

			resume, err := client.ImportResume(ctx, profile.ID, path)
			if err != nil {
				return fmt.Errorf("importing resume: %w", err)
			}
			fmt.Printf("✅ Resume imported: %s (ID: %s)\n", resume.Name, resume.ID)

			suggestProfileFromResume(ctx, client, resume.MarkdownContent, profile)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all imported resumes",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()
			profile, err := client.GetProfile(ctx)
			if err != nil {
				return fmt.Errorf("no candidate profile found: %w", err)
			}

			resumes, err := client.ListResumes(ctx, profile.ID)
			if err != nil {
				return fmt.Errorf("listing resumes: %w", err)
			}

			if len(resumes) == 0 {
				fmt.Println("No resumes imported yet.")
				return nil
			}

			fmt.Println("Imported Resumes:")
			for i, r := range resumes {
				fmt.Printf("%d. %s (ID: %s)\n", i+1, r.Name, r.ID)
			}
			return nil
		},
	})

	cmd.AddCommand(deleteCmd(client, "delete [id]", "Delete a resume; applications that used it keep what was made from it",
		domain.DeleteResume, func(args []string) string { return "resume " + args[0] }))

	return cmd
}
