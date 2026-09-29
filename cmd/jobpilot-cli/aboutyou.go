package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"

	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/services"
)

// newProfileAboutCmd is "profile about": what the candidate says about
// themselves, which letters and written answers are built from
// (docs/02_features/16_about_you.md).
func newProfileAboutCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "about",
		Short: "Tell JobPilot about yourself, so letters and answers sound like you",
		Long: "Asks each About you question in turn, showing what you answered before.\n" +
			"Press Enter to keep an answer, edit it to change it, or clear it to remove it.\n" +
			"Nothing is required: skip any question with Enter.",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return askAboutYou(context.Background(), client)
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show what you have told JobPilot about yourself",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			about, err := client.GetAboutYou(context.Background())
			if err != nil {
				return fmt.Errorf("reading About you: %w", err)
			}
			printAboutYou(about)
			return nil
		},
	})

	var fromFile string
	setCmd := &cobra.Command{
		Use:   "set [key] [answer]",
		Short: "Set or clear one answer: a question's key, writing_sample or writing_preferences",
		Long: "Keys: " + strings.Join(aboutYouKeys(), ", ") + ".\n" +
			"An empty answer clears it. --file reads the answer from a file, which suits a writing sample.",
		Args: cobra.RangeArgs(1, 2),
		RunE: func(_ *cobra.Command, args []string) error {
			key := domain.PersonalAnswerKey(args[0])
			answer := ""
			if len(args) == 2 {
				answer = args[1]
			}
			if fromFile != "" {
				content, err := os.ReadFile(fromFile)
				if err != nil {
					return fmt.Errorf("reading %s: %w", fromFile, err)
				}
				answer = string(content)
			}

			var update services.AboutYouUpdate
			switch key {
			case domain.AnswerWritingSample:
				update.WritingSample = &answer
			case domain.AnswerWritingPreferences:
				update.WritingPreferences = &answer
			default:
				update.Answers = map[domain.PersonalAnswerKey]string{key: answer}
			}
			if _, err := client.SaveAboutYou(context.Background(), update); err != nil {
				return fmt.Errorf("saving %s: %w", key, err)
			}
			if strings.TrimSpace(answer) == "" {
				fmt.Printf("✅ Cleared %s.\n", key)
			} else {
				fmt.Printf("✅ Saved %s.\n", key)
			}
			return nil
		},
	}
	setCmd.Flags().StringVar(&fromFile, "file", "", "read the answer from this file")
	cmd.AddCommand(setCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "forget [kept-answer-id]",
		Short: "Forget an answer you kept for similar questions",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := client.DeleteKeptAnswer(context.Background(), args[0]); err != nil {
				return fmt.Errorf("forgetting the kept answer: %w", err)
			}
			fmt.Println("✅ Forgotten.")
			return nil
		},
	})

	return cmd
}

func aboutYouKeys() []string {
	keys := make([]string, 0, len(domain.StandingQuestions)+2)
	for _, question := range domain.StandingQuestions {
		keys = append(keys, string(question.Key))
	}
	return append(keys, string(domain.AnswerWritingSample), string(domain.AnswerWritingPreferences))
}

// askAboutYou walks the candidate through the About you questions, saving
// each answer as it is given, so stopping partway keeps what was said.
func askAboutYou(ctx context.Context, client *httpapi.Client) error {
	about, err := client.GetAboutYou(ctx)
	if err != nil {
		return fmt.Errorf("reading About you: %w", err)
	}
	fmt.Println("Tell JobPilot about yourself. Letters and written answers are built from what you say here, and nothing is made up in its place.")
	fmt.Println("Enter keeps an answer; clear one to remove it.")

	save := func(update services.AboutYouUpdate) error {
		_, err := client.SaveAboutYou(ctx, update)
		return err
	}
	ask := func(question, hint, current string) (string, bool, error) {
		fmt.Printf("\n%s\n", question)
		if hint != "" {
			fmt.Printf("  %s\n", hint)
		}
		answer, err := (&promptui.Prompt{Label: "Your answer", Default: current, AllowEdit: true}).Run()
		if errors.Is(err, promptui.ErrInterrupt) || errors.Is(err, promptui.ErrEOF) {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		return answer, true, nil
	}

	for _, question := range about.Questions {
		answer, ok, err := ask(question.Question, question.Hint, question.Answer)
		if err != nil || !ok {
			return err
		}
		if answer == question.Answer {
			continue
		}
		if err := save(services.AboutYouUpdate{Answers: map[domain.PersonalAnswerKey]string{question.Key: answer}}); err != nil {
			return fmt.Errorf("saving your answer: %w", err)
		}
	}

	prefs, ok, err := ask("How do you want letters and answers to sound?", "Tone and length: warm or formal, short or thorough.", about.WritingPreferences)
	if err != nil || !ok {
		return err
	}
	if prefs != about.WritingPreferences {
		if err := save(services.AboutYouUpdate{WritingPreferences: &prefs}); err != nil {
			return fmt.Errorf("saving your writing preferences: %w", err)
		}
	}

	fmt.Println("\n✅ Saved. To add a writing sample, for your voice only: jobpilot-cli profile about set writing_sample --file <path>")
	return nil
}

func printAboutYou(about *services.AboutYou) {
	for _, question := range about.Questions {
		fmt.Printf("%s (%s)\n", question.Question, question.Key)
		if question.Answer == "" {
			fmt.Println("  —")
		} else {
			fmt.Printf("  %s\n", question.Answer)
		}
	}
	fmt.Printf("\nWriting preferences (writing_preferences)\n  %s\n", orDash(about.WritingPreferences))
	fmt.Printf("\nWriting sample (writing_sample)\n  %s\n", orDash(about.WritingSample))
	if len(about.Kept) > 0 {
		fmt.Println("\nKept for similar questions:")
		for _, kept := range about.Kept {
			fmt.Printf("  %s  %s\n    %s\n", kept.ID, kept.Question, kept.Answer)
		}
	}
}

func orDash(text string) string {
	if strings.TrimSpace(text) == "" {
		return "—"
	}
	return text
}

// askApplicationQuestions offers the questions asked before a letter for
// one application, and saves what the candidate answers. Skipping every
// question is fine: the letter is still written.
func askApplicationQuestions(ctx context.Context, client *httpapi.Client, applicationID string) error {
	about, err := client.GetAboutYou(ctx)
	if err == nil && aboutYouIsEmpty(about) {
		fmt.Println("Letters stay generic until you tell JobPilot about yourself — about five minutes: jobpilot-cli profile about")
	}

	questions, err := client.PreparePersonalQuestions(ctx, applicationID)
	if err != nil {
		return fmt.Errorf("preparing questions: %w", err)
	}
	fmt.Println("A few questions about this job make the letter yours. Enter skips one.")

	var answers []services.ApplicationAnswer
	for _, question := range questions {
		fmt.Printf("\n%s\n", question.Question)
		answer, err := (&promptui.Prompt{Label: "Your answer", Default: question.Answer, AllowEdit: true}).Run()
		if errors.Is(err, promptui.ErrInterrupt) || errors.Is(err, promptui.ErrEOF) {
			return errors.New("stopped; nothing was written")
		}
		if err != nil {
			return err
		}
		if answer != question.Answer {
			answers = append(answers, services.ApplicationAnswer{ID: question.ID, Key: question.Key, Answer: answer})
		}
	}
	if len(answers) == 0 {
		return nil
	}
	if _, err := client.SavePersonalAnswers(ctx, applicationID, answers); err != nil {
		return fmt.Errorf("saving your answers: %w", err)
	}
	return nil
}

func aboutYouIsEmpty(about *services.AboutYou) bool {
	for _, question := range about.Questions {
		if question.Answer != "" {
			return false
		}
	}
	return about.WritingSample == "" && about.WritingPreferences == ""
}
