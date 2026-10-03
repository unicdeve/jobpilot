package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
	"github.com/unicdev/jobpilot/internal/core/domain"
)

// newNotificationsCmd lists what JobPilot told the user: what a scout
// found, or that it could not run (ADR-0028). Listing what has not been
// seen marks it seen, as the desktop's Dashboard does when it is
// dismissed.
func newNotificationsCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notifications",
		Short: "Show what your scouts found while you were away",
		Long: `Shows the notifications you have not seen yet, and marks them seen.
--all shows every notification, seen or not, and marks nothing.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			all, _ := cmd.Flags().GetBool("all")
			ctx := context.Background()
			notifications, err := client.ListNotifications(ctx, !all)
			if err != nil {
				return fmt.Errorf("listing notifications: %w", err)
			}
			if len(notifications) == 0 {
				if all {
					fmt.Println("No notifications yet.")
				} else {
					fmt.Println("Nothing new since you last looked.")
				}
				return nil
			}
			for _, notification := range notifications {
				printNotification(notification)
			}
			if all {
				return nil
			}
			if _, err := client.MarkNotificationsSeen(ctx, ""); err != nil {
				return fmt.Errorf("marking notifications seen: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().Bool("all", false, "show every notification, including those already seen")
	return cmd
}

func printNotification(notification *domain.Notification) {
	fmt.Printf("  %s  %s\n", notification.CreatedAt.Local().Format("Mon 2 Jan 15:04"), notification.Title)
	fmt.Printf("    %s\n", notification.Body)
	if notification.RunID != "" {
		fmt.Printf("    See the run with 'activity show %s'.\n", notification.RunID)
	}
}

// newNotificationSettingsCmd shows or changes the user's choices about
// notifications: whether to post them, and what counts as a strong match.
func newNotificationSettingsCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notifications",
		Short: "Show or change notification settings",
		Long: `Without flags, shows whether JobPilot posts notifications about scout runs,
and the score a strong match needs. --on and --off turn posting on and off;
turned off, notifications still wait for you on the Dashboard and in
'jobpilot-cli notifications'. --strong-match sets the score, 1 to 100.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := context.Background()
			settings, err := client.GetNotificationSettings(ctx)
			if err != nil {
				return fmt.Errorf("loading notification settings: %w", err)
			}
			on, _ := cmd.Flags().GetBool("on")
			off, _ := cmd.Flags().GetBool("off")
			changed := on || off || cmd.Flags().Changed("strong-match")
			if on {
				settings.Enabled = true
			}
			if off {
				settings.Enabled = false
			}
			if cmd.Flags().Changed("strong-match") {
				settings.StrongMatchScore, _ = cmd.Flags().GetInt("strong-match")
			}
			if changed {
				if err := client.SaveNotificationSettings(ctx, settings); err != nil {
					return fmt.Errorf("saving notification settings: %w", err)
				}
			}

			state := "on"
			if !settings.Enabled {
				state = "off"
			}
			fmt.Printf("Notifications: %s\nA strong match scores at least: %d\n", state, settings.StrongMatchScore)
			return nil
		},
	}
	cmd.Flags().Bool("on", false, "post notifications")
	cmd.Flags().Bool("off", false, "post no notifications")
	cmd.Flags().Int("strong-match", 0, "the score, 1 to 100, a strong match needs")
	cmd.MarkFlagsMutuallyExclusive("on", "off")
	return cmd
}
