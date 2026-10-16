package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/domain/geo"
)

func newProfileCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "profile", Short: "Manage candidate profile"}

	cmd.AddCommand(deleteCmd(client, "delete", "Delete your profile, resumes, matches and applications",
		domain.DeleteProfile, func([]string) string { return "your profile" }))
	cmd.AddCommand(newProfileAboutCmd(client))

	cmd.AddCommand(&cobra.Command{
		Use:   "get",
		Short: "View current profile",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()
			profile, err := client.GetProfile(ctx)
			if err != nil {
				return err
			}
			fmt.Printf("First Name: %s\nLast Name: %s\nEmail: %s\nPhone: %s\nLocation: %s\nTime zone: %s\nSummary: %s\nSkills: %s\n",
				profile.FirstName, profile.LastName, profile.Email, profile.Phone,
				profile.LocationDisplay(), describeTimeZone(profile.TimeZone), profile.Summary, profile.RenderSkills())
			if experience, ok := profile.CurrentExperience(); ok {
				fmt.Printf("Current role: %s at %s\n", experience.Title, experience.Company)
			}
			if years, ok := profile.TotalYearsOfExperience(time.Now()); ok {
				fmt.Printf("Experience: %.1f years across %d roles\n", years, len(profile.Experiences))
			}
			for _, link := range profile.Links {
				fmt.Printf("%s: %s\n", link.Kind, link.URL)
			}
			for _, auth := range profile.WorkAuthorizations {
				fmt.Printf("Work authorization (%s): authorized=%t sponsorship required=%t\n",
					auth.Country, auth.IsAuthorized, auth.RequiresSponsorship)
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "update",
		Short: "Update current profile",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()
			profile, err := client.GetProfile(ctx)
			if err != nil {
				return fmt.Errorf("no profile found, run 'onboard' first: %w", err)
			}

			fmt.Println("Updating profile (leave blank to keep current value):")

			updateField := func(label string, current string, setter func(string)) {
				prompt := promptui.Prompt{
					Label:   label,
					Default: current,
				}
				val, err := prompt.Run()
				if err == nil {
					setter(val)
				}
			}

			updateField("First Name", profile.FirstName, func(v string) { profile.FirstName = v })
			updateField("Last Name", profile.LastName, func(v string) { profile.LastName = v })
			updateField("Email", profile.Email, func(v string) { profile.Email = v })
			updateField("Phone", profile.Phone, func(v string) { profile.Phone = v })
			updateField("City", profile.LocationCity, func(v string) { profile.LocationCity = v })
			updateField("State/Region", profile.LocationRegion, func(v string) { profile.LocationRegion = v })
			updateField("Country", profile.LocationCountry, func(v string) { profile.LocationCountry = v })
			updateField("Time zone (e.g. Africa/Lagos)", suggestedTimeZone(*profile), func(v string) { profile.TimeZone = strings.TrimSpace(v) })
			updateField("Summary", profile.Summary, func(v string) { profile.Summary = v })
			updateField("Skills (comma separated)", profile.RenderSkills(),
				func(v string) { profile.Skills = parseSkills(v) })
			updateField("Remote preference (remote|hybrid|onsite, blank to leave unsaid)",
				string(profile.RemotePreference), func(v string) {
					if preference := domain.RemotePreference(strings.ToLower(strings.TrimSpace(v))); preference.IsValid() {
						profile.RemotePreference = preference
						return
					}
					fmt.Printf("⚠️  %q is not a remote preference — leaving it unchanged\n", v)
				})

			if err := client.UpdateProfile(ctx, profile); err != nil {
				return fmt.Errorf("updating profile: %w", err)
			}
			fmt.Println("✅ Profile updated successfully.")
			return nil
		},
	})

	return cmd
}

// describeTimeZone is a profile's time zone with its offset.
func describeTimeZone(id string) string {
	zone, ok := geo.TimeZoneByID(id)
	if !ok {
		return "not set"
	}
	return fmt.Sprintf("%s (%s)", zone.ID, domain.FormatOffset(zone.Offset))
}

// suggestedTimeZone is the profile's zone, or its country's when the
// country has only one (ADR-0025 §2).
func suggestedTimeZone(profile domain.Candidate) string {
	if profile.TimeZone != "" {
		return profile.TimeZone
	}
	if code, ok := geo.ResolveCountry(profile.LocationCountry); ok {
		if zones := geo.TimeZonesOf(code); len(zones) == 1 {
			return zones[0].ID
		}
	}
	return ""
}
