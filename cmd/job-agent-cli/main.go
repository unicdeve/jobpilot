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
	var rootCmd = &cobra.Command{Use: "job-agent-cli"}

	// Dependencies
	dir, err := system.GetDataDir()
	if err != nil {
		log.Fatalf("Error getting data dir: %v", err)
	}
	dbPath := fmt.Sprintf("%s/job-agent.db", dir)
	db, err := sqlite.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	candRepo := sqlite.NewCandidateRepository(db.Conn())
	resRepo := sqlite.NewResumeRepository(db.Conn())
	jobRepo := sqlite.NewJobRepository(db.Conn())
	matchRepo := sqlite.NewMatchRepository(db.Conn())
	llmProv := llm.NewOllamaProvider("http://localhost:11434")

	mockSource := discovery.NewMockJobSource()
	sources := []ports.JobSource{mockSource}

	onboardingSvc := services.NewOnboardingService(candRepo, resRepo, llmProv, db)
	profileSvc := services.NewProfileService(candRepo)
	resumeSvc := services.NewResumeService(resRepo)
	discoverySvc := services.NewJobDiscoveryService(jobRepo, sources)
	matchingSvc := services.NewMatchingService(matchRepo, jobRepo, candRepo, resRepo, llmProv)

	rootCmd.AddCommand(newOnboardCmd(onboardingSvc))
	rootCmd.AddCommand(newProfileCmd(profileSvc, candRepo))
	rootCmd.AddCommand(newResumeCmd(resumeSvc, candRepo))
	rootCmd.AddCommand(newSearchCmd(discoverySvc))
	rootCmd.AddCommand(newMatchCmd(matchingSvc, candRepo))

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func newOnboardCmd(svc *services.OnboardingService) *cobra.Command {
	return &cobra.Command{
		Use:   "onboard",
		Short: "Initialize a new Job Agent installation",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()

			fmt.Println("🚀 Starting onboarding...")
			dir, err := system.EnsureDataDir()
			if err != nil {
				log.Fatalf("Error initializing data directory: %v", err)
			}
			fmt.Printf("✅ Data directory initialized at: %s\n", dir)

			if _, err := svc.SetupEnvironment(ctx); err != nil {
				log.Fatalf("Error initializing schema: %v", err)
			}
			fmt.Println("✅ Database schema initialized.")

			fmt.Println("\n--- LLM Configuration ---")
			if err := svc.ConfigureLLM(ctx, "ollama"); err != nil {
				fmt.Printf("❌ LLM connectivity check failed: %v\n", err)
				fmt.Println("Please ensure Ollama is running on http://localhost:11434")
				os.Exit(1)
			}
			fmt.Println("✅ Ollama connectivity verified.")

			fmt.Println("\n--- Candidate Profile ---")
			profile := domain.Candidate{}

			namePrompt := promptui.Prompt{Label: "Full Name"}
			name, err := namePrompt.Run()
			if err != nil {
				log.Fatalf("Error prompting for name: %v", err)
			}
			profile.Name = name

			contactPrompt := promptui.Prompt{Label: "Contact (Email/Phone)"}
			contact, err := contactPrompt.Run()
			if err != nil {
				log.Fatalf("Error prompting for contact: %v", err)
			}
			profile.Contact = contact

			locationPrompt := promptui.Prompt{Label: "Location"}
			location, err := locationPrompt.Run()
			if err != nil {
				log.Fatalf("Error prompting for location: %v", err)
			}
			profile.Location = location

			if err := svc.InitializeProfile(ctx, profile); err != nil {
				log.Fatalf("Error saving profile: %v", err)
			}
			fmt.Println("✅ Candidate profile created.")

			fmt.Println("\n--- Resume Import (Optional) ---")
			resumePrompt := promptui.Prompt{Label: "Path to resume file (leave empty to skip)"}
			resumePath, err := resumePrompt.Run()
			if err != nil {
				log.Fatalf("Error prompting for resume path: %v", err)
			}

			if resumePath != "" {
				if err := svc.ProcessResume(ctx, profile.ID, resumePath); err != nil {
					fmt.Printf("❌ Error processing resume: %v\n", err)
				} else {
					fmt.Println("✅ Resume imported and converted successfully.")
				}
			}

			fmt.Println("\n🎉 Onboarding complete! You can now use Job Agent.")
		},
	}
}

func newProfileCmd(svc *services.ProfileService, repo *sqlite.CandidateRepository) *cobra.Command {
	cmd := &cobra.Command{Use: "profile", Short: "Manage candidate profile"}

	cmd.AddCommand(&cobra.Command{
		Use:   "get",
		Short: "View current profile",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()
			profile, err := repo.GetFirstCandidate(ctx)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			fmt.Printf("Name: %s\nContact: %s\nLocation: %s\nSummary: %s\nSkills: %s\n",
				profile.Name, profile.Contact, profile.Location, profile.Summary, profile.Skills)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "update",
		Short: "Update current profile",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()
			profile, err := repo.GetFirstCandidate(ctx)
			if err != nil {
				fmt.Printf("Error: No profile found. Run 'onboard' first.\n")
				return
			}

			fmt.Println("Updating profile (leave blank to keep current value):")

			updateField := func(label string, current string, setter func(string)) {
				prompt := promptui.Prompt{
					Label: label,
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
				fmt.Printf("Error updating profile: %v\n", err)
				return
			}
			fmt.Println("✅ Profile updated successfully.")
		},
	})

	return cmd
}

func newResumeCmd(svc *services.ResumeService, repo *sqlite.CandidateRepository) *cobra.Command {
	cmd := &cobra.Command{Use: "resume", Short: "Manage resumes"}

	cmd.AddCommand(&cobra.Command{
		Use:   "import [path]",
		Short: "Import a resume file",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()
			path := args[0]

			profile, err := repo.GetFirstCandidate(ctx)
			if err != nil {
				fmt.Printf("Error: No candidate profile found. Please run 'onboard' first.\n")
				return
			}

			resume, err := svc.ImportResume(ctx, profile.ID, path)
			if err != nil {
				fmt.Printf("Error importing resume: %v\n", err)
				return
			}
			fmt.Printf("✅ Resume imported: %s (ID: %s)\n", resume.Name, resume.ID)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all imported resumes",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()
			profile, err := repo.GetFirstCandidate(ctx)
			if err != nil {
				fmt.Printf("Error: No candidate profile found.\n")
				return
			}

			resumes, err := svc.GetResumesByCandidate(ctx, profile.ID)
			if err != nil {
				fmt.Printf("Error listing resumes: %v\n", err)
				return
			}

			if len(resumes) == 0 {
				fmt.Println("No resumes imported yet.")
				return
			}

			fmt.Println("Imported Resumes:")
			for i, r := range resumes {
				fmt.Printf("%d. %s (ID: %s)\n", i+1, r.Name, r.ID)
			}
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "delete [id]",
		Short: "Delete a resume by ID",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()
			id := args[0]

			if err := svc.DeleteResume(ctx, id); err != nil {
				fmt.Printf("Error deleting resume: %v\n", err)
				return
			}
			fmt.Println("✅ Resume deleted successfully.")
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
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()
			keywords := args[0]

			criteria := domain.SearchCriteria{
				Keywords: keywords,
			}

			jobs, err := svc.Search(ctx, criteria)
			if err != nil {
				fmt.Printf("Error searching for jobs: %v\n", err)
				return
			}

			if len(jobs) == 0 {
				fmt.Println("No jobs found matching your criteria.")
				return
			}

			fmt.Printf("Found %d jobs:\n", len(jobs))
			for i, j := range jobs {
				fmt.Printf("%d. %s at %s (ID: %s)\n", i+1, j.Title, j.CompanyName, j.ID)
			}
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all discovered jobs",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()
			jobs, err := svc.Search(ctx, domain.SearchCriteria{})
			if err != nil {
				fmt.Printf("Error listing jobs: %v\n", err)
				return
			}

			if len(jobs) == 0 {
				fmt.Println("No jobs discovered yet.")
				return
			}

			fmt.Printf("Discovered Jobs:\n")
			for i, j := range jobs {
				fmt.Printf("%d. %s at %s (ID: %s)\n", i+1, j.Title, j.CompanyName, j.ID)
			}
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "import [url]",
		Short: "Import a job from a specific URL",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()
			url := args[0]

			job, err := svc.ImportJob(ctx, url)
			if err != nil {
				fmt.Printf("Error importing job: %v\n", err)
				return
			}
			fmt.Printf("✅ Job imported: %s at %s (ID: %s)\n", job.Title, job.CompanyName, job.ID)
		},
	})

	return cmd
}

func newMatchCmd(svc *services.MatchingService, repo *sqlite.CandidateRepository) *cobra.Command {
	cmd := &cobra.Command{Use: "match", Short: "Analyze job fit"}

	cmd.AddCommand(&cobra.Command{
		Use:   "analyze [job-id]",
		Short: "Analyze how well a job matches the candidate",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()
			jobID := args[0]

			profile, err := repo.GetFirstCandidate(ctx)
			if err != nil {
				fmt.Printf("Error: No candidate profile found. Please run 'onboard' first.\n")
				return
			}

			match, err := svc.AnalyzeJobForCandidate(ctx, profile.ID, jobID)
			if err != nil {
				fmt.Printf("Error analyzing job: %v\n", err)
				return
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
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all match analyses",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()
			profile, err := repo.GetFirstCandidate(ctx)
			if err != nil {
				fmt.Printf("Error: No candidate profile found.\n")
				return
			}

			matches, err := svc.ListMatches(ctx, profile.ID)
			if err != nil {
				fmt.Printf("Error listing matches: %v\n", err)
				return
			}

			if len(matches) == 0 {
				fmt.Println("No matches analyzed yet.")
				return
			}

			fmt.Println("Match Analyses:")
			for i, m := range matches {
				fmt.Printf("%d. Job ID: %s | Score: %d/100 | Confidence: %.2f\n", i+1, m.JobID, m.Score, m.Confidence)
			}
		},
	})

	return cmd
}
