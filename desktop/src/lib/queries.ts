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
  type Query,
  type QueryClient,
  type UseMutationOptions,
} from "@tanstack/react-query";
import { useMemo } from "react";
import { useApi } from "./daemon";
import { since, type TimeRange } from "./job-filters";
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
  DeletionReach,
  DeletionResult,
  DeletionTarget,
  DiscoveryResult,
  DiscoverySettings,
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
  SourceInfo,
} from "./types";

/// keys centralizes every cache key. Invalidating a prefix invalidates
/// everything under it: keys.jobs.all covers every job list regardless of
/// its keywords or page.
export const keys = {
  profile: ["profile"] as const,
  llmSettings: ["llm-settings"] as const,
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
  /// discovery is a mutation key, not a query key: every discovery run
  /// carries it, so any screen can see what is searching.
  discovery: ["discovery"] as const,
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
    queryFn: () => api.getActivityRun(id!),
    enabled: !!id,
  });
}

/// useJob loads one stored job. A job's record changes only when the user
/// rules it out or back in, which replaces this cache entry directly, so it
/// stays fresh far longer than a list does.
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

/// useJobMatch loads one job's latest match, or null. The job's page reads
/// it this way rather than from useMatches, which leaves out the jobs the
/// user ruled out — a page they can still open.
export function useJobMatch(jobId: string | undefined) {
  const api = useApi();
  return useQuery<JobMatch | null>({
    queryKey: keys.jobMatch(jobId ?? ""),
    queryFn: () => api.getJobMatch(jobId!),
    enabled: !!jobId,
  });
}

/// useDeletionPreview loads what deleting target would take with it, each
/// time it is asked for: a preview is only worth showing if it is current,
/// so it is never cached. target null asks for nothing.
export function useDeletionPreview(target: DeletionTarget | null, id?: string) {
  const api = useApi();
  return useQuery<DeletionReach>({
    queryKey: ["deletion", target, id ?? ""],
    queryFn: () => api.previewDeletion(target!, id),
    enabled: target !== null,
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
    mutationFn: ({ id, notInterested }: { id: string; notInterested: boolean }) => api.setJobInterest(id, notInterested),
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
