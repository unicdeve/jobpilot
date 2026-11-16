// A port of the Ollama provider (internal/adapters/secondary/llm/ollama.go), ADR-0037.
// The Go side reads the stream as it arrives, with stall timeouts; here the body is read whole
// (text()), then its lines joined, so a caller bounds the request with an AbortSignal instead.
import {
  errorMessage,
  isObject,
  toJsonSchema,
  type FetchInit,
  type LLMFetch,
  type LLMProvider,
  type LLMRequest,
  type LLMResponse,
} from '../llm-provider.ts';

export const OLLAMA_DEFAULT_MODEL = 'llama3';
export const OLLAMA_DEFAULT_BASE_URL = 'http://localhost:11434';

// Joins the lines of a streamed generation. It needs the line that says the model is done:
// a stream that ends without it was cut off.
export function readGenerateStream(body: string): LLMResponse {
  let content = '';
  let model = '';
  for (const raw of body.split('\n')) {
    const line = raw.trim();
    if (line === '') continue;
    let part: unknown;
    try {
      part = JSON.parse(line);
    } catch {
      throw new Error('failed to decode response: not JSON');
    }
    if (!isObject(part)) throw new Error('failed to decode response: not an object');
    if (typeof part.error === 'string' && part.error !== '')
      throw new Error(`ollama failed while writing: ${part.error}`);
    if (typeof part.response === 'string') content += part.response;
    if (typeof part.model === 'string' && part.model !== '') model = part.model;
    if (part.done === true) return { content, model };
  }
  throw new Error("ollama's response ended before the model finished");
}

export function ollamaProvider(
  doFetch: LLMFetch,
  baseUrl: string = OLLAMA_DEFAULT_BASE_URL,
  model: string = '',
): LLMProvider {
  const chosen = model === '' ? OLLAMA_DEFAULT_MODEL : model;
  const call = async (url: string, init: FetchInit) => {
    try {
      return await doFetch(url, init);
    } catch (e) {
      throw new Error(`failed to call Ollama: ${errorMessage(e)}`);
    }
  };
  return {
    provider: () => 'ollama',
    async testConnectivity(signal) {
      let response;
      try {
        response = await doFetch(`${baseUrl}/api/tags`, { method: 'GET', signal });
      } catch (e) {
        throw new Error(`failed to connect to Ollama at ${baseUrl}: ${errorMessage(e)}`);
      }
      if (!response.ok) throw new Error(`ollama returned non-OK status: ${response.status}`);
    },
    async generate(req: LLMRequest, signal) {
      const body: Record<string, unknown> = {
        model: chosen,
        prompt: req.userPrompt,
        system: req.systemPrompt,
        stream: true,
      };
      if (req.responseSchema) body.format = toJsonSchema(req.responseSchema);
      const temperature = req.temperature ?? 0;
      const maxTokens = req.maxTokens ?? 0;
      if (temperature !== 0 || maxTokens !== 0) {
        const options: Record<string, number> = {};
        if (temperature !== 0) options.temperature = temperature;
        if (maxTokens !== 0) options.num_predict = maxTokens;
        body.options = options;
      }
      const response = await call(`${baseUrl}/api/generate`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
        signal,
      });
      if (!response.ok) throw new Error(`ollama returned non-OK status: ${response.status}`);
      return readGenerateStream(await response.text());
    },
  };
}
