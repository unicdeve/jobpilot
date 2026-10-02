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
  /// The IANA time zone the candidate works in, such as Africa/Lagos, or
  /// empty (ADR-0025).
  time_zone?: string;
  notice_period_days?: number | null;
  earliest_start_date?: string | null;
  summary: string;
  remote_preference: RemotePreference;
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
  /// When the user ruled the job out; absent while it is in their lists.
  not_interested_at?: string;
  /// Where the job can be done from, and what it says about relocation and
  /// visa sponsorship, as the daemon reads them (ADR-0023).
  workable: Workable;
  relocation: Offer;
  visa_sponsorship: Offer;
  /// The working hours the job asks for, as the daemon reads them (ADR-0025).
  hours?: WorkingHours;
}

/// HoursStatus is whether a job requires its working hours or only prefers
/// them. Empty is a job that asks nothing about when the candidate works.
export type HoursStatus = "required" | "preferred" | "";

/// WorkingHours mirrors domain.WorkingHours: a window of UTC offsets without
/// daylight saving, and the sentence it was read from.
export interface WorkingHours {
  status: HoursStatus;
  from: number;
  to: number;
  evidence?: string;
}

/// WorkableScope is how far a job says it can be done from.
export type WorkableScope = "anywhere" | "listed" | "unknown";

/// Workable mirrors domain.Workable. places are the job's locations as the
/// daemon read them ("EMEA", "Berlin"); countries are their ISO codes,
/// regions expanded. note is the posting's sentence that made the job
/// unknown, when one did.
export interface Workable {
  scope: WorkableScope;
  places: string[] | null;
  countries: string[] | null;
  note?: string;
}

/// OfferStatus is what a posting says about relocation or visa sponsorship.
/// Empty is a posting that does not mention it.
export type OfferStatus = "offered" | "mentioned" | "declined" | "";

/// Offer mirrors domain.Offer: a status and the sentence it came from.
export interface Offer {
  status: OfferStatus;
  evidence?: string;
}

/// Country is one country a search can work from.
export interface Country {
  code: string;
  name: string;
}

/// Countries mirrors GET /v1/countries: every country, and the profile's
/// own as a code, empty when it has none JobPilot can place.
export interface Countries {
  countries: Country[];
  profile_country: string;
  time_zones: TimeZone[];
}

/// TimeZone is an IANA zone, its country, and its offset from UTC in hours
/// without daylight saving.
export interface TimeZone {
  id: string;
  country: string;
  offset: number;
}

/// JobListFilter narrows the stored jobs the daemon lists. Every field is
/// optional; an empty one excludes nothing.
export interface JobListFilter {
  keywords?: string;
  source?: string;
  remote?: string;
  /// Keep jobs posted at or after this instant. A job whose source gave no
  /// posting date is judged by when it was found.
  postedSince?: Date;
  /// Which jobs to include by whether they were ruled out. Absent lists the
  /// ones the user has not, as every list does by default.
  interest?: "not_interested" | "all";
  /// The country the candidate works from, as a code: keeps the jobs open
  /// there or anywhere, and the ones that do not say.
  workFrom?: string;
  /// With workFrom, also keeps jobs elsewhere that offer or mention
  /// relocation or visa sponsorship.
  includeRelocation?: boolean;
}

/// JobPage is one page of stored jobs plus the total matching the same
/// filter, so a caller can tell whether more pages remain.
export interface JobPage {
  jobs: Job[];
  total: number;
}

/// SourceStatus is how one job source answered a search. "unreachable" is
/// no answer at all, which is what being offline looks like; "failed" is an
/// answer that was not jobs.
export type SourceStatus = "ok" | "unreachable" | "failed";

/// SourceInfo mirrors domain.SourceInfo: a source a search asks.
export interface SourceInfo {
  id: string;
  name: string;
}

/// SourceOutcome mirrors domain.SourceOutcome. It carries no error text:
/// the daemon logs the detail and never returns it.
export interface SourceOutcome {
  source_id: string;
  source_name: string;
  status: SourceStatus;
  found: number;
  /// failing names the parts of a source that did not answer: the watched
  /// companies whose pages failed (ADR-0024 §5).
  failing?: string[];
}

/// DiscoveryResult mirrors domain.DiscoveryResult: the jobs a search
/// matched, how many were not stored before, and how each source answered.
export interface DiscoveryResult {
  jobs: Job[];
  new: number;
  sources: SourceOutcome[];
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
  /// The ISO code of the country the candidate works from, or empty.
  work_from?: string;
  /// With work_from, also keeps jobs elsewhere that help the candidate move.
  include_relocation?: boolean;
  /// Leaves out jobs whose required hours the profile's time zone cannot
  /// work, moving the candidate's day by up to hours_shift hours.
  fit_hours?: boolean;
  hours_shift?: number;
}

/// SavedSearch is a named set of criteria the user keeps and re-runs.
/// Discovery only runs against one of these, so an installation with none
/// cannot import jobs at all.
export interface SavedSearch {
  id: string;
  name: string;
  criteria: SearchCriteria;
  is_default: boolean;
  /// last_run_at is when the search last ran against the job sources and
  /// one answered; null if it never has.
  last_run_at: string | null;
  created_at: string;
  updated_at: string;
}

/// DiscoverySettings mirrors domain.DiscoverySettings.
export interface DiscoverySettings {
  /// refresh_on_launch runs the searches not run in the last six hours
  /// when the app opens.
  refresh_on_launch: boolean;
}

/// CredentialRef names one stored secret. It never carries the value: the
/// daemon has no route that returns one, because it has no authentication
/// and a readable secret would be less protected there than in a file
/// (ADR-0019 §4).
export interface CredentialRef {
  source: string;
  label: string;
}

/// SearchTemplate is a ready-made saved search. Criteria are matched
/// strictly — every word must appear, and keywords are read against the whole
/// job description — so a search that reads well can still return almost
/// nothing. These are criteria that have been measured against live postings.
export interface SearchTemplate {
  id: string;
  name: string;
  summary: string;
  criteria: SearchCriteria;
}

/// SearchTemplateGroup is the set of templates for one kind of work. It is a
/// menu, not a bundle: criteria cannot express "or", so Engineer and
/// Developer are separate entries and adopting one alone hides the other.
export interface SearchTemplateGroup {
  id: string;
  name: string;
  summary: string;
  templates: SearchTemplate[];
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

/// DraftFlag is something in a letter or written answer the reviewer
/// should look at, worked out from its text each time it is read
/// (docs/02_features/19_draft_checks.md).
export interface DraftFlag {
  kind: "stock_phrase" | "length" | "unsupported_claim";
  /// The flagged words as the text has them; a length flag has none.
  excerpt?: string;
  reason: string;
}

export interface CoverLetter {
  id: string;
  application_id: string;
  content: string;
  model_provider: string;
  model_name: string;
  user_edited: boolean;
  created_at: string;
  flags?: DraftFlag[];
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

/// PersonalAnswerKey mirrors domain.PersonalAnswerKey: what a thing the
/// candidate said about themselves is (ADR-0022).
export type PersonalAnswerKey = string;

/// PersonalAnswer mirrors domain.PersonalAnswer. Only the candidate writes
/// answer; a proposed question is the model's, read from the posting.
export interface PersonalAnswer {
  id: string;
  candidate_id?: string;
  application_id?: string;
  key: PersonalAnswerKey;
  question: string;
  answer: string;
  created_at: string;
  updated_at: string;
}

/// AboutYouQuestion is a standing question, its wording the daemon's, and
/// the candidate's answer to it.
export interface AboutYouQuestion {
  key: PersonalAnswerKey;
  question: string;
  hint: string;
  answer: string;
}

/// AboutYou mirrors services.AboutYou (docs/02_features/16_about_you.md).
export interface AboutYou {
  questions: AboutYouQuestion[];
  writing_sample: string;
  writing_preferences: string;
  kept: PersonalAnswer[];
}

/// AboutYouUpdate changes some of what the candidate said; a field left
/// out is unchanged, and an empty answer clears one.
export interface AboutYouUpdate {
  answers?: Record<PersonalAnswerKey, string>;
  writing_sample?: string;
  writing_preferences?: string;
}

/// ApplicationAnswer answers one question asked before a letter: the fixed
/// one by key, a proposed one by id.
export interface ApplicationAnswer {
  id?: string;
  key: PersonalAnswerKey;
  answer: string;
}

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
  flags?: DraftFlag[];
}

export type AgentRunStatus = "running" | "completed" | "blocked" | "failed";

/// What a run was doing. Mirrors domain.AgentTask* in the daemon.
export type AgentTaskType = "prepare_application" | "scout_run";

export interface AgentRun {
  id: string;
  application_id: string;
  /// Set for a scout run, which belongs to no single application.
  scout_id?: string;
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

/// A run as the activity list shows it: named by the scout that started
/// it, or by the job its application is for. Either name is absent once
/// the scout or application has been deleted; the run itself is kept.
export interface AgentRunSummary extends AgentRun {
  scout_name?: string;
  job_title?: string;
  company?: string;
  event_count: number;
}

export interface AgentRunPage {
  runs: AgentRunSummary[];
  total: number;
}

export interface AgentRunDetail {
  run: AgentRunSummary;
  events: AgentEvent[];
}

/// Narrows the activity list. An empty field means every value.
export interface AgentRunFilter {
  task_type: AgentTaskType | "";
  status: AgentRunStatus | "";
}

export type ScoutSource = "local" | "sources";

/**
 * A Scout is a standing instruction: run a saved search, judge what it
 * finds against the candidate, and create applications to review.
 *
 * It carries no salary floor, company blocklist or location rule — its
 * saved search holds those already (ADR-0018). It never contacts an
 * employer and never submits, and there is no field here that could
 * change that.
 */
export interface Scout {
  id: string;
  name: string;
  saved_search_id: string;
  min_match_score: number;
  max_applications_per_run: number;
  source: ScoutSource;
  is_enabled: boolean;
  last_run_at?: string | null;
  created_at?: string;
  updated_at?: string;
}

/** What a Scout decided about one job, and why. */
export interface ScoutVerdict {
  job_id: string;
  job_title: string;
  eligible: boolean;
  reason: string;
  score: number;
  has_score: boolean;
  considered: boolean;
}

/** The outcome of one Scout run. */
export interface ScoutRunReport {
  scout_id: string;
  run_id: string;
  considered: number;
  created: number;
  verdicts: ScoutVerdict[] | null;
  ceiling_hit: boolean;
  error?: string;
}

/// DeletionTarget mirrors domain.DeletionTarget: what can be deleted. The
/// first four name one record by ID.
export type DeletionTarget =
  | "job"
  | "application"
  | "cover_letter"
  | "resume"
  | "profile"
  | "activity"
  | "jobs"
  | "everything";

/// RecordKind mirrors domain.RecordKind, the kinds of record a deletion
/// reports by. A kind a newer daemon adds is shown by its name.
export type RecordKind = string;

/// DeletionReach mirrors domain.DeletionReach: what a deletion takes with it,
/// shown before it happens, and the token that carrying it out requires.
export interface DeletionReach {
  target: DeletionTarget;
  id?: string;
  deleted: Record<RecordKind, number>;
  detached: Record<RecordKind, number>;
  sent_applications: number;
  kept_not_interested?: number;
  outside?: string[];
  environment_credentials?: string[];
  token: string;
}

/// DeletionResult mirrors domain.DeletionResult.
export interface DeletionResult {
  reach: DeletionReach;
  notices?: string[];
}

/// BoardProvider is an applicant-tracking system whose hiring pages
/// JobPilot reads (docs/02_features/17_company_watchlists.md).
export type BoardProvider = "greenhouse" | "lever" | "ashby" | "workable" | "smartrecruiters";

/// Board mirrors domain.Board: one company's hiring page on a provider.
export interface Board {
  provider: BoardProvider;
  region?: string;
  name: string;
}

/// BoardCheck is how reading a watched company's page last went; empty
/// before the first.
export type BoardCheck = "" | "ok" | "gone" | "unreachable" | "failed";

/// WatchedCompany mirrors domain.WatchedCompany.
export interface WatchedCompany {
  id: string;
  board: Board;
  name: string;
  /// url is the company's hiring page.
  url: string;
  added_from: string;
  added_at: string;
  checked_at?: string;
  check: BoardCheck;
  /// open_roles is how many postings the page held when last read, before
  /// any search's criteria.
  open_roles: number;
}

/// StarterCompany mirrors domain.StarterCompany: a suggestion, with what
/// was measured to put it on the list.
export interface StarterCompany {
  board: Board;
  name: string;
  url: string;
  measured_on: string;
  open_roles: number;
  qualifying_roles: number;
}

/// JobCompany is the hiring page a job's posting is on, and the watched
/// company it is, if any.
export interface JobCompany {
  board: Board | null;
  url?: string;
  company: WatchedCompany | null;
}
