package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/unicdev/job-agent/internal/adapters/secondary/llm"
	"github.com/unicdev/job-agent/internal/adapters/secondary/sqlite"
	"github.com/unicdev/job-agent/internal/adapters/secondary/system"
	"github.com/unicdev/job-agent/internal/core/domain"
	"github.com/unicdev/job-agent/internal/core/services"
)

func main() {
	var rootCmd = &cobra.Command{Use: "job-agent-cli"}

	rootCmd.AddCommand(newOnboardCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func newOnboardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "onboard",
		Short: "Initialize a new Job Agent installation",
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()

			// 1. Initialize Environment
			fmt.Println("🚀 Starting onboarding...")
			dir, err := system.EnsureDataDir()
			if err != nil {
				log.Fatalf("Error initializing data directory: %v", err)
			}
			fmt.Printf("✅ Data directory initialized at: %s\n", dir)

			// Setup SQLite
			dbPath := fmt.Sprintf("%s/job-agent.db", dir)
			db, err := sqlite.NewDB(dbPath)
			if err != nil {
				log.Fatalf("Error opening database: %v", err)
			}
			defer db.Close()

			// Setup repositories and provider
			candRepo := sqlite.NewCandidateRepository(db.Conn())
			resRepo := sqlite.NewResumeRepository(db.Conn())
			llmProv := llm.NewOllamaProvider("http://localhost:11434")

			onboardingSvc := services.NewOnboardingService(candRepo, resRepo, llmProv, db)

			// Initialize schema
			if _, err := onboardingSvc.SetupEnvironment(ctx); err != nil {
				log.Fatalf("Error initializing schema: %v", err)
			}
			fmt.Println("✅ Database schema initialized.")

			// 2. Configure LLM
			fmt.Println("\n--- LLM Configuration ---")
			if err := onboardingSvc.ConfigureLLM(ctx, "ollama"); err != nil {
				fmt.Printf("❌ LLM connectivity check failed: %v\n", err)
				fmt.Println("Please ensure Ollama is running on http://localhost:11434")
				os.Exit(1)
			}
			fmt.Println("✅ Ollama connectivity verified.")

			// 3. Create Candidate Profile
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

			if err := onboardingSvc.InitializeProfile(ctx, profile); err != nil {
				log.Fatalf("Error saving profile: %v", err)
			}
			fmt.Println("✅ Candidate profile created.")

			// 4. Import Optional Resume
			fmt.Println("\n--- Resume Import (Optional) ---")
			resumePrompt := promptui.Prompt{Label: "Path to resume file (leave empty to skip)"}
			resumePath, err := resumePrompt.Run()
			if err != nil {
				log.Fatalf("Error prompting for resume path: %v", err)
			}

			if resumePath != "" {
				if err := onboardingSvc.ProcessResume(ctx, profile.ID, resumePath); err != nil {
					fmt.Printf("❌ Error processing resume: %v\n", err)
				} else {
					fmt.Println("✅ Resume imported and converted successfully.")
				}
			}

			fmt.Println("\n🎉 Onboarding complete! You can now use Job Agent.")
		},
	}
}
