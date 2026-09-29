package sqlite

import "github.com/unicdev/jobpilot/internal/core/domain"

// recordTables says which kind of record each table holds.
//
// It is the one place the adapter translates between the tables and the
// product's words for what is in them, so domain.DeletionRules can be
// checked against the schema's foreign keys (ADR-0021 §3). Every table
// JobPilot creates is listed; a table missing from here fails that check.
var recordTables = map[string]domain.RecordKind{
	"candidates":                    domain.RecordCandidate,
	"candidate_experiences":         domain.RecordCandidateDetail,
	"candidate_skills":              domain.RecordCandidateDetail,
	"candidate_links":               domain.RecordCandidateDetail,
	"candidate_work_authorizations": domain.RecordCandidateDetail,
	"resumes":                       domain.RecordResume,
	"jobs":                          domain.RecordJob,
	"job_matches":                   domain.RecordMatch,
	"applications":                  domain.RecordApplication,
	"application_questions":         domain.RecordQuestion,
	"cover_letters":                 domain.RecordCoverLetter,
	"application_state_transitions": domain.RecordStateTransition,
	"agent_runs":                    domain.RecordAgentRun,
	"agent_events":                  domain.RecordAgentStep,
	"scouts":                        domain.RecordScout,
	"saved_searches":                domain.RecordSavedSearch,
	"credential_names":              domain.RecordCredentialName,
	"user_settings":                 domain.RecordSettings,
	"personal_answers":              domain.RecordPersonalAnswer,
}
