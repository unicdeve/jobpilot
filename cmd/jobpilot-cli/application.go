package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
	"github.com/unicdev/jobpilot/internal/core/domain"
)

func newApplicationCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "application", Short: "Manage job applications"}

	cmd.AddCommand(newApplicationCreateCmd(client))
	cmd.AddCommand(newApplicationListCmd(client))
	cmd.AddCommand(newApplicationShowCmd(client))
	cmd.AddCommand(newApplicationTransitionCmd(client))
	cmd.AddCommand(newApplicationSubmittedCmd(client))
	cmd.AddCommand(newCoverLetterCmd(client))
	cmd.AddCommand(newQuestionCmd(client))
	cmd.AddCommand(newApplicationPrepareInBrowserCmd(client))
	cmd.AddCommand(newApplicationAgentRunsCmd(client))
	cmd.AddCommand(deleteCmd(client, "delete [application-id]", "Delete an application, with its answers, cover letters and history",
		domain.DeleteApplication, func(args []string) string { return "application " + args[0] }))

	return cmd
}

func newApplicationCreateCmd(client *httpapi.Client) *cobra.Command {
	var resumeID, mode string

	cmd := &cobra.Command{
		Use:   "create [job-id]",
		Short: "Start tracking an application to a job",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			jobID := args[0]

			profile, err := client.GetProfile(ctx)
			if err != nil {
				return fmt.Errorf("no candidate profile found, run 'onboard' first: %w", err)
			}

			app, err := client.CreateApplication(ctx, jobID, profile.ID, resumeID, domain.ApplicationMode(mode))
			if err != nil {
				return fmt.Errorf("creating application: %w", err)
			}
			fmt.Printf("✅ Application created: %s (state: %s)\n", app.ID, app.State)
			return nil
		},
	}
	cmd.Flags().StringVar(&resumeID, "resume", "", "resume ID to associate with this application")
	cmd.Flags().StringVar(&mode, "mode", string(domain.ApplicationModeReview), "application mode (assist|review)")
	return cmd
}

func newApplicationListCmd(client *httpapi.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all applications",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()
			profile, err := client.GetProfile(ctx)
			if err != nil {
				return fmt.Errorf("no candidate profile found: %w", err)
			}

			apps, err := client.ListApplications(ctx, profile.ID)
			if err != nil {
				return fmt.Errorf("listing applications: %w", err)
			}

			if len(apps) == 0 {
				fmt.Println("No applications yet.")
				return nil
			}

			fmt.Println("Applications:")
			for i, a := range apps {
				fmt.Printf("%d. %s | job %s | state %s | mode %s\n", i+1, a.ID, a.JobID, a.State, a.Mode)
			}
			return nil
		},
	}
}

func newApplicationShowCmd(client *httpapi.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "show [application-id]",
		Short: "Show an application's state, history, cover letter, and questions",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			id := args[0]

			app, err := client.GetApplication(ctx, id)
			if err != nil {
				return fmt.Errorf("fetching application: %w", err)
			}
			fmt.Printf("Application %s\nJob: %s\nState: %s\nMode: %s\n", app.ID, app.JobID, app.State, app.Mode)
			if app.SubmittedAt != nil {
				fmt.Printf("Submitted: %s\n", app.SubmittedAt.Format(time.RFC1123))
			}

			// Which moves are possible is the daemon's answer, not one this
			// command works out for itself: a second copy of the rule table
			// here could disagree with the one that enforces it.
			next, err := client.ListAllowedTransitions(ctx, id)
			if err != nil {
				return fmt.Errorf("fetching allowed transitions: %w", err)
			}
			if len(next) > 0 {
				labels := make([]string, 0, len(next))
				for _, state := range next {
					labels = append(labels, string(state))
				}
				fmt.Printf("Can move to: %s\n", strings.Join(labels, ", "))
			} else {
				fmt.Println("Can move to: nothing — this application is closed.")
			}

			history, err := client.ListApplicationHistory(ctx, id)
			if err != nil {
				return fmt.Errorf("fetching history: %w", err)
			}
			if len(history) > 0 {
				fmt.Println("\nHistory:")
				for _, h := range history {
					fmt.Printf("- %s -> %s (%s)\n", h.FromState, h.ToState, h.Reason)
				}
			}

			versions, err := client.ListCoverLetterVersions(ctx, id)
			if err != nil {
				return fmt.Errorf("fetching cover letters: %w", err)
			}
			if len(versions) > 0 {
				fmt.Printf("\nCover letters (%d version(s)):\n", len(versions))
				for _, v := range versions {
					current := ""
					if v.ID == app.CurrentCoverLetterID {
						current = " (current)"
					}
					fmt.Printf("- %s%s\n", v.ID, current)
				}
			}

			questions, err := client.ListQuestions(ctx, id)
			if err != nil {
				return fmt.Errorf("fetching questions: %w", err)
			}
			if len(questions) > 0 {
				fmt.Println("\nQuestions:")
				for _, q := range questions {
					fmt.Printf("- [%s] %s -> %q (category: %s)\n", q.ID, q.Label, q.Answer, q.Category)
				}
			}

			return nil
		},
	}
}

func newApplicationTransitionCmd(client *httpapi.Client) *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:   "transition [application-id] [to-state]",
		Short: "Move an application to a new state",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			id := args[0]
			toState := domain.ApplicationState(strings.ToUpper(args[1]))

			app, err := client.TransitionApplication(ctx, id, toState, reason)
			if err != nil {
				return fmt.Errorf("transitioning application: %w", err)
			}
			fmt.Printf("✅ Application %s is now %s\n", app.ID, app.State)
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "reason for this transition")
	return cmd
}

// newApplicationSubmittedCmd exists because "I sent it" is the one
// transition a user makes away from the machine, after submitting the form
// themselves in the browser JobPilot handed them. `transition <id>
// SUBMITTED` already does it; this is the same act under the name the user
// would look for.
func newApplicationSubmittedCmd(client *httpapi.Client) *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:   "submitted [application-id]",
		Short: "Record that you submitted this application yourself",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			app, err := client.TransitionApplication(ctx, args[0], domain.ApplicationStateSubmitted, reason)
			if err != nil {
				return fmt.Errorf("recording submission: %w", err)
			}
			fmt.Printf("✅ Application %s recorded as submitted.\n", app.ID)
			fmt.Println("   Tell JobPilot what happens next with 'application transition' — interviewing, offered, rejected, or no-response.")
			return nil
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "note to record with the submission")
	return cmd
}

func newApplicationPrepareInBrowserCmd(client *httpapi.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "prepare-in-browser [application-id]",
		Short: "Open the job posting in a real browser, extract its form, and fill in known answers",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			app, err := client.PrepareInBrowser(ctx, args[0])
			if err != nil {
				fmt.Printf("⚠️  %v\nRun 'application agent-runs %s' to see what happened, or 'application show %s' for details.\n", err, args[0], args[0])
				return nil
			}
			fmt.Printf("✅ Application %s is now %s. The browser window was left open for you to review and submit.\n", app.ID, app.State)
			return nil
		},
	}
}

func newApplicationAgentRunsCmd(client *httpapi.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "agent-runs [application-id]",
		Short: "Show the browser automation activity recorded for an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			runs, err := client.ListAgentRuns(ctx, args[0])
			if err != nil {
				return fmt.Errorf("listing agent runs: %w", err)
			}
			if len(runs) == 0 {
				fmt.Println("No agent runs yet.")
				return nil
			}
			for _, r := range runs {
				fmt.Printf("Run %s | %s | status: %s", r.Run.ID, r.Run.TaskType, r.Run.Status)
				if r.Run.Error != "" {
					fmt.Printf(" | %s", r.Run.Error)
				}
				fmt.Println()
				for _, e := range r.Events {
					fmt.Printf("  - %s (%s): %s\n", e.Action, e.Status, e.OutputSummary)
				}
			}
			return nil
		},
	}
}

func newCoverLetterCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "cover-letter", Short: "Manage an application's cover letter"}

	cmd.AddCommand(deleteCmd(client, "delete [cover-letter-id]", "Delete a cover letter version other than the current one",
		domain.DeleteCoverLetter, func(args []string) string { return "cover letter " + args[0] }))

	cmd.AddCommand(&cobra.Command{
		Use:   "generate [application-id]",
		Short: "Generate (or regenerate) a cover letter for an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			coverLetter, err := client.GenerateCoverLetter(ctx, args[0])
			if err != nil {
				return fmt.Errorf("generating cover letter: %w", err)
			}
			fmt.Printf("✅ Cover letter generated (ID: %s):\n\n%s\n", coverLetter.ID, coverLetter.Content)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list [application-id]",
		Short: "List every cover letter version generated for an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			versions, err := client.ListCoverLetterVersions(ctx, args[0])
			if err != nil {
				return fmt.Errorf("listing cover letter versions: %w", err)
			}
			if len(versions) == 0 {
				fmt.Println("No cover letters generated yet.")
				return nil
			}
			for i, v := range versions {
				edited := ""
				if v.UserEdited {
					edited = " (edited)"
				}
				fmt.Printf("%d. %s%s\n%s\n\n", i+1, v.ID, edited, v.Content)
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "edit [cover-letter-id] [content]",
		Short: "Replace a cover letter version's content",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			coverLetter, err := client.EditCoverLetter(ctx, args[0], args[1])
			if err != nil {
				return fmt.Errorf("editing cover letter: %w", err)
			}
			fmt.Printf("✅ Cover letter %s updated.\n", coverLetter.ID)
			return nil
		},
	})

	return cmd
}

func newQuestionCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "question", Short: "Manage an application's questions and answers"}

	var questionType string
	var required bool
	addCmd := &cobra.Command{
		Use:   "add [application-id] [label]",
		Short: "Add a question to an application",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			question, err := client.AddQuestion(ctx, args[0], args[1], domain.QuestionType(questionType), required)
			if err != nil {
				return fmt.Errorf("adding question: %w", err)
			}
			fmt.Printf("✅ Question added: %s\n", question.ID)
			return nil
		},
	}
	addCmd.Flags().StringVar(&questionType, "type", string(domain.QuestionTypeFactual), "question type (factual|narrative)")
	addCmd.Flags().BoolVar(&required, "required", false, "whether this question is required")
	cmd.AddCommand(addCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "resolve [question-id]",
		Short: "Attempt to resolve a question's answer automatically",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			question, err := client.ResolveQuestion(ctx, args[0])
			if err != nil {
				return fmt.Errorf("resolving question: %w", err)
			}
			if question.Category == domain.AnswerCategoryNeedsUserInput {
				fmt.Printf("⚠️  Could not resolve automatically: %s\nRun 'application question answer %s <answer>' to supply one.\n", question.Label, question.ID)
				return nil
			}
			fmt.Printf("✅ Resolved (%s): %s\n", question.Category, question.Answer)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "answer [question-id] [answer]",
		Short: "Manually set (or override) a question's answer",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			question, err := client.SetQuestionAnswer(ctx, args[0], args[1])
			if err != nil {
				return fmt.Errorf("setting answer: %w", err)
			}
			fmt.Printf("✅ Answer saved for question %s\n", question.ID)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list [application-id]",
		Short: "List every question on an application",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			questions, err := client.ListQuestions(ctx, args[0])
			if err != nil {
				return fmt.Errorf("listing questions: %w", err)
			}
			if len(questions) == 0 {
				fmt.Println("No questions added yet.")
				return nil
			}
			for i, q := range questions {
				fmt.Printf("%d. [%s] %s -> %q (category: %s, status: %s)\n", i+1, q.ID, q.Label, q.Answer, q.Category, q.Status)
			}
			return nil
		},
	})

	return cmd
}
