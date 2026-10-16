import { Briefcase, Search } from "lucide-react";
import { useEffect, useState } from "react";
import { Outlet, useMatch, useSearchParams } from "react-router-dom";
import { ListPane, NoSelection } from "../../components/list-pane";
import { SplitView } from "../../components/split-view";
import { Input } from "../../components/ui/input";
import { Tabs, TabsList, TabsTrigger } from "../../components/ui/tabs";
import { type JobsView, readJobFilters, VIEW_FILTERS } from "../../lib/job-filters";
import { AllJobs } from "./all-jobs";
import { AnalyzedJobs } from "./analyzed-jobs";
import { AddByUrlButton, DiscoverMenu } from "./discover-menu";
import { FilterMenu } from "./filter-menu";

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
            <AllJobs
              query={query}
              filters={filters}
              onClearFilters={clearFilters}
              notInterested={view === "not_interested"}
            />
          )}
        </ListPane>
      }
      detail={<Outlet />}
    />
  );
}

export function NoJobSelected() {
  return (
    <NoSelection
      icon={<Briefcase />}
      title="No job selected"
      description="Choose a job to see its details and match."
    />
  );
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
