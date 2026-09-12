import { useEffect, useState } from "react";
import { Button } from "../components/ui/button";
import { Card, CardContent } from "../components/ui/card";
import { Input } from "../components/ui/input";
import { useApi } from "../lib/daemon";
import { useToast } from "../lib/toast";
import type { Job } from "../lib/types";

export function Jobs() {
  const api = useApi();
  const { notify } = useToast();
  const [jobs, setJobs] = useState<Job[]>([]);
  const [keywords, setKeywords] = useState("");
  const [importUrl, setImportUrl] = useState("");
  const [loading, setLoading] = useState(false);
  const [importing, setImporting] = useState(false);

  const load = async (criteriaKeywords = "") => {
    setLoading(true);
    try {
      setJobs(await api.searchJobs(criteriaKeywords ? { keywords: criteriaKeywords } : {}));
    } catch (err) {
      notify(String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api]);

  const importJob = async () => {
    if (!importUrl.trim()) return;
    setImporting(true);
    try {
      await api.importJob(importUrl.trim());
      setImportUrl("");
      await load(keywords);
      notify("Job imported.", "success");
    } catch (err) {
      notify(String(err));
    } finally {
      setImporting(false);
    }
  };

  return (
    <div>
      <h1 className="mb-4 text-xl font-semibold">Jobs</h1>

      <Card className="mb-4">
        <CardContent className="flex flex-wrap items-end gap-3">
          <div className="flex-1 min-w-48">
            <Input
              placeholder="Search keywords…"
              value={keywords}
              onChange={(e) => setKeywords(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && load(keywords)}
            />
          </div>
          <Button variant="secondary" onClick={() => load(keywords)} disabled={loading}>
            Search
          </Button>
          <div className="flex-1 min-w-48">
            <Input
              placeholder="Import job by URL…"
              value={importUrl}
              onChange={(e) => setImportUrl(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && importJob()}
              disabled={importing}
            />
          </div>
          <Button onClick={importJob} disabled={importing}>
            {importing ? "Importing…" : "Import"}
          </Button>
        </CardContent>
      </Card>
      <p className="mb-4 -mt-2 text-xs text-slate-500">
        Importing a URL not from a known job board briefly opens a browser window to read the page.
      </p>

      {jobs.length === 0 ? (
        <p className="text-sm text-slate-500">No jobs discovered yet.</p>
      ) : (
        <div className="space-y-2">
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
        </div>
      )}
    </div>
  );
}
