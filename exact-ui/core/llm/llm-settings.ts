// The user's LLM backend settings (ADR-0037), ported from internal/core/domain/llm_settings.go.
// The API key is never part of these: `apiKeyLabel` only names a credential held in secure storage (ADR-0030 section 1).
export const llmCredentialSource = 'llm';
export const llmProviderOllama = 'ollama';

export interface LLMSettings { provider: string; baseUrl: string; model: string; apiKeyLabel?: string }
export interface DiscoverySettings { refreshOnLaunch: boolean }
export const defaultDiscoverySettings = (): DiscoverySettings => ({ refreshOnLaunch: true });

// Hermes' URL is incomplete, so the address is split by hand.
const parse = (raw: string): { scheme: string; host: string; hostname: string } | null => {
  const m = /^([a-zA-Z][a-zA-Z0-9+.-]*):\/\/([^/?#\s]*)/.exec(raw);
  if (!m || m[2] === '' || /\s/.test(raw)) return null;
  const authority = m[2].slice(m[2].lastIndexOf('@') + 1);
  const hostname = authority.startsWith('[') ? authority.slice(0, authority.indexOf(']') + 1) : authority.replace(/:\d*$/, '');
  if (hostname === '') return null;
  return { scheme: m[1].toLowerCase(), host: authority, hostname };
};

export function isLoopbackHost(host: string): boolean {
  const h = host.toLowerCase().replace(/\.$/, '');
  if (h === 'localhost' || h.endsWith('.localhost')) return true;
  const bare = h.replace(/^\[|\]$/g, '');
  if (bare === '::1') return true;
  const m = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(bare);
  return !!m && m.slice(1).every((p) => Number(p) <= 255) && Number(m[1]) === 127;
}

export const apiKeyRef = (s: LLMSettings): { source: string; label: string } | null =>
  s.apiKeyLabel ? { source: llmCredentialSource, label: s.apiKeyLabel } : null;

// The message when the settings cannot be saved, else null.
export function validateLLMSettings(s: LLMSettings): string | null {
  if (s.provider === '') return 'provider is required';
  if (s.model === '') return 'model is required';
  const u = parse(s.baseUrl);
  if (!u) return `base URL must be a valid absolute http(s) URL, got "${s.baseUrl}"`;
  if (u.scheme !== 'http' && u.scheme !== 'https') return `base URL must use http or https, got "${s.baseUrl}"`;
  if (!s.apiKeyLabel) return null;
  if (s.apiKeyLabel.trim() === '') return 'API key: label is required';
  if (s.provider === llmProviderOllama) return 'an API key is for an OpenAI-compatible endpoint; Ollama keeps its own sign-in and takes none';
  if (u.scheme === 'http' && !isLoopbackHost(u.hostname))
    return `an API key is sent only over https, or to this computer; ${u.host} would send it unencrypted`;
  return null;
}

// The host the model is reached at when that is not this computer, else ''.
export function hostedAt(s: LLMSettings): string {
  const u = parse(s.baseUrl);
  return !u || isLoopbackHost(u.hostname) ? '' : u.hostname;
}

export const hostedModelNotice = (host: string): string =>
  `Your profile, resume and the job postings JobPilot reads are sent to ${host} whenever it reads, scores or writes, including when a scout runs on its own.`;
