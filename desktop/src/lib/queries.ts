// TanStack Query hooks over the typed daemon client in api.ts.
//
// api.ts stays the single description of the daemon's HTTP surface (it
// mirrors internal/adapters/primary/httpapi/client.go method-for-method);
// this module only adds caching, request deduplication and invalidation on
// top. Screens use these hooks rather than calling the client directly, so
// that navigating away and back does not refetch, and so that a mutation
// states exactly which cached data it invalidates.

import {
  useInfiniteQuery,
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
  type UseMutationOptions,
} from "@tanstack/react-query";
import { useApi } from "./daemon";
import type { ScoutInput } from "./api";
import type {
  AgentRun,
  AgentRunDetail,
  AgentRunFilter,
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
  LLMHealth,
  LLMSettings,
  ProfileDraft,
  QuestionType,
  Resume,
  SavedSearch,
  SaveLLMSettingsResult,
  Scout,
  ScoutRunReport,
  CredentialRef,
  SearchCriteria,
  SearchTemplateGroup,
} from "./types";

/// keys centralizes every cache key. Invalidating a prefix invalidates
/// everything under it: keys.jobs.all covers every job list regardless of
/// its keywords or page.
export const keys = {
  profile: ["profile"] as const,
  llmSettings: ["llm-settings"] as const,
  jobs: {
    all: ["jobs"] as const,
    count: ["jobs", "count"] as const,
    detail: (id: string) => ["jobs", "detail", id] as const,
    infinite: (keywords: string, pageSize: number) =>
      ["jobs", "infinite", { keywords, pageSize }] as const,
  },
  savedSearches: ["saved-searches"] as const,
  searchTemplates: ["search-templates"] as const,
  credentials: ["credentials"] as const,
  scouts: ["scouts"] as const,
  scoutRuns: ["scout-runs"] as const,
  activity: {
    all: ["activity"] as const,
    infinite: (filter: AgentRunFilter, pageSize: number) =>
      ["activity", "infinite", { ...filter, pageSize }] as const,
    detail: (id: string) => ["activity", "detail", id] as const,
  },
  resumes: (candidateId: string) => ["resumes", candidateId] as const,
  matches: (candidateId: string) => ["matches", candidateId] as const,
  applications: {
    all: ["applications"] as const,
    list: (candidateId: string) => ["applications", "list", candidateId] as const,
    detail: (id: string) => ["applications", "detail", id] as const,
    history: (id: string) => ["applications", "history", id] as const,
    transitions: (id: string) => ["applications", "transitions", id] as const,
    coverLetters: (id: string) => ["applications", "cover-letters", id] as const,
    questions: (id: string) => ["applications", "questions", id] as const,
    agentRuns: (id: string) => ["applications", "agent-runs", id] as const,
  },
} as const;

// --- queries ---

/// useProfile loads the single candidate profile. A daemon that has no
/// profile yet answers with an error rather than an empty body, so this is
/// not retried: "no profile" is an ordinary first-run state, not a transient
/// failure, and screens read it off isError.
export function useProfile() {
  const api = useApi();
  return useQuery<Candidate>({
    queryKey: keys.profile,
    queryFn: () => api.getProfile(),
    retry: false,
  });
}

/// useCandidateId is the common case of useProfile: a screen that only needs
/// the candidate's ID to load its own data. It reads the shared cached
/// profile rather than fetching it again per screen.
export function useCandidateId() {
  const { data, isPending, isError } = useProfile();
  return { candidateId: data?.id ?? null, isPending, isError };
}

/// useJobCount reports how many jobs are stored locally. `total` describes
/// the whole stored list independently of the page around it, so asking for
/// a single row is the cheapest way to read it.
export function useJobCount() {
  const api = useApi();
  return useQuery({
    queryKey: keys.jobs.count,
    queryFn: () => api.listJobs("", 1, 0),
    select: (page: JobPage) => page.total,
  });
}

/// useInfiniteJobs pages through stored jobs. Every page already loaded stays
/// in the cache, so returning to a list that was scrolled several pages deep
/// renders at the same height it was left at — which is what makes restoring
/// the scroll position meaningful rather than a jump to a half-built list.
export function useInfiniteJobs(keywords: string, pageSize: number) {
  const api = useApi();
  return useInfiniteQuery({
    queryKey: keys.jobs.infinite(keywords, pageSize),
    queryFn: ({ pageParam }) => api.listJobs(keywords, pageSize, pageParam),
    initialPageParam: 0,
    getNextPageParam: (lastPage, allPages) => {
      // The next offset is however many rows are already loaded; total counts
      // the whole filtered result, so this stops exactly at the end.
      const loaded = allPages.reduce((count, page) => count + page.jobs.length, 0);
      return loaded < lastPage.total ? loaded : undefined;
    },
  });
}

/// useInfiniteActivity pages through every agent run, newest first, the
/// same way useInfiniteJobs pages through stored jobs.
export function useInfiniteActivity(filter: AgentRunFilter, pageSize: number) {
  const api = useApi();
  return useInfiniteQuery({
    queryKey: keys.activity.infinite(filter, pageSize),
    queryFn: ({ pageParam }) => api.listActivity(filter, pageSize, pageParam),
    initialPageParam: 0,
    getNextPageParam: (lastPage, allPages) => {
      const loaded = allPages.reduce((count, page) => count + page.runs.length, 0);
      return loaded < lastPage.total ? loaded : undefined;
    },
  });
}

/// useActivityRun loads one run with every step it recorded.
export function useActivityRun(id: string | undefined) {
  const api = useApi();
  return useQuery<AgentRunDetail>({
    queryKey: keys.activity.detail(id ?? ""),
    queryFn: () => api.getActivityRun(id!),
    enabled: !!id,
  });
}

/// useJob loads one stored job. Job records do not change once discovered,
/// so this stays fresh far longer than a list does.
export function useJob(id: string | undefined) {
  const api = useApi();
  return useQuery<Job>({
    queryKey: keys.jobs.detail(id ?? ""),
    queryFn: () => api.getJob(id!),
    enabled: !!id,
    staleTime: 5 * 60_000,
  });
}

/// useJobsByID loads several jobs at once, for screens that hold job IDs and
/// need something human-readable to show for them — a match or an
/// application knows only its job_id.
///
/// This is one request per job rather than a bulk endpoint. It is a local
/// SQLite read over loopback, the counts involved are however many jobs the
/// user has personally analysed or applied to, and every result is cached
/// under the same key useJob uses — so opening one of those jobs afterwards
/// needs no request at all. If these lists ever grow large enough for that
/// to bite, the fix is a bulk read on the daemon, not a cache here.
export function useJobsByID(ids: string[]) {
  const api = useApi();
  const unique = [...new Set(ids)];

  const results = useQueries({
    queries: unique.map((id) => ({
      queryKey: keys.jobs.detail(id),
      queryFn: () => api.getJob(id),
      staleTime: 5 * 60_000,
    })),
  });

  const byID = new Map<string, Job>();
  results.forEach((result, i) => {
    if (result.data) byID.set(unique[i], result.data);
  });
  return byID;
}

/// useSavedSearches loads the named searches that drive discovery, default
/// first. An empty list is the state a new installation starts in, and the
/// state in which discovery cannot run at all.
export function useSavedSearches() {
  const api = useApi();
  return useQuery<SavedSearch[]>({
    queryKey: keys.savedSearches,
    queryFn: () => api.listSavedSearches(),
  });
}

export function useResumes(candidateId: string | null) {
  const api = useApi();
  return useQuery<Resume[]>({
    queryKey: keys.resumes(candidateId ?? ""),
    queryFn: () => api.listResumes(candidateId!),
    enabled: candidateId !== null,
  });
}

export function useMatches(candidateId: string | null) {
  const api = useApi();
  return useQuery<JobMatch[]>({
    queryKey: keys.matches(candidateId ?? ""),
    queryFn: () => api.listMatches(candidateId!),
    enabled: candidateId !== null,
  });
}

export function useApplications(candidateId: string | null) {
  const api = useApi();
  return useQuery<Application[]>({
    queryKey: keys.applications.list(candidateId ?? ""),
    queryFn: () => api.listApplications(candidateId!),
    enabled: candidateId !== null,
  });
}

export function useApplication(id: string | undefined) {
  const api = useApi();
  return useQuery<Application>({
    queryKey: keys.applications.detail(id ?? ""),
    queryFn: () => api.getApplication(id!),
    enabled: !!id,
  });
}

export function useApplicationHistory(id: string | undefined) {
  const api = useApi();
  return useQuery<ApplicationStateTransition[]>({
    queryKey: keys.applications.history(id ?? ""),
    queryFn: () => api.listApplicationHistory(id!),
    enabled: !!id,
  });
}

/// useAllowedTransitions asks the daemon which states an application may
/// move to next. The rule table lives in Go and is enforced there; fetching
/// it rather than restating it here is what keeps the buttons on screen and
/// the transitions the server accepts from drifting apart.
export function useAllowedTransitions(id: string | undefined) {
  const api = useApi();
  return useQuery<ApplicationState[]>({
    queryKey: keys.applications.transitions(id ?? ""),
    queryFn: () => api.listAllowedTransitions(id!),
    enabled: !!id,
  });
}

export function useCoverLetters(id: string | undefined) {
  const api = useApi();
  return useQuery<CoverLetter[]>({
    queryKey: keys.applications.coverLetters(id ?? ""),
    queryFn: () => api.listCoverLetterVersions(id!),
    enabled: !!id,
  });
}

export function useQuestions(id: string | undefined) {
  const api = useApi();
  return useQuery<ApplicationQuestion[]>({
    queryKey: keys.applications.questions(id ?? ""),
    queryFn: () => api.listQuestions(id!),
    enabled: !!id,
  });
}

export function useAgentRuns(id: string | undefined) {
  const api = useApi();
  return useQuery<AgentRunWithEvents[]>({
    queryKey: keys.applications.agentRuns(id ?? ""),
    queryFn: () => api.listAgentRuns(id!),
    enabled: !!id,
  });
}

export function useLLMSettings() {
  const api = useApi();
  return useQuery<LLMSettings>({
    queryKey: keys.llmSettings,
    queryFn: () => api.getLLMSettings(),
  });
}

// --- mutations ---

/// MutationOpts is the per-call slice of mutation options a screen supplies
/// (onSuccess/onError for toasts and navigation). Each hook below owns its
/// own cache invalidation, so callers never have to remember it.
type MutationOpts<TData, TVars> = Omit<
  UseMutationOptions<TData, Error, TVars>,
  "mutationFn"
>;

export function useImportJob(opts?: MutationOpts<Job, string>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (url: string) => api.importJob(url),
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.jobs.all });
      opts?.onSuccess?.(...args);
    },
  });
}

export function useCreateSavedSearch(
  opts?: MutationOpts<SavedSearch, { name: string; criteria: SearchCriteria }>,
) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ name, criteria }: { name: string; criteria: SearchCriteria }) =>
      api.createSavedSearch(name, criteria),
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.savedSearches });
      opts?.onSuccess?.(...args);
    },
  });
}

export function useUpdateSavedSearch(
  opts?: MutationOpts<SavedSearch, { id: string; name: string; criteria: SearchCriteria }>,
) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, name, criteria }: { id: string; name: string; criteria: SearchCriteria }) =>
      api.updateSavedSearch(id, name, criteria),
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.savedSearches });
      opts?.onSuccess?.(...args);
    },
  });
}

export function useDeleteSavedSearch(opts?: MutationOpts<void, string>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.deleteSavedSearch(id),
    ...opts,
    onSuccess: (...args) => {
      // Deleting the default promotes another, so the whole list is refetched
      // rather than the one row removed.
      void queryClient.invalidateQueries({ queryKey: keys.savedSearches });
      opts?.onSuccess?.(...args);
    },
  });
}

export function useSetDefaultSavedSearch(opts?: MutationOpts<SavedSearch, string>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.setDefaultSavedSearch(id),
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.savedSearches });
      opts?.onSuccess?.(...args);
    },
  });
}

/// useSearchTemplates loads the catalog of ready-made searches. The catalog
/// is static Go data served by the daemon, so it never changes within a
/// session and is cached for the life of one.
export function useSearchTemplates() {
  const api = useApi();
  return useQuery<SearchTemplateGroup[]>({
    queryKey: keys.searchTemplates,
    queryFn: () => api.listSearchTemplates(),
    staleTime: Infinity,
  });
}

/// useImportSearchTemplates adopts templates as ordinary saved searches.
export function useImportSearchTemplates(opts?: MutationOpts<SavedSearch[], string[]>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (templateIds: string[]) => api.importSearchTemplates(templateIds),
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.savedSearches });
      opts?.onSuccess?.(...args);
    },
  });
}

/// useCredentials loads which credentials are configured. It returns refs
/// and never values — there is no query that reads a secret, because the
/// daemon serves no route that returns one.
export function useCredentials() {
  const api = useApi();
  return useQuery<CredentialRef[]>({
    queryKey: keys.credentials,
    queryFn: () => api.listCredentials(),
  });
}

/// useSetCredential stores an API key in the OS keychain.
export function useSetCredential(
  opts?: MutationOpts<CredentialRef, { source: string; label: string; secret: string }>,
) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ source, label, secret }: { source: string; label: string; secret: string }) =>
      api.setCredential(source, label, secret),
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.credentials });
      opts?.onSuccess?.(...args);
    },
  });
}

/// useDeleteCredential removes a stored key.
export function useDeleteCredential(opts?: MutationOpts<void, CredentialRef>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (ref: CredentialRef) => api.deleteCredential(ref.source, ref.label),
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.credentials });
      opts?.onSuccess?.(...args);
    },
  });
}

/// useRunSavedSearch discovers jobs with a saved search's criteria and
/// stores whatever matched, so every cached job list is then stale.
export function useRunSavedSearch(opts?: MutationOpts<Job[], string>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.runSavedSearch(id),
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.jobs.all });
      opts?.onSuccess?.(...args);
    },
  });
}

export function useUpdateProfile(opts?: MutationOpts<Candidate, Candidate>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (profile: Candidate) => api.updateProfile(profile),
    ...opts,
    onSuccess: (updated, ...rest) => {
      queryClient.setQueryData(keys.profile, updated);
      opts?.onSuccess?.(updated, ...rest);
    },
  });
}

export function useInitializeProfile(opts?: MutationOpts<Candidate, Partial<Candidate>>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (profile: Partial<Candidate>) => api.initializeProfile(profile),
    ...opts,
    onSuccess: (created, ...rest) => {
      queryClient.setQueryData(keys.profile, created);
      opts?.onSuccess?.(created, ...rest);
    },
  });
}

export function useImportResume(
  opts?: MutationOpts<Resume, { candidateId: string; filePath: string }>,
) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ candidateId, filePath }: { candidateId: string; filePath: string }) =>
      api.importResume(candidateId, filePath),
    ...opts,
    onSuccess: (resume, variables, ...rest) => {
      void queryClient.invalidateQueries({ queryKey: keys.resumes(variables.candidateId) });
      opts?.onSuccess?.(resume, variables, ...rest);
    },
  });
}

export function useDeleteResume(
  opts?: MutationOpts<void, { id: string; candidateId: string }>,
) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id }: { id: string; candidateId: string }) => api.deleteResume(id),
    ...opts,
    onSuccess: (data, variables, ...rest) => {
      void queryClient.invalidateQueries({ queryKey: keys.resumes(variables.candidateId) });
      opts?.onSuccess?.(data, variables, ...rest);
    },
  });
}

export function useProposeProfileDraft(opts?: MutationOpts<ProfileDraft, string>) {
  const api = useApi();
  // Reads a resume's markdown and returns a suggestion; nothing is persisted,
  // so there is nothing to invalidate.
  return useMutation({
    mutationFn: (resumeMarkdown: string) => api.proposeProfileDraft(resumeMarkdown),
    ...opts,
  });
}

export function useAnalyzeMatch(
  opts?: MutationOpts<JobMatch, { candidateId: string; jobId: string }>,
) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ candidateId, jobId }: { candidateId: string; jobId: string }) =>
      api.analyzeMatch(candidateId, jobId),
    ...opts,
    onSuccess: (match, variables, ...rest) => {
      void queryClient.invalidateQueries({ queryKey: keys.matches(variables.candidateId) });
      opts?.onSuccess?.(match, variables, ...rest);
    },
  });
}

/// useCheckLLMHealth asks the daemon whether the configured backend would
/// answer right now. It is a mutation rather than a query because it runs on
/// demand, immediately before work that needs the LLM — probing on every
/// render of a screen would mean pinging the backend for nothing.
export function useCheckLLMHealth(opts?: MutationOpts<LLMHealth, void>) {
  const api = useApi();
  return useMutation({
    mutationFn: () => api.checkLLMHealth(),
    ...opts,
  });
}

export function useSaveLLMSettings(opts?: MutationOpts<SaveLLMSettingsResult, LLMSettings>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (settings: LLMSettings) => api.saveLLMSettings(settings),
    ...opts,
    onSuccess: (result, ...rest) => {
      queryClient.setQueryData(keys.llmSettings, result.settings);
      opts?.onSuccess?.(result, ...rest);
    },
  });
}

interface CreateApplicationVars {
  jobId: string;
  candidateId: string;
  resumeId: string;
  mode: ApplicationMode;
}

export function useCreateApplication(opts?: MutationOpts<Application, CreateApplicationVars>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ jobId, candidateId, resumeId, mode }: CreateApplicationVars) =>
      api.createApplication(jobId, candidateId, resumeId, mode),
    ...opts,
    onSuccess: (application, variables, ...rest) => {
      void queryClient.invalidateQueries({
        queryKey: keys.applications.list(variables.candidateId),
      });
      opts?.onSuccess?.(application, variables, ...rest);
    },
  });
}

interface TransitionVars {
  id: string;
  toState: ApplicationState;
  reason: string;
}

export function useTransitionApplication(opts?: MutationOpts<Application, TransitionVars>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, toState, reason }: TransitionVars) =>
      api.transitionApplication(id, toState, reason),
    ...opts,
    onSuccess: (application, variables, ...rest) => {
      queryClient.setQueryData(keys.applications.detail(variables.id), application);
      // A transition appends to the durable history and changes the state
      // every application list renders.
      void queryClient.invalidateQueries({ queryKey: keys.applications.history(variables.id) });
      void queryClient.invalidateQueries({ queryKey: keys.applications.all });
      opts?.onSuccess?.(application, variables, ...rest);
    },
  });
}

export function useGenerateCoverLetter(opts?: MutationOpts<CoverLetter, string>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (applicationId: string) => api.generateCoverLetter(applicationId),
    ...opts,
    onSuccess: (letter, applicationId, ...rest) => {
      // Generating adds a version rather than replacing one, so the whole
      // version list is refetched.
      void queryClient.invalidateQueries({
        queryKey: keys.applications.coverLetters(applicationId),
      });
      opts?.onSuccess?.(letter, applicationId, ...rest);
    },
  });
}

interface EditCoverLetterVars {
  coverLetterId: string;
  applicationId: string;
  content: string;
}

export function useEditCoverLetter(opts?: MutationOpts<CoverLetter, EditCoverLetterVars>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ coverLetterId, content }: EditCoverLetterVars) =>
      api.editCoverLetter(coverLetterId, content),
    ...opts,
    onSuccess: (letter, variables, ...rest) => {
      void queryClient.invalidateQueries({
        queryKey: keys.applications.coverLetters(variables.applicationId),
      });
      opts?.onSuccess?.(letter, variables, ...rest);
    },
  });
}

interface AddQuestionVars {
  applicationId: string;
  label: string;
  type: QuestionType;
  required: boolean;
}

export function useAddQuestion(opts?: MutationOpts<ApplicationQuestion, AddQuestionVars>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ applicationId, label, type, required }: AddQuestionVars) =>
      api.addQuestion(applicationId, label, type, required),
    ...opts,
    onSuccess: (question, variables, ...rest) => {
      void queryClient.invalidateQueries({
        queryKey: keys.applications.questions(variables.applicationId),
      });
      opts?.onSuccess?.(question, variables, ...rest);
    },
  });
}

interface QuestionVars {
  questionId: string;
  applicationId: string;
}

export function useResolveQuestion(opts?: MutationOpts<ApplicationQuestion, QuestionVars>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ questionId }: QuestionVars) => api.resolveQuestion(questionId),
    ...opts,
    onSuccess: (question, variables, ...rest) => {
      void queryClient.invalidateQueries({
        queryKey: keys.applications.questions(variables.applicationId),
      });
      opts?.onSuccess?.(question, variables, ...rest);
    },
  });
}

export function useSetQuestionAnswer(
  opts?: MutationOpts<ApplicationQuestion, QuestionVars & { answer: string }>,
) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ questionId, answer }: QuestionVars & { answer: string }) =>
      api.setQuestionAnswer(questionId, answer),
    ...opts,
    onSuccess: (question, variables, ...rest) => {
      void queryClient.invalidateQueries({
        queryKey: keys.applications.questions(variables.applicationId),
      });
      opts?.onSuccess?.(question, variables, ...rest);
    },
  });
}

export function usePrepareInBrowser(opts?: MutationOpts<Application, string>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (applicationId: string) => api.prepareInBrowser(applicationId),
    ...opts,
    // Invalidation happens on settle rather than on success because a failed
    // run still changes server state: the application moves to FAILED and
    // the agent run recording why is exactly what the user needs to see.
    onSettled: (application, error, applicationId, ...rest) => {
      void queryClient.invalidateQueries({ queryKey: keys.applications.detail(applicationId) });
      void queryClient.invalidateQueries({ queryKey: keys.applications.questions(applicationId) });
      void queryClient.invalidateQueries({ queryKey: keys.applications.agentRuns(applicationId) });
      void queryClient.invalidateQueries({ queryKey: keys.applications.history(applicationId) });
      void queryClient.invalidateQueries({ queryKey: keys.applications.all });
      void queryClient.invalidateQueries({ queryKey: keys.activity.all });
      opts?.onSettled?.(application, error, applicationId, ...rest);
    },
  });
}


/// useScouts loads the standing instructions that shortlist jobs for
/// review. An empty list is where every installation starts.
export function useScouts() {
  const api = useApi();
  return useQuery<Scout[]>({
    queryKey: keys.scouts,
    queryFn: () => api.listScouts(),
  });
}

export function useCreateScout(opts?: MutationOpts<Scout, ScoutInput>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (scout: ScoutInput) => api.createScout(scout),
    ...opts,
    onSuccess: (data, variables, ...rest) => {
      void queryClient.invalidateQueries({ queryKey: keys.scouts });
      opts?.onSuccess?.(data, variables, ...rest);
    },
  });
}

export function useUpdateScout(opts?: MutationOpts<Scout, { id: string; scout: ScoutInput }>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, scout }: { id: string; scout: ScoutInput }) => api.updateScout(id, scout),
    ...opts,
    onSuccess: (data, variables, ...rest) => {
      void queryClient.invalidateQueries({ queryKey: keys.scouts });
      opts?.onSuccess?.(data, variables, ...rest);
    },
  });
}

export function useDeleteScout(opts?: MutationOpts<void, string>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.deleteScout(id),
    ...opts,
    onSuccess: (data, variables, ...rest) => {
      void queryClient.invalidateQueries({ queryKey: keys.scouts });
      opts?.onSuccess?.(data, variables, ...rest);
    },
  });
}

/// useRunScout runs a scout once.
///
/// It invalidates applications as well as scouts, because a run's whole
/// point is that new applications are waiting.
export function useRunScout(opts?: MutationOpts<ScoutRunReport, string>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.runScout(id),
    ...opts,
    // On settle, not success: a run that fails partway is still recorded,
    // and may already have created applications (AC-006).
    onSettled: (data, error, variables, ...rest) => {
      void queryClient.invalidateQueries({ queryKey: keys.scouts });
      void queryClient.invalidateQueries({ queryKey: keys.scoutRuns });
      void queryClient.invalidateQueries({ queryKey: ["applications"] });
      void queryClient.invalidateQueries({ queryKey: keys.activity.all });
      opts?.onSettled?.(data, error, variables, ...rest);
    },
  });
}

export function useScoutRuns(limit = 10) {
  const api = useApi();
  return useQuery<AgentRun[]>({
    queryKey: keys.scoutRuns,
    queryFn: () => api.listScoutRuns(limit),
  });
}
