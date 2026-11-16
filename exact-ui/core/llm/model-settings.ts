// The model settings form and the one secret that holds every API key (ADR-0038 section 2, ADR-0030). Pure.
// A key is never part of the settings in SQLite: the settings hold a label, and the key lives in the secret `llm-keys`,
// a JSON object from label to key. Nothing here reads a key back out for display.
import { hostedAt, hostedModelNotice, validateLLMSettings, type LLMSettings } from './llm-settings.ts';

export const llmKeysSecret = 'llm-keys';

export const providerChoices: { code: string; label: string }[] = [
  { code: 'ollama', label: 'Ollama' },
  { code: 'openai', label: 'OpenAI-compatible' },
];

export interface ModelForm { provider: string; baseUrl: string; model: string; keyLabel: string; apiKey: string }

export function parseKeys(raw: string | null): Record<string, string> {
  if (raw === null || raw === '') return {};
  try {
    const v: unknown = JSON.parse(raw);
    if (v === null || typeof v !== 'object' || Array.isArray(v)) return {};
    return Object.fromEntries(Object.entries(v).filter((e): e is [string, string] => typeof e[1] === 'string'));
  } catch { return {}; }
}
export const hasKey = (raw: string | null, label: string): boolean => label !== '' && (parseKeys(raw)[label] ?? '') !== '';
export const withKey = (raw: string | null, label: string, key: string): string => JSON.stringify({ ...parseKeys(raw), [label]: key });
export const withoutKey = (raw: string | null, label: string): string => {
  const { [label]: _gone, ...rest } = parseKeys(raw);
  return JSON.stringify(rest);
};

export type Checked = { ok: true; settings: LLMSettings; newKey: string } | { ok: false; error: string };

// The deterministic checks before anything is kept: ADR-0030's (no key over plain http to another host, none for Ollama) and
// that a typed key has a label to be kept under.
export function checkModelForm(f: ModelForm): Checked {
  const label = f.keyLabel.trim();
  const key = f.apiKey.trim();
  if (key !== '' && label === '') return { ok: false, error: 'Name the key (for example "openrouter") so it can be kept under that name.' };
  const settings: LLMSettings = { provider: f.provider.trim(), baseUrl: f.baseUrl.trim(), model: f.model.trim(), ...(label === '' ? {} : { apiKeyLabel: label }) };
  const error = validateLLMSettings(settings);
  if (error !== null) return { ok: false, error: `${error.charAt(0).toUpperCase()}${error.slice(1)}${/[.!?]$/.test(error) ? '' : '.'}` };
  return { ok: true, settings, newKey: key };
}

// Where the user's data goes with these settings, or '' when it stays on this computer.
export const dataNotice = (f: Pick<ModelForm, 'baseUrl'>): string => {
  const host = hostedAt({ provider: '', model: '', baseUrl: f.baseUrl.trim() });
  return host === '' ? '' : hostedModelNotice(host);
};
