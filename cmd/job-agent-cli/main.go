// Command job-agent-cli is the Job Agent command-line interface: onboarding,
// profile/resume management, job discovery, and match analysis.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/unicdev/job-agent/internal/adapters/secondary/discovery"
	"github.com/unicdev/job-agent/internal/adapters/secondary/llm"
	"github.com/unicdev/job-agent/internal/adapters/secondary/sqlite"
	"github.com/unicdev/job-agent/internal/adapters/secondary/system"
	"github.com/unicdev/job-agent/internal/core/domain"
	"github.com/unicdev/job-agent/internal/core/ports"
	"github.com/unicdev/job-agent/internal/core/services"
)

func main() {
	// rootCmd.Execute() is the single place errors from any command are
	// printed and turned into a process exit code (below); silence Cobra's
	// own error/usage printing so each error is reported exactly once.
	var rootCmd = &cobra.Command{Use: "job-agent-cli", SilenceUsage: true, SilenceErrors: true}

	// Dependencies
	dir, err := system.EnsureDataDir()
	if err != nil {
		log.Fatalf("Error creating data dir: %v", err)
	}
	dbPath := fmt.Sprintf("%s/job-agent.db", dir)
	db, err := sqlite.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("error closing database: %v", err)
		}
	}()

	// Migrations must run on every startup, not just during `onboard`:
	// otherwise a database created before a schema change was added stays
	// stale forever for any user who doesn't happen to re-run onboarding.
	if err := db.InitializeSchema(context.Background()); err != nil {
		log.Fatalf("Error initializing database schema: %v", err)
	}

	candRepo := sqlite.NewCandidateRepository(db.Conn())
	resRepo := sqlite.NewResumeRepository(db.Conn())
	jobRepo := sqlite.NewJobRepository(db.Conn())
	matchRepo := sqlite.NewMatchRepository(db.Conn())
	llmProv := llm.NewOllamaProvider("http://localhost:11434", llm.DefaultModel)

	mockSource := discovery.NewMockJobSource()
	sources := []ports.JobSource{mockSource}

	resumeSvc := services.NewResumeService(resRepo)
	onboardingSvc := services.NewOnboardingService(candRepo, resumeSvc, llmProv, db)
	profileSvc := services.NewProfileService(candRepo)
	discoverySvc := services.NewJobDiscoveryService(jobRepo, sources)
	matchingSvc := services.NewMatchingService(matchRepo, jobRepo, candRepo, resRepo, llmProv)

	rootCmd.AddCommand(newOnboardCmd(onboardingSvc))
	rootCmd.AddCommand(newProfileCmd(profileSvc, candRepo))
	rootCmd.AddCommand(newResumeCmd(resumeSvc, candRepo))
	rootCmd.AddCommand(newSearchCmd(discoverySvc))
	rootCmd.AddCommand(newMatchCmd(matchingSvc, candRepo))

	// This is the only place a command error is printed and turned into a
	// non-zero exit code. Using RunE (rather than log.Fatalf/os.Exit inside
	// a command's handler) means every error surfaces here, after this
	// function's deferred db.Close() has already been registered and can
	// still run on the way out.
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func newOnboardCmd(svc *services.OnboardingService) *cobra.Command {
	return &cobra.Command{
		Use:   "onboard",
		Short: "Initialize a new Job Agent installation",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()

			fmt.Println("🚀 Starting onboarding...")
			dir, err := system.EnsureDataDir()
			if err != nil {
				return fmt.Errorf("initializing data directory: %w", err)
			}
			fmt.Printf("✅ Data directory initialized at: %s\n", dir)

			if _, err := svc.SetupEnvironment(ctx); err != nil {
				return fmt.Errorf("initializing schema: %w", err)
			}
			fmt.Println("✅ Database schema initialized.")

			fmt.Println("\n--- LLM Configuration ---")
			if err := svc.ConfigureLLM(ctx); err != nil {
				return fmt.Errorf("LLM connectivity check failed: %w (please ensure Ollama is running on http://localhost:11434)", err)
			}
			fmt.Println("✅ Ollama connectivity verified.")

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

			if err := svc.InitializeProfile(ctx, profile); err != nil {
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
				if err := svc.ProcessResume(ctx, profile.ID, resumePath); err != nil {
					fmt.Printf("❌ Error processing resume: %v\n", err)
				} else {
					fmt.Println("✅ Resume imported and converted successfully.")
				}
			}

			fmt.Println("\n🎉 Onboarding complete! You can now use Job Agent.")
			return nil
		},
	}
}

func newProfileCmd(svc *services.ProfileService, repo ports.CandidateRepository) *cobra.Command {
	cmd := &cobra.Command{Use: "profile", Short: "Manage candidate profile"}

	cmd.AddCommand(&cobra.Command{
		Use:   "get",
		Short: "View current profile",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()
			profile, err := repo.GetFirstCandidate(ctx)
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
			profile, err := repo.GetFirstCandidate(ctx)
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

			if err := svc.UpdateProfile(ctx, profile); err != nil {
				return fmt.Errorf("updating profile: %w", err)
			}
			fmt.Println("✅ Profile updated successfully.")
			return nil
		},
	})

	return cmd
}

func newResumeCmd(svc *services.ResumeService, repo ports.CandidateRepository) *cobra.Command {
	cmd := &cobra.Command{Use: "resume", Short: "Manage resumes"}

	cmd.AddCommand(&cobra.Command{
		Use:   "import [path]",
		Short: "Import a resume file",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			path := args[0]

			profile, err := repo.GetFirstCandidate(ctx)
			if err != nil {
				return fmt.Errorf("no candidate profile found, run 'onboard' first: %w", err)
			}

			resume, err := svc.ImportResume(ctx, profile.ID, path)
			if err != nil {
				return fmt.Errorf("importing resume: %w", err)
			}
			fmt.Printf("✅ Resume imported: %s (ID: %s)\n", resume.Name, resume.ID)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all imported resumes",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx := context.Background()
			profile, err := repo.GetFirstCandidate(ctx)
			if err != nil {
				return fmt.Errorf("no candidate profile found: %w", err)
			}

			resumes, err := svc.GetResumesByCandidate(ctx, profile.ID)
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

			if err := svc.DeleteResume(ctx, id); err != nil {
				return fmt.Errorf("deleting resume: %w", err)
			}
			fmt.Println("✅ Resume deleted successfully.")
			return nil
		},
	})

	return cmd
}

func newSearchCmd(svc *services.JobDiscoveryService) *cobra.Command {
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

			jobs, err := svc.Search(ctx, criteria)
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
			jobs, err := svc.Search(ctx, domain.SearchCriteria{})
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

			job, err := svc.ImportJob(ctx, url)
			if err != nil {
				return fmt.Errorf("importing job: %w", err)
			}
			fmt.Printf("✅ Job imported: %s at %s (ID: %s)\n", job.Title, job.CompanyName, job.ID)
			return nil
		},
	})

	return cmd
}

func newMatchCmd(svc *services.MatchingService, repo ports.CandidateRepository) *cobra.Command {
	cmd := &cobra.Command{Use: "match", Short: "Analyze job fit"}

	cmd.AddCommand(&cobra.Command{
		Use:   "analyze [job-id]",
		Short: "Analyze how well a job matches the candidate",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ctx := context.Background()
			jobID := args[0]

			profile, err := repo.GetFirstCandidate(ctx)
			if err != nil {
				return fmt.Errorf("no candidate profile found, run 'onboard' first: %w", err)
			}

			match, err := svc.AnalyzeJobForCandidate(ctx, profile.ID, jobID)
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
			profile, err := repo.GetFirstCandidate(ctx)
			if err != nil {
				return fmt.Errorf("no candidate profile found: %w", err)
			}

			matches, err := svc.ListMatches(ctx, profile.ID)
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
