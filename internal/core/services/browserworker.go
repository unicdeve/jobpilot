package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

const taskTypePrepareApplication = "prepare_application"

// BrowserWorkerService drives a ports.BrowserWorker to navigate to an
// application's job posting, extract its form, and fill in resolved
// answers, recording an AgentRun/AgentEvent trail as it goes
// (docs/02_features/12_agent_activity.md). It never submits the form
// itself — docs/01_architecture/03_browser_worker.md's responsibilities
// stop at filling fields and reporting results, and Review Mode
// (docs/02_features/09_review_mode.md) requires the human's own explicit
// approval before submission — so the browser window is always left open
// on every path (success, blocked, or failed) for the user to review.
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
}

// NewBrowserWorkerService constructs a BrowserWorkerService with its
// required collaborators. newWorker is called once per PrepareApplication
// run (never shared across runs) so each application being prepared gets
// its own browser window left open, rather than one run's window being
// reused or overwritten by the next.
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
		appSvc:       appSvc,
		jobRepo:      jobRepo,
		runRepo:      runRepo,
		eventRepo:    eventRepo,
		answers:      answers,
		newWorker:    newWorker,
		applyChooser: applyChooser,
	}
}

// PrepareApplication navigates to the application's job posting, extracts
// its form, resolves each field through the existing answer resolver chain
// (AnswerService.Resolve, unchanged), and fills in whatever resolved —
// leaving needs_user_input fields blank, exactly like a manually entered
// unresolved question does today. On success it transitions the
// application to READY_FOR_REVIEW; on any blocker (CAPTCHA, unreadable
// form, a fill failure) it transitions to FAILED with the reason and stops
// — both transitions go through ApplicationService.TransitionState, so an
// application not currently in PREPARING is rejected there rather than by
// any rule duplicated in this service.
func (s *BrowserWorkerService) PrepareApplication(ctx context.Context, applicationID string) error {
	application, err := s.appSvc.GetApplication(ctx, applicationID)
	if err != nil {
		return err
	}
	job, err := s.jobRepo.GetByID(ctx, application.JobID)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	run := &domain.AgentRun{
		ApplicationID: applicationID,
		TaskType:      taskTypePrepareApplication,
		Status:        domain.AgentRunStatusRunning,
	}
	if err := s.runRepo.Save(ctx, run); err != nil {
		return fmt.Errorf("failed to record agent run: %w", err)
	}

	worker, err := s.newWorker()
	if err != nil {
		return s.failRun(ctx, run, applicationID, fmt.Sprintf("failed to launch the browser: %v", err))
	}

	start := time.Now()
	navErr := worker.Navigate(ctx, job.CanonicalURL)
	s.recordEvent(ctx, run.ID, "navigate", "browser", statusFor(navErr), job.CanonicalURL, "", time.Since(start))
	if navErr != nil {
		return s.blockRun(ctx, run, applicationID, fmt.Sprintf("failed to open the job posting: %v", navErr))
	}

	fields, err := s.reachApplicationForm(ctx, worker, run, applicationID)
	if err != nil {
		return err
	}

	for _, field := range fields {
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
func (s *BrowserWorkerService) reachApplicationForm(
	ctx context.Context,
	worker ports.BrowserWorker,
	run *domain.AgentRun,
	applicationID string,
) ([]ports.FormField, error) {
	for hop := 0; ; hop++ {
		if reason, blocked := worker.DetectBlocker(ctx); blocked {
			s.recordEvent(ctx, run.ID, "detect_blocker", "browser", "blocked", "", reason, 0)
			return nil, s.blockRun(ctx, run, applicationID, reason)
		}

		start := time.Now()
		fields, err := worker.ExtractForm(ctx)
		s.recordEvent(ctx, run.ID, "extract_form", "browser", statusFor(err), worker.CurrentURL(ctx), fmt.Sprintf("%d fields found", len(fields)), time.Since(start))
		if err != nil {
			return nil, s.blockRun(ctx, run, applicationID, fmt.Sprintf("failed to read the application form: %v", err))
		}
		if LooksLikeApplicationForm(fields) {
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
	}
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
		s.recordEvent(ctx, run.ID, "skip_field", "form", "ok", field.Label,
			fmt.Sprintf("%q is not one of the choices offered", resolved.Answer), 0)
		return nil
	}

	start := time.Now()
	fillErr := worker.Fill(ctx, field.Ref, resolved.Answer)
	s.recordEvent(ctx, run.ID, "fill_field", "browser", statusFor(fillErr), field.Label, resolved.Answer, time.Since(start))
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
	event := &domain.AgentEvent{
		RunID:         runID,
		Action:        action,
		Tool:          tool,
		Status:        status,
		InputSummary:  input,
		OutputSummary: output,
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
