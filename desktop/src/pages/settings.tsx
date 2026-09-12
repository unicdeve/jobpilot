import { useState } from "react";
import { FormField } from "../components/form-field";
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
import { useLLMSettings, useSaveLLMSettings } from "../lib/queries";
import { useToast } from "../lib/toast";
import type { LLMSettings } from "../lib/types";

const presets = [
  { key: "ollama", label: "Ollama", defaultBaseUrl: "http://localhost:11434" },
  { key: "llama.cpp", label: "llama.cpp (llama-server)", defaultBaseUrl: "http://localhost:8080" },
  { key: "lmstudio", label: "LM Studio", defaultBaseUrl: "http://localhost:1234" },
  { key: "custom", label: "Custom (OpenAI-compatible endpoint)", defaultBaseUrl: "" },
];

export function Settings() {
  const { notify } = useToast();
  const { data: saved, error } = useLLMSettings();

  // The form is an editable draft over the saved settings: draft holds the
  // user's unsaved edits and falls back to what the daemon last returned.
  const [draft, setDraft] = useState<LLMSettings | null>(null);
  const settings = draft ?? saved;

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
    <div className="max-w-lg">
      <h1 className="mb-4 text-xl font-semibold">Settings</h1>
      <Card>
        <CardContent className="space-y-4">
          <FormField label="LLM provider" htmlFor="provider">
            <Select
              items={Object.fromEntries(presets.map((p) => [p.key, p.label]))}
              value={settings.provider || "ollama"}
              onValueChange={(value) => {
                if (value === null) return;
                const preset = presets.find((p) => p.key === value);
                set("provider", value);
                if (preset && !settings.base_url) set("base_url", preset.defaultBaseUrl);
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
    </div>
  );
}
