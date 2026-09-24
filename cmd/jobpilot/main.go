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

	"path/filepath"

	"github.com/unicdev/jobpilot/internal/adapters/primary/httpapi"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/browser"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/credentials"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/discovery"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/documents"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/llm"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/sqlite"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/system"
	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
	"github.com/unicdev/jobpilot/internal/core/services"
)

// browserProfileDirName is a JobPilot-owned Chrome profile, never the
// user's own daily-driver profile — so "prepare in browser" never
// conflicts with their regular browser and never needs their real
// passwords. The user logs into any job board once inside this profile,
// per docs/01_architecture/03_browser_worker.md.
const browserProfileDirName = "browser-profile"

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
	agentRunRepo := sqlite.NewAgentRunRepository(db.Conn())
	agentEventRepo := sqlite.NewAgentEventRepository(db.Conn())
	settingsSvc := services.NewSettingsService(settingsRepo)

	// The secret lives in the OS keychain and the table records only which
	// refs exist, because a keychain cannot be enumerated (ADR-0019 §6).
	credentialSvc := services.NewCredentialService(
		credentials.NewKeyringStore(),
		sqlite.NewCredentialNameRepository(db.Conn()),
	)

	dataDir, err := system.GetDataDir()
	if err != nil {
		return httpapi.Services{}, fmt.Errorf("resolving data directory: %w", err)
	}
	browserProfileDir := filepath.Join(dataDir, browserProfileDirName)

	// Resolved per request rather than once here: the daemon outlives both
	// the desktop app and the CLI, so a provider fixed at startup would
	// ignore settings the user saves later until they restarted it.
	llmProv := llm.NewReloadingProvider(func(ctx context.Context) (domain.LLMSettings, error) {
		settings, err := settingsSvc.GetLLMSettings(ctx)
		if err != nil {
			return domain.LLMSettings{}, err
		}
		return *settings, nil
	})

	// Every source here is a public API that needs no credentials, which
	// is what keeps discovery working on a fresh install with nothing
	// configured. Google Search is deliberately absent: it returns a
	// JavaScript gate rather than results to anything but a browser, and
	// getting past that is the one thing ADR-0013 and ADR-0014 say the
	// product never does. See docs/02_features/04_job_discovery.md.
	sources := []ports.JobSource{
		discovery.NewArbeitnowSource(discovery.DefaultArbeitnowBaseURL),
		discovery.NewRemotiveSource(discovery.DefaultRemotiveBaseURL),
		discovery.NewRemoteOKSource(discovery.DefaultRemoteOKBaseURL),
		discovery.NewHimalayasSource(discovery.DefaultHimalayasBaseURL),
		discovery.NewHackerNewsSource(discovery.DefaultHackerNewsBaseURL),
	}

	// headless: false — real use always shows a visible window so the user
	// can review and submit themselves; see docs/01_architecture/03_browser_worker.md.
	// One session, because Chrome allows one process per profile directory
	// and every task leaves its window open: each task gets its own tab
	// instead. JobDiscoveryService uses the same factory as a fallback for
	// importing a job by URL when no registered source recognizes it, but
	// closes its own tab once the page is read — see
	// JobDiscoveryService.importViaBrowser.
	chrome := browser.NewChromeSession(browserProfileDir, false)
	newBrowserWorker := chrome.NewWorker

	// One converter for the process: Go MarkItDown for most formats, and
	// PDFium for PDFs, whose engine starts on first use (ADR-0016).
	docConverter := documents.NewConverter()

	resumeSvc := services.NewResumeService(resRepo, llmProv, docConverter)
	savedSearchSvc := services.NewSavedSearchService(sqlite.NewSavedSearchRepository(db.Conn()))
	onboardingSvc := services.NewOnboardingService(candRepo, resumeSvc, savedSearchSvc, llmProv, db)
	profileSvc := services.NewProfileService(candRepo)
	discoverySvc := services.NewJobDiscoveryService(jobRepo, sources, llmProv, newBrowserWorker, docConverter)
	matchingSvc := services.NewMatchingService(matchRepo, jobRepo, candRepo, resRepo, llmProv)
	applicationSvc := services.NewApplicationService(appRepo)
	coverLetterSvc := services.NewCoverLetterService(coverLetterRepo, appRepo, jobRepo, candRepo, resRepo, llmProv)
	answerSvc := services.NewAnswerService(questionRepo, appRepo, jobRepo, candRepo, resRepo, llmProv)
	applyChooser := services.NewApplyControlChooser(llmProv)
	scoutSvc := services.NewScoutService(
		sqlite.NewScoutRepository(db.Conn()), sqlite.NewSavedSearchRepository(db.Conn()),
		candRepo, appRepo, agentRunRepo, agentEventRepo, discoverySvc, matchingSvc)
	browserSvc := services.NewBrowserWorkerService(applicationSvc, jobRepo, agentRunRepo, agentEventRepo, answerSvc, newBrowserWorker, applyChooser)

	return httpapi.Services{
		Candidates:  candRepo,
		Onboarding:  onboardingSvc,
		Profile:     profileSvc,
		Resume:      resumeSvc,
		Discovery:   discoverySvc,
		Searches:    savedSearchSvc,
		Matching:    matchingSvc,
		Settings:    settingsSvc,
		Application: applicationSvc,
		CoverLetter: coverLetterSvc,
		Answers:     answerSvc,
		Browser:     browserSvc,
		Scouts:      scoutSvc,
		Credentials: credentialSvc,
	}, nil
}
