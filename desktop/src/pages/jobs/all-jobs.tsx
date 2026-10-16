import { useMemo } from "react";
import { ListNote } from "../../components/list-pane";
import { LoadMore } from "../../components/load-more";
import { activeFilterCount, type JobFilters, type JobsView } from "../../lib/job-filters";
import { useInfiniteJobs } from "../../lib/queries/job.queries";
import { useMatches } from "../../lib/queries/match.queries";
import { useCandidateId } from "../../lib/queries/profile.queries";
import { useCountries } from "../../lib/queries/saved-search.queries";
import { JobRow } from "./job-row";
import { CountNote, NoJobsYet, NoneMatch } from "./list-notes";

const PAGE_SIZE = 50;

/// AllJobs lists stored jobs, newest first. With notInterested it lists the
/// jobs the user ruled out instead, most recently ruled out first, which is
/// the one place they can be found again.
export function AllJobs({
  query,
  filters,
  onClearFilters,
  notInterested = false,
}: {
  query: string;
  filters: JobFilters;
  onClearFilters: () => void;
  notInterested?: boolean;
}) {
  const view: JobsView = notInterested ? "not_interested" : "all";
  const { data: countries } = useCountries();
  const workFrom = filters.where ? countries?.profile_country || undefined : undefined;
  const { data, error, fetchNextPage, hasNextPage, isFetchingNextPage, isPending } = useInfiniteJobs(
    {
      keywords: query,
      source: filters.source,
      remote: filters.remote,
      posted: filters.posted,
      notInterested,
      workFrom,
      includeRelocation: workFrom !== undefined && filters.where === "relocate",
    },
    PAGE_SIZE,
  );
  const { candidateId } = useCandidateId();
  const { data: matches } = useMatches(candidateId);
  const scoreByJob = useMemo(() => new Map(matches?.map((m) => [m.job_id, m.score])), [matches]);
  const filtered = activeFilterCount(filters, view) > 0;

  if (error) return <ListNote tone="error">{error.message}</ListNote>;
  if (isPending) return <ListNote>Loading…</ListNote>;

  const jobs = data.pages.flatMap((page) => page.jobs);
  const total = data.pages[0]?.total ?? 0;

  if (jobs.length === 0) {
    if (filtered) return <NoneMatch query={query} onClearFilters={onClearFilters} />;
    if (notInterested) {
      return (
        <ListNote>
          {query
            ? `No jobs marked not interested match “${query}”.`
            : "No jobs marked not interested. A job you rule out is listed here, and scouts pass it over."}
        </ListNote>
      );
    }
    return <NoJobsYet query={query} />;
  }

  return (
    <>
      <CountNote filtered={filtered} onClearFilters={onClearFilters}>
        {total} {total === 1 ? "job" : "jobs"}
        {notInterested && " marked not interested"}
        {query && ` matching “${query}”`}
      </CountNote>
      <div className="border-t border-border">
        {jobs.map((job) => (
          <JobRow key={job.id} job={job} score={scoreByJob.get(job.id)} />
        ))}
      </div>
      {hasNextPage && (
        <div className="px-4 pb-4">
          <LoadMore hasNextPage={hasNextPage} isFetchingNextPage={isFetchingNextPage} fetchNextPage={fetchNextPage} />
        </div>
      )}
    </>
  );
}
