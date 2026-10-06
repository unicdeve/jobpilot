// TanStack Query hooks over the typed daemon client in api.ts.
//
// api.ts stays the single description of the daemon's HTTP surface (it
// mirrors internal/adapters/primary/httpapi/client.go method-for-method);
// this module only adds caching, request deduplication and invalidation on
// top. Screens use these hooks rather than calling the client directly, so
// that navigating away and back does not refetch, and so that a mutation
// states exactly which cached data it invalidates.

import {
  type Query,
  type QueryClient,
  skipToken,
  type UseMutationOptions,
  useInfiniteQuery,
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { invoke } from "@tauri-apps/api/core";
import { useMemo } from "react";
import type { ScoutInput } from "./api";
import { useApi } from "./daemon";
import { since, type TimeRange } from "./job-filters";
import type {
  AboutYou,
  AboutYouUpdate,
  AgentRun,
  AgentRunDetail,
  AgentRunFilter,
  AgentRunWithEvents,
  Application,
  ApplicationAnswer,
  ApplicationMode,
  ApplicationQuestion,
  ApplicationState,
  ApplicationStateTransition,
  Candidate,
  Countries,
  CoverLetter,
  CredentialRef,
  DeletionReach,
  DeletionResult,
  DeletionTarget,
  DiscoveryResult,
  DiscoverySettings,
  Job,
  JobCompany,
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
  QuestionType,
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
} from "./types";

/// keys centralizes every cache key. Invalidating a prefix invalidates
/// everything under it: keys.jobs.all covers every job list regardless of
/// its keywords or page.
export const keys = {
  profile: ["profile"] as const,
  llmSettings: ["llm-settings"] as const,
  aboutYou: ["about-you"] as const,
  discoverySettings: ["discovery-settings"] as const,
  jobs: {
    all: ["jobs"] as const,
    count: ["jobs", "count"] as const,
    detail: (id: string) => ["jobs", "detail", id] as const,
    infinite: (filter: StoredJobsFilter, pageSize: number) => ["jobs", "infinite", { ...filter, pageSize }] as const,
  },
  savedSearches: ["saved-searches"] as const,
  searchTemplates: ["search-templates"] as const,
  sources: ["sources"] as const,
  countries: ["countries"] as const,
  /// companies covers the watched companies, the suggestions left, and
  /// each job's company, which all change when one is watched.
  companies: {
    all: ["companies"] as const,
    list: ["companies", "list"] as const,
    suggestions: ["companies", "suggestions"] as const,
    job: (jobId: string) => ["companies", "job", jobId] as const,
  },
  /// discovery is a mutation key, not a query key: every discovery run
  /// carries it, so any screen can see what is searching.
  discovery: ["discovery"] as const,
  credentials: ["credentials"] as const,
  scouts: ["scouts"] as const,
  notifications: ["notifications"] as const,
  notificationSettings: ["notification-settings"] as const,
  promptGuidance: ["prompt-guidance"] as const,
  versions: ["versions"] as const,
  scoutRuns: ["scout-runs"] as const,
  activity: {
    all: ["activity"] as const,
    infinite: (filter: AgentRunFilter, pageSize: number) => ["activity", "infinite", { ...filter, pageSize }] as const,
    detail: (id: string) => ["activity", "detail", id] as const,
  },
  resumes: (candidateId: string) => ["resumes", candidateId] as const,
  allMatches: ["matches"] as const,
  matches: (candidateId: string) => ["matches", candidateId] as const,
  jobMatch: (jobId: string) => ["matches", "job", jobId] as const,
  applications: {
    all: ["applications"] as const,
    list: (candidateId: string) => ["applications", "list", candidateId] as const,
    detail: (id: string) => ["applications", "detail", id] as const,
    history: (id: string) => ["applications", "history", id] as const,
    transitions: (id: string) => ["applications", "transitions", id] as const,
    coverLetters: (id: string) => ["applications", "cover-letters", id] as const,
    questions: (id: string) => ["applications", "questions", id] as const,
    personalQuestions: (id: string) => ["applications", "personal-questions", id] as const,
    agentRuns: (id: string) => ["applications", "agent-runs", id] as const,
  },
} as const;

// --- queries ---

/// useProfile loads the single candidate profile, which is null until one
/// is created. "No profile" is an ordinary first-run state, so it is data
/// rather than an error: an errored query with no data is refetched by
/// every component that mounts it and reads as pending meanwhile, and
/// RequireSetup, which unmounts the screens while the profile is pending,
/// turned that into a loop of requests that never settled.
export function useProfile() {
  const api = useApi();
  return useQuery<Candidate | null>({
    queryKey: keys.profile,
    queryFn: () => api.getProfile(),
  });
}

/// useCandidateId is the common case of useProfile: a screen that only needs
/// the candidate's ID to load its own data. It reads the shared cached
/// profile rather than fetching it again per screen.
export function useCandidateId() {
  const { data, isPending, isError } = useProfile();
  return { candidateId: data?.id ?? null, isPending, isError, noProfile: data === null };
}

/// useJobCount reports how many jobs are stored locally. `total` describes
/// the whole stored list independently of the page around it, so asking for
/// a single row is the cheapest way to read it.
export function useJobCount() {
  const api = useApi();
  return useQuery({
    queryKey: keys.jobs.count,
    queryFn: () => api.listJobs({}, 1, 0),
    select: (page: JobPage) => page.total,
  });
}

/// StoredJobsFilter is what the job list is filtered by. The posting window
/// is a range rather than an instant so the query key stays the same from
/// one render to the next.
export interface StoredJobsFilter {
  keywords: string;
  source: string;
  remote: string;
  posted: TimeRange | "";
  /// notInterested lists only the jobs the user ruled out.
  notInterested?: boolean;
  /// workFrom and includeRelocation narrow the list as a saved search's
  /// "I can work from" does.
  workFrom?: string;
  includeRelocation?: boolean;
}

/// useInfiniteJobs pages through stored jobs. Every page already loaded stays
/// in the cache, so returning to a list that was scrolled several pages deep
/// renders at the same height it was left at — which is what makes restoring
/// the scroll position meaningful rather than a jump to a half-built list.
export function useInfiniteJobs(filter: StoredJobsFilter, pageSize: number) {
  const api = useApi();
  // One instant per chosen window, so every page of a list counts back from
  // the same moment and none shifts between pages.
  const postedSince = useMemo(() => (filter.posted ? since(filter.posted) : undefined), [filter.posted]);
  return useInfiniteQuery({
    queryKey: keys.jobs.infinite(filter, pageSize),
    queryFn: ({ pageParam }) =>
      api.listJobs(
        {
          keywords: filter.keywords,
          source: filter.source,
          remote: filter.remote,
          postedSince,
          interest: filter.notInterested ? "not_interested" : undefined,
          workFrom: filter.workFrom,
          includeRelocation: filter.includeRelocation,
        },
        pageSize,
        pageParam,
      ),
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
    queryFn: id ? () => api.getActivityRun(id) : skipToken,
  });
}

/// useJob loads one stored job. A job's record changes only when the user
/// rules it out or back in, which replaces this cache entry directly, so it
/// stays fresh far longer than a list does.
export function useJob(id: string | undefined) {
  const api = useApi();
  return useQuery<Job>({
    queryKey: keys.jobs.detail(id ?? ""),
    queryFn: id ? () => api.getJob(id) : skipToken,
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
    queryFn: candidateId !== null ? () => api.listResumes(candidateId) : skipToken,
  });
}

export function useMatches(candidateId: string | null) {
  const api = useApi();
  return useQuery<JobMatch[]>({
    queryKey: keys.matches(candidateId ?? ""),
    queryFn: candidateId !== null ? () => api.listMatches(candidateId) : skipToken,
  });
}

export function useApplications(candidateId: string | null) {
  const api = useApi();
  return useQuery<Application[]>({
    queryKey: keys.applications.list(candidateId ?? ""),
    queryFn: candidateId !== null ? () => api.listApplications(candidateId) : skipToken,
  });
}

export function useApplication(id: string | undefined) {
  const api = useApi();
  return useQuery<Application>({
    queryKey: keys.applications.detail(id ?? ""),
    queryFn: id ? () => api.getApplication(id) : skipToken,
  });
}

export function useApplicationHistory(id: string | undefined) {
  const api = useApi();
  return useQuery<ApplicationStateTransition[]>({
    queryKey: keys.applications.history(id ?? ""),
    queryFn: id ? () => api.listApplicationHistory(id) : skipToken,
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
    queryFn: id ? () => api.listAllowedTransitions(id) : skipToken,
  });
}

export function useCoverLetters(id: string | undefined) {
  const api = useApi();
  return useQuery<CoverLetter[]>({
    queryKey: keys.applications.coverLetters(id ?? ""),
    queryFn: id ? () => api.listCoverLetterVersions(id) : skipToken,
  });
}

export function useQuestions(id: string | undefined) {
  const api = useApi();
  return useQuery<ApplicationQuestion[]>({
    queryKey: keys.applications.questions(id ?? ""),
    queryFn: id ? () => api.listQuestions(id) : skipToken,
  });
}

export function useAgentRuns(id: string | undefined) {
  const api = useApi();
  return useQuery<AgentRunWithEvents[]>({
    queryKey: keys.applications.agentRuns(id ?? ""),
    queryFn: id ? () => api.listAgentRuns(id) : skipToken,
  });
}

/// useUnseenNotifications loads what JobPilot told the user that they have
/// not seen in the app yet. A scheduled scout can add one at any time, so
/// it is read again every half minute: a local read of a small table.
export function useUnseenNotifications() {
  const api = useApi();
  return useQuery<Notification[]>({
    queryKey: keys.notifications,
    queryFn: () => api.listNotifications(true),
    refetchInterval: 30_000,
  });
}

/// useMarkNotificationsSeen marks one notification seen, or every one when
/// called without an id.
export function useMarkNotificationsSeen(opts?: MutationOpts<void, string | undefined>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id?: string) => api.markNotificationsSeen(id),
    ...opts,
    onSettled: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.notifications });
      opts?.onSettled?.(...args);
    },
  });
}

/// useVersions is this app's version and the daemon's, which can differ: an
/// app finds a newer daemon and uses it rather than replacing it
/// (ADR-0029 §4).
export function useVersions() {
  const api = useApi();
  return useQuery<{ app: string; daemon: string }>({
    queryKey: keys.versions,
    queryFn: async () => {
      const [app, health] = await Promise.all([invoke<string>("app_version"), api.health()]);
      return { app, daemon: health.version };
    },
  });
}

export function useNotificationSettings() {
  const api = useApi();
  return useQuery<NotificationSettings>({
    queryKey: keys.notificationSettings,
    queryFn: () => api.getNotificationSettings(),
  });
}

export function useSaveNotificationSettings(opts?: MutationOpts<NotificationSettings, NotificationSettings>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (settings: NotificationSettings) => api.saveNotificationSettings(settings),
    ...opts,
    onSuccess: (saved, ...rest) => {
      queryClient.setQueryData(keys.notificationSettings, saved);
      opts?.onSuccess?.(saved, ...rest);
    },
  });
}

/// usePromptGuidance loads the prompts the user may give guidance to
/// (ADR-0031).
export function usePromptGuidance() {
  const api = useApi();
  return useQuery<PromptGuidanceList>({
    queryKey: keys.promptGuidance,
    queryFn: () => api.listPromptGuidance(),
  });
}

/// replacePrompt writes one prompt, as the daemon returned it, into the
/// cached list.
function replacePrompt(list: PromptGuidanceList | undefined, prompt: PromptGuidance) {
  return list && { ...list, prompts: list.prompts.map((p) => (p.key === prompt.key ? prompt : p)) };
}

export function useSavePromptGuidance(opts?: MutationOpts<PromptGuidance, { key: PromptKey; text: string }>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ key, text }: { key: PromptKey; text: string }) => api.savePromptGuidance(key, text),
    ...opts,
    onSuccess: (saved, ...rest) => {
      queryClient.setQueryData<PromptGuidanceList>(keys.promptGuidance, (list) => replacePrompt(list, saved));
      opts?.onSuccess?.(saved, ...rest);
    },
  });
}

export function useResetPromptGuidance(opts?: MutationOpts<PromptGuidance, PromptKey>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (key: PromptKey) => api.resetPromptGuidance(key),
    ...opts,
    onSuccess: (reset, ...rest) => {
      queryClient.setQueryData<PromptGuidanceList>(keys.promptGuidance, (list) => replacePrompt(list, reset));
      opts?.onSuccess?.(reset, ...rest);
    },
  });
}

/// useDiscoverySettings loads how discovery behaves without being asked.
export function useDiscoverySettings() {
  const api = useApi();
  return useQuery<DiscoverySettings>({
    queryKey: keys.discoverySettings,
    queryFn: () => api.getDiscoverySettings(),
  });
}

export function useSaveDiscoverySettings(opts?: MutationOpts<DiscoverySettings, DiscoverySettings>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (settings: DiscoverySettings) => api.saveDiscoverySettings(settings),
    ...opts,
    onSuccess: (saved, ...rest) => {
      queryClient.setQueryData(keys.discoverySettings, saved);
      opts?.onSuccess?.(saved, ...rest);
    },
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
type MutationOpts<TData, TVars> = Omit<UseMutationOptions<TData, Error, TVars>, "mutationFn">;

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

export function useCreateSavedSearch(opts?: MutationOpts<SavedSearch, { name: string; criteria: SearchCriteria }>) {
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

/// useImportSearchTemplates adopts templates as ordinary saved searches,
/// each starting with the profile's country as the one the candidate works
/// from, which no template sets (ADR-0023 §4).
export function useImportSearchTemplates(opts?: MutationOpts<SavedSearch[], string[]>) {
  const api = useApi();
  const queryClient = useQueryClient();
  const { data: countries } = useCountries();
  return useMutation({
    mutationFn: (templateIds: string[]) => api.importSearchTemplates(templateIds, countries?.profile_country),
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

/// useCountries lists the countries a search can work from, and the
/// profile's own. The list is the daemon's and does not change while it
/// runs; the profile's country is refreshed when the profile is saved.
export function useCountries() {
  const api = useApi();
  return useQuery<Countries>({
    queryKey: keys.countries,
    queryFn: () => api.listCountries(),
    staleTime: Infinity,
  });
}

/// useJobSources names the sources a search asks. The daemon registers
/// them as it starts, so they do not change while it runs.
export function useJobSources() {
  const api = useApi();
  return useQuery<SourceInfo[]>({
    queryKey: keys.sources,
    queryFn: () => api.listSources(),
    staleTime: Infinity,
  });
}

/// DiscoveryRun is the saved search a discovery run uses. It carries the
/// name as well as the ID, so whatever shows the run can say which search
/// it is without looking it up.
export type DiscoveryRun = Pick<SavedSearch, "id" | "name">;

/// useRunSavedSearch discovers jobs with a saved search's criteria and
/// stores whatever matched, so every cached job list is then stale.
///
/// Runs share one scope, so they go one at a time in the order asked, from
/// whichever screen asked: two runs at once would put the same sources'
/// traffic and the same postings' writes side by side for no gain. A run
/// outlives the screen that started it.
export function useRunSavedSearch(opts?: MutationOpts<DiscoveryResult, DiscoveryRun>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: keys.discovery,
    scope: { id: "discovery" },
    mutationFn: (search: DiscoveryRun) => api.runSavedSearch(search.id),
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.jobs.all });
      // The search now says when it last ran.
      void queryClient.invalidateQueries({ queryKey: keys.savedSearches });
      opts?.onSuccess?.(...args);
    },
    onSettled: (...args) => {
      // A search checks every watched company, whether or not it found jobs.
      void queryClient.invalidateQueries({ queryKey: keys.companies.list });
      opts?.onSettled?.(...args);
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
      void queryClient.invalidateQueries({ queryKey: keys.countries });
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
      void queryClient.invalidateQueries({ queryKey: keys.countries });
      opts?.onSuccess?.(created, ...rest);
    },
  });
}

export function useImportResume(opts?: MutationOpts<Resume, { candidateId: string; filePath: string }>) {
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

/// useJobMatch loads one job's latest match, or null. The job's page reads
/// it this way rather than from useMatches, which leaves out the jobs the
/// user ruled out — a page they can still open.
export function useJobMatch(jobId: string | undefined) {
  const api = useApi();
  return useQuery<JobMatch | null>({
    queryKey: keys.jobMatch(jobId ?? ""),
    queryFn: jobId ? () => api.getJobMatch(jobId) : skipToken,
  });
}

/// useDeletionPreview loads what deleting target would take with it, each
/// time it is asked for: a preview is only worth showing if it is current,
/// so it is never cached. target null asks for nothing.
export function useDeletionPreview(target: DeletionTarget | null, id?: string) {
  const api = useApi();
  return useQuery<DeletionReach>({
    queryKey: ["deletion", target, id ?? ""],
    queryFn: target !== null ? () => api.previewDeletion(target, id) : skipToken,
    staleTime: 0,
    gcTime: 0,
    retry: false,
  });
}

export interface DeletionRequest {
  target: DeletionTarget;
  id?: string;
  token: string;
}

/// useDeleteRecords carries out a confirmed deletion. What one reaches is
/// not known to the client ahead of time — a job takes its applications,
/// the profile nearly everything — so every query is refreshed after it,
/// and after "delete all data" the cache is emptied instead.
///
/// The deleted record's own queries are the exception. The screen showing
/// it may still be mounted while it navigates away, and refreshed then, it
/// would ask the daemon for a record that no longer exists. They are left
/// as they are, and dropped once nothing shows them (dropWhenUnobserved).
export function useDeleteRecords(opts?: MutationOpts<DeletionResult, DeletionRequest>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ target, id, token }: DeletionRequest) => api.deleteRecords(target, id, token),
    ...opts,
    onSuccess: (result, variables, ...rest) => {
      if (variables.target === "everything") {
        opts?.onSuccess?.(result, variables, ...rest);
        queryClient.clear();
        return;
      }
      const gone = dropWhenUnobserved(queryClient, deletedRecordKeys(variables));
      void queryClient.invalidateQueries({ predicate: (query) => !gone.has(query.queryHash) });
      opts?.onSuccess?.(result, variables, ...rest);
    },
  });
}

/// dropWhenUnobserved removes the queries under keys from the cache as soon
/// as no component observes them, now or when the last one unmounts, and
/// returns their hashes.
function dropWhenUnobserved(queryClient: QueryClient, queryKeys: readonly (readonly unknown[])[]): Set<string> {
  const cache = queryClient.getQueryCache();
  const pending = new Map<string, Query>();
  for (const queryKey of queryKeys) {
    for (const query of cache.findAll({ queryKey })) pending.set(query.queryHash, query);
  }
  const hashes = new Set(pending.keys());

  const dropIfUnobserved = (query: Query) => {
    if (query.getObserversCount() > 0) return;
    cache.remove(query);
    pending.delete(query.queryHash);
  };
  pending.forEach(dropIfUnobserved);
  if (pending.size > 0) {
    const unsubscribe = cache.subscribe((event) => {
      if (event.type === "observerRemoved" && pending.has(event.query.queryHash)) dropIfUnobserved(event.query);
      if (pending.size === 0) unsubscribe();
    });
  }
  return hashes;
}

function deletedRecordKeys({ target, id }: DeletionRequest): readonly (readonly unknown[])[] {
  const previews = [["deletion"]] as const;
  if (!id) return previews;
  switch (target) {
    case "job":
      return [...previews, keys.jobs.detail(id), keys.jobMatch(id)];
    case "application":
      return [
        ...previews,
        keys.applications.detail(id),
        keys.applications.history(id),
        keys.applications.transitions(id),
        keys.applications.coverLetters(id),
        keys.applications.questions(id),
        keys.applications.agentRuns(id),
      ];
    default:
      return previews;
  }
}

/// useSetJobInterest rules a job out or back in. The job's own record
/// changes, so its cached copy is replaced, and every list it leaves or
/// rejoins is refreshed.
export function useSetJobInterest(opts?: MutationOpts<Job, { id: string; notInterested: boolean }>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, notInterested }: { id: string; notInterested: boolean }) =>
      api.setJobInterest(id, notInterested),
    ...opts,
    onSuccess: (job, variables, ...rest) => {
      queryClient.setQueryData(keys.jobs.detail(job.id), job);
      void queryClient.invalidateQueries({ queryKey: keys.jobs.all });
      void queryClient.invalidateQueries({ queryKey: keys.allMatches });
      opts?.onSuccess?.(job, variables, ...rest);
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

export function useAnalyzeMatch(opts?: MutationOpts<JobMatch, { candidateId: string; jobId: string }>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ candidateId, jobId }: { candidateId: string; jobId: string }) =>
      api.analyzeMatch(candidateId, jobId),
    ...opts,
    onSuccess: (match, variables, ...rest) => {
      void queryClient.invalidateQueries({ queryKey: keys.matches(variables.candidateId) });
      queryClient.setQueryData(keys.jobMatch(variables.jobId), match);
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
    mutationFn: ({ id, toState, reason }: TransitionVars) => api.transitionApplication(id, toState, reason),
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

/// useAboutYou loads what the candidate has said about themselves. With no
/// profile yet there is nothing to load, which is null rather than an
/// error, as it is for the profile.
export function useAboutYou() {
  const api = useApi();
  return useQuery<AboutYou | null>({
    queryKey: keys.aboutYou,
    queryFn: () => api.getAboutYou(),
  });
}

export function useSaveAboutYou(opts?: MutationOpts<AboutYou, AboutYouUpdate>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (update: AboutYouUpdate) => api.saveAboutYou(update),
    ...opts,
    onSuccess: (saved, ...rest) => {
      queryClient.setQueryData(keys.aboutYou, saved);
      opts?.onSuccess?.(saved, ...rest);
    },
  });
}

export function useDeleteKeptAnswer(opts?: MutationOpts<void, string>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.deleteKeptAnswer(id),
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.aboutYou });
      opts?.onSuccess?.(...args);
    },
  });
}

export function useKeepAnswer(opts?: MutationOpts<PersonalAnswer, string>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (questionId: string) => api.keepAnswer(questionId),
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.aboutYou });
      opts?.onSuccess?.(...args);
    },
  });
}

/// usePersonalQuestions reads the questions asked before a letter for one
/// application, without asking a model; usePreparePersonalQuestions adds
/// the ones the model proposes.
export function usePersonalQuestions(applicationId: string, enabled: boolean) {
  const api = useApi();
  return useQuery<PersonalAnswer[]>({
    queryKey: keys.applications.personalQuestions(applicationId),
    queryFn: () => api.listPersonalQuestions(applicationId),
    enabled,
  });
}

export function usePreparePersonalQuestions(opts?: MutationOpts<PersonalAnswer[], string>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (applicationId: string) => api.preparePersonalQuestions(applicationId),
    ...opts,
    onSuccess: (questions, applicationId, ...rest) => {
      queryClient.setQueryData(keys.applications.personalQuestions(applicationId), questions);
      opts?.onSuccess?.(questions, applicationId, ...rest);
    },
  });
}

export function useSavePersonalAnswers(
  opts?: MutationOpts<PersonalAnswer[], { applicationId: string; answers: ApplicationAnswer[] }>,
) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ applicationId, answers }: { applicationId: string; answers: ApplicationAnswer[] }) =>
      api.savePersonalAnswers(applicationId, answers),
    ...opts,
    onSuccess: (questions, variables, ...rest) => {
      queryClient.setQueryData(keys.applications.personalQuestions(variables.applicationId), questions);
      opts?.onSuccess?.(questions, variables, ...rest);
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
      // version list is refetched — and the application, which records the
      // new version as current. Without it a rewrite kept showing the
      // letter it replaced until the application happened to refetch.
      void queryClient.invalidateQueries({
        queryKey: keys.applications.coverLetters(applicationId),
      });
      void queryClient.invalidateQueries({ queryKey: keys.applications.detail(applicationId) });
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
    mutationFn: ({ coverLetterId, content }: EditCoverLetterVars) => api.editCoverLetter(coverLetterId, content),
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

export function useSetQuestionAnswer(opts?: MutationOpts<ApplicationQuestion, QuestionVars & { answer: string }>) {
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
///
/// While a scout is running or waiting its turn, or poll is set because a
/// run was just asked for, the list is read every two seconds, so a run
/// the schedule started shows, and so does its end.
export function useScouts(poll = false) {
  const api = useApi();
  return useQuery<Scout[]>({
    queryKey: keys.scouts,
    queryFn: () => api.listScouts(),
    refetchInterval: (query) => (poll || query.state.data?.some((scout) => scout.activity) ? 2000 : false),
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
      void queryClient.invalidateQueries({ queryKey: keys.notifications });
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

/// useCompanies lists the watched companies, with how each was last checked.
/// A search checks them, so the list is refetched whenever a search ends.
export function useCompanies() {
  const api = useApi();
  return useQuery<WatchedCompany[]>({ queryKey: keys.companies.list, queryFn: () => api.listCompanies() });
}

/// useCompanySuggestions is the starter list, less what is watched.
export function useCompanySuggestions() {
  const api = useApi();
  return useQuery<StarterCompany[]>({
    queryKey: keys.companies.suggestions,
    queryFn: () => api.companySuggestions(),
  });
}

/// useJobCompany is the hiring page a job's posting is on, and the watched
/// company it is, so the job can offer to watch it (spec AC-002).
export function useJobCompany(jobId: string) {
  const api = useApi();
  return useQuery<JobCompany>({ queryKey: keys.companies.job(jobId), queryFn: () => api.jobCompany(jobId) });
}

function useCompanyMutation<TData, TVars>(fn: (vars: TVars) => Promise<TData>, opts?: MutationOpts<TData, TVars>) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: fn,
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.companies.all });
      opts?.onSuccess?.(...args);
    },
  });
}

export function useWatchCompany(opts?: MutationOpts<WatchedCompany, { address: string } | { job_id: string }>) {
  const api = useApi();
  return useCompanyMutation((by) => api.watchCompany(by), opts);
}

export function useRenameCompany(opts?: MutationOpts<WatchedCompany, { id: string; name: string }>) {
  const api = useApi();
  return useCompanyMutation(({ id, name }) => api.renameCompany(id, name), opts);
}

export function useUnwatchCompany(opts?: MutationOpts<void, string>) {
  const api = useApi();
  return useCompanyMutation((id) => api.unwatchCompany(id), opts);
}
