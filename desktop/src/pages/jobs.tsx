import { useState } from "react";
import { Link } from "react-router-dom";
import { FormField } from "../components/form-field";
import { LoadMore } from "../components/load-more";
import { Button } from "../components/ui/button";
import { Card, CardContent } from "../components/ui/card";
import { Input } from "../components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../components/ui/select";
import type { SavedSearch } from "../lib/types";
import {
  useCandidateId,
  useImportJob,
  useInfiniteJobs,
  useMatches,
  useRunSavedSearch,
  useSavedSearches,
} from "../lib/queries";
import { useToast } from "../lib/toast";

const PAGE_SIZE = 50;

export function Jobs() {
  const { notify } = useToast();
  const [keywords, setKeywords] = useState("");
  const [appliedKeywords, setAppliedKeywords] = useState("");
  const [importUrl, setImportUrl] = useState("");

  // The list is stored discovery history, so opening this screen costs a
  // local read rather than a round trip to every job source. "Search
  // sources" is the separate, explicit action that goes to the network; it
  // stores what it finds, which invalidates this query.
  const { data, error, fetchNextPage, hasNextPage, isFetchingNextPage, isPending } =
    useInfiniteJobs(appliedKeywords, PAGE_SIZE);

  // Matches are already loaded as one list per candidate, so showing a score
  // beside a job costs nothing beyond a lookup.
  const { candidateId } = useCandidateId();
  const { data: matches } = useMatches(candidateId);
  const scoreByJob = new Map(matches?.map((m) => [m.job_id, m.score]));

  // Discovery runs a saved search, never a free-text box that defaults to
  // everything: neither public board filters server-side, so an unrestricted
  // search imports the entire listing rather than the jobs the user wants.
  const { data: savedSearches } = useSavedSearches();
  const [selectedSearch, setSelectedSearch] = useState("");
  const runSearch = useRunSavedSearch({
    onSuccess: (found) => notify(`Found ${found.length} matching jobs.`, "success"),
    onError: (err) => notify(String(err)),
  });

  const defaultSearch = savedSearches?.find((search) => search.is_default) ?? savedSearches?.[0];
  const searchToRun = savedSearches?.find((s) => s.id === selectedSearch) ?? defaultSearch;
  const searchLabels = Object.fromEntries((savedSearches ?? []).map((s) => [s.id, s.name]));

  const importJob = useImportJob({
    onSuccess: () => {
      setImportUrl("");
      notify("Job imported.", "success");
    },
    onError: (err) => notify(String(err)),
  });

  const filterStored = () => setAppliedKeywords(keywords.trim());

  const runImport = () => {
    if (!importUrl.trim()) return;
    importJob.mutate(importUrl.trim());
  };

  const jobs = data?.pages.flatMap((page) => page.jobs) ?? [];
  const total = data?.pages[0]?.total ?? 0;

  return (
    <div>
      <h1 className="mb-4 text-xl font-semibold">Jobs</h1>

      <Card className="mb-4">
        <CardContent className="flex flex-wrap items-end gap-3">
          <div className="flex-1 min-w-48">
            <Input
              placeholder="Filter stored jobs…"
              value={keywords}
              onChange={(e) => setKeywords(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && filterStored()}
            />
          </div>
          <Button variant="secondary" onClick={filterStored}>
            Filter
          </Button>
          <div className="flex-1 min-w-48">
            <Input
              placeholder="Import job by URL…"
              value={importUrl}
              onChange={(e) => setImportUrl(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && runImport()}
              disabled={importJob.isPending}
            />
          </div>
          <Button onClick={runImport} disabled={importJob.isPending}>
            {importJob.isPending ? "Importing…" : "Import"}
          </Button>
        </CardContent>
      </Card>
      <p className="mb-4 -mt-2 text-xs text-slate-500">
        Filtering searches jobs already stored. Importing a URL not from a known job board briefly
        opens a browser window to read the page.
      </p>

      <DiscoverPanel
        searches={savedSearches}
        selected={searchToRun?.id ?? ""}
        onSelect={setSelectedSearch}
        labels={searchLabels}
        pending={runSearch.isPending}
        onRun={() => searchToRun && runSearch.mutate(searchToRun.id)}
      />

      {error ? (
        <p className="text-sm text-red-600">{String(error.message)}</p>
      ) : isPending ? (
        <p className="text-sm text-slate-500">Loading…</p>
      ) : jobs.length === 0 ? (
        <p className="text-sm text-slate-500">
          {appliedKeywords
            ? `No stored jobs match "${appliedKeywords}".`
            : "No jobs discovered yet — use “Search sources” or import one by URL."}
        </p>
      ) : (
        <div className="space-y-2">
          <p className="text-xs text-slate-500">
            {jobs.length === total
              ? `${total} stored ${total === 1 ? "job" : "jobs"}`
              : `Showing ${jobs.length} of ${total} stored jobs`}
          </p>
          {jobs.map((job) => (
            <Link key={job.id} to={`/jobs/${job.id}`} className="block">
              <Card className="hover:ring-foreground/20">
                <CardContent className="flex items-start justify-between gap-3">
                  <div>
                    <p className="font-medium">
                      {job.title}{" "}
                      <span className="font-normal text-slate-500">at {job.company_name}</span>
                    </p>
                    <p className="mt-1 text-xs text-slate-500">
                      {job.remote_type} · {job.employment_type} ·{" "}
                      {job.locations?.join(", ") || "location unspecified"}
                    </p>
                  </div>
                  {scoreByJob.has(job.id) && (
                    <span className="shrink-0 text-xs font-medium text-slate-600">
                      Match {scoreByJob.get(job.id)}/100
                    </span>
                  )}
                </CardContent>
              </Card>
            </Link>
          ))}
          <LoadMore
            hasNextPage={hasNextPage}
            isFetchingNextPage={isFetchingNextPage}
            fetchNextPage={fetchNextPage}
          />
        </div>
      )}
    </div>
  );
}

/// DiscoverPanel runs one of the user's saved searches against the job
/// sources. When there are none it says so and points at Settings rather
/// than offering a button that cannot work: discovery without criteria is
/// refused by the daemon, because the boards return their whole listing
/// regardless of what is asked for.
function DiscoverPanel({
  searches,
  selected,
  onSelect,
  labels,
  pending,
  onRun,
}: {
  searches: SavedSearch[] | undefined;
  selected: string;
  onSelect: (id: string) => void;
  labels: Record<string, string>;
  pending: boolean;
  onRun: () => void;
}) {
  if (!searches) return null;

  if (searches.length === 0) {
    return (
      <Card className="mb-4">
        <CardContent>
          <p className="text-sm text-slate-700">
            No job searches configured yet. JobPilot only looks for what you ask it to, so
            discovery needs at least one search.
          </p>
          <p className="mt-1 text-sm text-slate-500">
            Add one under{" "}
            <Link to="/settings" className="underline">
              Settings → Job searches
            </Link>
            .
          </p>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className="mb-4">
      <CardContent className="flex flex-wrap items-end gap-3">
        <div className="min-w-48 flex-1">
          <FormField label="Discover jobs using" htmlFor="saved-search">
            <Select items={labels} value={selected} onValueChange={(value) => onSelect(value ?? "")}>
              <SelectTrigger id="saved-search" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {searches.map((search) => (
                  <SelectItem key={search.id} value={search.id}>
                    {search.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </FormField>
        </div>
        <Button onClick={onRun} disabled={pending}>
          {pending ? "Searching…" : "Search sources"}
        </Button>
      </CardContent>
    </Card>
  );
}
