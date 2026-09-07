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

			namePrompt := promptui.Prompt{Label: "Full Name"}
			name, err := namePrompt.Run()
			if err != nil {
				return fmt.Errorf("prompting for name: %w", err)
			}
			profile.Name = name

			contactPrompt := promptui.Prompt{Label: "Contact (Email/Phone)"}
			contact, err := contactPrompt.Run()
			if err != nil {
				return fmt.Errorf("prompting for contact: %w", err)
			}
			profile.Contact = contact

			locationPrompt := promptui.Prompt{Label: "Location"}
			location, err := locationPrompt.Run()
			if err != nil {
				return fmt.Errorf("prompting for location: %w", err)
			}
			profile.Location = location

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

			fmt.Println("\n🎉 Onboarding complete! You can now use JobPilot.")
			return nil
		},
	}
}

// suggestProfileFromResume asks the daemon's LLM to draft a Summary and
// Skills list from resumeMarkdown, shows the draft to the user for
// review/edit, and persists whatever they accept. A resume has already been
// imported successfully by the time this runs, so any failure here (LLM
// unavailable, bad response, etc.) is reported as a warning rather than an
// error — it must never undo or block the import.
func suggestProfileFromResume(ctx context.Context, client *httpapi.Client, resumeMarkdown string, profile *domain.Candidate) {
	draft, err := client.ProposeProfileDraft(ctx, resumeMarkdown)
	if err != nil {
		fmt.Printf("⚠️  Could not generate profile suggestions from resume: %v\n", err)
		return
	}

	fmt.Println("\n📝 Suggested profile details from resume (edit or accept):")

	updateField := func(label string, current string, setter func(string)) {
		prompt := promptui.Prompt{Label: label, Default: current}
		val, err := prompt.Run()
		if err == nil {
			setter(val)
		}
	}

	updateField("Summary", draft.Summary, func(v string) { profile.Summary = v })
	updateField("Skills", strings.Join(draft.Skills, ", "), func(v string) { profile.Skills = v })

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
			fmt.Printf("Name: %s\nContact: %s\nLocation: %s\nSummary: %s\nSkills: %s\n",
				profile.Name, profile.Contact, profile.Location, profile.Summary, profile.Skills)
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

			updateField("Name", profile.Name, func(v string) { profile.Name = v })
			updateField("Contact", profile.Contact, func(v string) { profile.Contact = v })
			updateField("Location", profile.Location, func(v string) { profile.Location = v })
			updateField("Summary", profile.Summary, func(v string) { profile.Summary = v })
			updateField("Skills", profile.Skills, func(v string) { profile.Skills = v })
			updateField("Salary Preferences", profile.SalaryPreferences, func(v string) { profile.SalaryPreferences = v })
			updateField("Work Authorization", profile.WorkAuthorization, func(v string) { profile.WorkAuthorization = v })
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

	cmd.AddCommand(&cobra.Command{
		Use:   "run [keywords]",
		Short: "Search for jobs using keywords",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			keywords := args[0]

			criteria := domain.SearchCriteria{
				Keywords: keywords,
			}

			jobs, err := client.SearchJobs(ctx, criteria)
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
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all discovered jobs",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()
			jobs, err := client.SearchJobs(ctx, domain.SearchCriteria{})
			if err != nil {
				return fmt.Errorf("listing jobs: %w", err)
			}

			if len(jobs) == 0 {
				fmt.Println("No jobs discovered yet.")
				return nil
			}

			fmt.Printf("Discovered Jobs:\n")
			for i, j := range jobs {
				fmt.Printf("%d. %s at %s (ID: %s)\n", i+1, j.Title, j.CompanyName, j.ID)
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
