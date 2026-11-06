// Command jobpilot is the JobPilot background daemon: it owns the SQLite
// database and every core service, and exposes them to local clients (the
// CLI, eventually the desktop app) over a loopback HTTP API. See
// docs/01_architecture/01_daemon.md and
// docs/09_decisions/0012-loopback-http-ipc.md.
//
// Run as `jobpilot scout wake <scout-id>`, it is instead what a scheduled
// Scout's alarm starts: a client that asks the daemon, starting one if
// none is running, to run that Scout if it is due (ADR-0027 §4).
//
// `jobpilot stop` asks the running daemon to stop and waits for it to go,
// which the Windows installer runs before replacing or removing it, and
// `jobpilot alarms remove` removes every Scout's alarm, which its
// uninstaller runs after that, and `jobpilot alarms restore` schedules them
// again, which the installer runs once its files are in place (ADR-0029 §5).
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
	"github.com/unicdev/jobpilot/internal/adapters/secondary/alarms"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/browser"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/credentials"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/desktopapp"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/discovery"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/documents"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/llm"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/sqlite"
	"github.com/unicdev/jobpilot/internal/adapters/secondary/system"
	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
	"github.com/unicdev/jobpilot/internal/core/services"
	"github.com/unicdev/jobpilot/internal/version"
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
	args := os.Args[1:]
	switch {
	case len(args) == 0:
		if err := run(); err != nil {
			log.Fatalf("jobpilot daemon: %v", err)
		}
	case len(args) == 1 && args[0] == "--version":
		fmt.Println("jobpilot " + version.Version)
	case len(args) == 1 && args[0] == "stop":
		if err := httpapi.StopDaemon(context.Background()); err != nil {
			log.Fatalf("jobpilot stop: %v", err)
		}
	case len(args) == 2 && args[0] == "alarms" && args[1] == "remove":
		if err := removeAlarms(); err != nil {
			log.Fatalf("jobpilot alarms remove: %v", err)
		}
	case len(args) == 2 && args[0] == "alarms" && args[1] == "restore":
		if err := restoreAlarms(); err != nil {
			log.Fatalf("jobpilot alarms restore: %v", err)
		}
	case len(args) == 3 && args[0] == "scout" && args[1] == "wake":
		if err := wake(args[2]); err != nil {
			log.Fatalf("jobpilot scout wake: %v", err)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: jobpilot                      run the daemon")
		fmt.Fprintln(os.Stderr, "       jobpilot --version            print the version")
		fmt.Fprintln(os.Stderr, "       jobpilot stop                 stop the running daemon, and wait for it to go")
		fmt.Fprintln(os.Stderr, "       jobpilot alarms remove        remove every scheduled scout's alarm")
		fmt.Fprintln(os.Stderr, "       jobpilot alarms restore       schedule every scheduled scout again")
		fmt.Fprintln(os.Stderr, "       jobpilot scout wake <id>      run a scheduled scout if it is due")
		os.Exit(2)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// A newer client asks over the API, and is answered as SIGTERM is.
	ctx, shutdown := context.WithCancel(ctx)
	defer shutdown()

	// Before the database is opened: a second daemon would migrate it under
	// the first, and share its browser profile. The lock is held until the
	// process exits, after the database has closed, which is what a client
	// replacing this daemon waits for (ADR-0029 §4).
	lock, err := system.LockDaemon(ctx, daemonLockWait)
	if errors.Is(err, system.ErrDaemonRunning) {
		return errors.New("another jobpilot daemon is already running, or still stopping; stop it first with `jobpilot stop`")
	}
	if err != nil {
		return fmt.Errorf("locking the data directory: %w", err)
	}
	defer func() { _ = lock.Release() }()

	catalog, err := system.OpenWorkspaceStore()
	if err != nil {
		return fmt.Errorf("opening workspace catalog: %w", err)
	}
	workspaces, err := newWorkspaceManager(ctx, shutdown, catalog)
	if err != nil {
		return err
	}
	defer workspaces.close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("binding loopback listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	if err := httpapi.WriteDiscoveryInfo(httpapi.DiscoveryInfo{PID: os.Getpid(), Port: port}); err != nil {
		return fmt.Errorf("writing discovery file: %w", err)
	}
	defer func() {
		if err := httpapi.RemoveDiscoveryInfo(os.Getpid()); err != nil {
			log.Printf("error removing discovery file: %v", err)
		}
	}()

	server := &http.Server{Handler: httpapi.NewWorkspaceHandler(catalog, workspaces)}

	serveErr := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	log.Printf("jobpilot daemon %s listening on 127.0.0.1:%d (pid %d)", version.Version, port, os.Getpid())

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

// daemonLockWait is how long a starting daemon waits for the lock, which a
// client checking whether a daemon is running holds for a moment.
const daemonLockWait = 2 * time.Second

// wireServices constructs every core service the daemon exposes, exactly as
// cmd/jobpilot-cli used to wire them in-process before this package existed.
func wireServices(db *sqlite.DB, workspaceDir, credentialNamespace string) (httpapi.Services, error) {
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
		credentials.NewKeyringStore(credentialNamespace),
		sqlite.NewCredentialNameRepository(db.Conn()),
	)

	browserProfileDir := filepath.Join(workspaceDir, browserProfileDirName)

	// Resolved per request rather than once here: the daemon outlives both
	// the desktop app and the CLI, so a provider fixed at startup would
	// ignore settings the user saves later until they restarted it.
	// The API key the settings name is read from the keychain the same
	// way, so the HTTP layer never handles it (ADR-0030 §2).
	llmProv := llm.NewReloadingProvider(
		func(ctx context.Context) (domain.LLMSettings, error) {
			settings, err := settingsSvc.GetLLMSettings(ctx)
			if err != nil {
				return domain.LLMSettings{}, err
			}
			return *settings, nil
		},
		func(ctx context.Context, label string) (string, error) {
			return credentialSvc.Get(ctx, domain.LLMCredentialSource, label)
		},
	)

	// Every source here is a public API that needs no credentials, which
	// is what keeps discovery working on a fresh install with nothing
	// configured. Google Search is deliberately absent (ADR-0020): it returns a
	// JavaScript gate rather than results to anything but a browser, and
	// getting past that is the one thing ADR-0014 and ADR-0033 say the
	// product never does. Working Nomads is absent too: it publishes a feed
	// and no terms for using it (ADR-0023 §7). See
	// docs/02_features/04_job_discovery.md.
	sources := []ports.JobSource{
		discovery.NewArbeitnowSource(discovery.DefaultArbeitnowBaseURL),
		discovery.NewRemotiveSource(discovery.DefaultRemotiveBaseURL),
		discovery.NewRemoteOKSource(discovery.DefaultRemoteOKBaseURL),
		discovery.NewHimalayasSource(discovery.DefaultHimalayasBaseURL),
		discovery.NewHackerNewsSource(discovery.DefaultHackerNewsBaseURL),
		discovery.NewWeWorkRemotelySource(discovery.DefaultWeWorkRemotelyBaseURL),
		discovery.NewJobicySource(discovery.DefaultJobicyBaseURL),
	}

	// The browser reads a job posting no registered source recognizes, for
	// JobDiscoveryService.ImportJob, and closes its tab once the page is
	// read. It never fills or submits a form (ADR-0034). headless: false,
	// so a page that will not load, or asks whether the user is a person,
	// is not hidden from them; see docs/01_architecture/03_browser_worker.md.
	// One session, because Chrome allows one process per profile directory.
	chrome := browser.NewChromeSession(browserProfileDir, false)
	newBrowserWorker := chrome.NewWorker

	// One converter for the process: Go MarkItDown for most formats, and
	// PDFium for PDFs, whose engine starts on first use (ADR-0016).
	docConverter := documents.NewConverter()

	resumeSvc := services.NewResumeService(resRepo, candRepo, llmProv, docConverter)
	savedSearchSvc := services.NewSavedSearchService(sqlite.NewSavedSearchRepository(db.Conn()))
	onboardingSvc := services.NewOnboardingService(candRepo, resumeSvc, savedSearchSvc, llmProv, db)
	profileSvc := services.NewProfileService(candRepo)
	// The companies the candidate watches are one more source, "Company
	// pages", read from each company's own Greenhouse, Lever or Ashby page
	// (ADR-0024).
	companySvc := services.NewCompanyWatchService(sqlite.NewWatchedCompanyRepository(db.Conn()),
		discovery.NewCompanyBoards(discovery.DefaultBoardEndpoints), jobRepo)
	sources = append(sources, companySvc.Source())

	discoverySvc := services.NewJobDiscoveryService(jobRepo, sources, llmProv, newBrowserWorker, docConverter).
		WithCandidates(candRepo)
	// Matching reads the user's guidance from the settings on every
	// request, as the model settings are (ADR-0031).
	matchingSvc := services.NewMatchingService(matchRepo, jobRepo, candRepo, resRepo, llmProv).
		WithGuidance(settingsSvc)
	applicationSvc := services.NewApplicationService(appRepo, jobRepo, candRepo, resRepo)
	aboutYouSvc := services.NewAboutYouService(
		sqlite.NewPersonalAnswerRepository(db.Conn()), candRepo, appRepo, questionRepo)
	coverLetterSvc := services.NewCoverLetterService(coverLetterRepo, appRepo, candRepo, resRepo, aboutYouSvc)
	answerSvc := services.NewAnswerService(questionRepo, appRepo, jobRepo, candRepo, resRepo, aboutYouSvc)
	scoutSvc := services.NewScoutService(
		sqlite.NewScoutRepository(db.Conn()), sqlite.NewSavedSearchRepository(db.Conn()),
		candRepo, appRepo, agentRunRepo, agentEventRepo, discoverySvc, matchingSvc)
	// A scheduled Scout is an alarm in the operating system's scheduler
	// that runs this executable (ADR-0027).
	if scoutAlarms, err := alarms.New(); err != nil {
		log.Printf("scouts can be run by hand but not scheduled: %v", err)
	} else if target, err := alarmTarget(); err != nil {
		log.Printf("scouts can be run by hand but not scheduled: %v", err)
	} else {
		if credentialNamespace == "" {
			scoutAlarms = alarms.LegacyScoped(scoutAlarms)
		} else {
			scoutAlarms = alarms.Scoped(scoutAlarms, credentialNamespace)
		}
		scoutSvc.WithAlarms(scoutAlarms, target)
	}
	savedSearchSvc.WithScoutsChanged(scoutSvc.ScoutsChanged)
	// What a scout run found is told to the user through the desktop app,
	// which posts it under JobPilot's own name (ADR-0028).
	notificationSvc := services.NewNotificationService(sqlite.NewNotificationRepository(db.Conn()),
		settingsSvc, desktopapp.Launcher{})
	scoutSvc.WithNotifier(notificationSvc)
	// "Delete all data" reaches past the database to the browser profile
	// and the daemon's log (ADR-0021 §7).
	deletionSvc := services.NewDeletionService(sqlite.NewDeletionStore(db), credentialSvc,
		chrome.ProfileEraser(), nil).WithScoutAlarms(scoutSvc)

	return httpapi.Services{
		Candidates:    candRepo,
		Onboarding:    onboardingSvc,
		Profile:       profileSvc,
		Resume:        resumeSvc,
		Discovery:     discoverySvc,
		Searches:      savedSearchSvc,
		Matching:      matchingSvc,
		Settings:      settingsSvc,
		LLM:           llmProv,
		Application:   applicationSvc,
		CoverLetter:   coverLetterSvc,
		Answers:       answerSvc,
		Scouts:        scoutSvc,
		Credentials:   credentialSvc,
		Activity:      services.NewAgentActivityService(agentRunRepo, agentEventRepo),
		Deletion:      deletionSvc,
		AboutYou:      aboutYouSvc,
		Companies:     companySvc,
		Notifications: notificationSvc,
	}, nil
}
