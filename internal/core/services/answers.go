package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// ErrEmptyQuestion is returned for a question with no text.
var ErrEmptyQuestion = errors.New("a question cannot be empty")

// AnswerService keeps the questions the candidate notes on an application
// and the answers they write to them (docs/02_features/07_application_answers.md).
//
// It never writes an answer (ADR-0034 §4). It may suggest one, but only
// from a deterministic source: a profile fact, a derivation from the
// profile, or an answer the candidate kept. A suggestion is worked out on
// every read and never stored, so nothing is filled in on the candidate's
// behalf; they use it, edit it, or ignore it.
type AnswerService struct {
	questionRepo ports.ApplicationQuestionRepository
	appRepo      ports.ApplicationRepository
	jobRepo      ports.JobRepository
	candRepo     ports.CandidateRepository
	resumeRepo   ports.ResumeRepository
	about        *AboutYouService
}

// NewAnswerService constructs an AnswerService.
func NewAnswerService(
	questionRepo ports.ApplicationQuestionRepository,
	appRepo ports.ApplicationRepository,
	jobRepo ports.JobRepository,
	candRepo ports.CandidateRepository,
	resumeRepo ports.ResumeRepository,
	about *AboutYouService,
) *AnswerService {
	return &AnswerService{
		questionRepo: questionRepo,
		appRepo:      appRepo,
		jobRepo:      jobRepo,
		candRepo:     candRepo,
		resumeRepo:   resumeRepo,
		about:        about,
	}
}

// AddQuestion notes a question on an application, unanswered.
func (s *AnswerService) AddQuestion(ctx context.Context, applicationID, label string) (*domain.ApplicationQuestion, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, ErrEmptyQuestion
	}
	if _, err := s.appRepo.GetByID(ctx, applicationID); err != nil {
		return nil, fmt.Errorf("failed to fetch application: %w", err)
	}

	question := &domain.ApplicationQuestion{ApplicationID: applicationID, Label: label}
	if err := s.questionRepo.Save(ctx, question); err != nil {
		return nil, fmt.Errorf("failed to add application question: %w", err)
	}
	if err := s.annotate(ctx, applicationID, question); err != nil {
		return nil, err
	}
	return question, nil
}

// ListByApplication returns every question on an application, oldest
// first, each with its suggestion and its answer's draft flags.
func (s *AnswerService) ListByApplication(ctx context.Context, applicationID string) ([]*domain.ApplicationQuestion, error) {
	questions, err := s.questionRepo.ListByApplication(ctx, applicationID)
	if err != nil {
		return nil, fmt.Errorf("failed to list application questions: %w", err)
	}
	if len(questions) == 0 {
		return questions, nil
	}
	if err := s.annotate(ctx, applicationID, questions...); err != nil {
		return nil, err
	}
	return questions, nil
}

// SetAnswer records the candidate's answer to a question. An empty answer
// clears it (AC-004).
func (s *AnswerService) SetAnswer(ctx context.Context, questionID, answer string) (*domain.ApplicationQuestion, error) {
	question, err := s.questionRepo.GetByID(ctx, questionID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch question: %w", err)
	}

	question.Answer = strings.TrimSpace(strings.ReplaceAll(answer, "\r\n", "\n"))
	if err := s.questionRepo.Update(ctx, question); err != nil {
		return nil, fmt.Errorf("failed to save the answer: %w", err)
	}
	if err := s.annotate(ctx, question.ApplicationID, question); err != nil {
		return nil, err
	}
	return question, nil
}

// DeleteQuestion removes a question and its answer.
func (s *AnswerService) DeleteQuestion(ctx context.Context, questionID string) error {
	if _, err := s.questionRepo.GetByID(ctx, questionID); err != nil {
		return fmt.Errorf("failed to fetch question: %w", err)
	}
	if err := s.questionRepo.Delete(ctx, questionID); err != nil {
		return fmt.Errorf("failed to delete question: %w", err)
	}
	return nil
}

// annotate sets each question's suggestion and its answer's draft flags,
// from the profile as it is now. The questions are all one application's.
func (s *AnswerService) annotate(ctx context.Context, applicationID string, questions ...*domain.ApplicationQuestion) error {
	application, err := s.appRepo.GetByID(ctx, applicationID)
	if err != nil {
		return fmt.Errorf("failed to fetch application: %w", err)
	}
	candidate, err := s.candRepo.GetByID(ctx, application.CandidateID)
	if err != nil {
		return fmt.Errorf("failed to fetch candidate: %w", err)
	}
	// A job that cannot be loaded is not fatal: the one resolver that
	// needs it, work authorization, declines without one.
	job, err := s.jobRepo.GetByID(ctx, application.JobID)
	if err != nil {
		job = nil
	}
	kept, err := s.about.KeptAnswers(ctx, candidate.ID)
	if err != nil {
		return err
	}

	var answered []*domain.ApplicationQuestion
	for _, question := range questions {
		question.Suggestion = suggestAnswer(candidate, job, kept, question.Label)
		if question.Answer != "" {
			answered = append(answered, question)
		}
	}
	if len(answered) == 0 {
		return nil
	}

	basis, err := draftBasis(ctx, application, s.candRepo, s.resumeRepo, s.about, false)
	if err != nil {
		return fmt.Errorf("failed to check the answer: %w", err)
	}
	for _, question := range answered {
		question.Flags = domain.CheckDraft(question.Answer, basis)
	}
	return nil
}

// suggestAnswer is the deterministic resolver chain
// (docs/02_features/07_application_answers.md): a profile fact, then work
// authorization, then a derivation, then an answer the candidate kept to a
// similar question. Nothing else suggests anything, so a question none of
// them answers is left to the candidate alone.
func suggestAnswer(c *domain.Candidate, job *domain.Job, kept []domain.PersonalAnswer, label string) *domain.AnswerSuggestion {
	if field, answer := profileFactAnswer(c, label); answer != "" {
		return &domain.AnswerSuggestion{Answer: answer, Source: domain.SuggestionFromProfile, Field: field}
	}
	if field, answer := workAuthorizationAnswer(c, job, label); answer != "" {
		return &domain.AnswerSuggestion{Answer: answer, Source: domain.SuggestionFromProfile, Field: field}
	}
	if field, answer := derivedAnswer(c, label); answer != "" {
		return &domain.AnswerSuggestion{Answer: answer, Source: domain.SuggestionDerived, Field: field}
	}
	if relevant := domain.RelevantKeptAnswers(kept, label); len(relevant) > 0 {
		return &domain.AnswerSuggestion{Answer: relevant[0].Answer, Source: domain.SuggestionKeptAnswer, Field: relevant[0].Question}
	}
	return nil
}

// decisionOpeners begin a label that is asking the candidate to decide
// something rather than naming a fact about them.
//
// This one guard removes most of the resolver's measured mistakes.
// "Where are you based?" wants a location. "Are you currently based in any
// of these countries? Please note these are the only countries where we are
// accepting applications" contains the same word and wants a choice, and
// the old matcher answered it with the candidate's city.
//
// It is a prefix test on purpose: "Where are you based?" contains "are you"
// but does not open with it.
var decisionOpeners = []string{
	"are you", "do you", "did you", "have you", "has ", "will you",
	"would you", "can you", "could you", "is there", "please confirm",
	"please select", "by submitting", "i acknowledge", "i confirm",
}

// factRule maps a question to a fact the profile holds. Order matters:
// "first name" must be tried before "name", and "city" before "location",
// because a real form asks for both and the narrower reading is the right
// one.
type factRule struct {
	// source is recorded on the answer so the user can see where it came
	// from, and so a wrong answer can be traced to the rule that made it.
	source string
	match  func(label string) bool
	value  func(c *domain.Candidate) string
}

func linkValue(kind domain.LinkKind) func(*domain.Candidate) string {
	return func(c *domain.Candidate) string {
		link, ok := c.LinkOf(kind)
		if !ok {
			return ""
		}
		return link.URL
	}
}

// factRules is the whole mapping from a form's words to a candidate fact.
//
// Every entry is here because a real recorded form asks for it
// (internal/adapters/secondary/browser/testdata/ats), and each kind of link
// has its own rule because a form asks for them one at a time — which is
// the defect ADR-0015 exists to fix. One "links" string could only ever put
// every URL into each of LinkedIn, GitHub and Website.
var factRules = []factRule{
	{
		source: "candidate.first_name",
		match:  labelMentions("first name", "given name", "forename"),
		value:  func(c *domain.Candidate) string { return c.FirstName },
	},
	{
		source: "candidate.last_name",
		match:  labelMentions("last name", "surname", "family name"),
		value:  func(c *domain.Candidate) string { return c.LastName },
	},
	{
		source: "candidate.name",
		// Not every "name" is the candidate's: a form asks for the current
		// employer's too, and one of those in the wrong box is the same
		// class of mistake as an email in a phone field.
		match: both(
			labelMentions("name"),
			labelAvoids("company", "employer", "school", "university", "college",
				"institution", "reference", "referee", "username", "user name",
				"nickname", "file name", "filename"),
		),
		value: func(c *domain.Candidate) string { return c.FullName() },
	},
	{
		source: "candidate.email",
		match:  labelMentions("email", "e-mail", "courriel"),
		value:  func(c *domain.Candidate) string { return c.Email },
	},
	{
		// The defect this whole change started from: phone shared a rule
		// with email, so a real form's phone field was filled with the
		// candidate's email address.
		source: "candidate.phone",
		match:  labelMentions("phone", "mobile", "telephone", "téléphone", "cell"),
		value:  func(c *domain.Candidate) string { return c.Phone },
	},
	{
		source: "candidate.links.linkedin",
		match:  labelMentions("linkedin"),
		value:  linkValue(domain.LinkKindLinkedIn),
	},
	{
		source: "candidate.links.github",
		match:  labelMentions("github"),
		value:  linkValue(domain.LinkKindGitHub),
	},
	{
		source: "candidate.links.twitter",
		match:  labelMentions("twitter"),
		value:  linkValue(domain.LinkKindTwitter),
	},
	{
		source: "candidate.links.portfolio",
		match:  labelMentions("portfolio"),
		value:  linkValue(domain.LinkKindPortfolio),
	},
	{
		source: "candidate.links.website",
		match:  labelMentions("website", "web site", "personal site", "blog"),
		value:  linkValue(domain.LinkKindWebsite),
	},
	{
		source: "candidate.location_country",
		match:  labelMentions("country"),
		value:  func(c *domain.Candidate) string { return c.LocationCountry },
	},
	{
		source: "candidate.location_city",
		match:  labelMentions("city", "town"),
		value:  func(c *domain.Candidate) string { return c.LocationCity },
	},
	{
		source: "candidate.location_region",
		match:  labelMentions("state", "province", "region", "county"),
		value:  func(c *domain.Candidate) string { return c.LocationRegion },
	},
	{
		source: "candidate.location",
		match:  labelMentions("location", "where are you based", "based in", "are you based"),
		value:  func(c *domain.Candidate) string { return c.LocationDisplay() },
	},
	{
		// Asked by two of the three recorded forms, and unanswerable
		// before the work history had dates.
		source: "candidate.experiences.current.company",
		match: both(
			labelMentions("current company", "current employer", "present employer", "current organisation", "current organization"),
			labelAvoids("why", "how long"),
		),
		value: func(c *domain.Candidate) string {
			experience, ok := c.CurrentExperience()
			if !ok {
				return ""
			}
			return experience.Company
		},
	},
	{
		source: "candidate.experiences.current.title",
		match:  labelMentions("current title", "current role", "current position", "current job title"),
		value: func(c *domain.Candidate) string {
			experience, ok := c.CurrentExperience()
			if !ok {
				return ""
			}
			return experience.Title
		},
	},
	{
		source: "candidate.notice_period_days",
		match:  labelMentions("notice period", "notice"),
		value:  noticePeriodAnswer,
	},
	{
		source: "candidate.earliest_start_date",
		match:  labelMentions("start date", "available", "availability", "when can you start"),
		value:  availabilityAnswer,
	},
}

// profileFactAnswer looks a question up against the profile.
//
// It answers only labels that name a fact. A label that opens by asking the
// candidate something is a decision, and a decision is not a lookup — see
// decisionOpeners.
func profileFactAnswer(c *domain.Candidate, label string) (source, answer string) {
	l := normalizeLabel(label)
	if l == "" || isDecisionQuestion(l) {
		return "", ""
	}

	for _, rule := range factRules {
		if !rule.match(l) {
			continue
		}
		// A rule that matched but has nothing to say is not an answer:
		// fall through and let the user be asked, rather than filling a
		// form field with an empty string.
		if value := strings.TrimSpace(rule.value(c)); value != "" {
			return rule.source, value
		}
	}

	return "", ""
}

// workAuthorizationAnswer answers a sponsorship or right-to-work question,
// and is deliberately allowed to answer decision-shaped labels — "Will you
// require Visa Sponsorship now, or in the future?" is a decision, and the
// profile genuinely knows it.
//
// It answers only about a country the candidate has actually spoken about,
// matched by name against the job's own stated locations. A sponsorship
// answer for the wrong country is worse than no answer: it is a claim about
// someone's right to work, made to an employer, under their name.
func workAuthorizationAnswer(c *domain.Candidate, job *domain.Job, label string) (source, answer string) {
	if job == nil || len(c.WorkAuthorizations) == 0 {
		return "", ""
	}

	l := normalizeLabel(label)
	asksSponsorship := labelMentions("sponsorship", "sponsor", "visa")(l)
	asksAuthorization := labelMentions("authorized to work", "authorised to work", "work authorization",
		"work authorisation", "right to work", "work permit", "eligible to work", "legally able to work")(l)
	if !asksSponsorship && !asksAuthorization {
		return "", ""
	}

	where := strings.ToLower(strings.Join(job.Locations, " "))
	if where == "" {
		return "", ""
	}
	for _, auth := range c.WorkAuthorizations {
		country := strings.ToLower(strings.TrimSpace(auth.Country))
		if country == "" || !strings.Contains(where, country) {
			continue
		}
		// Sponsorship first: a label can mention both, and "will you need
		// sponsorship" is the narrower question.
		if asksSponsorship {
			return "candidate.work_authorization." + country, yesNo(auth.RequiresSponsorship)
		}
		return "candidate.work_authorization." + country, yesNo(auth.IsAuthorized)
	}

	return "", ""
}

func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

// noticePeriodAnswer renders a notice period, where zero is an answer.
func noticePeriodAnswer(c *domain.Candidate) string {
	if c.NoticePeriodDays == nil {
		return ""
	}
	switch days := *c.NoticePeriodDays; {
	case days <= 0:
		return "Immediately"
	case days == 1:
		return "1 day"
	default:
		return fmt.Sprintf("%d days", days)
	}
}

// availabilityAnswer prefers a stated date, because "available from
// 2026-11-01" is a more useful answer than "60 days" to a form that asked
// when someone can start.
func availabilityAnswer(c *domain.Candidate) string {
	if c.EarliestStartDate != nil {
		return c.EarliestStartDate.Format(time.DateOnly)
	}
	return noticePeriodAnswer(c)
}

func normalizeLabel(label string) string {
	return strings.ToLower(strings.TrimSpace(label))
}

func isDecisionQuestion(normalizedLabel string) bool {
	for _, opener := range decisionOpeners {
		if strings.HasPrefix(normalizedLabel, opener) {
			return true
		}
	}
	return false
}

// labelMentions builds a matcher for a label containing any of the terms.
func labelMentions(terms ...string) func(string) bool {
	return func(label string) bool {
		for _, term := range terms {
			if strings.Contains(label, term) {
				return true
			}
		}
		return false
	}
}

// labelAvoids builds a matcher for a label containing none of the terms.
func labelAvoids(terms ...string) func(string) bool {
	return func(label string) bool {
		for _, term := range terms {
			if strings.Contains(label, term) {
				return false
			}
		}
		return true
	}
}

func both(a, b func(string) bool) func(string) bool {
	return func(label string) bool { return a(label) && b(label) }
}

// derivedAnswer computes an answer from candidate data rather than looking
// up a single field.
//
// These are the answers ADR-0015 stage two exists to make possible: a
// paragraph of work history cannot be counted, and until it had dates the
// only rule here was about remote work.
func derivedAnswer(c *domain.Candidate, label string) (source, answer string) {
	l := normalizeLabel(label)

	if source, answer := remoteWorkAnswer(c, l); answer != "" {
		return source, answer
	}

	if source, answer := yearsOfExperienceAnswer(c, l, time.Now()); answer != "" {
		return source, answer
	}

	return "", ""
}

// remoteWorkAnswer answers a question about working remotely.
//
// This rule existed before ADR-0015 and had never fired for anybody: it
// read Candidate.WorkPreferences, a prose column no interface ever offered
// a field for, so it was always empty. A structured preference is what
// makes it reachable.
func remoteWorkAnswer(c *domain.Candidate, normalizedLabel string) (source, answer string) {
	if c.RemotePreference == "" || !strings.Contains(normalizedLabel, "remote") {
		return "", ""
	}
	if !labelMentions("open", "willing", "able", "comfortable", "interested", "happy")(normalizedLabel) {
		return "", ""
	}

	switch c.RemotePreference {
	case domain.RemotePreferenceRemote:
		return "candidate.remote_preference", "Yes, I am looking for remote work."
	case domain.RemotePreferenceHybrid:
		return "candidate.remote_preference", "Yes, I am open to remote work, and prefer hybrid."
	case domain.RemotePreferenceOnsite:
		// An honest no is worth as much as an honest yes, and is not the
		// same as leaving the question blank.
		return "candidate.remote_preference", "I am looking for onsite work."
	default:
		return "", ""
	}
}

// yearsOfExperienceAnswer answers "how many years…", for a named skill when
// the label names one, and otherwise for the candidate's career as a whole.
//
// A named skill is answered only from what the candidate said about that
// skill. Falling back to their total career would answer "how many years of
// Rust?" with the time they have spent writing Go, which is a fact invented
// about a person for an employer to read.
func yearsOfExperienceAnswer(c *domain.Candidate, normalizedLabel string, now time.Time) (source, answer string) {
	if !strings.Contains(normalizedLabel, "years") && !strings.Contains(normalizedLabel, "how long") {
		return "", ""
	}
	if !strings.Contains(normalizedLabel, "experience") && !strings.Contains(normalizedLabel, "worked") &&
		!strings.Contains(normalizedLabel, "using") && !strings.Contains(normalizedLabel, "how long") {
		return "", ""
	}

	if skill, ok := skillNamedInLabel(c, normalizedLabel); ok {
		if skill.YearsExperience == nil {
			// The skill is theirs but the duration is not recorded, and
			// this is the one question where that distinction matters.
			return "", ""
		}
		return "derived:skills." + strings.ToLower(skill.Name), formatYears(*skill.YearsExperience)
	}

	// The question named something, and it is not a skill the candidate
	// claims. Their career total is not an answer to it: "how many years
	// with Rust?" answered with the decade they spent writing Go is a fact
	// invented about a person for an employer to read.
	if asksAboutSomethingSpecific(normalizedLabel) {
		return "", ""
	}

	years, ok := c.TotalYearsOfExperience(now)
	if !ok {
		return "", ""
	}
	return "derived:experiences", formatYears(years)
}

// asksAboutSomethingSpecific reports whether a duration question is
// qualified — "years of experience *with* Kubernetes" — as opposed to
// asking about the candidate's career as a whole.
func asksAboutSomethingSpecific(normalizedLabel string) bool {
	return strings.Contains(normalizedLabel, " with ") ||
		strings.Contains(normalizedLabel, " using ") ||
		strings.Contains(normalizedLabel, " of experience in ") ||
		strings.Contains(normalizedLabel, " working with ")
}

// skillNamedInLabel finds the candidate's own skill that a question is
// about, matching whole words so that "Go" does not match "Google" and "R"
// does not match every other word on the form.
func skillNamedInLabel(c *domain.Candidate, normalizedLabel string) (domain.CandidateSkill, bool) {
	var best domain.CandidateSkill
	found := false
	for _, skill := range c.Skills {
		name := strings.ToLower(strings.TrimSpace(skill.Name))
		if name == "" || !strings.Contains(normalizedLabel, name) {
			continue
		}
		if !isWholeWordMatch(normalizedLabel, name) {
			continue
		}
		// The longest match wins, so "React Native" beats "React" on a
		// question about React Native.
		if !found || len(name) > len(strings.TrimSpace(best.Name)) {
			best, found = skill, true
		}
	}
	return best, found
}

// formatYears renders a duration the way a person writes it on a form.
func formatYears(years float64) string {
	rounded := math.Round(years*10) / 10
	if rounded <= 0 {
		return ""
	}
	if rounded == math.Trunc(rounded) {
		return strconv.Itoa(int(rounded))
	}
	return strconv.FormatFloat(rounded, 'f', 1, 64)
}

// isWholeWordMatch reports whether phrase appears in text as whole words:
// "go" in "go developer", not in "google".
func isWholeWordMatch(text, phrase string) bool {
	index := strings.Index(text, phrase)
	for index >= 0 {
		beforeOK := index == 0 || !isWordChar(rune(text[index-1]))
		end := index + len(phrase)
		afterOK := end >= len(text) || !isWordChar(rune(text[end]))
		if beforeOK && afterOK {
			return true
		}
		next := strings.Index(text[index+1:], phrase)
		if next < 0 {
			return false
		}
		index += 1 + next
	}
	return false
}

func isWordChar(r rune) bool {
	return r == '_' || r == '\'' ||
		(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
