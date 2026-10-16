import { Globe } from "lucide-react";
import { useRef, useState } from "react";
import { useCredentials, useSetCredential } from "../lib/queries/credential.queries";
import type { LLMSettings } from "../lib/types";
import { FormField } from "./form-field";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./ui/select";

/// Mirrors llm.Presets in internal/adapters/secondary/llm/presets.go, which
/// the daemon and CLI use for the same choice. An OpenAI-compatible base URL
/// includes its version, as providers publish it (ADR-0030 §4).
const presets = [
  { key: "ollama", label: "Ollama", defaultBaseUrl: "http://localhost:11434" },
  { key: "llama.cpp", label: "llama.cpp (llama-server)", defaultBaseUrl: "http://localhost:8080/v1" },
  { key: "lmstudio", label: "LM Studio", defaultBaseUrl: "http://localhost:1234/v1" },
  { key: "custom", label: "Custom (OpenAI-compatible endpoint)", defaultBaseUrl: "" },
];

const defaultProvider = "ollama";

/// The source the model's API keys are stored under, mirroring
/// domain.LLMCredentialSource (ADR-0030 §1).
export const llmCredentialSource = "llm";

/// withProviderDefaults shows the provider and base URL the daemon would use
/// for settings that have never been saved, so a fresh install reads
/// "Ollama / http://localhost:11434" rather than "Ollama" beside a blank URL
/// the user has to know the port for. The model is deliberately left empty:
/// which model is installed locally is not something we can guess.
export function withProviderDefaults(settings: LLMSettings): LLMSettings {
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
/// previous provider's port pointing at the new one — and for Custom, which
/// has no default, cleared, so the field shows what a URL looks like.
function nextBaseUrl(current: string, preset?: (typeof presets)[number]): string {
  const untouched = !current || presets.some((p) => p.defaultBaseUrl && p.defaultBaseUrl === current);
  return untouched ? (preset?.defaultBaseUrl ?? "") : current;
}

/// hostedAt is the host a base URL reaches when that is not this computer,
/// or "" — mirroring domain.LLMSettings.HostedAt, so the notice shows for
/// exactly the URLs the daemon counts as hosted.
function hostedAt(baseUrl: string): string {
  let host: string;
  try {
    host = new URL(baseUrl).hostname.toLowerCase().replace(/\.$/, "");
  } catch {
    return "";
  }
  const bare = host.replace(/^\[|\]$/g, "");
  const loopback =
    host === "" ||
    host === "localhost" ||
    host.endsWith(".localhost") ||
    /^127\.\d{1,3}\.\d{1,3}\.\d{1,3}$/.test(bare) ||
    bare === "::1";
  return loopback ? "" : host;
}

const NO_KEY = "__none__";
const ADD_KEY = "__add__";

/// LLMProviderFields is the provider/base URL/model trio, and for a custom
/// endpoint the API key it is sent, shared by Settings and the first-run
/// wizard so the two cannot drift apart about what a provider choice
/// implies. Saving is the caller's job: the two screens report the result
/// differently.
export function LLMProviderFields({
  settings,
  onChange,
}: {
  settings: LLMSettings;
  onChange: (settings: LLMSettings) => void;
}) {
  const custom = settings.provider === "custom";
  const host = hostedAt(settings.base_url);

  return (
    <>
      <FormField label="LLM provider" htmlFor="provider">
        <Select
          items={Object.fromEntries(presets.map((p) => [p.key, p.label]))}
          value={settings.provider}
          onValueChange={(value) => {
            if (value === null) return;
            const preset = presets.find((p) => p.key === value);
            // One update, not two: a second call derived from the same
            // `settings` snapshot would discard the provider just chosen.
            // Only a custom endpoint is sent a key, so leaving one drops it.
            onChange({
              ...settings,
              provider: value,
              base_url: nextBaseUrl(settings.base_url, preset),
              api_key_label: value === "custom" ? settings.api_key_label : undefined,
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
        <Input
          id="base_url"
          placeholder={custom ? "https://api.openai.com/v1" : undefined}
          spellCheck={false}
          value={settings.base_url}
          onChange={(e) => onChange({ ...settings, base_url: e.target.value })}
        />
        {/* The field's own help first, under the input; then where the
            data goes, which is about the choice rather than the spelling. */}
        {custom && (
          <p className="mt-1.5 text-xs text-muted-foreground">
            As your provider gives it, including the version — for example https://openrouter.ai/api/v1.
          </p>
        )}
        {host && (
          // ADR-0030 §6: a statement beside the field, for as long as the
          // setting stands, rather than a confirmation clicked through once.
          <p data-slot="hosted-notice" className="mt-2 flex gap-2 text-xs text-muted-foreground">
            <Globe aria-hidden className="mt-px size-3.5 shrink-0" />
            <span>
              Your profile, resume and the job postings JobPilot reads are sent to{" "}
              <span className="font-medium text-foreground">{host}</span> whenever it reads, scores or writes, including
              when a scout runs on its own.
            </span>
          </p>
        )}
      </FormField>
      <FormField label="Model" htmlFor="model">
        <Input
          id="model"
          placeholder={custom ? "e.g. gpt-5-mini" : "e.g. llama3"}
          spellCheck={false}
          value={settings.model}
          onChange={(e) => onChange({ ...settings, model: e.target.value })}
        />
      </FormField>
      {custom && <APIKeyField settings={settings} onChange={onChange} />}
    </>
  );
}

/// APIKeyField chooses the key a custom endpoint is sent: none, one already
/// stored for the model, or a new one stored here (ADR-0030 §7). Only the
/// model's own keys are offered, never a job source's (§1).
///
/// It is rendered inside the caller's form, so adding a key is a group of
/// fields with its own button rather than a nested form, and Enter in the
/// key field stores the key instead of submitting the settings.
function APIKeyField({ settings, onChange }: { settings: LLMSettings; onChange: (settings: LLMSettings) => void }) {
  const { data: credentials } = useCredentials();
  const [adding, setAdding] = useState(false);
  const [label, setLabel] = useState("");
  const [secret, setSecret] = useState("");

  // The settings as last rendered: a key finishes storing after the user may
  // have edited another field, and selecting it must not undo that edit.
  const latest = useRef(settings);
  latest.current = settings;

  const store = useSetCredential({
    onSuccess: (ref) => {
      setAdding(false);
      setLabel("");
      setSecret("");
      onChange({ ...latest.current, api_key_label: ref.label });
    },
  });

  const labels = (credentials ?? []).filter((ref) => ref.source === llmCredentialSource).map((ref) => ref.label);
  const selected = settings.api_key_label ?? "";
  // A key the settings name and the keychain no longer lists is shown as
  // such, rather than silently reading as "No key".
  const missing = selected !== "" && credentials !== undefined && !labels.includes(selected);

  const items: Record<string, string> = { [NO_KEY]: "No key" };
  for (const known of labels) items[known] = known;
  if (missing) items[selected] = `${selected} (not stored)`;
  items[ADD_KEY] = "Add a key…";

  const usable = label.trim() !== "" && secret.trim() !== "";
  const submit = () => {
    if (usable && !store.isPending) {
      store.mutate({ source: llmCredentialSource, label: label.trim(), secret });
    }
  };

  return (
    <FormField
      label="API key"
      htmlFor="api_key"
      // While a key is being added, the panel says all there is to say.
      hint={
        adding ? undefined : missing ? (
          <span className="text-warning">This key is no longer in your keychain. Add it again, or choose another.</span>
        ) : (
          "Kept in your operating system’s keychain, never in JobPilot’s database."
        )
      }
    >
      <Select
        items={items}
        value={adding ? ADD_KEY : selected || NO_KEY}
        onValueChange={(value) => {
          if (value === null) return;
          if (value === ADD_KEY) {
            setAdding(true);
            return;
          }
          setAdding(false);
          onChange({ ...settings, api_key_label: value === NO_KEY ? undefined : value });
        }}
      >
        <SelectTrigger id="api_key" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {Object.entries(items).map(([value, text]) => (
            <SelectItem key={value} value={value}>
              {text}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {adding && (
        <div data-slot="add-key" className="mt-3 space-y-3 rounded-lg border border-border p-3">
          <div className="grid grid-cols-[minmax(0,2fr)_minmax(0,3fr)] gap-3">
            <FormField label="Name" htmlFor="api_key_label">
              <Input
                id="api_key_label"
                placeholder="openrouter"
                spellCheck={false}
                autoComplete="off"
                value={label}
                onChange={(e) => setLabel(e.target.value)}
              />
            </FormField>
            <FormField label="Key" htmlFor="api_key_secret">
              <Input
                id="api_key_secret"
                type="password"
                autoComplete="off"
                spellCheck={false}
                value={secret}
                onChange={(e) => setSecret(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    submit();
                  }
                }}
              />
            </FormField>
          </div>
          <p className="text-xs text-muted-foreground">
            You will not be able to view this key again. Lower-case letters, digits and hyphens for the name.
          </p>
          {store.error && <p className="text-xs text-destructive">{String(store.error.message)}</p>}
          <div className="flex items-center gap-2">
            <Button type="button" size="sm" disabled={!usable || store.isPending} onClick={submit}>
              {store.isPending ? "Storing…" : "Store in keychain"}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => {
                setAdding(false);
                store.reset();
              }}
            >
              Cancel
            </Button>
          </div>
        </div>
      )}
    </FormField>
  );
}
