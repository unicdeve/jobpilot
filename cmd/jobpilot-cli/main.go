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
	"log"
	"os"
	"strings"

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
	updateField("Email", orCurrent(draft.Contact, profile.Email), func(v string) { profile.Email = v })
	updateField("City", orCurrent(draft.Location, profile.LocationCity), func(v string) { profile.LocationCity = v })
	updateField("Summary", orCurrent(draft.Summary, profile.Summary), func(v string) { profile.Summary = v })
	updateField("Skills", orCurrent(strings.Join(draft.Skills, ", "), profile.Skills), func(v string) { profile.Skills = v })

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
				profile.LocationDisplay(), profile.Summary, profile.Skills)
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
			updateField("Skills", profile.Skills, func(v string) { profile.Skills = v })
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
