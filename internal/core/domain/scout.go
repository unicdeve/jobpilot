package domain

import (
	"fmt"
	"strings"
	"time"
)

// ScoutSource is where a run gets the jobs it considers.
type ScoutSource string

// Scout sources. Both stop before the browser: fetching means the
// job-board APIs discovery already calls, and nothing else.
const (
	// ScoutSourceLocal considers only jobs already stored locally.
	ScoutSourceLocal ScoutSource = "local"
	// ScoutSourceFetch searches the configured job sources first, then
	// considers what it found along with what was already stored.
	ScoutSourceFetch ScoutSource = "sources"
)

// ScoutSources lists every source, for UI that offers a choice.
func ScoutSources() []ScoutSource { return []ScoutSource{ScoutSourceLocal, ScoutSourceFetch} }

// IsValid reports whether a source is one we have.
func (s ScoutSource) IsValid() bool {
	for _, known := range ScoutSources() {
		if s == known {
			return true
		}
	}
	return false
}

// Scout is a standing instruction: run this saved search, judge what it
// finds against the candidate, and create applications for the ones worth
// the user's attention (ADR-0014).
//
// A Scout carries no salary floor, company blocklist or location rule.
// Its saved search holds all of those already, and SearchCriteria.Matches
// already applies them; a second copy here would be a second place to
// change one rule (ADR-0018 §1).
//
// A Scout never contacts an employer and never submits an application.
// That is not a setting on this struct because it is not a setting
// anywhere.
type Scout struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	SavedSearchID string `json:"saved_search_id"`
	// MinMatchScore is the lowest match score that earns an application.
	// Zero means every job the search matches is worth reviewing.
	MinMatchScore int `json:"min_match_score"`
	// MaxApplicationsPerRun bounds one run. It exists because a run that
	// is right about twenty jobs is still twenty things to read.
	MaxApplicationsPerRun int         `json:"max_applications_per_run"`
	Source                ScoutSource `json:"source"`
	IsEnabled             bool        `json:"is_enabled"`
	LastRunAt             *time.Time  `json:"last_run_at,omitempty"`
	CreatedAt             time.Time   `json:"created_at"`
	UpdatedAt             time.Time   `json:"updated_at"`
}

// maxApplicationsPerRunCeiling bounds what a user can ask for in one run.
//
// It is not arithmetic, it is a judgement: the output of a run is a
// reading list, and a hundred applications to review is not a reading
// list. A user who wants more can run again.
const maxApplicationsPerRunCeiling = 50

// Validate reports whether a Scout is usable, and says what is wrong.
func (s Scout) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("a scout needs a name")
	}
	if strings.TrimSpace(s.SavedSearchID) == "" {
		return fmt.Errorf("a scout needs a saved search to run")
	}
	if !s.Source.IsValid() {
		return fmt.Errorf("unknown scout source %q", s.Source)
	}
	if s.MinMatchScore < 0 || s.MinMatchScore > 100 {
		return fmt.Errorf("the minimum match score must be between 0 and 100, got %d", s.MinMatchScore)
	}
	if s.MaxApplicationsPerRun < 1 || s.MaxApplicationsPerRun > maxApplicationsPerRunCeiling {
		return fmt.Errorf("a run must be allowed to create between 1 and %d applications, got %d",
			maxApplicationsPerRunCeiling, s.MaxApplicationsPerRun)
	}
	return nil
}

// ScoutVerdict is what a Scout decided about one job, and why.
//
// The reason travels with the decision rather than being reconstructed
// later: AC-004 requires a run to record what it rejected and why, and a
// reason derived separately from the decision is a second implementation
// that can disagree with the first.
type ScoutVerdict struct {
	JobID      string `json:"job_id"`
	JobTitle   string `json:"job_title"`
	Eligible   bool   `json:"eligible"`
	Reason     string `json:"reason"`
	Score      int    `json:"score"`
	HasScore   bool   `json:"has_score"`
	Considered bool   `json:"considered"`
}

// ScoutRunReport is the outcome of one run.
type ScoutRunReport struct {
	ScoutID    string         `json:"scout_id"`
	RunID      string         `json:"run_id"`
	Considered int            `json:"considered"`
	Created    int            `json:"created"`
	Verdicts   []ScoutVerdict `json:"verdicts"`
	CeilingHit bool           `json:"ceiling_hit"`
	Error      string         `json:"error,omitempty"`
}

// EligibilityInput is everything a Scout needs to judge one job.
type EligibilityInput struct {
	Job      *Job
	Criteria SearchCriteria
	// Score is the job's match score, and HasScore says whether one could
	// be obtained. A job that could not be scored is not silently treated
	// as scoring zero, nor as clearing the floor.
	Score    int
	HasScore bool
	// AlreadyApplied reports whether an application exists for this job.
	AlreadyApplied bool
	// CreatedSoFar is how many applications this run has already made.
	CreatedSoFar int
}

// PreScreen applies the checks that do not need a match score, and says
// why when a job fails one.
//
// It exists because scoring is the expensive step: it calls the model once
// per job. Running it on a job the saved search already excludes, or one
// the user has already applied to, is time they wait for nothing.
func (s Scout) PreScreen(in EligibilityInput) (bool, string) {
	switch {
	case in.Job == nil:
		return false, "the job could not be read"
	case !in.Criteria.Matches(*in.Job):
		return false, "does not match the saved search"
	case in.AlreadyApplied:
		return false, "an application for this job already exists"
	case in.CreatedSoFar >= s.MaxApplicationsPerRun:
		return false, fmt.Sprintf("this run already created its limit of %d applications",
			s.MaxApplicationsPerRun)
	}
	return true, ""
}

// Eligible decides whether a job earns an application, and says why.
//
// Every check here is arithmetic or a comparison against stored facts. No
// model is consulted (AGENTS.md principle 7). The match score is the
// model's, as it already was; whether a score clears a floor is not a
// judgement call.
//
// The reason travels with the decision rather than being reconstructed by
// the caller, because AC-004 requires a run to record why it rejected
// something, and a reason derived separately is a second implementation
// that can disagree with the first.
func (s Scout) Eligible(in EligibilityInput) (bool, string) {
	if ok, reason := s.PreScreen(in); !ok {
		return false, reason
	}
	if !in.HasScore {
		// Not scoring zero, and not clearing the floor either. A job
		// nobody could judge is not a job judged badly.
		return false, "could not be scored against the candidate"
	}
	if in.Score < s.MinMatchScore {
		return false, fmt.Sprintf("match score %d is below the scout's floor of %d",
			in.Score, s.MinMatchScore)
	}
	return true, fmt.Sprintf("match score %d clears the scout's floor of %d", in.Score, s.MinMatchScore)
}
