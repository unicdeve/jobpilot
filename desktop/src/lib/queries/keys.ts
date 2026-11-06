// TanStack Query hooks over the typed daemon client in api.ts, one module per
// kind of record (job.queries.ts, application.queries.ts, ...).
//
// api.ts stays the single description of the daemon's HTTP surface (it
// mirrors internal/adapters/primary/httpapi/client.go method-for-method);
// these modules only add caching, request deduplication and invalidation on
// top. Screens use these hooks rather than calling the client directly, so
// that navigating away and back does not refetch, and so that a mutation
// states exactly which cached data it invalidates.
//
// This module centralizes every cache key, so the modules can invalidate
// each other's data without reaching into each other.

import type { StoredJobsFilter } from "../job-filters";
import type { AgentRunFilter, DeletionTarget } from "../types";

/// keys is every query key. Invalidating a prefix invalidates everything
/// under it: keys.jobs.all covers every job list regardless of its keywords
/// or page.
export const keys = {
  profile: ["profile"] as const,
  aboutYou: ["about-you"] as const,
  llmSettings: ["llm-settings"] as const,
  discoverySettings: ["discovery-settings"] as const,
  notificationSettings: ["notification-settings"] as const,
  promptGuidance: ["prompt-guidance"] as const,
  versions: ["versions"] as const,
  credentials: ["credentials"] as const,
  countries: ["countries"] as const,
  sources: ["sources"] as const,
  savedSearches: ["saved-searches"] as const,
  searchTemplates: ["search-templates"] as const,
  notifications: ["notifications"] as const,
  jobs: {
    all: ["jobs"] as const,
    count: ["jobs", "count"] as const,
    detail: (id: string) => ["jobs", "detail", id] as const,
    infinite: (filter: StoredJobsFilter, pageSize: number) => ["jobs", "infinite", { ...filter, pageSize }] as const,
  },
  /// companies covers the watched companies, the suggestions left, and
  /// each job's company, which all change when one is watched.
  companies: {
    all: ["companies"] as const,
    list: ["companies", "list"] as const,
    suggestions: ["companies", "suggestions"] as const,
    job: (jobId: string) => ["companies", "job", jobId] as const,
  },
  resumes: {
    all: ["resumes"] as const,
    list: (candidateId: string) => ["resumes", "list", candidateId] as const,
  },
  matches: {
    all: ["matches"] as const,
    list: (candidateId: string) => ["matches", "list", candidateId] as const,
    job: (jobId: string) => ["matches", "job", jobId] as const,
  },
  applications: {
    all: ["applications"] as const,
    list: (candidateId: string) => ["applications", "list", candidateId] as const,
    detail: (id: string) => ["applications", "detail", id] as const,
    history: (id: string) => ["applications", "history", id] as const,
    transitions: (id: string) => ["applications", "transitions", id] as const,
    coverLetters: (id: string) => ["applications", "cover-letters", id] as const,
    questions: (id: string) => ["applications", "questions", id] as const,
  },
  /// scouts covers the scouts and their recent runs, which a run changes
  /// together.
  scouts: {
    all: ["scouts"] as const,
    list: ["scouts", "list"] as const,
    runs: (limit: number) => ["scouts", "runs", limit] as const,
  },
  activity: {
    all: ["activity"] as const,
    infinite: (filter: AgentRunFilter, pageSize: number) => ["activity", "infinite", { ...filter, pageSize }] as const,
    detail: (id: string) => ["activity", "detail", id] as const,
  },
  deletion: {
    all: ["deletion"] as const,
    preview: (target: DeletionTarget | null, id: string) => ["deletion", target, id] as const,
  },
} as const;

/// mutationKeys name mutations that something other than the component
/// that started them watches, through useMutationState.
export const mutationKeys = {
  /// discovery is carried by every discovery run, so any screen can see
  /// what is searching.
  discovery: ["discovery"] as const,
  /// analysis is carried by every match analysis: the button that asks for
  /// a match and the section that shows it are apart, and both say it is
  /// under way.
  analysis: ["analysis"] as const,
} as const;
