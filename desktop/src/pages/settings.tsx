import { useState } from "react";
import { LLMProviderFields, withProviderDefaults } from "../components/llm-provider-fields";
import { SavedSearchForm, emptyCriteria } from "../components/saved-search-form";
import { Button } from "../components/ui/button";
import { Card, CardContent } from "../components/ui/card";
import {
  useCreateSavedSearch,
  useDeleteSavedSearch,
  useLLMSettings,
  useSaveLLMSettings,
  useSavedSearches,
  useSetDefaultSavedSearch,
  useUpdateSavedSearch,
} from "../lib/queries";
import { useToast } from "../lib/toast";
import type { LLMSettings, SearchCriteria } from "../lib/types";

export function Settings() {
  const { notify } = useToast();
  const { data: saved, error } = useLLMSettings();

  // The form is an editable draft over the saved settings: draft holds the
  // user's unsaved edits and falls back to what the daemon last returned.
  const [draft, setDraft] = useState<LLMSettings | null>(null);
  const settings = draft ?? (saved && withProviderDefaults(saved));

  const saveSettings = useSaveLLMSettings({
    onSuccess: (result) => {
      // The mutation writes result.settings into the cache, so dropping the
      // draft shows exactly what was persisted.
      setDraft(null);
      if (result.connectivity_error) {
        notify(`Settings saved, but could not connect: ${result.connectivity_error}`);
      } else {
        notify("Settings saved and verified.", "success");
      }
    },
    onError: (err) => notify(String(err)),
  });

  if (error) return <p className="text-sm text-red-600">{String(error.message)}</p>;
  if (!settings) return <p className="text-sm text-slate-500">Loading…</p>;

  const save = () => saveSettings.mutate(settings);

  return (
    <div className="max-w-2xl space-y-6">
      <h1 className="text-xl font-semibold">Settings</h1>
      <Card>
        <CardContent className="space-y-4">
          <LLMProviderFields settings={settings} onChange={setDraft} />
          <Button onClick={save} disabled={saveSettings.isPending}>
            {saveSettings.isPending ? "Saving…" : "Save"}
          </Button>
        </CardContent>
      </Card>

      <JobSearchSettings />
    </div>
  );
}

/// JobSearchSettings manages the saved searches discovery runs against.
/// Without at least one, discovery refuses to run — the public job boards do
/// no filtering of their own, so a search with nothing set would import every
/// posting on every board rather than the jobs the user wants.
function JobSearchSettings() {
  const { notify } = useToast();
  const { data: searches, error, isPending } = useSavedSearches();
  const [adding, setAdding] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);

  const onError = (err: Error) => notify(String(err));

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
  const deleteSearch = useDeleteSavedSearch({ onError });
  const setDefault = useSetDefaultSavedSearch({ onError });

  return (
    <Card>
      <CardContent className="space-y-3">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h2 className="font-semibold">Job searches</h2>
            <p className="mt-1 text-sm text-slate-500">
              JobPilot only looks for what you ask it to. Discovery runs against one of these
              searches — without any, it will not import jobs.
            </p>
          </div>
          {!adding && <Button onClick={() => setAdding(true)}>Add search</Button>}
        </div>

        {adding && (
          <div className="rounded-md border border-slate-200 p-3">
            <SavedSearchForm
              submitLabel="Save search"
              pending={createSearch.isPending}
              onSubmit={(name, criteria) => createSearch.mutate({ name, criteria })}
              onCancel={() => setAdding(false)}
            />
          </div>
        )}

        {error ? (
          <p className="text-sm text-red-600">{String(error.message)}</p>
        ) : isPending ? (
          <p className="text-sm text-slate-500">Loading…</p>
        ) : searches.length === 0 ? (
          !adding && (
            <p className="text-sm text-slate-500">
              No searches yet — add one to start discovering jobs.
            </p>
          )
        ) : (
          <div className="space-y-2">
            {searches.map((search) =>
              editingId === search.id ? (
                <div key={search.id} className="rounded-md border border-slate-200 p-3">
                  <SavedSearchForm
                    initialName={search.name}
                    initialCriteria={{ ...emptyCriteria, ...search.criteria }}
                    submitLabel="Save changes"
                    pending={updateSearch.isPending}
                    onSubmit={(name, criteria) =>
                      updateSearch.mutate({ id: search.id, name, criteria })
                    }
                    onCancel={() => setEditingId(null)}
                  />
                </div>
              ) : (
                <div
                  key={search.id}
                  className="flex items-start justify-between gap-3 rounded-md border border-slate-200 p-3"
                >
                  <div>
                    <p className="font-medium">
                      {search.name}
                      {search.is_default && (
                        <span className="ml-2 text-xs font-normal text-slate-500">default</span>
                      )}
                    </p>
                    <p className="mt-1 text-xs text-slate-500">{describeCriteria(search.criteria)}</p>
                  </div>
                  <div className="flex shrink-0 items-center gap-1">
                    {!search.is_default && (
                      <Button variant="ghost" onClick={() => setDefault.mutate(search.id)}>
                        Make default
                      </Button>
                    )}
                    <Button variant="ghost" onClick={() => setEditingId(search.id)}>
                      Edit
                    </Button>
                    <Button variant="ghost" onClick={() => deleteSearch.mutate(search.id)}>
                      Delete
                    </Button>
                  </div>
                </div>
              ),
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

/// describeCriteria renders only the criteria that are set, so a search reads
/// as what it actually narrows.
function describeCriteria(criteria: SearchCriteria): string {
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
  return parts.join(" · ");
}
