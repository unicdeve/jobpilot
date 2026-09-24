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

// newActivityCmd reads the trail every agent run leaves: browser
// preparation and scout runs alike (docs/02_features/12_agent_activity.md).
func newActivityCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "activity", Short: "See what JobPilot's agent runs did"}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List agent runs, newest first",
		RunE: func(cmd *cobra.Command, _ []string) error {
			kind, err := cmd.Flags().GetString("kind")
			if err != nil {
				return err
			}
			status, err := cmd.Flags().GetString("status")
			if err != nil {
				return err
			}
			limit, err := cmd.Flags().GetInt("limit")
			if err != nil {
				return err
			}
			offset, err := cmd.Flags().GetInt("offset")
			if err != nil {
				return err
			}

			page, err := client.ListActivity(context.Background(), domain.AgentRunFilter{
				TaskType: kind,
				Status:   domain.AgentRunStatus(status),
				Limit:    limit,
				Offset:   offset,
			})
			if err != nil {
				return fmt.Errorf("listing agent runs: %w", err)
			}

			if len(page.Runs) == 0 {
				if offset > 0 {
					fmt.Printf("No runs at offset %d (%d in total).\n", offset, page.Total)
					return nil
				}
				fmt.Println("No agent runs yet.")
				return nil
			}

			fmt.Printf("Agent runs %d-%d of %d:\n", offset+1, offset+len(page.Runs), page.Total)
			for _, run := range page.Runs {
				fmt.Printf("  %s  %-9s  %s — %s\n",
					run.StartedAt.Local().Format("2006-01-02 15:04"), run.Status, runSubject(run), run.ID)
				if run.Error != "" {
					fmt.Printf("    %s\n", run.Error)
				}
			}

			if shown := offset + len(page.Runs); shown < page.Total {
				fmt.Printf("\n%d more — use --offset %d to see the next page.\n", page.Total-shown, shown)
			}
			fmt.Println("\nSee what one did with 'activity show <run-id>'.")
			return nil
		},
	}
	listCmd.Flags().String("kind", "", "Only list runs of this kind: prepare_application or scout_run")
	listCmd.Flags().String("status", "", "Only list runs with this status: running, completed, blocked or failed")
	listCmd.Flags().Int("limit", 0, "Maximum runs to list (default 50, maximum 200)")
	listCmd.Flags().Int("offset", 0, "Skip this many runs before listing")
	cmd.AddCommand(listCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "show [run-id]",
		Short: "Show every step one run recorded",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			detail, err := client.GetActivityRun(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("reading the run: %w", err)
			}

			run := detail.Run
			fmt.Printf("%s\n", runSubject(run))
			fmt.Printf("  Run:     %s\n", run.ID)
			fmt.Printf("  Status:  %s\n", run.Status)
			fmt.Printf("  Started: %s\n", run.StartedAt.Local().Format(time.RFC1123))
			if run.CompletedAt != nil {
				fmt.Printf("  Took:    %s\n", run.CompletedAt.Sub(run.StartedAt).Round(time.Second))
			}
			if run.Error != "" {
				fmt.Printf("  Error:   %s\n", run.Error)
			}

			if len(detail.Events) == 0 {
				fmt.Println("\nNo steps recorded.")
				return nil
			}
			fmt.Println()
			for _, event := range detail.Events {
				fmt.Printf("  %s\n", eventLine(event))
			}
			return nil
		},
	})

	return cmd
}

// runSubject names what a run served.
func runSubject(run *domain.AgentRunSummary) string {
	switch run.TaskType {
	case domain.AgentTaskScoutRun:
		if run.ScoutName == "" {
			return "Scout run (scout deleted)"
		}
		return fmt.Sprintf("Scout %q", run.ScoutName)
	case domain.AgentTaskPrepareApplication:
		switch {
		case run.JobTitle == "":
			return "Prepared in browser (application deleted)"
		case run.Company == "":
			return "Prepared in browser: " + run.JobTitle
		default:
			return fmt.Sprintf("Prepared in browser: %s at %s", run.JobTitle, run.Company)
		}
	default:
		return run.TaskType
	}
}

// eventLine renders one step as `action [tool] status: input — output (policy, duration)`.
func eventLine(event *domain.AgentEvent) string {
	var b strings.Builder
	b.WriteString(event.Action)
	if event.Tool != "" {
		fmt.Fprintf(&b, " [%s]", event.Tool)
	}
	if event.Status != "" && event.Status != "ok" {
		fmt.Fprintf(&b, " %s", event.Status)
	}
	if event.InputSummary != "" {
		fmt.Fprintf(&b, ": %s", event.InputSummary)
	}
	if event.OutputSummary != "" {
		fmt.Fprintf(&b, " — %s", event.OutputSummary)
	}

	var notes []string
	if event.PolicyResult != "" {
		notes = append(notes, "policy "+event.PolicyResult)
	}
	if event.DurationMs > 0 {
		notes = append(notes, (time.Duration(event.DurationMs) * time.Millisecond).String())
	}
	if len(notes) > 0 {
		fmt.Fprintf(&b, " (%s)", strings.Join(notes, ", "))
	}
	return b.String()
}
