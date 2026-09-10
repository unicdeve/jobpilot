// Mirrors the JSON shapes produced by internal/core/domain (Go). Field
// names match the Go structs' `json:` tags exactly — no transformation
// layer, so the wire format here is identical to what
// internal/adapters/primary/httpapi already serves and tests.

export interface Candidate {
  id: string;
  first_name: string;
  last_name: string;
  contact: string;
  location: string;
  summary: string;
  work_preferences: string;
  salary_preferences: string;
  work_authorization: string;
  application_preferences: string;
  skills: string;
  experiences: string;
  education: string;
  certifications: string;
  links: string;
  created_at: string;
  updated_at: string;
}

export interface ProfileDraft {
  first_name: string;
  last_name: string;
  contact: string;
  location: string;
  summary: string;
  skills: string[];
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

export interface SearchCriteria {
  keywords?: string;
  title?: string;
  location?: string;
  remote_preference?: string;
  employment_type?: string;
  salary_min?: number;
  salary_max?: number;
  experience_min?: number;
  experience_max?: number;
  included_sources?: string[];
  excluded_companies?: string[];
  custom_urls?: string[];
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

export interface SaveLLMSettingsResult {
  settings: LLMSettings;
  connectivity_error?: string;
}

export type ApplicationMode = "assist" | "review" | "autopilot";

export type ApplicationState =
  | "DISCOVERED"
  | "MATCHED"
  | "PREPARING"
  | "READY_FOR_REVIEW"
  | "APPROVED"
  | "SUBMITTED"
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
