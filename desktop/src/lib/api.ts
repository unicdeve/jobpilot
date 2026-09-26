// A typed client for the jobpilot daemon's loopback HTTP API, mirroring
// internal/adapters/primary/httpapi/client.go (Go) method-for-method. See
// docs/09_decisions/0012-loopback-http-ipc.md.

import type {
  AgentRunDetail,
  AgentRunFilter,
  AgentRunPage,
  AgentRunWithEvents,
  Application,
  ApplicationMode,
  ApplicationQuestion,
  ApplicationState,
  ApplicationStateTransition,
  Candidate,
  CoverLetter,
  DeletionReach,
  DeletionResult,
  DeletionTarget,
  DiscoveryResult,
  Job,
  JobMatch,
  JobListFilter,
  JobPage,
  LLMHealth,
  LLMSettings,
  ProfileDraft,
  QuestionType,
  Resume,
  SavedSearch,
  SaveLLMSettingsResult,
  CredentialRef,
  SearchCriteria,
  SearchTemplateGroup,
  SourceInfo,
  Scout,
  ScoutRunReport,
  AgentRun,
  AgentEvent,
} from "./types";

/**
 * ScoutInput is the part of a Scout a caller owns. Identity, timestamps
 * and the last-run time are the daemon's.
 */
export type ScoutInput = Omit<Scout, "id" | "last_run_at" | "created_at" | "updated_at">;

/// ApiError is a failed daemon request. status is the HTTP status, absent
/// when the daemon could not be reached at all.
///
/// Its message is shown to the user as it is, so it reads as a sentence:
/// the daemon's errors are Go's, which start in lower case, and String(err)
/// would otherwise put "Error: " in front of every one.
///
/// reach is set on the one error that carries data: a deletion refused
/// because what it reaches changed since it was shown (ADR-0021 §4).
export class ApiError extends Error {
  readonly status?: number;
  readonly reach?: DeletionReach;

  constructor(message: string, status?: number, reach?: DeletionReach) {
    super(message.charAt(0).toUpperCase() + message.slice(1));
    this.status = status;
    this.reach = reach;
  }

  override toString(): string {
    return this.message;
  }
}

export interface ApiClient {
  health(): Promise<void>;

  /// getProfile resolves to null when there is no profile yet, which is how
  /// every installation starts rather than something that went wrong.
  getProfile(): Promise<Candidate | null>;
  updateProfile(profile: Candidate): Promise<Candidate>;
  initializeProfile(profile: Partial<Candidate>): Promise<Candidate>;

  importResume(candidateId: string, filePath: string): Promise<Resume>;
  listResumes(candidateId: string): Promise<Resume[]>;
  /// previewDeletion returns what deleting target would take with it, and
  /// the token deleteRecords needs. A preview deletes nothing.
  previewDeletion(target: DeletionTarget, id?: string): Promise<DeletionReach>;
  /// deleteRecords carries out the deletion a preview's token names. If what
  /// it reaches changed since, it throws an ApiError carrying the new reach.
  deleteRecords(target: DeletionTarget, id: string | undefined, token: string): Promise<DeletionResult>;
  /// getJobMatch returns a job's latest match, or null if it has none. It
  /// finds one for a job ruled out, which listMatches leaves out.
  getJobMatch(jobId: string): Promise<JobMatch | null>;
  /// setJobInterest rules a job out, or back in. Both are reversible.
  setJobInterest(id: string, notInterested: boolean): Promise<Job>;
  proposeProfileDraft(resumeMarkdown: string): Promise<ProfileDraft>;

  /// listJobs reads stored discovery history and contacts no job source,
  /// so unlike searchJobs it works offline. A limit of 0 means the daemon's
  /// default page size.
  listJobs(filter: JobListFilter, limit: number, offset: number): Promise<JobPage>;
  getJob(id: string): Promise<Job>;
  searchJobs(criteria: SearchCriteria): Promise<DiscoveryResult>;
  /// listSources names the sources a search asks, in the order it asks them.
  listSources(): Promise<SourceInfo[]>;

  listSavedSearches(): Promise<SavedSearch[]>;
  createSavedSearch(name: string, criteria: SearchCriteria): Promise<SavedSearch>;
  updateSavedSearch(id: string, name: string, criteria: SearchCriteria): Promise<SavedSearch>;
  deleteSavedSearch(id: string): Promise<void>;
  setDefaultSavedSearch(id: string): Promise<SavedSearch>;
  listSearchTemplates(): Promise<SearchTemplateGroup[]>;
  /// There is no getCredential. A credential goes in and never comes out.
  listCredentials(): Promise<CredentialRef[]>;
  setCredential(source: string, label: string, secret: string): Promise<CredentialRef>;
  deleteCredential(source: string, label: string): Promise<void>;
  importSearchTemplates(templateIds: string[]): Promise<SavedSearch[]>;
  /// runSavedSearch discovers jobs using a saved search's criteria. The
  /// daemon reads the criteria itself, so no caller has to hold them and
  /// post them back — the shape that allowed an empty search to be sent.
  runSavedSearch(id: string): Promise<DiscoveryResult>;
  importJob(url: string): Promise<Job>;

  listScouts(): Promise<Scout[]>;
  createScout(scout: ScoutInput): Promise<Scout>;
  updateScout(id: string, scout: ScoutInput): Promise<Scout>;
  deleteScout(id: string): Promise<void>;
  /// runScout runs a scout once. A run that fails partway still returns a
  /// report, because the applications it created before failing are real.
  runScout(id: string): Promise<ScoutRunReport>;
  listScoutRuns(limit: number): Promise<AgentRun[]>;
  listScoutRunEvents(runId: string): Promise<AgentEvent[]>;

  /// listActivity pages through every agent run, newest first: browser
  /// preparation and scout runs alike. A limit of 0 means the daemon's
  /// default page size.
  listActivity(filter: AgentRunFilter, limit: number, offset: number): Promise<AgentRunPage>;
  getActivityRun(id: string): Promise<AgentRunDetail>;

  analyzeMatch(candidateId: string, jobId: string): Promise<JobMatch>;
  listMatches(candidateId: string): Promise<JobMatch[]>;

  getLLMSettings(): Promise<LLMSettings>;
  checkLLMHealth(): Promise<LLMHealth>;
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
  /// listAllowedTransitions asks the daemon which states this application
  /// may move to next, so the UI offers only actions that will work.
  listAllowedTransitions(id: string): Promise<ApplicationState[]>;

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
  }).catch((err: unknown) => {
    // fetch says only "Failed to fetch", whatever went wrong, so the detail
    // goes to the console rather than in front of the user.
    console.error(err);
    throw new ApiError("could not reach the JobPilot daemon; it may have stopped");
  });

  if (!res.ok) {
    let message = `daemon returned status ${res.status}`;
    let reach: DeletionReach | undefined;
    try {
      const errBody = (await res.json()) as { error?: string; reach?: DeletionReach };
      if (errBody.error) message = errBody.error;
      reach = errBody.reach;
    } catch {
      // response wasn't JSON; keep the generic status message
    }
    throw new ApiError(message, res.status, reach);
  }

  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

/// deletionPath is where target lives, as the daemon's deletion routes name
/// it (internal/adapters/primary/httpapi/deletion.go).
function deletionPath(target: DeletionTarget, id?: string): string {
  const one = (collection: string) => `/v1/${collection}/${encodeURIComponent(id ?? "")}`;
  switch (target) {
    case "job":
      return one("jobs");
    case "application":
      return one("applications");
    case "cover_letter":
      return one("cover-letters");
    case "resume":
      return one("resumes");
    case "profile":
      return "/v1/profile";
    case "activity":
      return "/v1/data/activity";
    case "jobs":
      return "/v1/data/jobs";
    case "everything":
      return "/v1/data";
  }
}

export function createApiClient(port: number): ApiClient {
  const baseUrl = `http://127.0.0.1:${port}`;
  const q = (params: Record<string, string>) => "?" + new URLSearchParams(params).toString();
  const credentialPath = (source: string, label: string) =>
    `/v1/credentials/${encodeURIComponent(source)}/${encodeURIComponent(label || "default")}`;

  return {
    health: () => request<void>(baseUrl, "GET", "/v1/health"),

    getProfile: () =>
      request<Candidate>(baseUrl, "GET", "/v1/profile").catch((err: unknown) => {
        if (err instanceof ApiError && err.status === 404) return null;
        throw err;
      }),
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
    previewDeletion: (target, id) => request<DeletionReach>(baseUrl, "GET", deletionPath(target, id) + "/deletion"),
    deleteRecords: (target, id, token) =>
      request<DeletionResult>(baseUrl, "DELETE", deletionPath(target, id) + q({ token })),
    getJobMatch: (jobId) =>
      request<JobMatch>(baseUrl, "GET", `/v1/jobs/${encodeURIComponent(jobId)}/match`).catch((err: unknown) => {
        if (err instanceof ApiError && err.status === 404) return null;
        throw err;
      }),
    setJobInterest: (id, notInterested) =>
      request<Job>(baseUrl, "PUT", `/v1/jobs/${encodeURIComponent(id)}/interest`, { not_interested: notInterested }),
    proposeProfileDraft: (resumeMarkdown) =>
      request<ProfileDraft>(baseUrl, "POST", "/v1/resumes/propose-draft", {
        resume_markdown: resumeMarkdown,
      }),

    listJobs: (filter, limit, offset) => {
      const params: Record<string, string> = {};
      if (filter.keywords) params.q = filter.keywords;
      if (filter.source) params.source = filter.source;
      if (filter.remote) params.remote = filter.remote;
      if (filter.postedSince) params.posted_since = filter.postedSince.toISOString();
      if (filter.interest) params.interest = filter.interest;
      if (limit > 0) params.limit = String(limit);
      if (offset > 0) params.offset = String(offset);
      const query = new URLSearchParams(params).toString();
      return request<JobPage>(baseUrl, "GET", "/v1/jobs" + (query ? `?${query}` : ""));
    },
    getJob: (id) => request<Job>(baseUrl, "GET", `/v1/jobs/${encodeURIComponent(id)}`),
    searchJobs: (criteria) => request<DiscoveryResult>(baseUrl, "POST", "/v1/jobs/search", criteria),
    listSources: () => request<SourceInfo[]>(baseUrl, "GET", "/v1/sources"),

    listSavedSearches: () => request<SavedSearch[]>(baseUrl, "GET", "/v1/searches"),
    createSavedSearch: (name, criteria) =>
      request<SavedSearch>(baseUrl, "POST", "/v1/searches", { name, criteria }),
    updateSavedSearch: (id, name, criteria) =>
      request<SavedSearch>(baseUrl, "PUT", `/v1/searches/${encodeURIComponent(id)}`, {
        name,
        criteria,
      }),
    deleteSavedSearch: (id) =>
      request<void>(baseUrl, "DELETE", `/v1/searches/${encodeURIComponent(id)}`),
    listSearchTemplates: () =>
      request<SearchTemplateGroup[]>(baseUrl, "GET", "/v1/search-templates"),
    listCredentials: () => request<CredentialRef[]>(baseUrl, "GET", "/v1/credentials"),
    setCredential: (source, label, secret) =>
      request<CredentialRef>(baseUrl, "PUT", credentialPath(source, label), { secret }),
    deleteCredential: (source, label) =>
      request<void>(baseUrl, "DELETE", credentialPath(source, label)),
    importSearchTemplates: (templateIds) =>
      request<SavedSearch[]>(baseUrl, "POST", "/v1/search-templates/import", {
        template_ids: templateIds,
      }),
    listScouts: () => request<Scout[]>(baseUrl, "GET", "/v1/scouts"),
    createScout: (scout) => request<Scout>(baseUrl, "POST", "/v1/scouts", scout),
    updateScout: (id, scout) =>
      request<Scout>(baseUrl, "PUT", `/v1/scouts/${encodeURIComponent(id)}`, scout),
    deleteScout: (id) => request<void>(baseUrl, "DELETE", `/v1/scouts/${encodeURIComponent(id)}`),
    runScout: (id) =>
      request<ScoutRunReport>(baseUrl, "POST", `/v1/scouts/${encodeURIComponent(id)}/run`),
    listScoutRuns: (limit) =>
      request<AgentRun[]>(baseUrl, "GET", `/v1/scout-runs?limit=${limit}`),
    listScoutRunEvents: (runId) =>
      request<AgentEvent[]>(baseUrl, "GET", `/v1/scout-runs/${encodeURIComponent(runId)}/events`),

    listActivity: (filter, limit, offset) => {
      const params: Record<string, string> = {};
      if (filter.task_type) params.task_type = filter.task_type;
      if (filter.status) params.status = filter.status;
      if (limit > 0) params.limit = String(limit);
      if (offset > 0) params.offset = String(offset);
      const query = new URLSearchParams(params).toString();
      return request<AgentRunPage>(baseUrl, "GET", "/v1/agent-runs" + (query ? `?${query}` : ""));
    },
    getActivityRun: (id) =>
      request<AgentRunDetail>(baseUrl, "GET", `/v1/agent-runs/${encodeURIComponent(id)}`),

    setDefaultSavedSearch: (id) =>
      request<SavedSearch>(baseUrl, "POST", `/v1/searches/${encodeURIComponent(id)}/default`),
    runSavedSearch: (id) =>
      request<DiscoveryResult>(baseUrl, "POST", `/v1/searches/${encodeURIComponent(id)}/run`),
    importJob: (url) => request<Job>(baseUrl, "POST", "/v1/jobs/import", { url }),

    analyzeMatch: (candidateId, jobId) =>
      request<JobMatch>(baseUrl, "POST", "/v1/matches/analyze", {
        candidate_id: candidateId,
        job_id: jobId,
      }),
    listMatches: (candidateId) =>
      request<JobMatch[]>(baseUrl, "GET", "/v1/matches" + q({ candidate_id: candidateId })),

    getLLMSettings: () => request<LLMSettings>(baseUrl, "GET", "/v1/settings/llm"),
    checkLLMHealth: () => request<LLMHealth>(baseUrl, "GET", "/v1/settings/llm/health"),
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
    listAllowedTransitions: (id) =>
      request<ApplicationState[]>(
        baseUrl,
        "GET",
        `/v1/applications/${encodeURIComponent(id)}/transitions`,
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
