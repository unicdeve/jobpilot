package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
	"github.com/unicdev/jobpilot/internal/core/domain"
)

func newOnboardCmd(client *httpapi.Client, llmSettings domain.LLMSettings) *cobra.Command {
	return &cobra.Command{
		Use:   "onboard",
		Short: "Initialize a new JobPilot installation",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()

			fmt.Println("🚀 Starting onboarding...")
			dir, err := client.SetupEnvironment(ctx)
			if err != nil {
				return fmt.Errorf("initializing schema: %w", err)
			}
			fmt.Printf("✅ Data directory initialized at: %s\n", dir)
			fmt.Println("✅ Database schema initialized.")

			fmt.Println("\n--- LLM Configuration ---")
			if err := client.ConfigureLLM(ctx); err != nil {
				return fmt.Errorf("LLM connectivity check failed: %w (please ensure %s is running at %s, or run 'jobpilot-cli settings set-llm' to configure a different provider)",
					err, llmSettings.Provider, llmSettings.BaseURL)
			}
			fmt.Printf("✅ %s connectivity verified.\n", llmSettings.Provider)

			fmt.Println("\n--- Candidate Profile ---")
			profile := domain.Candidate{}

			firstNamePrompt := promptui.Prompt{Label: "First Name"}
			firstName, err := firstNamePrompt.Run()
			if err != nil {
				return fmt.Errorf("prompting for first name: %w", err)
			}
			profile.FirstName = firstName

			lastNamePrompt := promptui.Prompt{Label: "Last Name"}
			lastName, err := lastNamePrompt.Run()
			if err != nil {
				return fmt.Errorf("prompting for last name: %w", err)
			}
			profile.LastName = lastName

			emailPrompt := promptui.Prompt{Label: "Email"}
			email, err := emailPrompt.Run()
			if err != nil {
				return fmt.Errorf("prompting for email: %w", err)
			}
			profile.Email = email

			phonePrompt := promptui.Prompt{Label: "Phone"}
			phone, err := phonePrompt.Run()
			if err != nil {
				return fmt.Errorf("prompting for phone: %w", err)
			}
			profile.Phone = phone

			cityPrompt := promptui.Prompt{Label: "City"}
			city, err := cityPrompt.Run()
			if err != nil {
				return fmt.Errorf("prompting for city: %w", err)
			}
			profile.LocationCity = city

			countryPrompt := promptui.Prompt{Label: "Country"}
			country, err := countryPrompt.Run()
			if err != nil {
				return fmt.Errorf("prompting for country: %w", err)
			}
			profile.LocationCountry = country

			if err := client.InitializeProfile(ctx, &profile); err != nil {
				return fmt.Errorf("saving profile: %w", err)
			}
			fmt.Println("✅ Candidate profile created.")

			fmt.Println("\n--- Resume Import (Optional) ---")
			resumePrompt := promptui.Prompt{Label: "Path to resume file (leave empty to skip)"}
			resumePath, err := resumePrompt.Run()
			if err != nil {
				return fmt.Errorf("prompting for resume path: %w", err)
			}

			if resumePath != "" {
				resume, err := client.ImportResume(ctx, profile.ID, resumePath)
				if err != nil {
					fmt.Printf("❌ Error processing resume: %v\n", err)
				} else {
					fmt.Println("✅ Resume imported and converted successfully.")
					suggestProfileFromResume(ctx, client, resume.MarkdownContent, &profile)
				}
			}

			fmt.Println("\n--- Job Search Preferences ---")
			if err := configureSearchPreferences(ctx, client); err != nil {
				return err
			}

			fmt.Println("\n🎉 Onboarding complete! You can now use JobPilot.")
			fmt.Println("   Run 'jobpilot-cli search run' to discover jobs matching your search.")
			return nil
		},
	}
}

// suggestProfileFromResume asks the daemon's LLM to draft profile fields
// (first/last name, contact, location, Summary, Skills) from resumeMarkdown,
// shows the draft to the user for review/edit, and persists whatever they
// accept. A resume has already been imported successfully by the time this
// runs, so any failure here (LLM unavailable, bad response, etc.) is
// reported as a warning rather than an error — it must never undo or block
// the import.
func suggestProfileFromResume(ctx context.Context, client *httpapi.Client, resumeMarkdown string, profile *domain.Candidate) {
	draft, err := client.ProposeProfileDraft(ctx, resumeMarkdown)
	if err != nil {
		fmt.Printf("⚠️  Could not generate profile suggestions from resume: %v\n", err)
		return
	}

	fmt.Println("\n📝 Suggested profile details from resume (edit or accept):")

	// A field the LLM didn't find in the resume comes back empty; fall back
	// to whatever's already on the profile so an unrelated blank suggestion
	// never clobbers real data.
	orCurrent := func(draftVal, current string) string {
		if draftVal != "" {
			return draftVal
		}
		return current
	}

	updateField := func(label string, current string, setter func(string)) {
		prompt := promptui.Prompt{Label: label, Default: current}
		val, err := prompt.Run()
		if err == nil {
			setter(val)
		}
	}

	updateField("First Name", orCurrent(draft.FirstName, profile.FirstName), func(v string) { profile.FirstName = v })
	updateField("Last Name", orCurrent(draft.LastName, profile.LastName), func(v string) { profile.LastName = v })
	updateField("Email", orCurrent(draft.Email, profile.Email), func(v string) { profile.Email = v })
	updateField("Phone", orCurrent(draft.Phone, profile.Phone), func(v string) { profile.Phone = v })
	updateField("City", orCurrent(draft.LocationCity, profile.LocationCity), func(v string) { profile.LocationCity = v })
	updateField("Country", orCurrent(draft.LocationCountry, profile.LocationCountry), func(v string) { profile.LocationCountry = v })
	updateField("Summary", orCurrent(draft.Summary, profile.Summary), func(v string) { profile.Summary = v })

	proposedSkills := draft.ProposedSkills()
	updateField("Skills (comma separated)", orCurrent(renderDraftSkills(proposedSkills), profile.RenderSkills()),
		func(v string) { profile.Skills = parseSkills(v) })

	// The work history and links are lists, which a prompt loop cannot
	// edit sensibly. They are proposed wholesale only into an empty
	// profile, and the user reviews them on the desktop's Profile screen —
	// which is what the message below points at.
	if len(profile.Experiences) == 0 && len(draft.Experiences) > 0 {
		profile.Experiences = draft.ProposedExperiences()
		fmt.Printf("   proposed %d roles from your resume — review them on the Profile screen\n",
			len(profile.Experiences))
	}
	if len(profile.Links) == 0 && len(draft.Links) > 0 {
		profile.Links = draft.ProposedLinks()
		fmt.Printf("   proposed %d links from your resume\n", len(profile.Links))
	}

	if err := client.UpdateProfile(ctx, profile); err != nil {
		fmt.Printf("⚠️  Could not save suggested profile updates: %v\n", err)
		return
	}
	fmt.Println("✅ Profile updated with resume suggestions.")
}

// configureSearchPreferences asks for the first saved search, completing the
// onboarding requirement in docs/02_features/01_onboarding.md.
//
// It is not optional the way resume import is: discovery refuses to run
// without criteria, because the public job boards do no filtering of their
// own and an unrestricted search would import every posting on every board.
// An installation with no saved search cannot find jobs at all, so the
// prompt repeats until it gets something usable.
func configureSearchPreferences(ctx context.Context, client *httpapi.Client) error {
	fmt.Println("JobPilot only searches for what you ask it to, so it needs to know what")
	fmt.Println("you are looking for. You can change or add more searches later with")
	fmt.Println("'jobpilot-cli search saved'.")

	for {
		namePrompt := promptui.Prompt{Label: "Name for this search (e.g. Backend Go roles)"}
		name, err := namePrompt.Run()
		if err != nil {
			return fmt.Errorf("prompting for search name: %w", err)
		}

		keywordsPrompt := promptui.Prompt{Label: "Keywords (e.g. golang backend)"}
		keywords, err := keywordsPrompt.Run()
		if err != nil {
			return fmt.Errorf("prompting for keywords: %w", err)
		}

		titlePrompt := promptui.Prompt{Label: "Job title to require (optional)"}
		title, err := titlePrompt.Run()
		if err != nil {
			return fmt.Errorf("prompting for title: %w", err)
		}

		remotePrompt := promptui.Prompt{Label: "Remote type: remote, hybrid, onsite (optional)"}
		remote, err := remotePrompt.Run()
		if err != nil {
			return fmt.Errorf("prompting for remote type: %w", err)
		}

		criteria := domain.SearchCriteria{
			Keywords:         strings.TrimSpace(keywords),
			Title:            strings.TrimSpace(title),
			RemotePreference: strings.TrimSpace(remote),
		}

		search, err := client.CreateSavedSearch(ctx, name, criteria)
		if err != nil {
			// The daemon rejects an unnamed or unrestricted search; say so
			// and ask again rather than finishing onboarding unusable.
			fmt.Printf("❌ %v\n\n", err)
			continue
		}

		fmt.Printf("✅ Saved search %q: %s\n", search.Name, describeCriteria(search.Criteria))
		return nil
	}
}

// parseSkills turns a comma-separated list into skills, which is all the
// CLI offers: years per skill and dated work history are edited in the
// desktop, where a list of entries has somewhere to live.
func parseSkills(list string) []domain.CandidateSkill {
	var skills []domain.CandidateSkill
	for _, name := range strings.Split(list, ",") {
		if name = strings.TrimSpace(name); name != "" {
			skills = append(skills, domain.CandidateSkill{Name: name})
		}
	}
	return skills
}

// renderDraftSkills is a proposed skill list as the comma-separated text
// the CLI edits.
func renderDraftSkills(skills []domain.CandidateSkill) string {
	names := make([]string, 0, len(skills))
	for _, skill := range skills {
		names = append(names, skill.Name)
	}
	return strings.Join(names, ", ")
}
