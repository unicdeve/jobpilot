// Command jobpilot-cli is the JobPilot command-line interface: onboarding,
// profile/resume management, job discovery, and match analysis. It is a
// thin client of the jobpilot daemon (see cmd/jobpilot), which owns the
// database and every core service; this binary talks to it over the
// loopback HTTP API in internal/adapters/primary/httpapi, starting it if
// it isn't already running.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/llm"
	"github.com/unicdev/jobpilot/internal/core/domain"
)

func main() {
	// rootCmd.Execute() is the single place errors from any command are
	// printed and turned into a process exit code (below); silence Cobra's
	// own error/usage printing so each error is reported exactly once.
	var rootCmd = &cobra.Command{Use: "jobpilot-cli", SilenceUsage: true, SilenceErrors: true}

	ctx := context.Background()

	client, err := ensureDaemon(ctx)
	if err != nil {
		log.Fatalf("Error connecting to jobpilot daemon: %v", err)
	}

	llmSettings, err := client.GetLLMSettings(ctx)
	if err != nil {
		log.Fatalf("Error loading LLM settings: %v", err)
	}
	resolvedLLMSettings := llm.ResolveSettings(*llmSettings)

	rootCmd.AddCommand(newOnboardCmd(client, resolvedLLMSettings))
	rootCmd.AddCommand(newProfileCmd(client))
	rootCmd.AddCommand(newResumeCmd(client))
	rootCmd.AddCommand(newSearchCmd(client))
	rootCmd.AddCommand(newMatchCmd(client))
	rootCmd.AddCommand(newSettingsCmd(client))
	rootCmd.AddCommand(newApplicationCmd(client))
	rootCmd.AddCommand(newScoutCmd(client))
	rootCmd.AddCommand(newCredentialCmd(client))

	// This is the only place a command error is printed and turned into a
	// non-zero exit code, via RunE (rather than log.Fatalf/os.Exit inside a
	// command's handler).
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

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

func newProfileCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "profile", Short: "Manage candidate profile"}

	cmd.AddCommand(&cobra.Command{
		Use:   "get",
		Short: "View current profile",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()
			profile, err := client.GetProfile(ctx)
			if err != nil {
				return err
			}
			fmt.Printf("First Name: %s\nLast Name: %s\nEmail: %s\nPhone: %s\nLocation: %s\nSummary: %s\nSkills: %s\n",
				profile.FirstName, profile.LastName, profile.Email, profile.Phone,
				profile.LocationDisplay(), profile.Summary, profile.RenderSkills())
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
			updateField("Application Preferences", profile.ApplicationPreferences, func(v string) { profile.ApplicationPreferences = v })

			if err := client.UpdateProfile(ctx, profile); err != nil {
				return fmt.Errorf("updating profile: %w", err)
			}
			fmt.Println("✅ Profile updated successfully.")
			return nil
		},
	})

	return cmd
}

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

	cmd.AddCommand(&cobra.Command{
		Use:   "delete [id]",
		Short: "Delete a resume by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			id := args[0]

			if err := client.DeleteResume(ctx, id); err != nil {
				return fmt.Errorf("deleting resume: %w", err)
			}
			fmt.Println("✅ Resume deleted successfully.")
			return nil
		},
	})

	return cmd
}

func newSearchCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "search", Short: "Discover jobs"}

	runCmd := &cobra.Command{
		Use:   "run [keywords]",
		Short: "Discover jobs, from a saved search or ad-hoc keywords",
		Long: "Runs discovery against every configured job source.\n\n" +
			"With no arguments it runs your default saved search. Pass --saved to run a\n" +
			"different one, or give keywords for a one-off search. Discovery always needs\n" +
			"criteria: the public job boards do no filtering of their own, so a search with\n" +
			"nothing set would import every posting on every board.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			saved, err := cmd.Flags().GetString("saved")
			if err != nil {
				return err
			}

			var jobs []*domain.Job
			switch {
			case saved != "" && len(args) > 0:
				return fmt.Errorf("give either --saved or keywords, not both")

			case len(args) > 0:
				jobs, err = client.SearchJobs(ctx, domain.SearchCriteria{Keywords: strings.Join(args, " ")})

			default:
				search, resolveErr := resolveSavedSearch(ctx, client, saved)
				if resolveErr != nil {
					return resolveErr
				}
				fmt.Printf("Running saved search %q…\n", search.Name)
				jobs, err = client.RunSavedSearch(ctx, search.ID)
			}
			if err != nil {
				return fmt.Errorf("searching for jobs: %w", err)
			}

			if len(jobs) == 0 {
				fmt.Println("No jobs found matching your criteria.")
				return nil
			}

			fmt.Printf("Found %d jobs:\n", len(jobs))
			for i, j := range jobs {
				fmt.Printf("%d. %s at %s (ID: %s)\n", i+1, j.Title, j.CompanyName, j.ID)
			}
			return nil
		},
	}
	runCmd.Flags().String("saved", "", "Name or ID of a saved search to run (default: your default saved search)")
	cmd.AddCommand(runCmd)

	cmd.AddCommand(newSavedSearchCmd(client))
	cmd.AddCommand(newSearchTemplateCmd(client))

	// "list" reads stored discovery history and contacts no job source, so
	// it stays usable offline. It previously ran an empty live search, which
	// re-queried every source and could not work without internet access.
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List previously discovered jobs from local history",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := context.Background()

			keywords, err := cmd.Flags().GetString("keywords")
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

			page, err := client.ListJobs(ctx, keywords, limit, offset)
			if err != nil {
				return fmt.Errorf("listing jobs: %w", err)
			}

			if len(page.Jobs) == 0 {
				if offset > 0 {
					fmt.Printf("No jobs at offset %d (%d stored in total).\n", offset, page.Total)
					return nil
				}
				fmt.Println("No jobs discovered yet. Run 'search run <keywords>' to discover some.")
				return nil
			}

			fmt.Printf("Discovered jobs %d-%d of %d:\n", offset+1, offset+len(page.Jobs), page.Total)
			for i, j := range page.Jobs {
				fmt.Printf("%d. %s at %s (ID: %s)\n", offset+i+1, j.Title, j.CompanyName, j.ID)
			}

			if shown := offset + len(page.Jobs); shown < page.Total {
				fmt.Printf("\n%d more — use --offset %d to see the next page.\n", page.Total-shown, shown)
			}
			return nil
		},
	}
	listCmd.Flags().String("keywords", "", "Only list jobs whose title or company matches these keywords")
	listCmd.Flags().Int("limit", 0, "Maximum jobs to list (default 50, maximum 200)")
	listCmd.Flags().Int("offset", 0, "Skip this many jobs before listing")
	cmd.AddCommand(listCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "show [id]",
		Short: "Show a stored job in full",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			job, err := client.GetJob(context.Background(), args[0])
			if err != nil {
				return fmt.Errorf("getting job: %w", err)
			}

			fmt.Printf("%s at %s\n", job.Title, job.CompanyName)
			fmt.Printf("ID:       %s\n", job.ID)
			fmt.Printf("URL:      %s\n", job.CanonicalURL)
			fmt.Printf("Location: %s\n", strings.Join(job.Locations, ", "))
			fmt.Printf("Type:     %s · %s\n", job.RemoteType, job.EmploymentType)
			if len(job.Skills) > 0 {
				fmt.Printf("Skills:   %s\n", strings.Join(job.Skills, ", "))
			}
			if job.DescriptionMarkdown != "" {
				fmt.Printf("\n%s\n", job.DescriptionMarkdown)
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "import [url]",
		Short: "Import a job from a specific URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			url := args[0]

			job, err := client.ImportJob(ctx, url)
			if err != nil {
				return fmt.Errorf("importing job: %w", err)
			}
			fmt.Printf("✅ Job imported: %s at %s (ID: %s)\n", job.Title, job.CompanyName, job.ID)
			return nil
		},
	})

	return cmd
}

func newMatchCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "match", Short: "Analyze job fit"}

	cmd.AddCommand(&cobra.Command{
		Use:   "analyze [job-id]",
		Short: "Analyze how well a job matches the candidate",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			jobID := args[0]

			profile, err := client.GetProfile(ctx)
			if err != nil {
				return fmt.Errorf("no candidate profile found, run 'onboard' first: %w", err)
			}

			match, err := client.AnalyzeMatch(ctx, profile.ID, jobID)
			if err != nil {
				return fmt.Errorf("analyzing job: %w", err)
			}

			fmt.Printf("✅ Analysis complete!\nScore: %d/100 (Confidence: %.2f)\n", match.Score, match.Confidence)
			fmt.Printf("Rationale: %s\n", match.Rationale)
			fmt.Println("\nStrengths:")
			for _, s := range match.Strengths {
				fmt.Printf("- %s\n", s)
			}
			fmt.Println("\nGaps:")
			for _, g := range match.Gaps {
				fmt.Printf("- %s\n", g)
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all match analyses",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()
			profile, err := client.GetProfile(ctx)
			if err != nil {
				return fmt.Errorf("no candidate profile found: %w", err)
			}

			matches, err := client.ListMatches(ctx, profile.ID)
			if err != nil {
				return fmt.Errorf("listing matches: %w", err)
			}

			if len(matches) == 0 {
				fmt.Println("No matches analyzed yet.")
				return nil
			}

			fmt.Println("Match Analyses:")
			for i, m := range matches {
				fmt.Printf("%d. Job ID: %s | Score: %d/100 | Confidence: %.2f\n", i+1, m.JobID, m.Score, m.Confidence)
			}
			return nil
		},
	})

	return cmd
}

func newSettingsCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "settings", Short: "Manage JobPilot settings"}

	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show the configured LLM provider and model",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()
			settings, err := client.GetLLMSettings(ctx)
			if err != nil {
				return fmt.Errorf("loading LLM settings: %w", err)
			}

			resolved := llm.ResolveSettings(*settings)
			note := ""
			if settings.Provider == "" {
				note = " (default, not yet configured)"
			}
			fmt.Printf("Provider: %s%s\nBase URL: %s\nModel: %s\n", resolved.Provider, note, resolved.BaseURL, resolved.Model)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "set-llm",
		Short: "Interactively configure the LLM provider and model",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()

			labels := make([]string, len(llm.Presets))
			for i, p := range llm.Presets {
				labels[i] = p.Label
			}
			providerSelect := promptui.Select{Label: "LLM Provider", Items: labels}
			idx, _, err := providerSelect.Run()
			if err != nil {
				return fmt.Errorf("selecting provider: %w", err)
			}
			preset := llm.Presets[idx]

			urlPrompt := promptui.Prompt{Label: "Base URL", Default: preset.DefaultBaseURL}
			baseURL, err := urlPrompt.Run()
			if err != nil {
				return fmt.Errorf("prompting for base URL: %w", err)
			}

			modelPrompt := promptui.Prompt{
				Label: "Model name",
				Validate: func(v string) error {
					if v == "" {
						return fmt.Errorf("model name is required")
					}
					return nil
				},
			}
			model, err := modelPrompt.Run()
			if err != nil {
				return fmt.Errorf("prompting for model: %w", err)
			}

			settings := domain.LLMSettings{Provider: preset.Key, BaseURL: baseURL, Model: model}
			result, err := client.SaveLLMSettings(ctx, settings)
			if err != nil {
				return fmt.Errorf("saving LLM settings: %w", err)
			}
			if result.ConnectivityError != "" {
				fmt.Printf("⚠️  Settings saved, but could not connect: %s\n", result.ConnectivityError)
				return nil
			}
			fmt.Println("✅ Settings saved and verified.")
			return nil
		},
	})

	return cmd
}

// resolveSavedSearch finds the saved search a command should use: the one
// named (by name or ID), or the default when nothing was named. It reports
// the absence of any saved search as an actionable message rather than an
// empty result, since that is the state a new installation starts in and
// discovery cannot run in it.
func resolveSavedSearch(ctx context.Context, client *httpapi.Client, nameOrID string) (*domain.SavedSearch, error) {
	searches, err := client.ListSavedSearches(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing saved searches: %w", err)
	}
	if len(searches) == 0 {
		return nil, fmt.Errorf("no saved searches yet — create one with 'search saved add', or run 'onboard' to set up your first")
	}

	if nameOrID == "" {
		for _, search := range searches {
			if search.IsDefault {
				return search, nil
			}
		}
		return nil, fmt.Errorf("no default saved search — pick one with 'search saved default <name>' or pass --saved")
	}

	for _, search := range searches {
		if strings.EqualFold(search.Name, nameOrID) || search.ID == nameOrID {
			return search, nil
		}
	}
	return nil, fmt.Errorf("no saved search named %q — 'search saved list' shows what you have", nameOrID)
}

// criteriaFlags registers the search criteria flags shared by the commands
// that build a SearchCriteria, so every surface offers the same set.
func criteriaFlags(cmd *cobra.Command) {
	cmd.Flags().String("keywords", "", "Words that must all appear in the title or description")
	cmd.Flags().String("title", "", "Words that must all appear in the job title")
	cmd.Flags().String("location", "", "Words that must all appear in the job's locations")
	cmd.Flags().String("remote", "", "Remote type to require, e.g. remote, hybrid, onsite")
	cmd.Flags().String("employment", "", "Employment type to require, e.g. full_time, contract")
	cmd.Flags().Float64("salary-min", 0, "Lowest acceptable advertised salary (jobs without a salary are kept)")
	cmd.Flags().Float64("salary-max", 0, "Highest acceptable advertised salary (jobs without a salary are kept)")
	cmd.Flags().StringSlice("exclude-company", nil, "Company to exclude; repeatable")
}

// criteriaFromFlags reads the flags registered by criteriaFlags.
func criteriaFromFlags(cmd *cobra.Command) (domain.SearchCriteria, error) {
	var c domain.SearchCriteria
	var err error

	if c.Keywords, err = cmd.Flags().GetString("keywords"); err != nil {
		return c, err
	}
	if c.Title, err = cmd.Flags().GetString("title"); err != nil {
		return c, err
	}
	if c.Location, err = cmd.Flags().GetString("location"); err != nil {
		return c, err
	}
	if c.RemotePreference, err = cmd.Flags().GetString("remote"); err != nil {
		return c, err
	}
	if c.EmploymentType, err = cmd.Flags().GetString("employment"); err != nil {
		return c, err
	}
	if c.SalaryMin, err = cmd.Flags().GetFloat64("salary-min"); err != nil {
		return c, err
	}
	if c.SalaryMax, err = cmd.Flags().GetFloat64("salary-max"); err != nil {
		return c, err
	}
	if c.ExcludedCompanies, err = cmd.Flags().GetStringSlice("exclude-company"); err != nil {
		return c, err
	}
	return c, nil
}

// describeCriteria renders the criteria that are set, for listing commands.
func describeCriteria(c domain.SearchCriteria) string {
	parts := make([]string, 0, 8)
	add := func(label, value string) {
		if strings.TrimSpace(value) != "" {
			parts = append(parts, label+"="+value)
		}
	}

	add("keywords", c.Keywords)
	add("title", c.Title)
	add("location", c.Location)
	add("remote", c.RemotePreference)
	add("employment", c.EmploymentType)
	if c.SalaryMin > 0 {
		parts = append(parts, fmt.Sprintf("salary>=%.0f", c.SalaryMin))
	}
	if c.SalaryMax > 0 {
		parts = append(parts, fmt.Sprintf("salary<=%.0f", c.SalaryMax))
	}
	if len(c.ExcludedCompanies) > 0 {
		parts = append(parts, "excluding "+strings.Join(c.ExcludedCompanies, ", "))
	}
	return strings.Join(parts, "  ")
}

// newSearchTemplateCmd exposes the ready-made searches. The same catalog
// backs the desktop application, served by the daemon, so the two cannot
// offer different templates under the same name.
func newSearchTemplateCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "templates",
		Short: "Ready-made searches you can adopt and edit",
		Long: "Search criteria are matched strictly: every word you set must appear, and\n" +
			"keywords are matched against the whole job description rather than the title.\n" +
			"A search that reads well can therefore return almost nothing. These templates\n" +
			"are criteria that have been measured against live postings from every source.",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "Show the available templates",
		RunE: func(_ *cobra.Command, _ []string) error {
			groups, err := client.ListSearchTemplates(context.Background())
			if err != nil {
				return fmt.Errorf("listing search templates: %w", err)
			}

			for _, group := range groups {
				fmt.Printf("\n%s\n  %s\n\n", group.Name, group.Summary)
				for _, template := range group.Templates {
					fmt.Printf("  %-32s %s\n", template.ID, template.Name)
					fmt.Printf("  %-32s %s\n", "", describeCriteria(template.Criteria))
					fmt.Printf("  %-32s %s\n\n", "", template.Summary)
				}
			}
			fmt.Println("Adopt some with 'search templates import <id> [<id>...]'.")
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "import [id...]",
		Short: "Create a saved search from each named template",
		Long: "The searches created are ordinary saved searches. Edit, rename or delete them\n" +
			"as you like — nothing links them back to the template they came from.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			created, err := client.ImportSearchTemplates(context.Background(), args)
			if err != nil {
				return fmt.Errorf("importing search templates: %w", err)
			}

			for _, search := range created {
				fmt.Printf("✅ %s\n    %s\n", search.Name, describeCriteria(search.Criteria))
			}
			fmt.Printf("\nCreated %d saved %s. Run one with 'search run --saved \"%s\"'.\n",
				len(created), plural(len(created), "search", "searches"), created[0].Name)
			return nil
		},
	})

	return cmd
}

// plural picks the right word for n, so a count of one does not read as
// "1 searches".
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// newCredentialCmd manages API keys for job sources.
//
// There is no "show" or "get": a credential goes in and never comes out
// (ADR-0019 §4). The daemon has no authentication, so a route that returned
// a secret would make it readable by any local process.
func newCredentialCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "credentials",
		Short: "API keys for job sources that need one",
		Long: "Keys are kept in your operating system's keychain, never in JobPilot's\n" +
			"database. Once stored, a key cannot be read back — not by the desktop app,\n" +
			"not by this CLI, and not by anything else on your machine talking to the\n" +
			"daemon. Replace one by setting it again.",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "Show which credentials are configured, without their values",
		RunE: func(_ *cobra.Command, _ []string) error {
			refs, err := client.ListCredentials(context.Background())
			if err != nil {
				return fmt.Errorf("listing credentials: %w", err)
			}
			if len(refs) == 0 {
				fmt.Println("No credentials configured. Add one with 'credentials set <source>'.")
				return nil
			}

			for _, ref := range refs {
				fmt.Printf("  %s\n", ref.String())
			}
			fmt.Println("\nValues are never shown. Replace one by setting it again.")
			return nil
		},
	})

	setCmd := &cobra.Command{
		Use:   "set [source]",
		Short: "Store an API key for a job source",
		Long: "The key is read from a prompt that does not echo it, or from standard\n" +
			"input when that is a pipe. It is never taken as a flag, because a flag\n" +
			"would put it in your shell history and in the process list.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			label, err := cmd.Flags().GetString("label")
			if err != nil {
				return err
			}

			secret, err := readSecret("API key")
			if err != nil {
				return err
			}

			ref, err := client.SetCredential(context.Background(), args[0], label, secret)
			if err != nil {
				return fmt.Errorf("storing credential: %w", err)
			}
			fmt.Printf("✅ Stored %s in your keychain. It cannot be shown again.\n", ref.String())
			return nil
		},
	}
	setCmd.Flags().String("label", "", "Name this key, to hold more than one for a source (default: \"default\")")
	cmd.AddCommand(setCmd)

	deleteCmd := &cobra.Command{
		Use:   "delete [source]",
		Short: "Remove a stored API key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			label, err := cmd.Flags().GetString("label")
			if err != nil {
				return err
			}
			if err := client.DeleteCredential(context.Background(), args[0], label); err != nil {
				return fmt.Errorf("deleting credential: %w", err)
			}
			fmt.Println("✅ Removed.")
			return nil
		},
	}
	deleteCmd.Flags().String("label", "", "Which key to remove (default: \"default\")")
	cmd.AddCommand(deleteCmd)

	return cmd
}

// readSecret reads a secret without echoing it, or from standard input when
// that is a pipe, so a key can be stored from a script without appearing on
// a command line.
func readSecret(label string) (string, error) {
	info, err := os.Stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("reading standard input: %w", err)
	}

	if info.Mode()&os.ModeCharDevice == 0 {
		piped, readErr := io.ReadAll(os.Stdin)
		if readErr != nil {
			return "", fmt.Errorf("reading the key from standard input: %w", readErr)
		}
		secret := strings.TrimRight(string(piped), "\r\n")
		if secret == "" {
			return "", fmt.Errorf("nothing was piped in")
		}
		return secret, nil
	}

	prompt := promptui.Prompt{Label: label, Mask: '*'}
	secret, err := prompt.Run()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(secret) == "" {
		return "", fmt.Errorf("an empty key cannot be stored")
	}
	return secret, nil
}

func newSavedSearchCmd(client *httpapi.Client) *cobra.Command {
	cmd := &cobra.Command{Use: "saved", Short: "Manage saved searches"}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List saved searches",
		RunE: func(_ *cobra.Command, _ []string) error {
			searches, err := client.ListSavedSearches(context.Background())
			if err != nil {
				return fmt.Errorf("listing saved searches: %w", err)
			}
			if len(searches) == 0 {
				fmt.Println("No saved searches yet. Add one with 'search saved add <name> --keywords ...'.")
				return nil
			}

			for _, search := range searches {
				marker := " "
				if search.IsDefault {
					marker = "*"
				}
				fmt.Printf("%s %s\n    %s\n    ID: %s\n", marker, search.Name, describeCriteria(search.Criteria), search.ID)
			}
			fmt.Println("\n* = default, used by 'search run' when no search is named.")
			return nil
		},
	})

	addCmd := &cobra.Command{
		Use:   "add [name]",
		Short: "Save a new named search",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			criteria, err := criteriaFromFlags(cmd)
			if err != nil {
				return err
			}

			search, err := client.CreateSavedSearch(context.Background(), args[0], criteria)
			if err != nil {
				return fmt.Errorf("saving search: %w", err)
			}

			fmt.Printf("✅ Saved search %q: %s\n", search.Name, describeCriteria(search.Criteria))
			if search.IsDefault {
				fmt.Println("   It is your default, so 'search run' will use it.")
			}
			return nil
		},
	}
	criteriaFlags(addCmd)
	cmd.AddCommand(addCmd)

	editCmd := &cobra.Command{
		Use:   "edit [name-or-id]",
		Short: "Replace a saved search's criteria",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			existing, err := resolveSavedSearch(ctx, client, args[0])
			if err != nil {
				return err
			}

			criteria, err := criteriaFromFlags(cmd)
			if err != nil {
				return err
			}
			name, err := cmd.Flags().GetString("name")
			if err != nil {
				return err
			}
			if name == "" {
				name = existing.Name
			}

			updated, err := client.UpdateSavedSearch(ctx, existing.ID, name, criteria)
			if err != nil {
				return fmt.Errorf("updating saved search: %w", err)
			}
			fmt.Printf("✅ Updated %q: %s\n", updated.Name, describeCriteria(updated.Criteria))
			return nil
		},
	}
	criteriaFlags(editCmd)
	editCmd.Flags().String("name", "", "Rename the saved search")
	cmd.AddCommand(editCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "delete [name-or-id]",
		Short: "Delete a saved search",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			search, err := resolveSavedSearch(ctx, client, args[0])
			if err != nil {
				return err
			}
			if err := client.DeleteSavedSearch(ctx, search.ID); err != nil {
				return fmt.Errorf("deleting saved search: %w", err)
			}
			fmt.Printf("✅ Deleted saved search %q.\n", search.Name)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "default [name-or-id]",
		Short: "Choose the search 'search run' uses by default",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			search, err := resolveSavedSearch(ctx, client, args[0])
			if err != nil {
				return err
			}
			updated, err := client.SetDefaultSavedSearch(ctx, search.ID)
			if err != nil {
				return fmt.Errorf("setting default saved search: %w", err)
			}
			fmt.Printf("✅ %q is now your default search.\n", updated.Name)
			return nil
		},
	})

	return cmd
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
				fmt.Printf("  %s (%s)\n    score ≥ %d, up to %d per run, from %s\n    ID: %s\n",
					scout.Name, state, scout.MinMatchScore, scout.MaxApplicationsPerRun,
					scout.Source, scout.ID)
				if scout.LastRunAt != nil {
					fmt.Printf("    last run: %s\n", scout.LastRunAt.Format(time.RFC1123))
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

			scout, err := client.CreateScout(context.Background(), &domain.Scout{
				Name:                  args[0],
				SavedSearchID:         searchID,
				MinMatchScore:         minScore,
				MaxApplicationsPerRun: perRun,
				Source:                domain.ScoutSource(source),
				IsEnabled:             true,
			})
			if err != nil {
				return fmt.Errorf("creating scout: %w", err)
			}

			fmt.Printf("✅ Scout %q created.\n   Run it with 'scout run %s'.\n", scout.Name, scout.ID)
			return nil
		},
	}
	addCmd.Flags().String("search", "", "ID of the saved search this scout runs (required)")
	addCmd.Flags().Int("min-score", 70, "lowest match score that earns an application")
	addCmd.Flags().Int("max-per-run", 10, "most applications one run may create")
	addCmd.Flags().String("source", string(domain.ScoutSourceLocal),
		"'local' to consider stored jobs, 'sources' to fetch from the job boards first")
	_ = addCmd.MarkFlagRequired("search")
	cmd.AddCommand(addCmd)

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
				fmt.Printf("  %s  %s  %s\n", run.StartedAt.Format(time.RFC1123), run.Status, run.ID)
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
