package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

const taskTypePrepareApplication = domain.AgentTaskPrepareApplication

// challengeTimeout is how long a run waits for the user to solve a check
// before it gives up (ADR-0033 §2).
const challengeTimeout = 5 * time.Minute

// challengePoll is how often a waiting run looks at the page again. A check
// counts as solved once two looks in a row find none, so a page caught
// between two documents does not read as solved.
const challengePoll = time.Second

// ErrPreparationInProgress is returned for a second start while a run for
// the same application has not finished.
var ErrPreparationInProgress = errors.New("this application is already being prepared in the browser")

// ErrCannotPrepare is returned for an application whose state does not let
// it be prepared: one not yet matched, or one already past review.
var ErrCannotPrepare = errors.New("only a matched, preparing or failed application can be prepared in the browser")

// ErrRunNotInProgress is returned for stopping a run that is not running.
var ErrRunNotInProgress = errors.New("this run is not in progress")

// errStoppedByUser is the cause a run is cancelled with when the user stops
// it, which is how its record tells that apart from JobPilot stopping.
var errStoppedByUser = errors.New("stopped by the user")

// BrowserWorkerService drives a ports.BrowserWorker to navigate to an
// application's job posting, extract its form, and fill in resolved
// answers, recording an AgentRun/AgentEvent trail as it goes
// (docs/02_features/12_agent_activity.md). It never submits the form
// itself — docs/01_architecture/03_browser_worker.md's responsibilities
// stop at filling fields and reporting results, and Review Mode
// (docs/02_features/09_review_mode.md) requires the human's own explicit
// approval before submission — so the browser window is always left open
// on every path (success, blocked, or failed) for the user to review.
//
// A run belongs to the daemon rather than to the request that started it,
// because it may wait minutes for the user to solve a check (ADR-0033 §5).
type BrowserWorkerService struct {
	appSvc    *ApplicationService
	jobRepo   ports.JobRepository
	runRepo   ports.AgentRunRepository
	eventRepo ports.AgentEventRepository
	answers   *AnswerService
	newWorker ports.BrowserWorkerFactory
	// applyChooser is consulted only when the deterministic rules find no
	// way to the application form. It may be nil, which simply means there
	// is no fallback and such a page stops the run.
	applyChooser *ApplyControlChooser

	challengeTimeout time.Duration
	challengePoll    time.Duration

	// background is what every run belongs to: the daemon's lifetime, once
	// Start has been called.
	background context.Context
	runs       sync.WaitGroup
	mu         sync.Mutex
	// active holds each application's run in progress.
	active map[string]activeRun
}

// activeRun is a run in progress, and how to stop it.
type activeRun struct {
	runID  string
	cancel context.CancelCauseFunc
}

// NewBrowserWorkerService constructs a BrowserWorkerService with its
// required collaborators. newWorker is called once per run (never shared
// across runs) so each application being prepared gets its own browser
// window left open, rather than one run's window being reused or
// overwritten by the next.
func NewBrowserWorkerService(
	appSvc *ApplicationService,
	jobRepo ports.JobRepository,
	runRepo ports.AgentRunRepository,
	eventRepo ports.AgentEventRepository,
	answers *AnswerService,
	newWorker ports.BrowserWorkerFactory,
	applyChooser *ApplyControlChooser,
) *BrowserWorkerService {
	return &BrowserWorkerService{
		appSvc:           appSvc,
		jobRepo:          jobRepo,
		runRepo:          runRepo,
		eventRepo:        eventRepo,
		answers:          answers,
		newWorker:        newWorker,
		applyChooser:     applyChooser,
		challengeTimeout: challengeTimeout,
		challengePoll:    challengePoll,
		background:       context.Background(),
		active:           map[string]activeRun{},
	}
}

// Start is called once when the daemon starts. Runs belong to ctx, which
// ends when the daemon stops, and a run in progress then ends as blocked.
func (s *BrowserWorkerService) Start(ctx context.Context) {
	s.background = ctx
}

// Wait blocks until every run has finished recording itself, so the daemon
// does not close the database under one.
func (s *BrowserWorkerService) Wait() {
	s.runs.Wait()
}

// ErrAssistDoesNotFillForms is returned for an attempt to prepare an Assist
// application in the browser. Assist writes an application's content and
// never interacts with an employer's form
// (docs/00_product/04_application_modes.md); the mode is the user's choice
// of how involved JobPilot is, and the browser worker is the involvement
// they declined.
var ErrAssistDoesNotFillForms = errors.New("this application is in Assist mode, which never fills in an employer's form; fill it in yourself, or start a Review application")

// StartPreparation records a run for the application and starts it in the
// background, returning the run as it began.
//
// The run navigates to the application's own link if the user gave one
// (ADR-0033 §4), else the job's, walks to the application form, resolves
// each field through the existing answer resolver chain
// (AnswerService.Resolve, unchanged), and fills in whatever resolved —
// leaving needs_user_input fields blank, exactly like a manually entered
// unresolved question does today. On success it transitions the
// application to READY_FOR_REVIEW; on a stop (a check the user did not
// solve, an unreadable form, a fill failure) it transitions to FAILED with
// the reason — both through ApplicationService.TransitionState, so an
// application not currently in PREPARING is rejected there rather than by
// any rule duplicated in this service.
//
// What can be refused at once is refused here, before a run is recorded:
// an Assist application, a missing job, a run already in progress, a state
// that cannot be prepared from. Starting moves an application that may
// enter PREPARING into it — a matched one, or a failed one being tried
// again — since that is what starting means (ADR-0033 §5).
func (s *BrowserWorkerService) StartPreparation(ctx context.Context, applicationID string) (*domain.AgentRun, error) {
	application, err := s.appSvc.GetApplication(ctx, applicationID)
	if err != nil {
		return nil, err
	}
	if application.Mode == domain.ApplicationModeAssist {
		// Refused before a run is recorded or a browser opens: nothing was
		// attempted, so there is nothing for the trail to say.
		return nil, ErrAssistDoesNotFillForms
	}
	job, err := s.jobRepo.GetByID(ctx, application.JobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job: %w", err)
	}
	startURL := job.CanonicalURL
	if application.ApplyURL != "" {
		startURL = application.ApplyURL
	}

	// Checked and claimed under one lock, so two starts arriving together
	// start one run.
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, busy := s.active[applicationID]; busy {
		return nil, ErrPreparationInProgress
	}
	if application.State != domain.ApplicationStatePreparing {
		if !isTransitionAllowed(application.State, domain.ApplicationStatePreparing) {
			return nil, fmt.Errorf("%w; this one is %s", ErrCannotPrepare, readableState(application.State))
		}
		if _, err := s.appSvc.TransitionState(ctx, applicationID, domain.ApplicationStatePreparing, "preparing in the browser"); err != nil {
			return nil, err
		}
	}

	run := &domain.AgentRun{
		ApplicationID: applicationID,
		TaskType:      taskTypePrepareApplication,
		Status:        domain.AgentRunStatusRunning,
	}
	if err := s.runRepo.Save(ctx, run); err != nil {
		return nil, fmt.Errorf("failed to record agent run: %w", err)
	}

	// A copy for the caller, taken before the run goroutine starts
	// changing its own.
	started := *run

	stop, cancel := context.WithCancelCause(s.background)
	s.active[applicationID] = activeRun{runID: run.ID, cancel: cancel}
	s.runs.Add(1)
	go func() {
		defer s.runs.Done()
		defer s.release(applicationID, cancel)
		if err := s.prepare(stop, run, applicationID, startURL); err != nil {
			slog.Info("browser preparation stopped", "run_id", run.ID, "error", err)
		}
	}()
	return &started, nil
}

// StopPreparation stops a run in progress. The run records why and ends
// as blocked; the browser window stays open.
func (s *BrowserWorkerService) StopPreparation(runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, active := range s.active {
		if active.runID == runID {
			active.cancel(errStoppedByUser)
			return nil
		}
	}
	return ErrRunNotInProgress
}

func (s *BrowserWorkerService) release(applicationID string, cancel context.CancelCauseFunc) {
	s.mu.Lock()
	delete(s.active, applicationID)
	s.mu.Unlock()
	cancel(nil)
}

// prepare is one run, from opening the page to the outcome. stop ends when
// the user or the daemon stops the run; the run's own record is written on
// a context that does not, so a stopped run still says why.
func (s *BrowserWorkerService) prepare(stop context.Context, run *domain.AgentRun, applicationID, startURL string) error {
	ctx := context.WithoutCancel(stop)

	worker, err := s.newWorker()
	if err != nil {
		return s.failRun(ctx, run, applicationID, fmt.Sprintf("failed to launch the browser: %v", err))
	}

	start := time.Now()
	navErr := worker.Navigate(ctx, startURL)
	s.recordEvent(ctx, run.ID, "navigate", "browser", statusFor(navErr), startURL, "", time.Since(start))
	if navErr != nil {
		return s.blockRun(ctx, run, applicationID, fmt.Sprintf("failed to open the job posting: %v", navErr))
	}

	fields, err := s.reachApplicationForm(ctx, stop, worker, run, applicationID)
	if err != nil {
		return err
	}

	for _, field := range fields {
		if stop.Err() != nil {
			return s.stopRequested(ctx, stop, run, applicationID)
		}
		if err := s.prepareField(ctx, worker, run, applicationID, field); err != nil {
			return err
		}
	}

	completedAt := time.Now()
	run.Status = domain.AgentRunStatusCompleted
	run.CompletedAt = &completedAt
	if err := s.runRepo.Update(ctx, run); err != nil {
		return fmt.Errorf("failed to record completed agent run: %w", err)
	}

	if _, err := s.appSvc.TransitionState(ctx, applicationID, domain.ApplicationStateReadyForReview, "prepared in browser"); err != nil {
		return fmt.Errorf("failed to transition application after preparing: %w", err)
	}

	return nil
}

// reachApplicationForm walks from the job posting to the page that actually
// holds the application form, and refuses to go on if it does not find one
// (ADR-0013).
//
// The posting is rarely the form. On both current boards the stored URL is
// a listing page whose only input is the board's own newsletter box, and an
// earlier version of this method extracted that box, filled it with the
// candidate's email address and reported a prepared application. Arriving
// somewhere is not the same as arriving at the form, so the fields decide.
//
// A page asking whether the user is a person waits for them to answer it
// (ADR-0033 §2). One that asks again on the same page after being answered
// is a site refusing JobPilot's browser, and stops the run (§3).
func (s *BrowserWorkerService) reachApplicationForm(
	ctx, stop context.Context,
	worker ports.BrowserWorker,
	run *domain.AgentRun,
	applicationID string,
) ([]ports.FormField, error) {
	solvedHere := false
	for hop := 0; ; {
		if stop.Err() != nil {
			return nil, s.stopRequested(ctx, stop, run, applicationID)
		}

		if reason, blocked := worker.DetectBlocker(ctx); blocked {
			if solvedHere {
				refused := refusedReason(worker.CurrentURL(ctx))
				s.recordPolicy(ctx, run.ID, "detect_blocker", "browser", "blocked", domain.PolicyDenied,
					worker.CurrentURL(ctx), refused)
				return nil, s.blockRun(ctx, run, applicationID, refused)
			}
			if err := s.waitForUser(ctx, stop, worker, run, applicationID, reason); err != nil {
				return nil, err
			}
			solvedHere = true
			continue
		}

		start := time.Now()
		fields, err := worker.ExtractForm(ctx)
		elapsed := time.Since(start)
		if err != nil {
			s.recordEvent(ctx, run.ID, "extract_form", "browser", statusFor(err), worker.CurrentURL(ctx), err.Error(), elapsed)
			return nil, s.blockRun(ctx, run, applicationID, fmt.Sprintf("failed to read the application form: %v", err))
		}

		// The application-form gate decides whether this page is filled,
		// so its answer is recorded as the step's policy result.
		isForm := LooksLikeApplicationForm(fields)
		policy, verdict := domain.PolicyDenied, "not an application form"
		if isForm {
			policy, verdict = domain.PolicyAllowed, "an application form"
		}
		s.recordTimedPolicy(ctx, run.ID, "extract_form", "browser", "ok", policy, worker.CurrentURL(ctx),
			fmt.Sprintf("%s found, %s", pluralize(len(fields), "field"), verdict), elapsed)
		if isForm {
			return fields, nil
		}

		if hop >= maxApplyHops {
			return nil, s.blockRun(ctx, run, applicationID,
				"could not find the application form: the job posting and the pages it links to do not ask for any of the details an application needs")
		}

		start = time.Now()
		controls, err := worker.FindApplyControls(ctx)
		if err != nil {
			return nil, s.blockRun(ctx, run, applicationID, fmt.Sprintf("failed to read the page's links: %v", err))
		}

		control, chosenBy := s.chooseApplyControl(ctx, run, controls)
		if control == nil {
			return nil, s.blockRun(ctx, run, applicationID,
				"could not find a way to the application form on this page — no link or button on it looks like an apply control")
		}
		s.recordEvent(ctx, run.ID, "follow_apply", "browser", "ok",
			control.Text, hopSummary(worker.CurrentURL(ctx), *control, chosenBy), time.Since(start))

		if err := worker.FollowControl(ctx, control.Ref); err != nil {
			return nil, s.blockRun(ctx, run, applicationID, fmt.Sprintf("failed to follow %q: %v", control.Text, err))
		}
		hop++
		solvedHere = false
	}
}

// waitForUser pauses the run on a check only the user can solve, until
// they solve it, stop the run, or the wait runs out (ADR-0033 §2). It only
// ever looks at the page: the check is the user's to answer.
//
// The page reloading while the check is still there is the site turning a
// solved check down, and asking again would only repeat it (§3).
func (s *BrowserWorkerService) waitForUser(
	ctx, stop context.Context,
	worker ports.BrowserWorker,
	run *domain.AgentRun,
	applicationID, reason string,
) error {
	s.recordPolicy(ctx, run.ID, "detect_blocker", "browser", "waiting", domain.PolicyDenied,
		worker.CurrentURL(ctx), reason+" — waiting for you to answer it in the browser window")
	if err := s.setRunStatus(ctx, run, domain.AgentRunStatusWaitingForUser); err != nil {
		return err
	}
	if err := worker.Focus(ctx); err != nil {
		// The window is still there for the user to find, so this is worth
		// a note in the log rather than the run.
		slog.WarnContext(ctx, "failed to bring the browser window forward", "run_id", run.ID, "error", err)
	}

	loads := worker.PageLoads(ctx)
	started := time.Now()
	deadline := time.NewTimer(s.challengeTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(s.challengePoll)
	defer ticker.Stop()

	clearOnce := false
	for {
		select {
		case <-stop.Done():
			return s.stopRequested(ctx, stop, run, applicationID)
		case <-deadline.C:
			return s.blockRun(ctx, run, applicationID,
				fmt.Sprintf("%s, and it was not answered within %s", reason, waitDescription(s.challengeTimeout)))
		case <-ticker.C:
		}

		_, blocked := worker.DetectBlocker(ctx)
		switch {
		case blocked && worker.PageLoads(ctx) > loads:
			refused := refusedReason(worker.CurrentURL(ctx))
			s.recordPolicy(ctx, run.ID, "detect_blocker", "browser", "blocked", domain.PolicyDenied,
				worker.CurrentURL(ctx), refused)
			return s.blockRun(ctx, run, applicationID, refused)
		case blocked:
			clearOnce = false
		case !clearOnce:
			clearOnce = true
		default:
			s.recordTimedPolicy(ctx, run.ID, "solve_check", "user", "ok", domain.PolicyAllowed,
				worker.CurrentURL(ctx), "the check was answered", time.Since(started))
			return s.setRunStatus(ctx, run, domain.AgentRunStatusRunning)
		}
	}
}

// refusedReason is the stop for a site that asked again after its check
// was answered, named so the user knows which site to visit themselves.
func refusedReason(pageURL string) string {
	site := hostOf(pageURL)
	if site == "" {
		site = "the site"
	}
	return site + " asked again after its check was answered — it does not let JobPilot's browser through"
}

// readableState is a state as a sentence says it: READY_FOR_REVIEW reads
// "ready for review".
func readableState(state domain.ApplicationState) string {
	return strings.ToLower(strings.ReplaceAll(string(state), "_", " "))
}

// waitDescription renders a wait as a person would say it.
func waitDescription(d time.Duration) string {
	if d >= time.Minute {
		return pluralize(int(d/time.Minute), "minute")
	}
	return pluralize(int(d/time.Second), "second")
}

// stopRequested ends a run the user or the daemon stopped, saying which.
func (s *BrowserWorkerService) stopRequested(ctx, stop context.Context, run *domain.AgentRun, applicationID string) error {
	reason := "JobPilot stopped"
	if errors.Is(context.Cause(stop), errStoppedByUser) {
		reason = "you stopped this run"
	}
	return s.blockRun(ctx, run, applicationID, reason)
}

func (s *BrowserWorkerService) setRunStatus(ctx context.Context, run *domain.AgentRun, status domain.AgentRunStatus) error {
	run.Status = status
	if err := s.runRepo.Update(ctx, run); err != nil {
		return fmt.Errorf("failed to record agent run status: %w", err)
	}
	return nil
}

// chooseApplyControl decides which control to follow, and reports how the
// decision was reached so the trail can say whether a model was involved
// (ADR-0013 §2).
//
// The deterministic rules go first and answer for every board measured so
// far. Only when they find nothing is the model asked, and then only to
// pick one of the controls they just considered — it cannot name a
// destination of its own. A model that is missing, unreachable or
// incoherent is not an error worth stopping over: it leaves the run in
// exactly the state it would have been in without a fallback at all.
func (s *BrowserWorkerService) chooseApplyControl(
	ctx context.Context,
	run *domain.AgentRun,
	controls []ports.ApplyControl,
) (*ports.ApplyControl, string) {
	if control := PickApplyControl(controls); control != nil {
		return control, "heuristics"
	}
	if s.applyChooser == nil {
		return nil, ""
	}

	start := time.Now()
	control, err := s.applyChooser.Choose(ctx, controls)
	switch {
	case err != nil:
		s.recordEvent(ctx, run.ID, "choose_apply", "llm", "error", "no control matched the apply rules", err.Error(), time.Since(start))
		return nil, ""
	case control == nil:
		s.recordEvent(ctx, run.ID, "choose_apply", "llm", "ok", "no control matched the apply rules", "the model abstained", time.Since(start))
		return nil, ""
	default:
		s.recordEvent(ctx, run.ID, "choose_apply", "llm", "ok", "no control matched the apply rules", control.Text, time.Since(start))
		return control, "the model"
	}
}

// pluralize renders a count with its noun: "1 field", "3 fields".
func pluralize(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// hopSummary records where a hop went and how it was decided, so the
// activity trail answers "why did it go there" and "was a model involved"
// after the fact (ADR-0013 §2), including when the hop left the origin —
// which is normal, since the employer's ATS is usually the point.
func hopSummary(fromURL string, control ports.ApplyControl, chosenBy string) string {
	switch {
	case control.URL == "":
		return fmt.Sprintf("no destination, followed by click (chosen by %s)", chosenBy)
	case hostOf(control.URL) != hostOf(fromURL):
		return fmt.Sprintf("%s (chosen by %s, cross-origin)", control.URL, chosenBy)
	default:
		return fmt.Sprintf("%s (chosen by %s)", control.URL, chosenBy)
	}
}

// prepareField turns one extracted form field into an ApplicationQuestion,
// resolves it through the existing chain, and fills the browser field if
// resolved.
func (s *BrowserWorkerService) prepareField(ctx context.Context, worker ports.BrowserWorker, run *domain.AgentRun, applicationID string, field ports.FormField) error {
	questionType := domain.QuestionTypeFactual
	if field.Kind == ports.FormFieldKindTextarea {
		questionType = domain.QuestionTypeNarrative
	}

	question, err := s.answers.AddExtractedQuestion(ctx, applicationID, field.Label, questionType, field.Required, field.Ref)
	if err != nil {
		return s.blockRun(ctx, run, applicationID, fmt.Sprintf("failed to record extracted question %q: %v", field.Label, err))
	}
	s.recordEvent(ctx, run.ID, "extract_question", "form", "ok", field.Label, string(field.Kind), 0)

	resolved, err := s.answers.Resolve(ctx, question.ID)
	if err != nil {
		return s.blockRun(ctx, run, applicationID, fmt.Sprintf("failed to resolve %q: %v", field.Label, err))
	}
	s.recordEvent(ctx, run.ID, "resolve_question", "answer_resolver", "ok", field.Label, string(resolved.Category), 0)

	if resolved.Answer == "" {
		// needs_user_input: left unfilled, surfaces in the Questions panel
		// exactly like a manually entered unresolved question does today.
		return nil
	}

	if !answerFitsField(field, resolved.Answer) {
		// A field that offers a fixed set of choices and an answer that is
		// not one of them. Filling it anyway is not a near miss: the
		// browser refuses an option a <select> does not have, and that
		// error would stop the whole run over one field. Left blank, it
		// joins the questions the user answers themselves.
		//
		// The answer itself is not recorded: it is stored on the question,
		// and the trail is shown on its own screen, where candidate data
		// has no reason to appear (docs/02_features/12_agent_activity.md).
		s.recordPolicy(ctx, run.ID, "skip_field", "form", "ok", domain.PolicyDenied, field.Label,
			"the answer is not one of the choices offered")
		return nil
	}

	start := time.Now()
	fillErr := worker.Fill(ctx, field.Ref, resolved.Answer)
	// No output, and in particular not the answer: see skip_field above.
	s.recordEvent(ctx, run.ID, "fill_field", "browser", statusFor(fillErr), field.Label, "", time.Since(start))
	if fillErr != nil {
		return s.blockRun(ctx, run, applicationID, fmt.Sprintf("failed to fill %q: %v", field.Label, fillErr))
	}

	return nil
}

// answerFitsField reports whether an answer can actually be given to a
// field, which is only in question when the field constrains the choice.
//
// The match is exact, ignoring case and surrounding space. A looser one
// would be guessing which option the user meant, on a form submitted under
// their name — and "Yes" is not evidence for "Yes, I have a work visa".
func answerFitsField(field ports.FormField, answer string) bool {
	if len(field.Options) == 0 {
		return true
	}
	for _, option := range field.Options {
		if strings.EqualFold(strings.TrimSpace(option), strings.TrimSpace(answer)) {
			return true
		}
	}
	return false
}

// blockRun marks run blocked (a content-related stop: CAPTCHA, an
// unreadable form, a fill failure) with reason, transitions the application
// to FAILED with the same reason, and returns an error describing the
// stoppage.
func (s *BrowserWorkerService) blockRun(ctx context.Context, run *domain.AgentRun, applicationID, reason string) error {
	return s.stopRun(ctx, run, applicationID, domain.AgentRunStatusBlocked, "browser automation stopped", reason)
}

// failRun marks run failed (an infrastructure-level problem: the browser
// itself wouldn't launch, before anything was ever shown to the user) with
// reason, transitions the application to FAILED, and returns an error
// describing the failure.
func (s *BrowserWorkerService) failRun(ctx context.Context, run *domain.AgentRun, applicationID, reason string) error {
	return s.stopRun(ctx, run, applicationID, domain.AgentRunStatusFailed, "browser automation failed", reason)
}

func (s *BrowserWorkerService) stopRun(ctx context.Context, run *domain.AgentRun, applicationID string, status domain.AgentRunStatus, errPrefix, reason string) error {
	now := time.Now()
	run.Status = status
	run.Error = reason
	run.CompletedAt = &now
	if err := s.runRepo.Update(ctx, run); err != nil {
		return fmt.Errorf("failed to record %s agent run: %w", status, err)
	}

	if _, err := s.appSvc.TransitionState(ctx, applicationID, domain.ApplicationStateFailed, reason); err != nil {
		return fmt.Errorf("failed to transition application after stopping: %w", err)
	}

	return fmt.Errorf("%s: %s", errPrefix, reason)
}

// recordEvent saves an AgentEvent, logging rather than failing the run if
// the write itself fails: losing one activity-log entry shouldn't abort an
// otherwise-successful preparation.
func (s *BrowserWorkerService) recordEvent(ctx context.Context, runID, action, tool, status, input, output string, duration time.Duration) {
	s.recordTimedPolicy(ctx, runID, action, tool, status, "", input, output, duration)
}

// recordPolicy records a step whose outcome was decided by a deterministic
// rule, with that rule's verdict as the policy result.
func (s *BrowserWorkerService) recordPolicy(ctx context.Context, runID, action, tool, status, policy, input, output string) {
	s.recordTimedPolicy(ctx, runID, action, tool, status, policy, input, output, 0)
}

func (s *BrowserWorkerService) recordTimedPolicy(ctx context.Context, runID, action, tool, status, policy, input, output string, duration time.Duration) {
	event := &domain.AgentEvent{
		RunID:         runID,
		Action:        action,
		Tool:          tool,
		Status:        status,
		InputSummary:  input,
		OutputSummary: output,
		PolicyResult:  policy,
		DurationMs:    duration.Milliseconds(),
	}
	if err := s.eventRepo.Save(ctx, event); err != nil {
		slog.WarnContext(ctx, "failed to record agent event", "action", action, "error", err)
	}
}

// ListRuns returns every agent run for an application, most recent first.
func (s *BrowserWorkerService) ListRuns(ctx context.Context, applicationID string) ([]*domain.AgentRun, error) {
	runs, err := s.runRepo.ListByApplication(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent runs: %w", err)
	}
	return runs, nil
}

// ListEvents returns every event for an agent run, in order.
func (s *BrowserWorkerService) ListEvents(ctx context.Context, runID string) ([]*domain.AgentEvent, error) {
	events, err := s.eventRepo.ListByRun(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent events: %w", err)
	}
	return events, nil
}

func statusFor(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}
