import { FormField } from "./form-field";
import { Input } from "./ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "./ui/select";
import type { LLMSettings } from "../lib/types";

/// Mirrors llm.Presets in internal/adapters/secondary/llm/presets.go, which
/// the daemon and CLI use for the same choice.
export const presets = [
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
/// previous provider's port pointing at the new one.
function nextBaseUrl(current: string, preset?: (typeof presets)[number]): string {
  if (!preset?.defaultBaseUrl) return current;
  const untouched = !current || presets.some((p) => p.defaultBaseUrl === current);
  return untouched ? preset.defaultBaseUrl : current;
}

/// LLMProviderFields is the provider/base URL/model trio, shared by Settings
/// and the first-run wizard so the two cannot drift apart about what a
/// provider choice implies. Saving is the caller's job: the two screens
/// report the result differently.
export function LLMProviderFields({
  settings,
  onChange,
}: {
  settings: LLMSettings;
  onChange: (settings: LLMSettings) => void;
}) {
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
            onChange({
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
        <Input
          id="base_url"
          value={settings.base_url}
          onChange={(e) => onChange({ ...settings, base_url: e.target.value })}
        />
      </FormField>
      <FormField label="Model" htmlFor="model">
        <Input
          id="model"
          placeholder="e.g. llama3"
          value={settings.model}
          onChange={(e) => onChange({ ...settings, model: e.target.value })}
        />
      </FormField>
    </>
  );
}
