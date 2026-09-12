import { useState } from "react";
import { Button } from "../components/ui/button";
import { Card, CardContent } from "../components/ui/card";
import { Input } from "../components/ui/input";
import { useImportJob, useJobs, useSearchJobs } from "../lib/queries";
import { useToast } from "../lib/toast";

// One page of stored jobs. Paging through the rest of the list is the next
// phase's work; until then this is the cap on what the screen renders.
const PAGE_SIZE = 50;

export function Jobs() {
  const { notify } = useToast();
  const [keywords, setKeywords] = useState("");
  const [appliedKeywords, setAppliedKeywords] = useState("");
  const [importUrl, setImportUrl] = useState("");

  // The list is stored discovery history, so opening this screen costs a
  // local read rather than a round trip to every job source. "Search
  // sources" is the separate, explicit action that goes to the network;
  // it stores what it finds, which invalidates this query.
  const { data: page, error } = useJobs(appliedKeywords, PAGE_SIZE, 0);

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
      ) : !page ? (
        <p className="text-sm text-slate-500">Loading…</p>
      ) : page.jobs.length === 0 ? (
        <p className="text-sm text-slate-500">
          {appliedKeywords
            ? `No stored jobs match "${appliedKeywords}".`
            : "No jobs discovered yet — use “Search sources” or import one by URL."}
        </p>
      ) : (
        <div className="space-y-2">
          {page.jobs.map((job) => (
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
          {page.total > page.jobs.length && (
            <p className="pt-1 text-xs text-slate-500">
              Showing {page.jobs.length} of {page.total} stored jobs.
            </p>
          )}
        </div>
      )}
    </div>
  );
}
