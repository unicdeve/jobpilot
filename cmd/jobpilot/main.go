// Command jobpilot is the JobPilot background daemon: it owns the SQLite
// database and every core service, and exposes them to local clients (the
// CLI, eventually the desktop app) over a loopback HTTP API. See
// docs/01_architecture/01_daemon.md and
// docs/09_decisions/0012-loopback-http-ipc.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/discovery"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/llm"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/sqlite"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/system"
	"github.com/unicdev/jobpilot/internal/core/ports"
	"github.com/unicdev/jobpilot/internal/core/services"
)

// shutdownTimeout bounds how long the daemon waits for in-flight requests to
// finish once asked to stop, per docs/01_architecture/01_daemon.md's
// graceful-shutdown requirement.
const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		log.Fatalf("jobpilot daemon: %v", err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbPath, err := system.DefaultDBPath()
	if err != nil {
		return fmt.Errorf("resolving database path: %w", err)
	}
	db, err := sqlite.NewDB(dbPath)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("error closing database: %v", err)
		}
	}()

	if err := db.InitializeSchema(ctx); err != nil {
		return fmt.Errorf("initializing database schema: %w", err)
	}

	svc, err := wireServices(db)
	if err != nil {
		return fmt.Errorf("wiring services: %w", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("binding loopback listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	if err := httpapi.WriteDiscoveryInfo(httpapi.DiscoveryInfo{PID: os.Getpid(), Port: port}); err != nil {
		return fmt.Errorf("writing discovery file: %w", err)
	}
	defer func() {
		if err := httpapi.RemoveDiscoveryInfo(); err != nil {
			log.Printf("error removing discovery file: %v", err)
		}
	}()

	server := &http.Server{Handler: httpapi.NewHandler(svc)}

	serveErr := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	log.Printf("jobpilot daemon listening on 127.0.0.1:%d (pid %d)", port, os.Getpid())

	select {
	case <-ctx.Done():
		log.Println("jobpilot daemon shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutting down HTTP server: %w", err)
		}
		return <-serveErr
	case err := <-serveErr:
		return err
	}
}

// wireServices constructs every core service the daemon exposes, exactly as
// cmd/jobpilot-cli used to wire them in-process before this package existed.
func wireServices(db *sqlite.DB) (httpapi.Services, error) {
	candRepo := sqlite.NewCandidateRepository(db.Conn())
	resRepo := sqlite.NewResumeRepository(db.Conn())
	jobRepo := sqlite.NewJobRepository(db.Conn())
	matchRepo := sqlite.NewMatchRepository(db.Conn())
	settingsRepo := sqlite.NewSettingsRepository(db.Conn())
	appRepo := sqlite.NewApplicationRepository(db.Conn())
	coverLetterRepo := sqlite.NewCoverLetterRepository(db.Conn())
	questionRepo := sqlite.NewApplicationQuestionRepository(db.Conn())
	settingsSvc := services.NewSettingsService(settingsRepo)

	llmSettings, err := settingsSvc.GetLLMSettings(context.Background())
	if err != nil {
		return httpapi.Services{}, fmt.Errorf("loading LLM settings: %w", err)
	}
	llmProv, err := llm.NewProvider(*llmSettings)
	if err != nil {
		return httpapi.Services{}, fmt.Errorf("configuring LLM provider: %w", err)
	}

	mockSource := discovery.NewMockJobSource()
	sources := []ports.JobSource{mockSource}

	resumeSvc := services.NewResumeService(resRepo, llmProv)
	onboardingSvc := services.NewOnboardingService(candRepo, resumeSvc, llmProv, db)
	profileSvc := services.NewProfileService(candRepo)
	discoverySvc := services.NewJobDiscoveryService(jobRepo, sources)
	matchingSvc := services.NewMatchingService(matchRepo, jobRepo, candRepo, resRepo, llmProv)
	applicationSvc := services.NewApplicationService(appRepo)
	coverLetterSvc := services.NewCoverLetterService(coverLetterRepo, appRepo, jobRepo, candRepo, resRepo, llmProv)
	answerSvc := services.NewAnswerService(questionRepo, appRepo, jobRepo, candRepo, resRepo, llmProv)

	return httpapi.Services{
		Candidates:  candRepo,
		Onboarding:  onboardingSvc,
		Profile:     profileSvc,
		Resume:      resumeSvc,
		Discovery:   discoverySvc,
		Matching:    matchingSvc,
		Settings:    settingsSvc,
		Application: applicationSvc,
		CoverLetter: coverLetterSvc,
		Answers:     answerSvc,
	}, nil
}
