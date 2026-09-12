import { useEffect, useRef, useState } from "react";
import { Button } from "../components/ui/button";
import { Card, CardContent } from "../components/ui/card";
import { Input } from "../components/ui/input";
import { useImportJob, useInfiniteJobs, useSearchJobs } from "../lib/queries";
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

  const searchJobs = useSearchJobs({
    onSuccess: (found) => notify(`Found ${found.length} jobs from sources.`, "success"),
    onError: (err) => notify(String(err)),
  });

  const importJob = useImportJob({
    onSuccess: () => {
      setImportUrl("");
      notify("Job imported.", "success");
    },
    onError: (err) => notify(String(err)),
  });

  const filterStored = () => setAppliedKeywords(keywords.trim());

  const searchSources = () => {
    filterStored();
    searchJobs.mutate(keywords.trim() ? { keywords: keywords.trim() } : {});
  };

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
          <Button variant="secondary" onClick={searchSources} disabled={searchJobs.isPending}>
            {searchJobs.isPending ? "Searching…" : "Search sources"}
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
        Typing filters jobs already stored; "Search sources" queries job boards for new ones and needs
        internet. Importing a URL not from a known job board briefly opens a browser window to read the page.
      </p>

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
            <Card key={job.id}>
              <CardContent>
                <p className="font-medium">
                  {job.title} <span className="font-normal text-slate-500">at {job.company_name}</span>
                </p>
                <p className="mt-1 text-xs text-slate-500">
                  {job.remote_type} · {job.employment_type} ·{" "}
                  {job.locations?.join(", ") || "location unspecified"}
                </p>
                <p className="mt-1 text-xs text-slate-400">ID: {job.id}</p>
              </CardContent>
            </Card>
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

/// LoadMore fetches the next page when it scrolls into view, and is also a
/// real button: an observer that never fires (or a user on a keyboard) still
/// has a way to reach the rest of the list.
function LoadMore({
  hasNextPage,
  isFetchingNextPage,
  fetchNextPage,
}: {
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  fetchNextPage: () => void;
}) {
  const sentinel = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = sentinel.current;
    if (!el || !hasNextPage) return;

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting && !isFetchingNextPage) fetchNextPage();
      },
      // Start fetching a little before the end is actually reached, so the
      // next page is usually there by the time the user scrolls to it.
      { rootMargin: "200px" },
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  if (!hasNextPage) return null;

  return (
    <div ref={sentinel} className="pt-2">
      <Button variant="secondary" onClick={fetchNextPage} disabled={isFetchingNextPage}>
        {isFetchingNextPage ? "Loading…" : "Load more"}
      </Button>
    </div>
  );
}
