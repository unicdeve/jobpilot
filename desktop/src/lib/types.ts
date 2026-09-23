// Mirrors the JSON shapes produced by internal/core/domain (Go). Field
// names match the Go structs' `json:` tags exactly — no transformation
// layer, so the wire format here is identical to what
// internal/adapters/primary/httpapi already serves and tests.

// A fact a form asks for separately is stored separately (ADR-0015):
// email and phone were one `contact` string until a real form's phone field
// was filled with an email address, and links were one string until
// LinkedIn, Portfolio and Website each received all of them.
export type LinkKind =
  | "linkedin"
  | "github"
  | "portfolio"
  | "website"
  | "twitter"
  | "other";

export const LINK_KINDS: LinkKind[] = [
  "linkedin",
  "github",
  "portfolio",
  "website",
  "twitter",
  "other",
];

export interface CandidateLink {
  id: string;
  candidate_id: string;
  kind: LinkKind;
  url: string;
  // Server-assigned. A row the UI has just created has neither, and
  // sending an empty string for one is rejected outright.
  created_at?: string;
  updated_at?: string;
}

export interface WorkAuthorization {
  id: string;
  candidate_id: string;
  country: string;
  is_authorized: boolean;
  requires_sponsorship: boolean;
  // Server-assigned. A row the UI has just created has neither, and
  // sending an empty string for one is rejected outright.
  created_at?: string;
  updated_at?: string;
}

export interface CandidateSkill {
  id: string;
  candidate_id: string;
  name: string;
  years_experience?: number | null;
  // Server-assigned. A row the UI has just created has neither, and
  // sending an empty string for one is rejected outright.
  created_at?: string;
  updated_at?: string;
}

export interface CandidateExperience {
  id: string;
  candidate_id: string;
  company: string;
  title: string;
  start_date?: string | null;
  end_date?: string | null;
  is_current: boolean;
  description: string;
  // Server-assigned. A row the UI has just created has neither, and
  // sending an empty string for one is rejected outright.
  created_at?: string;
  updated_at?: string;
}

export type RemotePreference = "" | "remote" | "hybrid" | "onsite";

export interface Candidate {
  id: string;
  first_name: string;
  last_name: string;
  email: string;
  phone: string;
  location_city: string;
  location_region: string;
  location_country: string;
  notice_period_days?: number | null;
  earliest_start_date?: string | null;
  summary: string;
  remote_preference: RemotePreference;
  application_preferences: string;
  links: CandidateLink[] | null;
  work_authorizations: WorkAuthorization[] | null;
  skills: CandidateSkill[] | null;
  experiences: CandidateExperience[] | null;
  // Server-assigned, like the rows above.
  created_at?: string;
  updated_at?: string;
}

// An LLM-proposed profile read from a resume, pending the user's review
// before any of it is applied (ADR-0011). It proposes no work
// authorization: a resume does not reliably say which countries someone may
// work in, and a guess there becomes a claim about their right to work.
export interface ProfileDraft {
  first_name: string;
  last_name: string;
  email: string;
  phone: string;
  location_city: string;
  location_region: string;
  location_country: string;
  summary: string;
  skills: { name: string; years_experience?: number | null }[] | null;
  experiences:
    | {
        company: string;
        title: string;
        start_date: string;
        end_date: string;
        is_current: boolean;
        description: string;
      }[]
    | null;
  links: { kind: LinkKind; url: string }[] | null;
}

export interface Resume {
  id: string;
  candidate_id: string;
  name: string;
  original_file_path: string;
  markdown_content: string;
  parsed_metadata: string;
  created_at: string;
  updated_at: string;
}

export interface Job {
  id: string;
  canonical_url: string;
  source_id: string;
  source_job_id: string;
  title: string;
  company_name: string;
  company_url: string;
  description_markdown: string;
  locations: string[];
  remote_type: string;
  employment_type: string;
  salary_min: number;
  salary_max: number;
  salary_currency: string;
  salary_period: string;
  experience_min: number;
  experience_max: number;
  skills: string[];
  posted_at: string;
  expires_at: string;
  discovered_at: string;
  updated_at: string;
  raw_source_reference: string;
}

/// JobPage is one page of stored jobs plus the total matching the same
/// filter, so a caller can tell whether more pages remain.
export interface JobPage {
  jobs: Job[];
  total: number;
}

/// SearchCriteria mirrors domain.SearchCriteria. Every field here is applied
/// by the daemon; fields no source could populate were removed rather than
/// accepted and ignored.
export interface SearchCriteria {
  keywords?: string;
  title?: string;
  location?: string;
  remote_preference?: string;
  employment_type?: string;
  salary_min?: number;
  salary_max?: number;
  excluded_companies?: string[];
}

/// SavedSearch is a named set of criteria the user keeps and re-runs.
/// Discovery only runs against one of these, so an installation with none
/// cannot import jobs at all.
export interface SavedSearch {
  id: string;
  name: string;
  criteria: SearchCriteria;
  is_default: boolean;
  created_at: string;
  updated_at: string;
}

export interface JobMatch {
  id: string;
  job_id: string;
  candidate_id: string;
  score: number;
  confidence: number;
  strengths: string[];
  gaps: string[];
  concerns: string[];
  matched_skills: string[];
  missing_skills: string[];
  rationale: string;
  model_provider: string;
  model_name: string;
  created_at: string;
}

export interface LLMSettings {
  provider: string;
  base_url: string;
  model: string;
}

/// LLMHealth reports whether a generation request would work right now.
/// The two failures need different advice, so they are separate flags:
/// nothing configured yet, versus configured but not answering.
export interface LLMHealth {
  configured: boolean;
  ok: boolean;
  error?: string;
}

export interface SaveLLMSettingsResult {
  settings: LLMSettings;
  connectivity_error?: string;
}

export type ApplicationMode = "assist" | "review";

export type ApplicationState =
  | "DISCOVERED"
  | "MATCHED"
  | "PREPARING"
  | "READY_FOR_REVIEW"
  | "APPROVED"
  | "SUBMITTED"
  | "INTERVIEWING"
  | "OFFERED"
  | "NO_RESPONSE"
  | "REJECTED"
  | "WITHDRAWN"
  | "FAILED";

export interface Application {
  id: string;
  job_id: string;
  candidate_id: string;
  resume_id: string;
  mode: ApplicationMode;
  state: ApplicationState;
  current_cover_letter_id: string;
  policy_result: string;
  approval_status: string;
  submitted_at?: string;
  failure_reason: string;
  created_at: string;
  updated_at: string;
}

export interface ApplicationStateTransition {
  id: string;
  application_id: string;
  from_state: ApplicationState;
  to_state: ApplicationState;
  reason: string;
  created_at: string;
}

export interface CoverLetter {
  id: string;
  application_id: string;
  content: string;
  model_provider: string;
  model_name: string;
  user_edited: boolean;
  created_at: string;
}

export type QuestionType = "factual" | "narrative";

export type AnswerCategory =
  | "profile_fact"
  | "derived"
  | "generated"
  | "needs_user_input"
  | "";

export type QuestionStatus = "pending" | "answered";

export type QuestionSource = "manual" | "form_extracted";

export interface ApplicationQuestion {
  id: string;
  application_id: string;
  label: string;
  type: QuestionType;
  required: boolean;
  answer: string;
  answer_source: string;
  category: AnswerCategory;
  confidence: number;
  user_edited: boolean;
  status: QuestionStatus;
  source: QuestionSource;
  field_ref?: string;
}

export type AgentRunStatus = "running" | "completed" | "blocked" | "failed";

export interface AgentRun {
  id: string;
  application_id: string;
  task_type: string;
  status: AgentRunStatus;
  started_at: string;
  completed_at?: string;
  error?: string;
}

export interface AgentEvent {
  id: string;
  run_id: string;
  action: string;
  tool: string;
  status: string;
  input_summary?: string;
  output_summary?: string;
  policy_result?: string;
  duration_ms: number;
  created_at: string;
}

export interface AgentRunWithEvents {
  run: AgentRun;
  events: AgentEvent[];
}
