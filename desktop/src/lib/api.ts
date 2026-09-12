// A typed client for the jobpilot daemon's loopback HTTP API, mirroring
// internal/adapters/primary/httpapi/client.go (Go) method-for-method. See
// docs/09_decisions/0012-loopback-http-ipc.md.

import type {
  AgentRunWithEvents,
  Application,
  ApplicationMode,
  ApplicationQuestion,
  ApplicationState,
  ApplicationStateTransition,
  Candidate,
  CoverLetter,
  Job,
  JobMatch,
  JobPage,
  LLMSettings,
  ProfileDraft,
  QuestionType,
  Resume,
  SaveLLMSettingsResult,
  SearchCriteria,
} from "./types";

export class ApiError extends Error {}

export interface ApiClient {
  health(): Promise<void>;

  getProfile(): Promise<Candidate>;
  updateProfile(profile: Candidate): Promise<Candidate>;
  initializeProfile(profile: Partial<Candidate>): Promise<Candidate>;

  importResume(candidateId: string, filePath: string): Promise<Resume>;
  listResumes(candidateId: string): Promise<Resume[]>;
  deleteResume(id: string): Promise<void>;
  proposeProfileDraft(resumeMarkdown: string): Promise<ProfileDraft>;

  /// listJobs reads stored discovery history and contacts no job source,
  /// so unlike searchJobs it works offline. A limit of 0 means the daemon's
  /// default page size.
  listJobs(keywords: string, limit: number, offset: number): Promise<JobPage>;
  getJob(id: string): Promise<Job>;
  searchJobs(criteria: SearchCriteria): Promise<Job[]>;
  importJob(url: string): Promise<Job>;

  analyzeMatch(candidateId: string, jobId: string): Promise<JobMatch>;
  listMatches(candidateId: string): Promise<JobMatch[]>;

  getLLMSettings(): Promise<LLMSettings>;
  saveLLMSettings(settings: LLMSettings): Promise<SaveLLMSettingsResult>;

  createApplication(
    jobId: string,
    candidateId: string,
    resumeId: string,
    mode: ApplicationMode,
  ): Promise<Application>;
  listApplications(candidateId: string): Promise<Application[]>;
  getApplication(id: string): Promise<Application>;
  transitionApplication(
    id: string,
    toState: ApplicationState,
    reason: string,
  ): Promise<Application>;
  listApplicationHistory(id: string): Promise<ApplicationStateTransition[]>;

  generateCoverLetter(applicationId: string): Promise<CoverLetter>;
  listCoverLetterVersions(applicationId: string): Promise<CoverLetter[]>;
  editCoverLetter(coverLetterId: string, content: string): Promise<CoverLetter>;

  addQuestion(
    applicationId: string,
    label: string,
    type: QuestionType,
    required: boolean,
  ): Promise<ApplicationQuestion>;
  listQuestions(applicationId: string): Promise<ApplicationQuestion[]>;
  resolveQuestion(questionId: string): Promise<ApplicationQuestion>;
  setQuestionAnswer(
    questionId: string,
    answer: string,
  ): Promise<ApplicationQuestion>;

  prepareInBrowser(applicationId: string): Promise<Application>;
  listAgentRuns(applicationId: string): Promise<AgentRunWithEvents[]>;
}

async function request<T>(
  baseUrl: string,
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(`${baseUrl}${path}`, {
    method,
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  }).catch((err) => {
    throw new ApiError(`failed to reach jobpilot daemon: ${err}`);
  });

  if (!res.ok) {
    let message = `daemon returned status ${res.status}`;
    try {
      const errBody = (await res.json()) as { error?: string };
      if (errBody.error) message = errBody.error;
    } catch {
      // response wasn't JSON; keep the generic status message
    }
    throw new ApiError(message);
  }

  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export function createApiClient(port: number): ApiClient {
  const baseUrl = `http://127.0.0.1:${port}`;
  const q = (params: Record<string, string>) => "?" + new URLSearchParams(params).toString();

  return {
    health: () => request<void>(baseUrl, "GET", "/v1/health"),

    getProfile: () => request<Candidate>(baseUrl, "GET", "/v1/profile"),
    updateProfile: (profile) => request<Candidate>(baseUrl, "PUT", "/v1/profile", profile),
    initializeProfile: (profile) =>
      request<Candidate>(baseUrl, "POST", "/v1/onboarding/profile", profile),

    importResume: (candidateId, filePath) =>
      request<Resume>(baseUrl, "POST", "/v1/resumes", {
        candidate_id: candidateId,
        file_path: filePath,
      }),
    listResumes: (candidateId) =>
      request<Resume[]>(baseUrl, "GET", "/v1/resumes" + q({ candidate_id: candidateId })),
    deleteResume: (id) => request<void>(baseUrl, "DELETE", `/v1/resumes/${encodeURIComponent(id)}`),
    proposeProfileDraft: (resumeMarkdown) =>
      request<ProfileDraft>(baseUrl, "POST", "/v1/resumes/propose-draft", {
        resume_markdown: resumeMarkdown,
      }),

    listJobs: (keywords, limit, offset) => {
      const params: Record<string, string> = {};
      if (keywords) params.q = keywords;
      if (limit > 0) params.limit = String(limit);
      if (offset > 0) params.offset = String(offset);
      const query = new URLSearchParams(params).toString();
      return request<JobPage>(baseUrl, "GET", "/v1/jobs" + (query ? `?${query}` : ""));
    },
    getJob: (id) => request<Job>(baseUrl, "GET", `/v1/jobs/${encodeURIComponent(id)}`),
    searchJobs: (criteria) => request<Job[]>(baseUrl, "POST", "/v1/jobs/search", criteria),
    importJob: (url) => request<Job>(baseUrl, "POST", "/v1/jobs/import", { url }),

    analyzeMatch: (candidateId, jobId) =>
      request<JobMatch>(baseUrl, "POST", "/v1/matches/analyze", {
        candidate_id: candidateId,
        job_id: jobId,
      }),
    listMatches: (candidateId) =>
      request<JobMatch[]>(baseUrl, "GET", "/v1/matches" + q({ candidate_id: candidateId })),

    getLLMSettings: () => request<LLMSettings>(baseUrl, "GET", "/v1/settings/llm"),
    saveLLMSettings: (settings) =>
      request<SaveLLMSettingsResult>(baseUrl, "PUT", "/v1/settings/llm", settings),

    createApplication: (jobId, candidateId, resumeId, mode) =>
      request<Application>(baseUrl, "POST", "/v1/applications", {
        job_id: jobId,
        candidate_id: candidateId,
        resume_id: resumeId,
        mode,
      }),
    listApplications: (candidateId) =>
      request<Application[]>(baseUrl, "GET", "/v1/applications" + q({ candidate_id: candidateId })),
    getApplication: (id) =>
      request<Application>(baseUrl, "GET", `/v1/applications/${encodeURIComponent(id)}`),
    transitionApplication: (id, toState, reason) =>
      request<Application>(baseUrl, "POST", `/v1/applications/${encodeURIComponent(id)}/transition`, {
        to_state: toState,
        reason,
      }),
    listApplicationHistory: (id) =>
      request<ApplicationStateTransition[]>(
        baseUrl,
        "GET",
        `/v1/applications/${encodeURIComponent(id)}/history`,
      ),

    generateCoverLetter: (applicationId) =>
      request<CoverLetter>(
        baseUrl,
        "POST",
        `/v1/applications/${encodeURIComponent(applicationId)}/cover-letter`,
      ),
    listCoverLetterVersions: (applicationId) =>
      request<CoverLetter[]>(
        baseUrl,
        "GET",
        `/v1/applications/${encodeURIComponent(applicationId)}/cover-letters`,
      ),
    editCoverLetter: (coverLetterId, content) =>
      request<CoverLetter>(baseUrl, "PUT", `/v1/cover-letters/${encodeURIComponent(coverLetterId)}`, {
        content,
      }),

    addQuestion: (applicationId, label, type, required) =>
      request<ApplicationQuestion>(
        baseUrl,
        "POST",
        `/v1/applications/${encodeURIComponent(applicationId)}/questions`,
        { label, type, required },
      ),
    listQuestions: (applicationId) =>
      request<ApplicationQuestion[]>(
        baseUrl,
        "GET",
        `/v1/applications/${encodeURIComponent(applicationId)}/questions`,
      ),
    resolveQuestion: (questionId) =>
      request<ApplicationQuestion>(baseUrl, "POST", `/v1/questions/${encodeURIComponent(questionId)}/resolve`),
    setQuestionAnswer: (questionId, answer) =>
      request<ApplicationQuestion>(baseUrl, "PUT", `/v1/questions/${encodeURIComponent(questionId)}/answer`, {
        answer,
      }),

    prepareInBrowser: (applicationId) =>
      request<Application>(
        baseUrl,
        "POST",
        `/v1/applications/${encodeURIComponent(applicationId)}/prepare-in-browser`,
      ),
    listAgentRuns: (applicationId) =>
      request<AgentRunWithEvents[]>(
        baseUrl,
        "GET",
        `/v1/applications/${encodeURIComponent(applicationId)}/agent-runs`,
      ),
  };
}
