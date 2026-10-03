import { cn } from "cn";
import { Bell, Database, Ellipsis, KeyRound, Monitor, Moon, Palette, Plus, Search, Sparkles, Sun } from "lucide-react";
import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { CompanySettings } from "../components/company-settings";
import { ConfirmDialog } from "../components/confirm";
import { CredentialSettings } from "../components/credential-settings";
import { DataSettings } from "../components/data-settings";
import { LLMProviderFields, withProviderDefaults } from "../components/llm-provider-fields";
import { Page, Panel } from "../components/page";
import { SavedSearchForm, emptyCriteria } from "../components/saved-search-form";
import { SearchTemplatePicker } from "../components/search-template-picker";
import { SettingsHeading } from "../components/settings-heading";
import { ErrorLine, Loading } from "../components/status-line";
import { Button } from "../components/ui/button";
import { Checkbox } from "../components/ui/checkbox";
import { Input } from "../components/ui/input";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../components/ui/dropdown-menu";
import {
  useCreateSavedSearch,
  useDeleteSavedSearch,
  useDiscoverySettings,
  useLLMSettings,
  useNotificationSettings,
  useSaveNotificationSettings,
  useSaveDiscoverySettings,
  useSaveLLMSettings,
  useSavedSearches,
  useCountries,
  useSetDefaultSavedSearch,
  useUpdateSavedSearch,
} from "../lib/queries";
import { dateTime, relativeTime } from "../lib/format";
import { useTheme, type ThemePreference } from "../lib/theme";
import { useToast } from "../lib/toast";
import type { LLMSettings, SavedSearch, SearchCriteria } from "../lib/types";

const SECTIONS = [
  { key: "appearance", label: "Appearance", icon: Palette },
  { key: "model", label: "AI model", icon: Sparkles },
  { key: "searches", label: "Job searches", icon: Search },
  { key: "notifications", label: "Notifications", icon: Bell },
  { key: "keys", label: "API keys", icon: KeyRound },
  { key: "data", label: "Data", icon: Database },
] as const;

type SectionKey = (typeof SECTIONS)[number]["key"];

/// Settings is one category at a time, chosen from a list beside it, as
/// System Settings is. The category is in the URL so another screen can
/// link straight to it — Jobs' "Manage searches…" does.
export function Settings() {
  const [params, setParams] = useSearchParams();
  const requested = params.get("section");
  const section: SectionKey = SECTIONS.some((s) => s.key === requested) ? (requested as SectionKey) : "appearance";

  return (
    <Page title="Settings" width="full" className="px-0 pt-0">
      <div className="flex min-h-[calc(100vh-3.25rem)]">
        <nav className="w-52 shrink-0 border-r border-border px-2.5 py-4">
          {SECTIONS.map(({ key, label, icon: Icon }) => (
            <button
              key={key}
              type="button"
              onClick={() => setParams({ section: key }, { replace: true })}
              className={cn(
                "flex h-7 w-full items-center gap-2.5 rounded-md px-2.5 text-left text-[13px] outline-none focus-visible:ring-2 focus-visible:ring-ring",
                section === key ? "bg-accent font-medium" : "text-subtle-foreground hover:bg-accent/60",
              )}
            >
              <Icon className={cn("size-4", section === key ? "text-primary" : "text-muted-foreground")} strokeWidth={1.75} />
              {label}
            </button>
          ))}
        </nav>
        <div className="min-w-0 flex-1 px-8 py-6">
          <div className="max-w-2xl">
            {section === "appearance" && <AppearanceSettings />}
            {section === "model" && <ModelSettings />}
            {section === "searches" && <JobSearchSettings />}
            {section === "notifications" && <NotificationSettingsSection />}
            {section === "keys" && <CredentialSettings />}
            {section === "data" && <DataSettings />}
          </div>
        </div>
      </div>
    </Page>
  );
}


const THEMES: { value: ThemePreference; label: string; icon: typeof Sun }[] = [
  { value: "system", label: "System", icon: Monitor },
  { value: "light", label: "Light", icon: Sun },
  { value: "dark", label: "Dark", icon: Moon },
];

/// AppearanceSettings is the one setting kept in the desktop rather than the
/// daemon: how this window looks is a preference of this device, and the CLI
/// has no use for it.
function AppearanceSettings() {
  const { preference, setPreference } = useTheme();
  return (
    <>
      <SettingsHeading title="Appearance" description="System follows your computer’s light or dark setting as it changes." />
      <div role="radiogroup" aria-label="Theme" className="grid grid-cols-3 gap-3">
        {THEMES.map(({ value, label, icon: Icon }) => {
          const selected = preference === value;
          return (
            <button
              key={value}
              type="button"
              role="radio"
              aria-checked={selected}
              onClick={() => setPreference(value)}
              className={cn(
                "flex flex-col items-center gap-2 rounded-lg border bg-card p-3 text-[13px] outline-none focus-visible:ring-2 focus-visible:ring-ring",
                selected ? "border-primary ring-1 ring-primary" : "border-border hover:bg-accent/40",
              )}
            >
              <ThemePreview theme={value} />
              <span className="flex items-center gap-1.5">
                <Icon className="size-3.5 text-muted-foreground" />
                {label}
              </span>
            </button>
          );
        })}
      </div>
    </>
  );
}

/// ThemePreview is a miniature window in the theme it names. It is drawn
/// with fixed colours on purpose: it shows the other theme, not this one.
function ThemePreview({ theme }: { theme: ThemePreference }) {
  const half = (dark: boolean) => (
    <div className={cn("flex h-full flex-1", dark ? "bg-[#1f2024]" : "bg-[#fcfcfd]")}>
      <div className={cn("w-1/3 border-r", dark ? "border-white/10 bg-[#26272b]" : "border-black/8 bg-[#f1f2f4]")} />
      <div className="flex flex-1 flex-col gap-1 p-1.5">
        <div className={cn("h-1.5 w-3/4 rounded-sm", dark ? "bg-white/25" : "bg-black/20")} />
        <div className={cn("h-1.5 w-1/2 rounded-sm", dark ? "bg-white/12" : "bg-black/10")} />
        <div className="mt-auto h-2 w-1/3 rounded-sm bg-[#3b82f6]" />
      </div>
    </div>
  );
  return (
    <div className="flex h-16 w-full overflow-hidden rounded-md border border-border">
      {theme === "system" ? (
        <>
          {half(false)}
          {half(true)}
        </>
      ) : (
        half(theme === "dark")
      )}
    </div>
  );
}

/// NotificationSettingsSection is whether JobPilot tells the user what a
/// scout found while they were elsewhere, and what counts as a strong
/// match (ADR-0028 §6). They are the daemon's settings, so the CLI shares
/// them.
function NotificationSettingsSection() {
  const { notify } = useToast();
  const { data: settings, error } = useNotificationSettings();
  const save = useSaveNotificationSettings({ onError: (err) => notify(String(err)) });
  // The score is edited here and saved when the field is left, so typing
  // "85" does not save an 8 on the way.
  const [score, setScore] = useState("");
  useEffect(() => {
    if (settings) setScore(String(settings.strong_match_score));
  }, [settings]);

  if (error) return <ErrorLine error={error} />;
  if (!settings) return <Loading />;

  const typed = Number(score);
  const scoreValid = Number.isInteger(typed) && typed >= 1 && typed <= 100;
  const saveScore = () => {
    if (scoreValid && typed !== settings.strong_match_score) save.mutate({ ...settings, strong_match_score: typed });
    if (!scoreValid) setScore(String(settings.strong_match_score));
  };

  return (
    <>
      <SettingsHeading
        title="Notifications"
        description="What a scout finds while you’re elsewhere. A notification says how many jobs and which scout, never the jobs themselves, since it can show on a locked screen."
      />
      <Panel className="divide-y divide-border">
        <label className="flex items-start gap-3 px-4 py-3">
          <Checkbox
            className="mt-0.5"
            checked={settings.enabled}
            disabled={save.isPending}
            onCheckedChange={(checked) => save.mutate({ ...settings, enabled: checked === true })}
          />
          <span>
            <span className="block text-[13px] font-medium">Notify me about scout runs</span>
            <span className="mt-0.5 block text-xs text-muted-foreground">
              When a scheduled scout finds jobs or can’t run, and when any run finds a strong match. Turned off,
              they still wait for you on the Dashboard.
            </span>
          </span>
        </label>
        <div className="flex items-center gap-4 px-4 py-3">
          <label htmlFor="strong-match-score" className="min-w-0 flex-1">
            <span className="block text-[13px] font-medium">A strong match scores at least</span>
            <span className="mt-0.5 block text-xs text-muted-foreground">Out of 100, as a job’s match is scored.</span>
          </label>
          <Input
            id="strong-match-score"
            type="number"
            min={1}
            max={100}
            className="w-20"
            value={score}
            aria-invalid={!scoreValid}
            onChange={(e) => setScore(e.target.value)}
            onBlur={saveScore}
            onKeyDown={(e) => e.key === "Enter" && e.currentTarget.blur()}
          />
        </div>
      </Panel>
    </>
  );
}

function ModelSettings() {
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

  return (
    <>
      <SettingsHeading
        title="AI model"
        description="The model JobPilot uses to read postings, score matches and write drafts, reached at the address below."
      />
      {error ? (
        <ErrorLine error={error} />
      ) : !settings ? (
        <Loading />
      ) : (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            saveSettings.mutate(settings);
          }}
        >
          <Panel className="flex flex-col gap-4 p-4">
            <LLMProviderFields settings={settings} onChange={setDraft} />
          </Panel>
          <div className="mt-4 flex items-center justify-end gap-2">
            {draft && (
              <Button type="button" variant="ghost" onClick={() => setDraft(null)}>
                Revert
              </Button>
            )}
            <Button type="submit" disabled={!draft || saveSettings.isPending}>
              {saveSettings.isPending ? "Checking…" : "Save and check"}
            </Button>
          </div>
        </form>
      )}
    </>
  );
}

/// RefreshOnLaunchSetting turns the launch refresh on and off
/// (docs/02_features/04_job_discovery.md AC-009). It is the daemon's
/// setting, so it holds for however the desktop is installed.
function RefreshOnLaunchSetting() {
  const { notify } = useToast();
  const { data: settings } = useDiscoverySettings();
  const save = useSaveDiscoverySettings({ onError: (err) => notify(String(err)) });

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
function JobSearchSettings() {
  const { notify } = useToast();
  const { data: searches, error, isPending } = useSavedSearches();
  const { data: countries } = useCountries();
  const countryName = (code: string) => countries?.countries.find((c) => c.code === code)?.name ?? code;
  const [adding, setAdding] = useState(false);
  const [browsing, setBrowsing] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<SavedSearch | null>(null);

  const onError = (err: unknown) => notify(String(err));

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
              notify(created.length === 1 ? `Added "${created[0].name}".` : `Added ${created.length} searches.`, "success");
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
                  <p className="mt-0.5 truncate text-xs text-muted-foreground">{describeCriteria(search.criteria, countryName)}</p>
                </div>
                <span className="shrink-0 text-xs text-muted-foreground" title={dateTime(search.last_run_at ?? undefined)}>
                  {search.last_run_at ? `Searched ${relativeTime(search.last_run_at)}` : "Never searched"}
                </span>
                <Button size="sm" variant="ghost" onClick={() => setEditingId(search.id)}>
                  Edit
                </Button>
                <DropdownMenu>
                  <DropdownMenuTrigger render={<Button size="icon-sm" variant="ghost" aria-label={`More for ${search.name}`} />}>
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
