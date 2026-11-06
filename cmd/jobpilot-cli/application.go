package main

import (
	"context"
	"fmt"
	"os"
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
	cmd.AddCommand(newApplicationLinkCmd(client))
	cmd.AddCommand(newApplicationApplyURLCmd(client))
	cmd.AddCommand(deleteCmd(client, "delete [application-id]", "Delete an application, with its answers, cover letters and history",
		domain.DeleteApplication, func(args []string) string { return "application " + args[0] }))

	return cmd
}

func newApplicationCreateCmd(client *httpapi.Client) *cobra.Command {
	var resumeID string

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

			app, err := client.CreateApplication(ctx, jobID, profile.ID, resumeID)
			if err != nil {
				return fmt.Errorf("creating application: %w", err)
			}
			fmt.Printf("✅ Application created: %s (state: %s)\n", app.ID, app.State)
			return nil
		},
	}
	cmd.Flags().StringVar(&resumeID, "resume", "", "resume ID to associate with this application")
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
				fmt.Printf("%d. %s | job %s | state %s\n", i+1, a.ID, a.JobID, a.State)
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
			fmt.Printf("Application %s\nJob: %s\nState: %s\n", app.ID, app.JobID, app.State)
			if app.ApplyURL != "" {
				fmt.Printf("Application page: %s\n", app.ApplyURL)
			}
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
					fmt.Printf("- [%s] %s -> %q\n", q.ID, q.Label, q.Answer)
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
// themselves in their own browser. `transition <id>
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

// newApplicationLinkCmd prints where to apply: the link the user saved, or
// the job's own. The user opens it in their own browser and applies
// themselves (ADR-0034 §2).
func newApplicationLinkCmd(client *httpapi.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "link [application-id]",
		Short: "Print the address to apply at, to open in your own browser",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			app, err := client.GetApplication(ctx, args[0])
			if err != nil {
				return fmt.Errorf("fetching application: %w", err)
			}
			if app.ApplyURL != "" {
				fmt.Println(app.ApplyURL)
				return nil
			}
			job, err := client.GetJob(ctx, app.JobID)
			if err != nil {
				return fmt.Errorf("fetching the job: %w", err)
			}
			fmt.Println(job.CanonicalURL)
			return nil
		},
	}
}

func newApplicationApplyURLCmd(client *httpapi.Client) *cobra.Command {
	var useJobLink bool

	cmd := &cobra.Command{
		Use:   "apply-url [application-id] [url]",
		Short: "Set where to apply for this application, or clear it to use the job's own link",
		Args: func(cmd *cobra.Command, args []string) error {
			if useJobLink {
				return cobra.ExactArgs(1)(cmd, args)
			}
			return cobra.ExactArgs(2)(cmd, args)
		},
		RunE: func(_ *cobra.Command, args []string) error {
			applyURL := ""
			if !useJobLink {
				applyURL = args[1]
			}
			app, err := client.SetApplyURL(context.Background(), args[0], applyURL)
			if err != nil {
				return fmt.Errorf("recording the application link: %w", err)
			}
			if app.ApplyURL == "" {
				fmt.Printf("✅ Application %s uses the job's own link again.\n", app.ID)
				return nil
			}
			fmt.Printf("✅ Application %s applies at %s.\n", app.ID, app.ApplyURL)
			return nil
		},
	}
	cmd.Flags().BoolVar(&useJobLink, "clear", false, "use the job's own link again")
	return cmd
}

func newCoverLetterCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "cover-letter", Short: "Manage an application's cover letter"}

	cmd.AddCommand(deleteCmd(client, "delete [cover-letter-id]", "Delete a cover letter version other than the current one",
		domain.DeleteCoverLetter, func(args []string) string { return "cover letter " + args[0] }))

	var writeFrom string
	cmd.AddCommand(withFileFlag(&cobra.Command{
		Use:   "write [application-id] [content]",
		Short: "Save a cover letter you wrote as the application's new version",
		Long: "Saves the letter you wrote as a new version, and makes it the current one.\n" +
			"Earlier versions are kept. --file reads the letter from a file.",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(_ *cobra.Command, args []string) error {
			content, err := contentFrom(args[1:], writeFrom)
			if err != nil {
				return err
			}
			coverLetter, err := client.CreateCoverLetter(context.Background(), args[0], content)
			if err != nil {
				return fmt.Errorf("saving the cover letter: %w", err)
			}
			fmt.Printf("✅ Cover letter saved (ID: %s).\n", coverLetter.ID)
			printFlags("", coverLetter.Flags)
			return nil
		},
	}, &writeFrom, "read the letter from this file"))

	cmd.AddCommand(&cobra.Command{
		Use:   "list [application-id]",
		Short: "List every version of an application's cover letter",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			versions, err := client.ListCoverLetterVersions(ctx, args[0])
			if err != nil {
				return fmt.Errorf("listing cover letter versions: %w", err)
			}
			if len(versions) == 0 {
				fmt.Println("No cover letter yet. Write one with 'application cover-letter write'.")
				return nil
			}
			for i, v := range versions {
				fmt.Printf("%d. %s\n%s\n", i+1, v.ID, v.Content)
				printFlags("", v.Flags)
				fmt.Println()
			}
			return nil
		},
	})

	var editFrom string
	cmd.AddCommand(withFileFlag(&cobra.Command{
		Use:   "edit [cover-letter-id] [content]",
		Short: "Replace a cover letter version's content",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(_ *cobra.Command, args []string) error {
			content, err := contentFrom(args[1:], editFrom)
			if err != nil {
				return err
			}
			coverLetter, err := client.EditCoverLetter(context.Background(), args[0], content)
			if err != nil {
				return fmt.Errorf("editing cover letter: %w", err)
			}
			fmt.Printf("✅ Cover letter %s updated.\n", coverLetter.ID)
			printFlags("", coverLetter.Flags)
			return nil
		},
	}, &editFrom, "read the letter from this file"))

	return cmd
}

func newQuestionCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "question",
		Short: "Note an application's questions and write your answers",
		Long: "Note a question the application form asks, and write your answer to it.\n" +
			"JobPilot suggests an answer only from your profile or an answer you kept; it never writes one.",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "add [application-id] [question]",
		Short: "Note a question on an application",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			question, err := client.AddQuestion(context.Background(), args[0], args[1])
			if err != nil {
				return fmt.Errorf("adding question: %w", err)
			}
			fmt.Printf("✅ Question added: %s\n", question.ID)
			printSuggestion("", question.Suggestion)
			return nil
		},
	})

	var answerFrom string
	cmd.AddCommand(withFileFlag(&cobra.Command{
		Use:   "answer [question-id] [answer]",
		Short: "Write your answer to a question; an empty answer clears it",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(_ *cobra.Command, args []string) error {
			answer, err := contentFrom(args[1:], answerFrom)
			if err != nil {
				return err
			}
			question, err := client.SetQuestionAnswer(context.Background(), args[0], answer)
			if err != nil {
				return fmt.Errorf("setting answer: %w", err)
			}
			fmt.Printf("✅ Answer saved for question %s\n", question.ID)
			printFlags("", question.Flags)
			return nil
		},
	}, &answerFrom, "read the answer from this file"))

	cmd.AddCommand(&cobra.Command{
		Use:   "keep [question-id]",
		Short: "Keep your answer, to be suggested for similar questions",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			kept, err := client.KeepAnswer(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("keeping the answer: %w", err)
			}
			fmt.Printf("✅ Kept for questions like %q. 'profile about show' lists what you kept.\n", kept.Question)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "delete [question-id]",
		Short: "Remove a question and your answer to it",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := client.DeleteQuestion(context.Background(), args[0]); err != nil {
				return fmt.Errorf("deleting the question: %w", err)
			}
			fmt.Println("✅ Question removed.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list [application-id]",
		Short: "List every question on an application, with your answers",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			questions, err := client.ListQuestions(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("listing questions: %w", err)
			}
			if len(questions) == 0 {
				fmt.Println("No questions added yet.")
				return nil
			}
			for i, q := range questions {
				fmt.Printf("%d. [%s] %s\n", i+1, q.ID, q.Label)
				if q.Answer == "" {
					fmt.Println("   Not answered yet.")
				} else {
					fmt.Printf("   %s\n", q.Answer)
				}
				if q.Suggestion != nil && q.Suggestion.Answer != q.Answer {
					printSuggestion("   ", q.Suggestion)
				}
				printFlags("   ", q.Flags)
			}
			return nil
		},
	})

	return cmd
}

// printSuggestion shows an answer the candidate's own material gives to a
// question, and where it came from.
func printSuggestion(indent string, suggestion *domain.AnswerSuggestion) {
	if suggestion == nil {
		return
	}
	from := "your profile"
	switch suggestion.Source {
	case domain.SuggestionDerived:
		from = "worked out from your profile"
	case domain.SuggestionKeptAnswer:
		from = fmt.Sprintf("your kept answer to %q", suggestion.Field)
	}
	fmt.Printf("%sSuggested (%s): %s\n", indent, from, suggestion.Answer)
}

// withFileFlag adds a --file flag to cmd, read into target.
func withFileFlag(cmd *cobra.Command, target *string, usage string) *cobra.Command {
	cmd.Flags().StringVar(target, "file", "", usage)
	return cmd
}

// contentFrom is text given as the one remaining argument or in a file,
// for a letter or an answer too long for the command line.
func contentFrom(args []string, file string) (string, error) {
	if file != "" {
		content, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", file, err)
		}
		return string(content), nil
	}
	if len(args) == 0 {
		return "", nil
	}
	return args[0], nil
}

// printFlags lists what the candidate may want to look at in a letter or
// answer (docs/02_features/19_draft_checks.md), or nothing when there is
// nothing to look at.
func printFlags(indent string, flags []domain.DraftFlag) {
	if len(flags) == 0 {
		return
	}
	fmt.Printf("%sWorth a look:\n", indent)
	for _, flag := range flags {
		if flag.Excerpt == "" {
			fmt.Printf("%s- %s\n", indent, flag.Reason)
			continue
		}
		fmt.Printf("%s- %q: %s\n", indent, flag.Excerpt, flag.Reason)
	}
}
