import { useDiscoveryRuns } from "../../components/discovery-activity";
import { ListNote } from "../../components/list-pane";
import { Button } from "../../components/ui/button";

/// CountNote is the line above a list saying how many it holds, and, when
/// filters are narrowing it, that they are — with the way to lift them —
/// since a short list otherwise reads as all there is.
export function CountNote({
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

export function NoneMatch({ query, onClearFilters }: { query: string; onClearFilters: () => void }) {
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

/// NoJobsYet is the empty list with nothing filtered. While a search is
/// running — the first one after setup, say — it says so, rather than
/// telling the user to start one.
export function NoJobsYet({ query }: { query: string }) {
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
