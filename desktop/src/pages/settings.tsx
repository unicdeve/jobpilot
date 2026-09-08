import { useEffect, useState } from "react";
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
import { useApi } from "../lib/daemon";
import { useToast } from "../lib/toast";
import type { LLMSettings } from "../lib/types";

const presets = [
  { key: "ollama", label: "Ollama", defaultBaseUrl: "http://localhost:11434" },
  { key: "llama.cpp", label: "llama.cpp (llama-server)", defaultBaseUrl: "http://localhost:8080" },
  { key: "lmstudio", label: "LM Studio", defaultBaseUrl: "http://localhost:1234" },
  { key: "custom", label: "Custom (OpenAI-compatible endpoint)", defaultBaseUrl: "" },
];

export function Settings() {
  const api = useApi();
  const { notify } = useToast();
  const [settings, setSettings] = useState<LLMSettings | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    api.getLLMSettings().then(setSettings).catch((err) => notify(String(err)));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api]);

  if (!settings) return <p className="text-sm text-slate-500">Loading…</p>;

  const set = <K extends keyof LLMSettings>(key: K, value: LLMSettings[K]) =>
    setSettings((s) => (s ? { ...s, [key]: value } : s));

  const save = async () => {
    setSaving(true);
    try {
      const result = await api.saveLLMSettings(settings);
      setSettings(result.settings);
      if (result.connectivity_error) {
        notify(`Settings saved, but could not connect: ${result.connectivity_error}`);
      } else {
        notify("Settings saved and verified.", "success");
      }
    } catch (err) {
      notify(String(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="max-w-lg">
      <h1 className="mb-4 text-xl font-semibold">Settings</h1>
      <Card>
        <CardContent className="space-y-4">
          <FormField label="LLM provider" htmlFor="provider">
            <Select
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
          <Button onClick={save} disabled={saving}>
            {saving ? "Saving…" : "Save"}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}
