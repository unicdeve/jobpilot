import { ListFilter } from "lucide-react";
import { Link } from "react-router-dom";
import { FILTER_ALL, FilterSelect } from "../../components/filter-select";
import { Button } from "../../components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "../../components/ui/popover";
import { REMOTE_LABELS, SOURCE_LABELS } from "../../lib/format";
import {
  activeFilterCount,
  type JobFilters,
  type JobsView,
  SCORE_FLOORS,
  TIME_RANGES,
  VIEW_FILTERS,
} from "../../lib/job-filters";
import { useCountries } from "../../lib/queries/saved-search.queries";

const TIME_ITEMS = {
  [FILTER_ALL]: "Any time",
  ...Object.fromEntries(Object.entries(TIME_RANGES).map(([range, { label }]) => [range, label])),
};

/// filterFields are the choices behind each filter, the first meaning "not
/// filtered". The order of VIEW_FILTERS decides the order they are shown in.
/// "Where" names the profile's country, so it is built with it.
function filterFields(
  country: string | undefined,
): Record<keyof JobFilters, { label: string; items: Record<string, string> }> {
  return {
    where: {
      label: "Where you can work",
      items: {
        [FILTER_ALL]: "Any location",
        open: `Doable from ${country ?? "your country"}`,
        relocate: `From ${country ?? "your country"}, or relocating`,
      },
    },
    posted: { label: "Posted", items: TIME_ITEMS },
    analyzed: { label: "Analyzed", items: TIME_ITEMS },
    score: { label: "Match score", items: { [FILTER_ALL]: "Any score", ...SCORE_FLOORS } },
    source: { label: "Source", items: { [FILTER_ALL]: "All sources", ...SOURCE_LABELS } },
    remote: { label: "Work type", items: { [FILTER_ALL]: "Any work type", ...REMOTE_LABELS } },
  };
}

/// FilterMenu holds the current view's filters behind one toolbar button,
/// which carries a dot while any of them is set.
export function FilterMenu({
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
  const { data: countries } = useCountries();
  const profileCountry = countries?.countries.find((c) => c.code === countries.profile_country);
  const fields = filterFields(profileCountry?.name);

  return (
    <Popover>
      <PopoverTrigger
        render={<Button size="icon-sm" variant="ghost" title={label} aria-label={label} className="relative" />}
      >
        <ListFilter />
        {count > 0 && (
          <span
            aria-hidden
            className="absolute top-1 right-1 size-1.5 rounded-full bg-primary ring-2 ring-background"
          />
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
          const field = fields[key];
          const id = `job-filter-${key}`;
          if (key === "where" && !profileCountry) {
            return (
              <div key={key} className="flex flex-col gap-1.5">
                <p className="text-xs font-medium text-muted-foreground">{field.label}</p>
                <p className="text-xs text-muted-foreground">
                  Add your country to your{" "}
                  <Link to="/profile" className="text-foreground underline">
                    profile
                  </Link>{" "}
                  to see the jobs you can do from it.
                </p>
              </div>
            );
          }
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
