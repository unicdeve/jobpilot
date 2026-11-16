// A port of the OpenAI-compatible provider (internal/adapters/secondary/llm/openai_compat.go, presets.go), ADR-0037, ADR-0030.
// Covers OpenAI and any server speaking its chat completions API. The key is passed in by the caller;
// nothing here stores one, and no part of a provider's reply is ever repeated in an error beside a key.
// Like the Go side, no response_format is sent: the schema is ignored and callers validate every reply.
import {
  errorMessage,
  isObject,
  type FetchInit,
  type LLMFetch,
  type LLMProvider,
  type LLMRequest,
  type LLMResponse,
} from '../llm-provider.ts';

export const PROVIDER_OLLAMA = 'ollama';
export const PROVIDER_LLAMA_CPP = 'llama.cpp';
export const PROVIDER_LM_STUDIO = 'lmstudio';
export const PROVIDER_CUSTOM = 'custom';

// The known local backends offered when configuring, as presets.go lists them. Custom has no default URL.
export const PRESETS: { key: string; label: string; defaultBaseUrl: string }[] = [
  { key: PROVIDER_OLLAMA, label: 'Ollama', defaultBaseUrl: 'http://localhost:11434' },
  {
    key: PROVIDER_LLAMA_CPP,
    label: 'llama.cpp (llama-server)',
    defaultBaseUrl: 'http://localhost:8080/v1',
  },
  { key: PROVIDER_LM_STUDIO, label: 'LM Studio', defaultBaseUrl: 'http://localhost:1234/v1' },
  { key: PROVIDER_CUSTOM, label: 'Custom (OpenAI-compatible endpoint)', defaultBaseUrl: '' },
];

// The key sent with every request, and the label it is stored under, which is what an error names.
export interface ApiKey {
  label: string;
  secret: string;
}

export interface OpenAICompatOptions {
  name: string;
  // As the provider publishes it, with its version: https://api.openai.com/v1.
  baseUrl: string;
  model: string;
  key?: ApiKey;
}

export function openAICompatProvider(doFetch: LLMFetch, options: OpenAICompatOptions): LLMProvider {
  const { name, model } = options;
  const baseUrl = options.baseUrl.replace(/\/+$/, '');
  const key = options.key ?? { label: '', secret: '' };
  let display = name;
  if (name === PROVIDER_CUSTOM) {
    const host = /^[a-z][a-z0-9+.-]*:\/\/([^/?#]+)/i.exec(baseUrl)?.[1];
    if (host) display = host;
  }

  const headers = (extra: Record<string, string> = {}): Record<string, string> =>
    key.secret === '' ? extra : { ...extra, Authorization: `Bearer ${key.secret}` };

  // What a refusal means, from its status alone: the body is never read into it, as it can quote the key.
  const statusError = (status: number): Error => {
    switch (status) {
      case 401:
      case 403:
        return new Error(
          key.label === ''
            ? `${display} refused the request (${status}): it needs an API key`
            : `${display} refused the API key "${key.label}" (${status})`,
        );
      case 404:
        return new Error(
          `${display} found nothing at ${baseUrl} for the model "${model}" (404): check the base URL and the model's name`,
        );
      case 429:
        return new Error(`${display} is limiting requests, or the account's quota is spent (429)`);
      default:
        return new Error(`${display} returned non-OK status: ${status}`);
    }
  };

  // The message is deliberately not repeated when a key was sent: a fetch failure message can carry the URL or headers.
  const call = async (url: string, init: FetchInit, verb: string) => {
    try {
      return await doFetch(url, init);
    } catch (e) {
      const detail = key.secret === '' ? `: ${errorMessage(e)}` : '';
      throw new Error(`${verb}${detail}`);
    }
  };

  const decode = (text: string): unknown => {
    try {
      return JSON.parse(text);
    } catch {
      throw new Error('failed to decode response: not JSON');
    }
  };

  // Joins the deltas of a streamed completion; it needs the closing [DONE].
  const readEventStream = (body: string): LLMResponse => {
    let content = '';
    let model = '';
    for (const raw of body.split('\n')) {
      const line = raw.trim();
      if (!line.startsWith('data:')) continue; // a blank line between events, or a comment
      const data = line.slice('data:'.length).trim();
      if (data === '[DONE]') return { content, model };
      const chunk = decode(data);
      if (!isObject(chunk)) throw new Error('failed to decode response: not an object');
      if (chunk.error !== undefined && chunk.error !== null) {
        // Kept only when no key was sent, so there is none it could quote (ADR-0030 section 5).
        if (key.secret !== '') throw new Error(`${display} failed while writing`);
        const message =
          isObject(chunk.error) && typeof chunk.error.message === 'string'
            ? chunk.error.message
            : '';
        throw new Error(`${display} failed while writing: ${message}`);
      }
      if (typeof chunk.model === 'string' && chunk.model !== '') model = chunk.model;
      if (Array.isArray(chunk.choices)) {
        for (const choice of chunk.choices) {
          const delta =
            isObject(choice) && isObject(choice.delta) ? choice.delta.content : undefined;
          if (typeof delta === 'string') content += delta;
        }
      }
    }
    throw new Error(`${display}'s response ended before the model finished`);
  };

  // Reads a completion sent in one piece.
  const readCompletion = (body: string): LLMResponse => {
    const parsed = decode(body);
    if (!isObject(parsed)) throw new Error('failed to decode response: not an object');
    if (!Array.isArray(parsed.choices) || parsed.choices.length === 0)
      throw new Error(`${display} returned no choices`);
    const first = parsed.choices[0];
    const message = isObject(first) && isObject(first.message) ? first.message.content : undefined;
    return {
      content: typeof message === 'string' ? message : '',
      model: typeof parsed.model === 'string' ? parsed.model : '',
    };
  };

  return {
    provider: () => name,
    async testConnectivity(signal) {
      const response = await call(
        `${baseUrl}/models`,
        { method: 'GET', headers: headers(), signal },
        `failed to connect to ${display} at ${baseUrl}`,
      );
      if (!response.ok) throw statusError(response.status);
    },
    async generate(req: LLMRequest, signal) {
      const body: Record<string, unknown> = {
        model,
        messages: [
          { role: 'system', content: req.systemPrompt },
          { role: 'user', content: req.userPrompt },
        ],
        stream: true,
      };
      if (req.temperature) body.temperature = req.temperature;
      if (req.maxTokens) body.max_tokens = req.maxTokens;
      const response = await call(
        `${baseUrl}/chat/completions`,
        {
          method: 'POST',
          headers: headers({ 'Content-Type': 'application/json' }),
          body: JSON.stringify(body),
          signal,
        },
        `failed to call ${display}`,
      );
      if (!response.ok) throw statusError(response.status);
      const text = await response.text();
      const mediaType = (response.headers?.get('Content-Type') ?? '')
        .split(';')[0]!
        .trim()
        .toLowerCase();
      const result =
        mediaType === 'text/event-stream' ? readEventStream(text) : readCompletion(text);
      return { content: result.content, model: result.model === '' ? model : result.model };
    },
  };
}
