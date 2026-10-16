import { Ellipsis, Plus } from "lucide-react";
import { useState } from "react";
import { CompanySettings } from "../../components/company-settings";
import { ConfirmDialog } from "../../components/confirm";
import { Panel } from "../../components/page";
import { emptyCriteria, SavedSearchForm } from "../../components/saved-search-form";
import { SearchTemplatePicker } from "../../components/search-template-picker";
import { SettingsHeading } from "../../components/settings-heading";
import { ErrorLine, Loading } from "../../components/status-line";
import { Button } from "../../components/ui/button";
import { Checkbox } from "../../components/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../../components/ui/dropdown-menu";
import { dateTime, relativeTime } from "../../lib/format";
import { useDiscoverySettings, useSaveDiscoverySettings } from "../../lib/queries/discovery.queries";
import {
  useCountries,
  useCreateSavedSearch,
  useDeleteSavedSearch,
  useSavedSearches,
  useSetDefaultSavedSearch,
  useUpdateSavedSearch,
} from "../../lib/queries/saved-search.queries";
import { useToast } from "../../lib/toast";
import type { SavedSearch, SearchCriteria } from "../../lib/types";

/// RefreshOnLaunchSetting turns the launch refresh on and off
/// (docs/02_features/04_job_discovery.md AC-009). It is the daemon's
/// setting, so it holds for however the desktop is installed.
function RefreshOnLaunchSetting() {
  const { notifyError } = useToast();
  const { data: settings } = useDiscoverySettings();
  const save = useSaveDiscoverySettings({ onError: notifyError });

  return (
    <Panel className="mt-4 px-4 py-3">
      <label className="flex items-start gap-3">
        <Checkbox
          className="mt-0.5"
          checked={settings?.refresh_on_launch ?? false}
          disabled={!settings || save.isPending}
          onCheckedChange={(checked) => save.mutate({ refresh_on_launch: checked === true })}
        />
        <span>
          <span className="block text-[13px] font-medium">Look for new jobs when JobPilot opens</span>
          <span className="mt-0.5 block text-xs text-muted-foreground">
            Runs each search that hasn’t run in the last 6 hours, in the background.
          </span>
        </span>
      </label>
    </Panel>
  );
}

/// JobSearchSettings manages the saved searches discovery runs against.
/// Without at least one, discovery refuses to run — the public job boards do
/// no filtering of their own, so a search with nothing set would import every
/// posting on every board rather than the jobs the user wants.
export function JobSearchSettings() {
  const { notify, notifyError } = useToast();
  const { data: searches, error, isPending } = useSavedSearches();
  const { data: countries } = useCountries();
  const countryName = (code: string) => countries?.countries.find((c) => c.code === code)?.name ?? code;
  const [adding, setAdding] = useState(false);
  const [browsing, setBrowsing] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<SavedSearch | null>(null);

  const onError = (err: unknown) => notifyError(err);

  const createSearch = useCreateSavedSearch({
    onSuccess: (search) => {
      setAdding(false);
      notify(`Saved search "${search.name}".`, "success");
    },
    onError,
  });
  const updateSearch = useUpdateSavedSearch({
    onSuccess: () => {
      setEditingId(null);
      notify("Search updated.", "success");
    },
    onError,
  });
  const deleteSearch = useDeleteSavedSearch({ onSuccess: () => setDeleting(null), onError });
  const setDefault = useSetDefaultSavedSearch({ onError });

  return (
    <>
      <SettingsHeading
        title="Job searches"
        description="JobPilot only looks for what you ask it to. Discovery and scouts run one of these searches — without any, nothing is imported."
        actions={
          !adding &&
          !browsing && (
            <>
              <Button size="sm" variant="outline" onClick={() => setBrowsing(true)}>
                Use a template
              </Button>
              <Button size="sm" onClick={() => setAdding(true)}>
                <Plus data-icon="inline-start" />
                Add search
              </Button>
            </>
          )
        }
      />

      {browsing && (
        <Panel className="mb-4 p-4">
          <SearchTemplatePicker
            onCancel={() => setBrowsing(false)}
            onError={onError}
            onImported={(created) => {
              setBrowsing(false);
              notify(
                created.length === 1 ? `Added "${created[0].name}".` : `Added ${created.length} searches.`,
                "success",
              );
            }}
          />
        </Panel>
      )}

      {adding && (
        <Panel className="mb-4 p-4">
          <SavedSearchForm
            submitLabel="Save search"
            pending={createSearch.isPending}
            onSubmit={(name, criteria) => createSearch.mutate({ name, criteria })}
            onCancel={() => setAdding(false)}
          />
        </Panel>
      )}

      {error ? (
        <ErrorLine error={error} />
      ) : isPending ? (
        <Loading />
      ) : searches.length === 0 ? (
        !adding &&
        !browsing && (
          <Panel className="px-4 py-6 text-center">
            <p className="text-sm text-muted-foreground">
              No searches yet. Start from a template, or add your own, to begin discovering jobs.
            </p>
          </Panel>
        )
      ) : (
        <Panel className="divide-y divide-border">
          {searches.map((search) =>
            editingId === search.id ? (
              <div key={search.id} className="p-4">
                <SavedSearchForm
                  initialName={search.name}
                  initialCriteria={{ ...emptyCriteria, ...search.criteria }}
                  submitLabel="Save changes"
                  pending={updateSearch.isPending}
                  onSubmit={(name, criteria) => updateSearch.mutate({ id: search.id, name, criteria })}
                  onCancel={() => setEditingId(null)}
                />
              </div>
            ) : (
              <div key={search.id} className="flex items-center gap-3 px-4 py-3">
                <div className="min-w-0 flex-1">
                  <p className="text-[13px] font-medium">
                    {search.name}
                    {search.is_default && (
                      <span className="ml-2 rounded bg-primary/12 px-1.5 py-0.5 text-[11px] font-medium text-primary">
                        Default
                      </span>
                    )}
                  </p>
                  <p className="mt-0.5 truncate text-xs text-muted-foreground">
                    {describeCriteria(search.criteria, countryName)}
                  </p>
                </div>
                <span
                  className="shrink-0 text-xs text-muted-foreground"
                  title={dateTime(search.last_run_at ?? undefined)}
                >
                  {search.last_run_at ? `Searched ${relativeTime(search.last_run_at)}` : "Never searched"}
                </span>
                <Button size="sm" variant="ghost" onClick={() => setEditingId(search.id)}>
                  Edit
                </Button>
                <DropdownMenu>
                  <DropdownMenuTrigger
                    render={<Button size="icon-sm" variant="ghost" aria-label={`More for ${search.name}`} />}
                  >
                    <Ellipsis />
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem disabled={search.is_default} onClick={() => setDefault.mutate(search.id)}>
                      Make default
                    </DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem variant="destructive" onClick={() => setDeleting(search)}>
                      Delete…
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            ),
          )}
        </Panel>
      )}

      <RefreshOnLaunchSetting />

      <CompanySettings />

      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(open) => !open && setDeleting(null)}
        title={`Delete “${deleting?.name}”?`}
        description="Jobs it found are kept. A scout that runs this search is deleted with it, since a scout without its search has nothing to look for."
        confirmLabel="Delete search"
        onConfirm={() => deleting && deleteSearch.mutate(deleting.id)}
        pending={deleteSearch.isPending}
      />
    </>
  );
}

/// describeCriteria renders only the criteria that are set, so a search reads
/// as what it actually narrows.
function describeCriteria(criteria: SearchCriteria, countryName: (code: string) => string): string {
  const parts: string[] = [];
  if (criteria.keywords?.trim()) parts.push(criteria.keywords.trim());
  if (criteria.title?.trim()) parts.push(`title: ${criteria.title.trim()}`);
  if (criteria.location?.trim()) parts.push(`in ${criteria.location.trim()}`);
  if (criteria.remote_preference?.trim()) parts.push(criteria.remote_preference.trim());
  if (criteria.employment_type?.trim()) parts.push(criteria.employment_type.trim());
  if (criteria.salary_min) parts.push(`≥ ${criteria.salary_min.toLocaleString()}`);
  if (criteria.salary_max) parts.push(`≤ ${criteria.salary_max.toLocaleString()}`);
  if (criteria.excluded_companies?.length) {
    parts.push(`excluding ${criteria.excluded_companies.join(", ")}`);
  }
  if (criteria.work_from) {
    parts.push(`from ${countryName(criteria.work_from)}${criteria.include_relocation ? ", or relocating" : ""}`);
  }
  if (criteria.fit_hours) {
    parts.push(criteria.hours_shift ? `hours I can do, moving up to ${criteria.hours_shift}h` : "hours I can do");
  }
  return parts.join(" · ");
}
