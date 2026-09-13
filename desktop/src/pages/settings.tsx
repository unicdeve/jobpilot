import { useState } from "react";
import { FormField } from "../components/form-field";
import { SavedSearchForm, emptyCriteria } from "../components/saved-search-form";
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

/// Mirrors llm.Presets in internal/adapters/secondary/llm/presets.go, which
/// the daemon and CLI use for the same choice.
const presets = [
  { key: "ollama", label: "Ollama", defaultBaseUrl: "http://localhost:11434" },
  { key: "llama.cpp", label: "llama.cpp (llama-server)", defaultBaseUrl: "http://localhost:8080" },
  { key: "lmstudio", label: "LM Studio", defaultBaseUrl: "http://localhost:1234" },
  { key: "custom", label: "Custom (OpenAI-compatible endpoint)", defaultBaseUrl: "" },
];

const defaultProvider = "ollama";

/// withProviderDefaults shows the provider and base URL the daemon would use
/// for settings that have never been saved, so a fresh install reads
/// "Ollama / http://localhost:11434" rather than "Ollama" beside a blank URL
/// the user has to know the port for. The model is deliberately left empty:
/// which model is installed locally is not something we can guess.
function withProviderDefaults(settings: LLMSettings): LLMSettings {
  const provider = settings.provider || defaultProvider;
  const preset = presets.find((p) => p.key === provider);
  return {
    ...settings,
    provider,
    base_url: settings.base_url || preset?.defaultBaseUrl || "",
  };
}

/// nextBaseUrl picks the base URL to show after switching provider. A URL
/// the user typed is theirs and is kept; one that is still some preset's
/// default was filled in here, so it is replaced rather than leaving the
/// previous provider's port pointing at the new one.
function nextBaseUrl(current: string, preset?: (typeof presets)[number]): string {
  if (!preset?.defaultBaseUrl) return current;
  const untouched = !current || presets.some((p) => p.defaultBaseUrl === current);
  return untouched ? preset.defaultBaseUrl : current;
}

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

  const set = <K extends keyof LLMSettings>(key: K, value: LLMSettings[K]) =>
    setDraft({ ...settings, [key]: value });

  const save = () => saveSettings.mutate(settings);

  return (
    <div className="max-w-2xl space-y-6">
      <h1 className="text-xl font-semibold">Settings</h1>
      <Card>
        <CardContent className="space-y-4">
          <FormField label="LLM provider" htmlFor="provider">
            <Select
              items={Object.fromEntries(presets.map((p) => [p.key, p.label]))}
              value={settings.provider}
              onValueChange={(value) => {
                if (value === null) return;
                const preset = presets.find((p) => p.key === value);
                // One setDraft, not two: each `set` derives from the same
                // `settings` snapshot, so a second call would discard the
                // provider the first one just chose.
                setDraft({
                  ...settings,
                  provider: value,
                  base_url: nextBaseUrl(settings.base_url, preset),
                });
              }}
            >
              <SelectTrigger id="provider" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {presets.map((p) => (
                  <SelectItem key={p.key} value={p.key}>
                    {p.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </FormField>
          <FormField label="Base URL" htmlFor="base_url">
            <Input id="base_url" value={settings.base_url} onChange={(e) => set("base_url", e.target.value)} />
          </FormField>
          <FormField label="Model" htmlFor="model">
            <Input id="model" value={settings.model} onChange={(e) => set("model", e.target.value)} />
          </FormField>
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
