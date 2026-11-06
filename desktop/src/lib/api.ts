// A typed client for the jobpilot daemon's loopback HTTP API, mirroring
// internal/adapters/primary/httpapi/client.go (Go) method-for-method. See
// docs/09_decisions/0012-loopback-http-ipc.md.

import type {
  AboutYou,
  AboutYouUpdate,
  AgentEvent,
  AgentRun,
  AgentRunDetail,
  AgentRunFilter,
  AgentRunPage,
  Application,
  ApplicationQuestion,
  ApplicationState,
  ApplicationStateTransition,
  Candidate,
  Countries,
  CoverLetter,
  CredentialRef,
  DaemonHealth,
  DeletionReach,
  DeletionResult,
  DeletionTarget,
  DiscoveryResult,
  DiscoverySettings,
  Job,
  JobCompany,
  JobListFilter,
  JobMatch,
  JobPage,
  LLMHealth,
  LLMSettings,
  Notification,
  NotificationSettings,
  PersonalAnswer,
  ProfileDraft,
  PromptGuidance,
  PromptGuidanceList,
  PromptKey,
  Resume,
  SavedSearch,
  SaveLLMSettingsResult,
  Scout,
  ScoutRunReport,
  SearchCriteria,
  SearchTemplateGroup,
  SourceInfo,
  StarterCompany,
  WatchedCompany,
  Workspace,
} from "./types";

/**
 * ScoutInput is the part of a Scout a caller owns. Identity, timestamps,
 * the last-run time and what the daemon works out on reading are its own.
 */
export type ScoutInput = Omit<
  Scout,
  "id" | "last_run_at" | "created_at" | "updated_at" | "schedule_set_at" | "next_run_at" | "alarm" | "activity"
>;

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

/// errorMessage is how err reads to the user: its message, which for the
/// daemon's errors is already a sentence, rather than String(err), which
/// puts "Error: " or "TypeError: " in front of anything that is not an
/// ApiError.
export function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

/// isNotFound reports whether err is the daemon saying the record a request
/// named does not exist, such as an application deleted since its page was
/// last open.
export function isNotFound(err: unknown): boolean {
  return err instanceof ApiError && err.status === 404;
}

export interface ApiClient {
  health(): Promise<DaemonHealth>;

  listWorkspaces(): Promise<{ workspaces: Workspace[]; active_workspace_id: string }>;
  createWorkspace(name: string): Promise<Workspace>;
  renameWorkspace(id: string, name: string): Promise<Workspace>;
  activateWorkspace(id: string): Promise<Workspace>;

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
  /// staleSavedSearches lists the searches due to run again: never run, or
  /// not in the last six hours. The daemon decides which.
  staleSavedSearches(): Promise<SavedSearch[]>;
  createSavedSearch(name: string, criteria: SearchCriteria): Promise<SavedSearch>;
  updateSavedSearch(id: string, name: string, criteria: SearchCriteria): Promise<SavedSearch>;
  deleteSavedSearch(id: string): Promise<void>;
  setDefaultSavedSearch(id: string): Promise<SavedSearch>;
  listSearchTemplates(): Promise<SearchTemplateGroup[]>;
  /// There is no getCredential. A credential goes in and never comes out.
  listCredentials(): Promise<CredentialRef[]>;
  setCredential(source: string, label: string, secret: string): Promise<CredentialRef>;
  deleteCredential(source: string, label: string): Promise<void>;
  /// importSearchTemplates adopts templates as saved searches, each starting
  /// with workFrom as the country the candidate works from.
  importSearchTemplates(templateIds: string[], workFrom?: string): Promise<SavedSearch[]>;
  /// listCountries returns every country a search can work from, and the
  /// profile's own.
  listCountries(): Promise<Countries>;
  /// listCompanies returns the companies whose hiring pages are watched.
  listCompanies(): Promise<WatchedCompany[]>;
  /// watchCompany watches a company by the address of its hiring page or a
  /// posting on it, or by a stored job whose posting is on it. A company
  /// already watched resolves to itself.
  watchCompany(by: { address: string } | { job_id: string }): Promise<WatchedCompany>;
  renameCompany(id: string, name: string): Promise<WatchedCompany>;
  unwatchCompany(id: string): Promise<void>;
  /// companySuggestions is the starter list, less what is watched.
  companySuggestions(): Promise<StarterCompany[]>;
  /// jobCompany is the hiring page a stored job is on, if any.
  jobCompany(jobId: string): Promise<JobCompany>;
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

  /// listActivity pages through every agent run, newest first. A limit of
  /// 0 means the daemon's default page size.
  listActivity(filter: AgentRunFilter, limit: number, offset: number): Promise<AgentRunPage>;
  getActivityRun(id: string): Promise<AgentRunDetail>;

  analyzeMatch(candidateId: string, jobId: string): Promise<JobMatch>;
  listMatches(candidateId: string): Promise<JobMatch[]>;

  getLLMSettings(): Promise<LLMSettings>;
  /// listNotifications returns what JobPilot told the user, newest first:
  /// only what they have not seen in the app when unseen is set.
  listNotifications(unseen: boolean): Promise<Notification[]>;
  /// markNotificationsSeen marks one notification seen, or every one when
  /// no id is given.
  markNotificationsSeen(id?: string): Promise<void>;
  getNotificationSettings(): Promise<NotificationSettings>;
  saveNotificationSettings(settings: NotificationSettings): Promise<NotificationSettings>;
  listPromptGuidance(): Promise<PromptGuidanceList>;
  savePromptGuidance(key: PromptKey, text: string): Promise<PromptGuidance>;
  /// resetPromptGuidance puts a prompt back on its default.
  resetPromptGuidance(key: PromptKey): Promise<PromptGuidance>;
  getDiscoverySettings(): Promise<DiscoverySettings>;
  saveDiscoverySettings(settings: DiscoverySettings): Promise<DiscoverySettings>;
  checkLLMHealth(): Promise<LLMHealth>;
  saveLLMSettings(settings: LLMSettings): Promise<SaveLLMSettingsResult>;

  createApplication(jobId: string, candidateId: string, resumeId: string): Promise<Application>;
  listApplications(candidateId: string): Promise<Application[]>;
  getApplication(id: string): Promise<Application>;
  transitionApplication(id: string, toState: ApplicationState, reason: string): Promise<Application>;
  listApplicationHistory(id: string): Promise<ApplicationStateTransition[]>;
  /// listAllowedTransitions asks the daemon which states this application
  /// may move to next, so the UI offers only actions that will work.
  listAllowedTransitions(id: string): Promise<ApplicationState[]>;
  /// setApplyUrl records where the application form is, opened in place of
  /// the job's own link; an empty link clears it.
  setApplyUrl(applicationId: string, url: string): Promise<Application>;

  /// About you (docs/02_features/16_about_you.md). Only the candidate's
  /// words are ever an answer.
  /// getAboutYou resolves to null when there is no profile yet.
  getAboutYou(): Promise<AboutYou | null>;
  saveAboutYou(update: AboutYouUpdate): Promise<AboutYou>;
  deleteKeptAnswer(id: string): Promise<void>;
  keepAnswer(questionId: string): Promise<PersonalAnswer>;

  /// Cover letters the candidate writes (ADR-0034 §1): createCoverLetter
  /// saves a new version and makes it current; editCoverLetter changes one
  /// in place.
  createCoverLetter(applicationId: string, content: string): Promise<CoverLetter>;
  listCoverLetterVersions(applicationId: string): Promise<CoverLetter[]>;
  editCoverLetter(coverLetterId: string, content: string): Promise<CoverLetter>;

  /// Questions the candidate notes, and their own answers (ADR-0034 §4).
  addQuestion(applicationId: string, label: string): Promise<ApplicationQuestion>;
  listQuestions(applicationId: string): Promise<ApplicationQuestion[]>;
  setQuestionAnswer(questionId: string, answer: string): Promise<ApplicationQuestion>;
  deleteQuestion(questionId: string): Promise<void>;
}

/// Daemon is where a client's requests go, and who to tell when they
/// cannot get there.
interface Daemon {
  baseUrl: string;
  onUnreachable?: () => void;
  workspaceId?: string;
}

async function request<T>(daemon: Daemon, method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {};
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (daemon.workspaceId && !path.startsWith("/v1/workspaces")) {
    headers["X-JobPilot-Workspace"] = daemon.workspaceId;
  }
  const res = await fetch(`${daemon.baseUrl}${path}`, {
    method,
    headers: Object.keys(headers).length > 0 ? headers : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  }).catch((err: unknown) => {
    // fetch says only "Failed to fetch", whatever went wrong, so the detail
    // goes to the console rather than in front of the user.
    console.error(err);
    daemon.onUnreachable?.();
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

/// nullIfNotFound turns the daemon saying a record does not exist into
/// null, for the reads where that is an ordinary state rather than a failure.
function nullIfNotFound(err: unknown): null {
  if (isNotFound(err)) return null;
  throw err;
}

/// query is a query string from params, leaving out the ones not set, or ""
/// when none is.
function query(params: Record<string, string | undefined>): string {
  const set = Object.entries(params).filter((entry): entry is [string, string] => !!entry[1]);
  return set.length > 0 ? `?${new URLSearchParams(set).toString()}` : "";
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

/// createApiClient returns a client for the daemon on port. onUnreachable
/// is called whenever a request cannot reach it at all, which is how a
/// daemon that stopped or restarted on another port is noticed.
export function createApiClient(port: number, onUnreachable?: () => void, workspaceId?: string): ApiClient {
  const daemon: Daemon = { baseUrl: `http://127.0.0.1:${port}`, onUnreachable, workspaceId };
  const credentialPath = (source: string, label: string) =>
    `/v1/credentials/${encodeURIComponent(source)}/${encodeURIComponent(label || "default")}`;

  return {
    health: () => request<DaemonHealth>(daemon, "GET", "/v1/health"),
    listWorkspaces: () =>
      request<{ workspaces: Workspace[]; active_workspace_id: string }>(daemon, "GET", "/v1/workspaces"),
    createWorkspace: (name) => request<Workspace>(daemon, "POST", "/v1/workspaces", { name }),
    renameWorkspace: (id, name) =>
      request<Workspace>(daemon, "PUT", `/v1/workspaces/${encodeURIComponent(id)}`, { name }),
    activateWorkspace: (id) => request<Workspace>(daemon, "POST", `/v1/workspaces/${encodeURIComponent(id)}/activate`),

    getProfile: () => request<Candidate>(daemon, "GET", "/v1/profile").catch(nullIfNotFound),
    updateProfile: (profile) => request<Candidate>(daemon, "PUT", "/v1/profile", profile),
    initializeProfile: (profile) => request<Candidate>(daemon, "POST", "/v1/onboarding/profile", profile),

    importResume: (candidateId, filePath) =>
      request<Resume>(daemon, "POST", "/v1/resumes", {
        candidate_id: candidateId,
        file_path: filePath,
      }),
    listResumes: (candidateId) =>
      request<Resume[]>(daemon, "GET", `/v1/resumes${query({ candidate_id: candidateId })}`),
    previewDeletion: (target, id) => request<DeletionReach>(daemon, "GET", `${deletionPath(target, id)}/deletion`),
    deleteRecords: (target, id, token) =>
      request<DeletionResult>(daemon, "DELETE", deletionPath(target, id) + query({ token })),
    getJobMatch: (jobId) =>
      request<JobMatch>(daemon, "GET", `/v1/jobs/${encodeURIComponent(jobId)}/match`).catch(nullIfNotFound),
    setJobInterest: (id, notInterested) =>
      request<Job>(daemon, "PUT", `/v1/jobs/${encodeURIComponent(id)}/interest`, { not_interested: notInterested }),
    proposeProfileDraft: (resumeMarkdown) =>
      request<ProfileDraft>(daemon, "POST", "/v1/resumes/propose-draft", {
        resume_markdown: resumeMarkdown,
      }),

    listJobs: (filter, limit, offset) =>
      request<JobPage>(
        daemon,
        "GET",
        `/v1/jobs${query({
          q: filter.keywords,
          source: filter.source,
          remote: filter.remote,
          posted_since: filter.postedSince?.toISOString(),
          interest: filter.interest,
          work_from: filter.workFrom,
          include_relocation: filter.workFrom && filter.includeRelocation ? "true" : undefined,
          limit: limit > 0 ? String(limit) : undefined,
          offset: offset > 0 ? String(offset) : undefined,
        })}`,
      ),
    getJob: (id) => request<Job>(daemon, "GET", `/v1/jobs/${encodeURIComponent(id)}`),
    searchJobs: (criteria) => request<DiscoveryResult>(daemon, "POST", "/v1/jobs/search", criteria),
    listSources: () => request<SourceInfo[]>(daemon, "GET", "/v1/sources"),

    listSavedSearches: () => request<SavedSearch[]>(daemon, "GET", "/v1/searches"),
    staleSavedSearches: () => request<SavedSearch[]>(daemon, "GET", "/v1/searches/stale"),
    createSavedSearch: (name, criteria) => request<SavedSearch>(daemon, "POST", "/v1/searches", { name, criteria }),
    updateSavedSearch: (id, name, criteria) =>
      request<SavedSearch>(daemon, "PUT", `/v1/searches/${encodeURIComponent(id)}`, {
        name,
        criteria,
      }),
    deleteSavedSearch: (id) => request<void>(daemon, "DELETE", `/v1/searches/${encodeURIComponent(id)}`),
    listSearchTemplates: () => request<SearchTemplateGroup[]>(daemon, "GET", "/v1/search-templates"),
    listCredentials: () => request<CredentialRef[]>(daemon, "GET", "/v1/credentials"),
    setCredential: (source, label, secret) =>
      request<CredentialRef>(daemon, "PUT", credentialPath(source, label), { secret }),
    deleteCredential: (source, label) => request<void>(daemon, "DELETE", credentialPath(source, label)),
    importSearchTemplates: (templateIds, workFrom) =>
      request<SavedSearch[]>(daemon, "POST", "/v1/search-templates/import", {
        template_ids: templateIds,
        work_from: workFrom || undefined,
      }),
    listCountries: () => request<Countries>(daemon, "GET", "/v1/countries"),
    listCompanies: () =>
      request<{ companies: WatchedCompany[] }>(daemon, "GET", "/v1/companies").then((body) => body.companies),
    watchCompany: (by) => request<WatchedCompany>(daemon, "POST", "/v1/companies", by),
    renameCompany: (id, name) =>
      request<WatchedCompany>(daemon, "PUT", `/v1/companies/${encodeURIComponent(id)}`, { name }),
    unwatchCompany: (id) => request<void>(daemon, "DELETE", `/v1/companies/${encodeURIComponent(id)}`),
    companySuggestions: () =>
      request<{ suggestions: StarterCompany[] }>(daemon, "GET", "/v1/companies/suggestions").then(
        (body) => body.suggestions,
      ),
    jobCompany: (jobId) => request<JobCompany>(daemon, "GET", `/v1/jobs/${encodeURIComponent(jobId)}/company`),
    listScouts: () => request<Scout[]>(daemon, "GET", "/v1/scouts"),
    createScout: (scout) => request<Scout>(daemon, "POST", "/v1/scouts", scout),
    updateScout: (id, scout) => request<Scout>(daemon, "PUT", `/v1/scouts/${encodeURIComponent(id)}`, scout),
    deleteScout: (id) => request<void>(daemon, "DELETE", `/v1/scouts/${encodeURIComponent(id)}`),
    runScout: (id) => request<ScoutRunReport>(daemon, "POST", `/v1/scouts/${encodeURIComponent(id)}/run`),
    listScoutRuns: (limit) => request<AgentRun[]>(daemon, "GET", `/v1/scout-runs${query({ limit: String(limit) })}`),
    listScoutRunEvents: (runId) =>
      request<AgentEvent[]>(daemon, "GET", `/v1/scout-runs/${encodeURIComponent(runId)}/events`),

    listActivity: (filter, limit, offset) =>
      request<AgentRunPage>(
        daemon,
        "GET",
        `/v1/agent-runs${query({
          task_type: filter.task_type,
          status: filter.status,
          limit: limit > 0 ? String(limit) : undefined,
          offset: offset > 0 ? String(offset) : undefined,
        })}`,
      ),
    getActivityRun: (id) => request<AgentRunDetail>(daemon, "GET", `/v1/agent-runs/${encodeURIComponent(id)}`),

    setDefaultSavedSearch: (id) =>
      request<SavedSearch>(daemon, "POST", `/v1/searches/${encodeURIComponent(id)}/default`),
    runSavedSearch: (id) => request<DiscoveryResult>(daemon, "POST", `/v1/searches/${encodeURIComponent(id)}/run`),
    importJob: (url) => request<Job>(daemon, "POST", "/v1/jobs/import", { url }),

    analyzeMatch: (candidateId, jobId) =>
      request<JobMatch>(daemon, "POST", "/v1/matches/analyze", {
        candidate_id: candidateId,
        job_id: jobId,
      }),
    listMatches: (candidateId) =>
      request<JobMatch[]>(daemon, "GET", `/v1/matches${query({ candidate_id: candidateId })}`),

    getLLMSettings: () => request<LLMSettings>(daemon, "GET", "/v1/settings/llm"),
    listNotifications: async (unseen) =>
      (
        await request<{ notifications: Notification[] }>(
          daemon,
          "GET",
          `/v1/notifications${query({ unseen: unseen ? "true" : undefined })}`,
        )
      ).notifications,
    markNotificationsSeen: async (id) => {
      await request(daemon, "POST", id ? `/v1/notifications/${encodeURIComponent(id)}/seen` : "/v1/notifications/seen");
    },
    getNotificationSettings: () => request<NotificationSettings>(daemon, "GET", "/v1/settings/notifications"),
    saveNotificationSettings: (settings) =>
      request<NotificationSettings>(daemon, "PUT", "/v1/settings/notifications", settings),
    listPromptGuidance: () => request<PromptGuidanceList>(daemon, "GET", "/v1/settings/prompts"),
    savePromptGuidance: (key, text) =>
      request<PromptGuidance>(daemon, "PUT", `/v1/settings/prompts/${encodeURIComponent(key)}`, { text }),
    resetPromptGuidance: (key) =>
      request<PromptGuidance>(daemon, "DELETE", `/v1/settings/prompts/${encodeURIComponent(key)}`),
    getDiscoverySettings: () => request<DiscoverySettings>(daemon, "GET", "/v1/settings/discovery"),
    saveDiscoverySettings: (settings) => request<DiscoverySettings>(daemon, "PUT", "/v1/settings/discovery", settings),
    checkLLMHealth: () => request<LLMHealth>(daemon, "GET", "/v1/settings/llm/health"),
    saveLLMSettings: (settings) => request<SaveLLMSettingsResult>(daemon, "PUT", "/v1/settings/llm", settings),

    createApplication: (jobId, candidateId, resumeId) =>
      request<Application>(daemon, "POST", "/v1/applications", {
        job_id: jobId,
        candidate_id: candidateId,
        resume_id: resumeId,
      }),
    listApplications: (candidateId) =>
      request<Application[]>(daemon, "GET", `/v1/applications${query({ candidate_id: candidateId })}`),
    getApplication: (id) => request<Application>(daemon, "GET", `/v1/applications/${encodeURIComponent(id)}`),
    transitionApplication: (id, toState, reason) =>
      request<Application>(daemon, "POST", `/v1/applications/${encodeURIComponent(id)}/transition`, {
        to_state: toState,
        reason,
      }),
    listApplicationHistory: (id) =>
      request<ApplicationStateTransition[]>(daemon, "GET", `/v1/applications/${encodeURIComponent(id)}/history`),
    listAllowedTransitions: (id) =>
      request<ApplicationState[]>(daemon, "GET", `/v1/applications/${encodeURIComponent(id)}/transitions`),
    setApplyUrl: (applicationId, url) =>
      request<Application>(daemon, "PUT", `/v1/applications/${encodeURIComponent(applicationId)}/apply-url`, { url }),

    getAboutYou: () => request<AboutYou>(daemon, "GET", "/v1/profile/about").catch(nullIfNotFound),
    saveAboutYou: (update) => request<AboutYou>(daemon, "PUT", "/v1/profile/about", update),
    deleteKeptAnswer: (id) => request<void>(daemon, "DELETE", `/v1/profile/about/kept/${encodeURIComponent(id)}`),
    keepAnswer: (questionId) =>
      request<PersonalAnswer>(daemon, "POST", `/v1/questions/${encodeURIComponent(questionId)}/keep`),

    createCoverLetter: (applicationId, content) =>
      request<CoverLetter>(daemon, "POST", `/v1/applications/${encodeURIComponent(applicationId)}/cover-letters`, {
        content,
      }),
    listCoverLetterVersions: (applicationId) =>
      request<CoverLetter[]>(daemon, "GET", `/v1/applications/${encodeURIComponent(applicationId)}/cover-letters`),
    editCoverLetter: (coverLetterId, content) =>
      request<CoverLetter>(daemon, "PUT", `/v1/cover-letters/${encodeURIComponent(coverLetterId)}`, {
        content,
      }),

    addQuestion: (applicationId, label) =>
      request<ApplicationQuestion>(daemon, "POST", `/v1/applications/${encodeURIComponent(applicationId)}/questions`, {
        label,
      }),
    listQuestions: (applicationId) =>
      request<ApplicationQuestion[]>(daemon, "GET", `/v1/applications/${encodeURIComponent(applicationId)}/questions`),
    setQuestionAnswer: (questionId, answer) =>
      request<ApplicationQuestion>(daemon, "PUT", `/v1/questions/${encodeURIComponent(questionId)}/answer`, {
        answer,
      }),
    deleteQuestion: (questionId) => request<void>(daemon, "DELETE", `/v1/questions/${encodeURIComponent(questionId)}`),
  };
}
