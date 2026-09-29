package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"

	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
	"github.com/unicdev/jobpilot/internal/core/domain"
)

// kindNames are how each kind of record reads in a confirmation, singular
// and plural.
var kindNames = map[domain.RecordKind][2]string{
	domain.RecordCandidate:       {"profile", "profiles"},
	domain.RecordCandidateDetail: {"profile entry (experience, skill, link or work authorization)", "profile entries (experience, skills, links, work authorizations)"},
	domain.RecordResume:          {"resume", "resumes"},
	domain.RecordJob:             {"job", "jobs"},
	domain.RecordMatch:           {"match analysis", "match analyses"},
	domain.RecordApplication:     {"application", "applications"},
	domain.RecordQuestion:        {"application question and answer", "application questions and answers"},
	domain.RecordCoverLetter:     {"cover letter version", "cover letter versions"},
	domain.RecordStateTransition: {"status change", "status changes"},
	domain.RecordAgentRun:        {"agent run", "agent runs"},
	domain.RecordAgentStep:       {"agent step", "agent steps"},
	domain.RecordScout:           {"scout", "scouts"},
	domain.RecordSavedSearch:     {"saved search", "saved searches"},
	domain.RecordCredentialName:  {"API key name", "API key names"},
	domain.RecordSettings:        {"setting", "settings"},
	domain.RecordPersonalAnswer:  {"thing you told JobPilot about yourself", "things you told JobPilot about yourself"},
}

// kindOrder lists kinds the way a person reads what they are deleting:
// the thing itself, then what hangs off it.
var kindOrder = []domain.RecordKind{
	domain.RecordCandidate, domain.RecordCandidateDetail, domain.RecordPersonalAnswer, domain.RecordResume, domain.RecordJob, domain.RecordMatch,
	domain.RecordApplication, domain.RecordCoverLetter, domain.RecordQuestion, domain.RecordStateTransition,
	domain.RecordAgentRun, domain.RecordAgentStep, domain.RecordSavedSearch, domain.RecordScout,
	domain.RecordCredentialName, domain.RecordSettings,
}

func countedKind(kind domain.RecordKind, n int) string {
	names, ok := kindNames[kind]
	if !ok {
		names = [2]string{string(kind), string(kind)}
	}
	if n == 1 {
		return "1 " + names[0]
	}
	return fmt.Sprintf("%d %s", n, names[1])
}

func countedKinds(counts map[domain.RecordKind]int) []string {
	var lines []string
	seen := map[domain.RecordKind]bool{}
	for _, kind := range kindOrder {
		if n := counts[kind]; n > 0 {
			lines = append(lines, countedKind(kind, n))
			seen[kind] = true
		}
	}
	// A kind a newer daemon reports is still shown.
	var rest []string
	for kind, n := range counts {
		if n > 0 && !seen[kind] {
			rest = append(rest, countedKind(kind, n))
		}
	}
	sort.Strings(rest)
	return append(lines, rest...)
}

// describeReach is what the user reads before confirming a deletion.
func describeReach(reach *domain.DeletionReach) string {
	var b strings.Builder
	deleted := countedKinds(reach.Deleted)
	if len(deleted) == 0 {
		b.WriteString("There is nothing to delete.\n")
	} else {
		b.WriteString("This deletes:\n")
		for _, line := range deleted {
			b.WriteString("  - " + line + "\n")
		}
	}
	switch sent := reach.SentApplications; {
	case sent == 1:
		b.WriteString("1 of those applications was sent to an employer. Deleting it here withdraws nothing there.\n")
	case sent > 1:
		fmt.Fprintf(&b, "%d of those applications were sent to employers. Deleting them here withdraws nothing there.\n", sent)
	}
	if detached := countedKinds(reach.Detached); len(detached) > 0 {
		b.WriteString("These stay, but lose their link to what is deleted:\n")
		for _, line := range detached {
			b.WriteString("  - " + line + "\n")
		}
	}
	switch kept := reach.KeptNotInterested; {
	case kept == 1:
		b.WriteString("Kept: 1 job marked not interested, so it is not offered again.\n")
	case kept > 1:
		fmt.Fprintf(&b, "Kept: %d jobs marked not interested, so they are not offered again.\n", kept)
	}
	if len(reach.Outside) > 0 {
		b.WriteString("Outside the database, it also removes:\n")
		for _, line := range reach.Outside {
			b.WriteString("  - " + line + "\n")
		}
	}
	for _, name := range reach.EnvironmentCredentials {
		fmt.Fprintf(&b, "The environment variable %s supplies an API key. JobPilot cannot unset it; remove it where it is set.\n", name)
	}
	if len(deleted) > 0 {
		b.WriteString("This cannot be undone.\n")
	}
	return b.String()
}

// runDeletion previews a deletion, shows what it reaches, asks, and
// carries it out. If what it reaches changes before it is carried out, the
// new reach is shown and asked about again, since the user consented to
// what they saw and nothing else.
func runDeletion(cmd *cobra.Command, client *httpapi.Client, target domain.DeletionTarget, id, what string) error {
	ctx := context.Background()
	yes, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	reach, err := client.PreviewDeletion(ctx, target, id)
	if err != nil {
		return err
	}
	for {
		fmt.Print(describeReach(reach))
		if total(reach.Deleted) == 0 {
			return nil
		}
		if !yes && !confirmDeletion(target, what) {
			fmt.Println("Nothing was deleted.")
			return nil
		}

		result, err := client.Delete(ctx, target, id, reach.Token)
		var changed *httpapi.ReachChangedError
		if errors.As(err, &changed) {
			fmt.Println("\nThat changed since it was shown. Here is what it reaches now:")
			reach = changed.Reach
			continue
		}
		if err != nil {
			return err
		}
		fmt.Printf("✅ Deleted %s.\n", what)
		for _, notice := range result.Notices {
			fmt.Printf("⚠️  %s\n", notice)
		}
		return nil
	}
}

func total(counts map[domain.RecordKind]int) int {
	n := 0
	for _, c := range counts {
		n += c
	}
	return n
}

// confirmDeletion asks before deleting. Deleting everything asks for a
// word rather than a keystroke, so it cannot be confirmed by reflex.
func confirmDeletion(target domain.DeletionTarget, what string) bool {
	if target == domain.DeleteEverything {
		prompt := promptui.Prompt{
			Label: "Type delete to delete all data",
			Validate: func(input string) error {
				if strings.TrimSpace(input) != "delete" {
					return errors.New(`type "delete", or press Ctrl-C to keep everything`)
				}
				return nil
			},
		}
		_, err := prompt.Run()
		return err == nil
	}
	prompt := promptui.Prompt{Label: "Delete " + what, IsConfirm: true}
	_, err := prompt.Run()
	return err == nil
}

// deleteCmd builds "<noun> delete", which previews and confirms.
func deleteCmd(client *httpapi.Client, use, short string, target domain.DeletionTarget, what func(args []string) string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			id := ""
			if target.TakesID() {
				id = args[0]
			}
			return runDeletion(cmd, client, target, id, what(args))
		},
	}
	if target.TakesID() {
		cmd.Args = cobra.ExactArgs(1)
	} else {
		cmd.Args = cobra.NoArgs
	}
	cmd.Flags().Bool("yes", false, "Delete without asking, after showing what goes")
	return cmd
}

// newJobCmd acts on one stored job: delete it, or rule it out or back in.
func newJobCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "job", Short: "Delete a stored job, or mark it not interested"}

	cmd.AddCommand(deleteCmd(client, "delete [job-id]", "Delete a job, with its match and applications",
		domain.DeleteJob, func(args []string) string { return "job " + args[0] }))

	for _, interest := range []struct {
		use, short    string
		notInterested bool
	}{
		{"not-interested [job-id]", "Rule a job out: hide it from lists and searches, and have scouts pass it over", true},
		{"interested [job-id]", "Undo not-interested", false},
	} {
		cmd.AddCommand(&cobra.Command{
			Use:   interest.use,
			Short: interest.short,
			Args:  cobra.ExactArgs(1),
			RunE: func(_ *cobra.Command, args []string) error {
				job, err := client.SetJobInterest(context.Background(), args[0], interest.notInterested)
				if err != nil {
					return err
				}
				if job.NotInterestedAt != nil {
					fmt.Printf("✅ %s at %s is marked not interested. 'job interested %s' undoes it.\n", job.Title, job.CompanyName, job.ID)
				} else {
					fmt.Printf("✅ %s at %s is back in your lists.\n", job.Title, job.CompanyName)
				}
				return nil
			},
		})
	}
	return cmd
}

// newDataCmd is the bulk deletions: history, and everything.
func newDataCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "data", Short: "Clear history, or delete all local data"}

	clearCmd := &cobra.Command{Use: "clear", Short: "Clear activity or stored jobs"}
	clearCmd.AddCommand(deleteCmd(client, "activity", "Delete every agent run and its steps",
		domain.ClearActivity, func([]string) string { return "the activity history" }))
	clearCmd.AddCommand(deleteCmd(client, "jobs", "Delete every stored job no application uses, except those marked not interested",
		domain.ClearJobs, func([]string) string { return "the unused jobs" }))
	cmd.AddCommand(clearCmd)

	cmd.AddCommand(deleteCmd(client, "delete-all", "Delete everything JobPilot stores, as if freshly installed",
		domain.DeleteEverything, func([]string) string { return "all data" }))
	return cmd
}
