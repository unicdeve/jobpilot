import { Briefcase, ChevronDown, Link2, Search } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, Outlet, useMatch, useNavigate, useSearchParams } from "react-router-dom";
import { ListItem, ListNote, ListPane, NoSelection } from "../components/list-pane";
import { LoadMore } from "../components/load-more";
import { SplitView } from "../components/split-view";
import { Button } from "../components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "../components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../components/ui/dropdown-menu";
import { Input } from "../components/ui/input";
import { Tabs, TabsList, TabsTrigger } from "../components/ui/tabs";
import { jobLocation, relativeTime, remoteLabel, scoreTone, sourceLabel } from "../lib/format";
import {
  useCandidateId,
  useImportJob,
  useInfiniteJobs,
  useJobsByID,
  useMatches,
  useRunSavedSearch,
  useSavedSearches,
} from "../lib/queries";
import { useToast } from "../lib/toast";
import type { Job } from "../lib/types";

const PAGE_SIZE = 50;

type View = "all" | "analyzed";

/// Jobs is every stored job beside the one selected. It is also where
/// analysed jobs are ranked: a match is a job with a score, so the "Analyzed"
/// view is the same list, narrowed and ordered by that score.
///
/// The view and the search live in the URL, so a link can open a filtered
/// list (the dashboard does) and selecting a job keeps the list as it was.
export function Jobs() {
  const hasSelection = useMatch("/jobs/:id") !== null;
  const [params, setParams] = useSearchParams();
  const view: View = params.get("view") === "analyzed" ? "analyzed" : "all";
  const query = params.get("q") ?? "";

  const setParam = (key: string, value: string) =>
    setParams(
      (current) => {
        const next = new URLSearchParams(current);
        if (value) next.set(key, value);
        else next.delete(key);
        return next;
      },
      { replace: true },
    );

  return (
    <SplitView
      hasSelection={hasSelection}
      list={
        <ListPane
          title="Jobs"
          actions={
            <>
              <AddByUrlButton />
              <DiscoverMenu />
            </>
          }
          controls={
            <>
              <SearchField value={query} onChange={(value) => setParam("q", value)} />
              <Tabs value={view} onValueChange={(value) => setParam("view", value === "analyzed" ? "analyzed" : "")}>
                <TabsList className="w-full">
                  <TabsTrigger value="all">All jobs</TabsTrigger>
                  <TabsTrigger value="analyzed">Analyzed</TabsTrigger>
                </TabsList>
              </Tabs>
            </>
          }
        >
          {view === "all" ? <AllJobs query={query} /> : <AnalyzedJobs query={query} />}
        </ListPane>
      }
      detail={<Outlet />}
    />
  );
}

export function NoJobSelected() {
  return <NoSelection icon={<Briefcase />} title="No job selected" description="Choose a job to see its details and match." />;
}

/// SearchField filters as the user types. The request waits for a pause, so
/// typing a word is one query rather than one per keystroke.
function SearchField({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  const [draft, setDraft] = useState(value);
  useEffect(() => setDraft(value), [value]);
  useEffect(() => {
    if (draft.trim() === value) return;
    const timer = window.setTimeout(() => onChange(draft.trim()), 250);
    return () => window.clearTimeout(timer);
  }, [draft, value, onChange]);

  return (
    <div className="relative">
      <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
      <Input
        type="search"
        aria-label="Search jobs"
        placeholder="Search title or company"
        value={draft}
        onChange={(event) => setDraft(event.target.value)}
        className="h-8 pl-8"
      />
    </div>
  );
}

function AllJobs({ query }: { query: string }) {
  const { data, error, fetchNextPage, hasNextPage, isFetchingNextPage, isPending } = useInfiniteJobs(query, PAGE_SIZE);
  const { candidateId } = useCandidateId();
  const { data: matches } = useMatches(candidateId);
  const scoreByJob = useMemo(() => new Map(matches?.map((m) => [m.job_id, m.score])), [matches]);

  if (error) return <ListNote tone="error">{error.message}</ListNote>;
  if (isPending) return <ListNote>Loading…</ListNote>;

  const jobs = data.pages.flatMap((page) => page.jobs);
  const total = data.pages[0]?.total ?? 0;

  if (jobs.length === 0) {
    return (
      <ListNote>
        {query ? `No stored jobs match “${query}”.` : "No jobs yet. Use Discover to search your job sources, or add one by URL."}
      </ListNote>
    );
  }

  return (
    <>
      <ListNote>
        {total} {total === 1 ? "job" : "jobs"}
        {query && ` matching “${query}”`}
      </ListNote>
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

/// AnalyzedJobs ranks the jobs that have a match, best first. Matches carry
/// only a job ID, so the jobs are read one by one — see useJobsByID.
function AnalyzedJobs({ query }: { query: string }) {
  const { candidateId } = useCandidateId();
  const { data: matches, error, isPending } = useMatches(candidateId);
  const jobs = useJobsByID(matches?.map((m) => m.job_id) ?? []);

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
  const ranked = [...matches]
    .sort((a, b) => b.score - a.score)
    .filter((m) => {
      if (!needle) return true;
      const job = jobs.get(m.job_id);
      return job ? `${job.title} ${job.company_name}`.toLowerCase().includes(needle) : false;
    });

  if (ranked.length === 0) return <ListNote>No analyzed jobs match “{query}”.</ListNote>;

  return (
    <>
      <ListNote>{ranked.length} analyzed, best match first</ListNote>
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

function JobRow({ job, score }: { job: Job; score?: number }) {
  const where = [remoteLabel(job.remote_type), jobLocation(job)].filter(Boolean).join(" · ");
  const age = relativeTime(job.posted_at) ?? relativeTime(job.discovered_at);
  return (
    <ListItem to={`/jobs/${job.id}`}>
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <p className="line-clamp-2 text-[13px] leading-snug font-medium">{job.title}</p>
          <p className="mt-0.5 truncate text-xs text-subtle-foreground">{job.company_name || "Unknown company"}</p>
          <p className="mt-0.5 truncate text-xs text-muted-foreground">
            {[where, sourceLabel(job.source_id), age].filter(Boolean).join(" · ")}
          </p>
        </div>
        {score !== undefined && (
          <span
            title="Match score"
            className={`mt-0.5 shrink-0 rounded-md px-1.5 py-0.5 text-xs font-semibold tabular-nums ${scoreTone(score)}`}
          >
            {score}
          </span>
        )}
      </div>
    </ListItem>
  );
}

/// DiscoverMenu runs one of the user's saved searches against the job
/// sources. Discovery without criteria is refused by the daemon — the
/// boards return their whole listing regardless of what is asked — so with
/// no saved search the menu points at Settings instead.
function DiscoverMenu() {
  const { notify } = useToast();
  const { data: searches } = useSavedSearches();
  const runSearch = useRunSavedSearch({
    onSuccess: (found) => notify(`Found ${found.length} matching ${found.length === 1 ? "job" : "jobs"}.`, "success"),
    onError: (err) => notify(String(err)),
  });

  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button size="sm" disabled={runSearch.isPending} />}>
        {runSearch.isPending ? "Searching…" : "Discover"}
        <ChevronDown data-icon="inline-end" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-60">
        <DropdownMenuGroup>
          <DropdownMenuLabel>Search your job sources with</DropdownMenuLabel>
          {searches?.map((search) => (
            <DropdownMenuItem key={search.id} onClick={() => runSearch.mutate(search.id)}>
              <span className="flex-1 truncate">{search.name}</span>
              {search.is_default && <span className="text-xs text-muted-foreground">Default</span>}
            </DropdownMenuItem>
          ))}
          {searches?.length === 0 && <DropdownMenuLabel className="font-normal">No saved searches yet.</DropdownMenuLabel>}
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem render={<Link to="/settings?section=searches" />}>Manage searches…</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/// AddByUrlButton imports one posting from its URL. A URL no registered
/// source recognises is read in a real browser window, which the dialog
/// says, since a window appearing unannounced looks like something wrong.
function AddByUrlButton() {
  const { notify } = useToast();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [url, setUrl] = useState("");
  const importJob = useImportJob({
    onSuccess: (job) => {
      setOpen(false);
      setUrl("");
      navigate(`/jobs/${job.id}`);
    },
    onError: (err) => notify(String(err)),
  });

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    if (url.trim()) importJob.mutate(url.trim());
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <Button
        size="icon-sm"
        variant="ghost"
        title="Add a job by URL"
        aria-label="Add a job by URL"
        onClick={() => setOpen(true)}
      >
        <Link2 />
      </Button>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogTitle>Add a job by URL</DialogTitle>
          <DialogDescription>
            Paste the link to a job posting. If it isn’t from a job board JobPilot knows, a browser window opens briefly to
            read the page.
          </DialogDescription>
          <Input
            autoFocus
            type="url"
            className="mt-4"
            placeholder="https://"
            value={url}
            onChange={(event) => setUrl(event.target.value)}
            disabled={importJob.isPending}
          />
          <DialogFooter>
            <DialogClose render={<Button type="button" variant="outline" />}>Cancel</DialogClose>
            <Button type="submit" disabled={!url.trim() || importJob.isPending}>
              {importJob.isPending ? "Adding…" : "Add job"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
