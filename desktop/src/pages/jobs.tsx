import { Briefcase, ChevronDown, EyeOff, Link2, ListFilter, Search, Undo2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, Outlet, useMatch, useNavigate, useSearchParams } from "react-router-dom";
import { FILTER_ALL, FilterSelect } from "../components/filter-select";
import { useDiscover, useDiscoveryRuns } from "../components/discovery-activity";
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
import { Popover, PopoverContent, PopoverTrigger } from "../components/ui/popover";
import { Tabs, TabsList, TabsTrigger } from "../components/ui/tabs";
import { jobLocation, relativeTime, REMOTE_LABELS, remoteLabel, scoreTone, SOURCE_LABELS, sourceLabel } from "../lib/format";
import {
  activeFilterCount,
  type JobFilters,
  type JobsView,
  matchPasses,
  readJobFilters,
  SCORE_FLOORS,
  TIME_RANGES,
  VIEW_FILTERS,
} from "../lib/job-filters";
import {
  useApplications,
  useCandidateId,
  useImportJob,
  useInfiniteJobs,
  useJobsByID,
  useMatches,
  useSetJobInterest,
  useSavedSearches,
} from "../lib/queries";
import { useToast } from "../lib/toast";
import type { Job } from "../lib/types";

const PAGE_SIZE = 50;

/// Jobs is every stored job beside the one selected. It is also where
/// analysed jobs are ranked: a match is a job with a score, so the "Analyzed"
/// view is the same list, narrowed and ordered by that score.
///
/// The view, the search and the filters live in the URL, so a link can open
/// a filtered list (the dashboard does) and selecting a job keeps the list as
/// it was.
export function Jobs() {
  const hasSelection = useMatch("/jobs/:id") !== null;
  const [params, setParams] = useSearchParams();
  const requestedView = params.get("view");
  const view: JobsView = requestedView === "analyzed" || requestedView === "not_interested" ? requestedView : "all";
  const query = params.get("q") ?? "";
  const filters = readJobFilters(params);

  const updateParams = (changes: Record<string, string>) =>
    setParams(
      (current) => {
        const next = new URLSearchParams(current);
        for (const [key, value] of Object.entries(changes)) {
          if (value) next.set(key, value);
          else next.delete(key);
        }
        return next;
      },
      { replace: true },
    );
  const setParam = (key: string, value: string) => updateParams({ [key]: value });
  // Clearing clears what this view filters by, and leaves the search, which
  // has its own field to clear it from.
  const clearFilters = () => updateParams(Object.fromEntries(VIEW_FILTERS[view].map((key) => [key, ""])));

  return (
    <SplitView
      hasSelection={hasSelection}
      list={
        <ListPane
          title="Jobs"
          actions={
            <>
              <AddByUrlButton />
              <FilterMenu view={view} filters={filters} onChange={setParam} onClear={clearFilters} />
              <DiscoverMenu />
            </>
          }
          controls={
            <>
              <SearchField value={query} onChange={(value) => setParam("q", value)} />
              <Tabs value={view} onValueChange={(value) => setParam("view", value === "all" ? "" : String(value))}>
                <TabsList className="w-full">
                  <TabsTrigger value="all">All jobs</TabsTrigger>
                  <TabsTrigger value="analyzed">Analyzed</TabsTrigger>
                  <TabsTrigger value="not_interested">Not interested</TabsTrigger>
                </TabsList>
              </Tabs>
            </>
          }
        >
          {view === "analyzed" ? (
            <AnalyzedJobs query={query} filters={filters} onClearFilters={clearFilters} />
          ) : (
            <AllJobs query={query} filters={filters} onClearFilters={clearFilters} notInterested={view === "not_interested"} />
          )}
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

/// AllJobs lists stored jobs, newest first. With notInterested it lists the
/// jobs the user ruled out instead, most recently ruled out first, which is
/// the one place they can be found again.
function AllJobs({
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
  const { data, error, fetchNextPage, hasNextPage, isFetchingNextPage, isPending } = useInfiniteJobs(
    { keywords: query, source: filters.source, remote: filters.remote, posted: filters.posted, notInterested },
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

/// AnalyzedJobs ranks the jobs that have a match, best first. Matches carry
/// only a job ID, so the jobs are read one by one — see useJobsByID. Every
/// match is already loaded, so the filters are applied here rather than by
/// the daemon.
function AnalyzedJobs({
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

/// CountNote is the line above a list saying how many it holds, and, when
/// filters are narrowing it, that they are — with the way to lift them —
/// since a short list otherwise reads as all there is.
function CountNote({
  filtered,
  onClearFilters,
  children,
}: {
  filtered: boolean;
  onClearFilters: () => void;
  children: React.ReactNode;
}) {
  return (
    <div className="flex items-center gap-2 px-4 py-3 text-xs text-muted-foreground">
      <p className="min-w-0 flex-1 truncate">
        {children}
        {filtered && " · filtered"}
      </p>
      {filtered && <ClearFiltersButton onClick={onClearFilters} />}
    </div>
  );
}

function NoneMatch({ query, onClearFilters }: { query: string; onClearFilters: () => void }) {
  return (
    <div className="flex flex-col items-start gap-2 px-4 py-3 text-xs text-muted-foreground">
      <p>{query ? `No jobs matching “${query}” pass these filters.` : "No jobs match these filters."}</p>
      <ClearFiltersButton onClick={onClearFilters} />
    </div>
  );
}

function ClearFiltersButton({ onClick }: { onClick: () => void }) {
  return (
    <Button size="xs" variant="outline" onClick={onClick}>
      Clear filters
    </Button>
  );
}

const TIME_ITEMS = {
  [FILTER_ALL]: "Any time",
  ...Object.fromEntries(Object.entries(TIME_RANGES).map(([range, { label }]) => [range, label])),
};

/// FILTER_FIELDS are the choices behind each filter, the first meaning "not
/// filtered". The order of VIEW_FILTERS decides the order they are shown in.
const FILTER_FIELDS: Record<keyof JobFilters, { label: string; items: Record<string, string> }> = {
  posted: { label: "Posted", items: TIME_ITEMS },
  analyzed: { label: "Analyzed", items: TIME_ITEMS },
  score: { label: "Match score", items: { [FILTER_ALL]: "Any score", ...SCORE_FLOORS } },
  source: { label: "Source", items: { [FILTER_ALL]: "All sources", ...SOURCE_LABELS } },
  remote: { label: "Work type", items: { [FILTER_ALL]: "Any work type", ...REMOTE_LABELS } },
};

/// FilterMenu holds the current view's filters behind one toolbar button,
/// which carries a dot while any of them is set.
function FilterMenu({
  view,
  filters,
  onChange,
  onClear,
}: {
  view: JobsView;
  filters: JobFilters;
  onChange: (key: keyof JobFilters, value: string) => void;
  onClear: () => void;
}) {
  const count = activeFilterCount(filters, view);
  const label = count > 0 ? `Filter jobs (${count} active)` : "Filter jobs";

  return (
    <Popover>
      <PopoverTrigger
        render={<Button size="icon-sm" variant="ghost" title={label} aria-label={label} className="relative" />}
      >
        <ListFilter />
        {count > 0 && (
          <span aria-hidden className="absolute top-1 right-1 size-1.5 rounded-full bg-primary ring-2 ring-background" />
        )}
      </PopoverTrigger>
      <PopoverContent align="end" className="w-64 gap-3 p-3">
        <div className="flex h-6 items-center justify-between">
          <p className="text-[13px] font-semibold">{view === "analyzed" ? "Filter analyzed jobs" : "Filter jobs"}</p>
          {count > 0 && (
            <Button size="xs" variant="ghost" className="-mr-1.5" onClick={onClear}>
              Clear
            </Button>
          )}
        </div>
        {VIEW_FILTERS[view].map((key) => {
          const field = FILTER_FIELDS[key];
          const id = `job-filter-${key}`;
          return (
            <div key={key} className="flex flex-col gap-1.5">
              <label htmlFor={id} className="text-xs font-medium text-muted-foreground">
                {field.label}
              </label>
              <FilterSelect
                id={id}
                label={field.label}
                items={field.items}
                value={filters[key]}
                onChange={(value) => onChange(key, value)}
                className="w-full"
              />
            </div>
          );
        })}
      </PopoverContent>
    </Popover>
  );
}

function JobRow({ job, score }: { job: Job; score?: number }) {
  const where = [remoteLabel(job.remote_type), jobLocation(job)].filter(Boolean).join(" · ");
  const age = relativeTime(job.posted_at) ?? relativeTime(job.discovered_at);
  // The row is a link, which cannot hold a button, so the quick action sits
  // beside it and shows itself while the row is hovered or focused.
  return (
    <div className="group relative">
      <JobRowLink job={job} score={score} where={where} age={age} />
      <RowInterestButton job={job} />
    </div>
  );
}

/// RowInterestButton rules a job out, or back in, from the list. Ruling one
/// out leaves its applications as they are, which the notice says.
function RowInterestButton({ job }: { job: Job }) {
  const { notify } = useToast();
  const { candidateId } = useCandidateId();
  const { data: applications } = useApplications(candidateId);
  const ruledOut = !!job.not_interested_at;
  const setInterest = useSetJobInterest({
    onSuccess: (updated) => {
      if (!updated.not_interested_at) {
        notify(`“${updated.title}” is back in your lists.`, "success");
        return;
      }
      const kept = applications?.filter((a) => a.job_id === updated.id).length ?? 0;
      notify(
        `“${updated.title}” marked not interested.` +
          (kept === 0 ? "" : kept === 1 ? " Its application is unchanged." : ` Its ${kept} applications are unchanged.`),
        "success",
      );
    },
    onError: (err) => notify(String(err)),
  });
  const label = ruledOut ? "Interested again" : "Not interested";
  return (
    <Button
      size="icon-xs"
      variant="outline"
      aria-label={`${label}: ${job.title}`}
      title={label}
      disabled={setInterest.isPending}
      onClick={() => setInterest.mutate({ id: job.id, notInterested: !ruledOut })}
      className="absolute right-3 bottom-2 bg-popover opacity-0 shadow-xs transition-opacity group-hover:opacity-100 focus-visible:opacity-100"
    >
      {ruledOut ? <Undo2 /> : <EyeOff />}
    </Button>
  );
}

function JobRowLink({ job, score, where, age }: { job: Job; score?: number; where: string; age?: string }) {
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

/// NoJobsYet is the empty list with nothing filtered. While a search is
/// running — the first one after setup, say — it says so, rather than
/// telling the user to start one.
function NoJobsYet({ query }: { query: string }) {
  const searching = useDiscoveryRuns().length > 0;
  if (query) return <ListNote>No stored jobs match “{query}”.</ListNote>;
  return (
    <ListNote>
      {searching
        ? "Searching your job sources. Jobs appear here as each search finishes."
        : "No jobs yet. Use Discover to search your job sources, or add one by URL."}
    </ListNote>
  );
}

/// DiscoverMenu runs one of the user's saved searches against the job
/// sources. Discovery without criteria is refused by the daemon — the
/// boards return their whole listing regardless of what is asked — so with
/// no saved search the menu points at Settings instead.
///
/// A run already going or queued, from here or anywhere else, cannot be
/// asked for twice; another search can, and waits its turn. "Search all"
/// queues every search not already waiting, in the order they are listed.
function DiscoverMenu() {
  const { data: searches } = useSavedSearches();
  const discover = useDiscover();
  const runs = useDiscoveryRuns();
  const pending = new Set(runs.map((run) => run.search.id));
  const notPending = (searches ?? []).filter((search) => !pending.has(search.id));
  const searchAll = () => {
    for (const search of notPending) discover.mutate({ id: search.id, name: search.name });
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button size="sm" />}>
        {runs.length > 0 ? "Searching…" : "Discover"}
        <ChevronDown data-icon="inline-end" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-60">
        <DropdownMenuGroup>
          <DropdownMenuLabel>Search your job sources with</DropdownMenuLabel>
          {(searches?.length ?? 0) > 1 && (
            <>
              <DropdownMenuItem disabled={notPending.length === 0} onClick={searchAll}>
                <span className="flex-1">All saved searches</span>
                <span className="text-xs text-muted-foreground tabular-nums">{searches?.length}</span>
              </DropdownMenuItem>
              <DropdownMenuSeparator />
            </>
          )}
          {searches?.map((search) => (
            <DropdownMenuItem
              key={search.id}
              disabled={pending.has(search.id)}
              onClick={() => discover.mutate({ id: search.id, name: search.name })}
            >
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
