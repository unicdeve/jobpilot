import { Link } from "react-router-dom";
import { ListItem, ListNote } from "../../components/list-pane";
import { activeFilterCount, type JobFilters, matchPasses } from "../../lib/job-filters";
import { useJobsByID } from "../../lib/queries/job.queries";
import { useMatches } from "../../lib/queries/match.queries";
import { useCandidateId } from "../../lib/queries/profile.queries";
import { JobRow } from "./job-row";
import { CountNote, NoneMatch } from "./list-notes";

/// AnalyzedJobs ranks the jobs that have a match, best first. Matches carry
/// only a job ID, so the jobs are read one by one — see useJobsByID. Every
/// match is already loaded, so the filters are applied here rather than by
/// the daemon.
export function AnalyzedJobs({
  query,
  filters,
  onClearFilters,
}: {
  query: string;
  filters: JobFilters;
  onClearFilters: () => void;
}) {
  const { candidateId } = useCandidateId();
  const { data: matches, error, isPending } = useMatches(candidateId);
  const jobs = useJobsByID(matches?.map((m) => m.job_id) ?? []);
  const filtered = activeFilterCount(filters, "analyzed") > 0;

  if (!candidateId) {
    return (
      <ListNote>
        Matching compares jobs with your profile.{" "}
        <Link to="/profile" className="underline">
          Create one
        </Link>{" "}
        to analyze jobs.
      </ListNote>
    );
  }
  if (error) return <ListNote tone="error">{error.message}</ListNote>;
  if (isPending) return <ListNote>Loading…</ListNote>;
  if (matches.length === 0) return <ListNote>No jobs analyzed yet. Open a job and choose Analyze match.</ListNote>;

  const needle = query.toLowerCase();
  const now = new Date();
  const ranked = [...matches]
    .sort((a, b) => b.score - a.score)
    .filter((m) => {
      const job = jobs.get(m.job_id);
      if (!matchPasses(m, job, filters, now)) return false;
      if (!needle) return true;
      return job ? `${job.title} ${job.company_name}`.toLowerCase().includes(needle) : false;
    });

  if (ranked.length === 0) {
    if (filtered) return <NoneMatch query={query} onClearFilters={onClearFilters} />;
    return <ListNote>No analyzed jobs match “{query}”.</ListNote>;
  }

  return (
    <>
      <CountNote filtered={filtered} onClearFilters={onClearFilters}>
        {ranked.length} analyzed
        {query && ` matching “${query}”`}, best match first
      </CountNote>
      <div className="border-t border-border">
        {ranked.map((m) => {
          const job = jobs.get(m.job_id);
          return job ? (
            <JobRow key={m.id} job={job} score={m.score} />
          ) : (
            <ListItem key={m.id} to={`/jobs/${m.job_id}`}>
              <p className="text-sm text-muted-foreground">Loading job…</p>
            </ListItem>
          );
        })}
      </div>
    </>
  );
}
