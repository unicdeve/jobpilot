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

// newScoutCmd builds the `scout` command tree.
//
// A scout runs a saved search, judges what it finds, and creates
// applications for review. It never contacts an employer and never
// submits (ADR-0014), so there is no flag here that could make it.
func newScoutCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "scout", Short: "Manage scouts that shortlist jobs for review"}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List scouts",
		RunE: func(_ *cobra.Command, _ []string) error {
			scouts, err := client.ListScouts(context.Background())
			if err != nil {
				return fmt.Errorf("listing scouts: %w", err)
			}
			if len(scouts) == 0 {
				fmt.Println("No scouts yet. Add one with 'scout add <name> --search <saved search id>'.")
				return nil
			}
			for _, scout := range scouts {
				state := "enabled"
				if !scout.IsEnabled {
					state = "disabled"
				}
				fmt.Printf("  %s (%s)\n    score ≥ %d, up to %d per run, from %s\n    runs %s\n    ID: %s\n",
					scout.Name, state, scout.MinMatchScore, scout.MaxApplicationsPerRun,
					scout.Source, describeSchedule(scout.Schedule), scout.ID)
				switch scout.Activity {
				case domain.ScoutRunning:
					fmt.Println("    running now")
				case domain.ScoutWaiting:
					fmt.Println("    waiting for another scout's run to finish")
				}
				if scout.NextRunAt != nil {
					fmt.Printf("    next run: %s\n", scout.NextRunAt.Local().Format(time.RFC1123))
				}
				if note := describeAlarm(scout.Alarm); note != "" {
					fmt.Printf("    %s\n", note)
				}
				if scout.LastRunAt != nil {
					fmt.Printf("    last run: %s\n", scout.LastRunAt.Local().Format(time.RFC1123))
				}
			}
			return nil
		},
	})

	addCmd := &cobra.Command{
		Use:   "add [name]",
		Short: "Create a scout over a saved search",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			searchID, _ := cmd.Flags().GetString("search")
			minScore, _ := cmd.Flags().GetInt("min-score")
			perRun, _ := cmd.Flags().GetInt("max-per-run")
			source, _ := cmd.Flags().GetString("source")
			schedule, err := scheduleFromFlags(cmd)
			if err != nil {
				return err
			}

			scout, err := client.CreateScout(context.Background(), &domain.Scout{
				Name:                  args[0],
				SavedSearchID:         searchID,
				MinMatchScore:         minScore,
				MaxApplicationsPerRun: perRun,
				Source:                domain.ScoutSource(source),
				IsEnabled:             true,
				Schedule:              schedule,
			})
			if err != nil {
				return fmt.Errorf("creating scout: %w", err)
			}

			fmt.Printf("✅ Scout %q created.\n", scout.Name)
			printScheduled(scout)
			fmt.Printf("   Run it now with 'scout run %s'.\n", scout.ID)
			return nil
		},
	}
	addScheduleFlags(addCmd)
	addCmd.Flags().String("search", "", "ID of the saved search this scout runs (required)")
	addCmd.Flags().Int("min-score", 70, "lowest match score that earns an application")
	addCmd.Flags().Int("max-per-run", 10, "most applications one run may create")
	addCmd.Flags().String("source", string(domain.ScoutSourceLocal),
		"'local' to consider stored jobs, 'sources' to fetch from the job boards first")
	_ = addCmd.MarkFlagRequired("search")
	cmd.AddCommand(addCmd)

	scheduleCmd := &cobra.Command{
		Use:   "schedule [scout-id]",
		Short: "Set when a scout runs on its own",
		Long: `Set when a scout runs on its own: every few hours, or once a day at a
local time. The operating system's scheduler starts it, whether or not
JobPilot is open, while you are logged in and the computer is awake.
A run missed while the computer was off or asleep runs once when it is
next on.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			schedule, err := scheduleFromFlags(cmd)
			if err != nil {
				return err
			}
			if off, _ := cmd.Flags().GetBool("off"); !off && !schedule.IsScheduled() {
				return fmt.Errorf("say when: --every <hours>, --daily <HH:MM>, or --off")
			}
			ctx := context.Background()
			scout, err := client.GetScout(ctx, args[0])
			if err != nil {
				return fmt.Errorf("reading scout: %w", err)
			}
			scout.Schedule = schedule
			updated, err := client.UpdateScout(ctx, scout)
			if err != nil {
				return fmt.Errorf("scheduling scout: %w", err)
			}
			fmt.Printf("✅ Scout %q now runs %s.\n", updated.Name, describeSchedule(updated.Schedule))
			printScheduled(updated)
			return nil
		},
	}
	addScheduleFlags(scheduleCmd)
	scheduleCmd.Flags().Bool("off", false, "run only when you run it")
	cmd.AddCommand(scheduleCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "run [scout-id]",
		Short: "Run a scout once",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			report, err := client.RunScout(context.Background(), args[0])
			// A run that fails partway still created real applications,
			// so the report is printed before the error is returned.
			if report != nil && report.RunID != "" {
				printScoutReport(report)
			}
			if err != nil {
				return fmt.Errorf("running scout: %w", err)
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "runs",
		Short: "Show recent scout runs",
		RunE: func(_ *cobra.Command, _ []string) error {
			runs, err := client.ListScoutRuns(context.Background(), 10)
			if err != nil {
				return fmt.Errorf("listing scout runs: %w", err)
			}
			if len(runs) == 0 {
				fmt.Println("No scout runs yet.")
				return nil
			}
			for _, run := range runs {
				startedBy := "by you"
				if run.Trigger == domain.RunTriggerSchedule {
					startedBy = "on schedule"
				}
				fmt.Printf("  %s  %s  %s  %s\n", run.StartedAt.Local().Format(time.RFC1123), run.Status, startedBy, run.ID)
				if run.Error != "" {
					fmt.Printf("    %s\n", run.Error)
				}
			}
			fmt.Println("\nSee what one did with 'scout trail <run-id>'.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "trail [run-id]",
		Short: "Show what one scout run considered, rejected and created",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			events, err := client.ListScoutRunEvents(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("reading the run's trail: %w", err)
			}
			for _, event := range events {
				line := event.Action
				if event.InputSummary != "" {
					line += ": " + event.InputSummary
				}
				if event.OutputSummary != "" {
					line += " — " + event.OutputSummary
				}
				fmt.Printf("  %s\n", line)
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "delete [scout-id]",
		Short: "Delete a scout",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := client.DeleteScout(context.Background(), args[0]); err != nil {
				return fmt.Errorf("deleting scout: %w", err)
			}
			fmt.Println("✅ Scout deleted.")
			return nil
		},
	})

	return cmd
}

// addScheduleFlags adds the flags that say when a scout runs on its own.
func addScheduleFlags(cmd *cobra.Command) {
	cmd.Flags().Int("every", 0, "run every this many hours, 1 to 24")
	cmd.Flags().String("daily", "", "run once a day at this local time, as HH:MM")
	cmd.MarkFlagsMutuallyExclusive("every", "daily")
}

// scheduleFromFlags reads --every and --daily, and --off where the command
// has it. Neither given is off.
func scheduleFromFlags(cmd *cobra.Command) (domain.ScoutSchedule, error) {
	every, _ := cmd.Flags().GetInt("every")
	daily, _ := cmd.Flags().GetString("daily")
	off := false
	if cmd.Flags().Lookup("off") != nil {
		off, _ = cmd.Flags().GetBool("off")
	}
	switch {
	case off && (every != 0 || daily != ""):
		return domain.ScoutSchedule{}, fmt.Errorf("--off cannot be given with --every or --daily")
	case every != 0:
		return domain.ScoutSchedule{Kind: domain.ScheduleEvery, EveryHours: every}, nil
	case daily != "":
		minutes, err := parseTimeOfDay(daily)
		if err != nil {
			return domain.ScoutSchedule{}, err
		}
		return domain.ScoutSchedule{Kind: domain.ScheduleDaily, DailyAt: minutes}, nil
	}
	return domain.ScoutSchedule{Kind: domain.ScheduleOff}, nil
}

// parseTimeOfDay reads HH:MM as minutes after midnight.
func parseTimeOfDay(value string) (int, error) {
	at, err := time.Parse("15:04", strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("--daily takes a time as HH:MM, such as 08:30, got %q", value)
	}
	return at.Hour()*60 + at.Minute(), nil
}

// describeSchedule says when a scout runs, to finish "runs ...".
func describeSchedule(schedule domain.ScoutSchedule) string {
	switch schedule.Kind {
	case domain.ScheduleEvery:
		if schedule.EveryHours == 1 {
			return "every hour"
		}
		return fmt.Sprintf("every %d hours", schedule.EveryHours)
	case domain.ScheduleDaily:
		return fmt.Sprintf("daily at %02d:%02d", schedule.DailyAt/60, schedule.DailyAt%60)
	}
	return "only when you run it"
}

// describeAlarm says what is wrong with a scheduled scout's alarm, or
// nothing when it is in place.
func describeAlarm(alarm *domain.ScoutAlarmStatus) string {
	if alarm == nil {
		return ""
	}
	switch alarm.State {
	case domain.AlarmMissing:
		return "⚠ not scheduled with the operating system: " + alarm.Reason
	case domain.AlarmSwitchedOff:
		return "⚠ switched off outside JobPilot (on macOS, in System Settings → General → Login Items), so it will not run on its own"
	case domain.AlarmUnavailable:
		return "⚠ this system has no scheduler JobPilot can use, so it runs only when you run it"
	}
	return ""
}

// printScheduled says when a scheduled scout next runs, and what stands
// in its way if anything does.
func printScheduled(scout *domain.Scout) {
	if !scout.Schedule.IsScheduled() {
		return
	}
	if scout.NextRunAt != nil {
		fmt.Printf("   Next run: %s.\n", scout.NextRunAt.Local().Format(time.RFC1123))
	}
	if note := describeAlarm(scout.Alarm); note != "" {
		fmt.Printf("   %s\n", note)
	}
}

// printScoutReport summarizes a run, listing what was rejected and why.
func printScoutReport(report *domain.ScoutRunReport) {
	fmt.Printf("Considered %d job(s), created %d application(s).\n",
		report.Considered, report.Created)
	if report.CeilingHit {
		fmt.Println("Stopped at this scout's per-run limit; run it again for more.")
	}

	for _, verdict := range report.Verdicts {
		mark := "–"
		if verdict.Eligible {
			mark = "✅"
		}
		fmt.Printf("  %s %s — %s\n", mark, verdict.JobTitle, verdict.Reason)
	}

	if report.Created > 0 {
		fmt.Println("\nThese are waiting for your review. Nothing has been sent.")
	}
}
