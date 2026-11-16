import { describe, expect, test } from 'bun:test';
import {
  toJsonSchema,
  type FetchInit,
  type FetchResponse,
  type LLMFetch,
} from '../llm-provider.ts';
import { matchExtractionSchema } from '../match.ts';
import { ollamaProvider } from './ollama.ts';
import { openAICompatProvider } from './openai-compat.ts';

// The cases are the Go tests' (adapters/secondary/llm/ollama_test.go, openai_compat_test.go).
interface Seen {
  url: string;
  init: FetchInit;
}
const reply = (status: number, body: string, contentType = 'application/json'): FetchResponse => ({
  ok: status >= 200 && status < 300,
  status,
  headers: { get: (n) => (n.toLowerCase() === 'content-type' ? contentType : null) },
  text: async () => body,
});
const fakeFetch =
  (res: FetchResponse | Error, seen: Seen[] = []): LLMFetch =>
  async (url, init = {}) => {
    seen.push({ url, init });
    if (res instanceof Error) throw res;
    return res;
  };
const sent = (seen: Seen[]) => JSON.parse(seen[0]!.init.body!);

describe('ollama', () => {
  const lines = (...o: object[]) => o.map((x) => JSON.stringify(x)).join('\n') + '\n';
  test('defaults the model and names itself', () =>
    expect(ollamaProvider(fakeFetch(reply(200, ''))).provider()).toBe('ollama'));
  test('connectivity: 200 healthy, non-200 an error', async () => {
    const seen: Seen[] = [];
    await ollamaProvider(fakeFetch(reply(200, '{}'), seen), 'http://h:1').testConnectivity();
    expect(seen[0]!.url).toBe('http://h:1/api/tags');
    await expect(ollamaProvider(fakeFetch(reply(500, ''))).testConnectivity()).rejects.toThrow(
      'non-OK status: 500',
    );
    await expect(
      ollamaProvider(fakeFetch(new Error('refused')), 'http://h:1').testConnectivity(),
    ).rejects.toThrow('failed to connect to Ollama at http://h:1');
  });
  test('sends the expected request and joins a streamed reply', async () => {
    const seen: Seen[] = [];
    const body = lines(
      { model: 'llama3', response: 'hel' },
      { model: 'llama3', response: 'lo' },
      { model: 'llama3', done: true },
    );
    const r = await ollamaProvider(
      fakeFetch(reply(200, body), seen),
      'http://h:1',
      'llama3',
    ).generate({ systemPrompt: 'system', userPrompt: 'user', temperature: 0.5, maxTokens: 100 });
    expect(r).toEqual({ content: 'hello', model: 'llama3' });
    expect(seen[0]!.url).toBe('http://h:1/api/generate');
    expect(sent(seen)).toEqual({
      model: 'llama3',
      prompt: 'user',
      system: 'system',
      stream: true,
      options: { temperature: 0.5, num_predict: 100 },
    });
  });
  test('sends the schema as format', async () => {
    const seen: Seen[] = [];
    await ollamaProvider(fakeFetch(reply(200, lines({ done: true })), seen)).generate({
      systemPrompt: '',
      userPrompt: '',
      responseSchema: matchExtractionSchema(),
    });
    const format = sent(seen).format;
    expect(format.additionalProperties).toBe(false);
    expect(format.required).toEqual([
      'concerns',
      'confidence',
      'gaps',
      'matched_skills',
      'missing_skills',
      'rationale',
      'score',
      'strengths',
    ]);
    expect(format.properties.strengths.items.type).toBe('string');
    expect(sent(seen).options).toBeUndefined();
  });
  test('errors', async () => {
    const gen = (res: FetchResponse) =>
      ollamaProvider(fakeFetch(res)).generate({ systemPrompt: '', userPrompt: '' });
    await expect(gen(reply(502, ''))).rejects.toThrow('non-OK status');
    await expect(gen(reply(200, 'not json'))).rejects.toThrow('failed to decode response');
    await expect(gen(reply(200, lines({ response: 'a' })))).rejects.toThrow(
      'ended before the model finished',
    );
    await expect(
      gen(reply(200, lines({ response: 'a' }, { error: 'out of memory' }))),
    ).rejects.toThrow('ollama failed while writing: out of memory');
  });
});

describe('openai-compatible', () => {
  const make = (
    res: FetchResponse | Error,
    over: Partial<Parameters<typeof openAICompatProvider>[1]> = {},
    seen: Seen[] = [],
  ) =>
    openAICompatProvider(fakeFetch(res, seen), {
      name: 'custom',
      baseUrl: 'http://h:1/v1/',
      model: 'gpt-5',
      ...over,
    });
  const completion = JSON.stringify({
    model: 'some-model',
    choices: [{ message: { role: 'assistant', content: 'hello' } }],
  });

  test('sends the expected request with a bearer key, and reads a one-piece reply', async () => {
    const seen: Seen[] = [];
    const r = await make(
      reply(200, completion),
      { key: { label: 'openai', secret: 'sk-test-123' } },
      seen,
    ).generate({ systemPrompt: 'system', userPrompt: 'user', temperature: 0.5 });
    expect(r).toEqual({ content: 'hello', model: 'some-model' });
    expect(seen[0]!.url).toBe('http://h:1/v1/chat/completions');
    expect(seen[0]!.init.headers!.Authorization).toBe('Bearer sk-test-123');
    expect(sent(seen)).toEqual({
      model: 'gpt-5',
      messages: [
        { role: 'system', content: 'system' },
        { role: 'user', content: 'user' },
      ],
      temperature: 0.5,
      stream: true,
    });
  });
  test('no key sends no Authorization header; the base URL keeps its version', async () => {
    const seen: Seen[] = [];
    await make(reply(200, '{}'), {}, seen).testConnectivity();
    expect(seen[0]!.url).toBe('http://h:1/v1/models');
    expect(seen[0]!.init.headers?.Authorization).toBeUndefined();
  });
  test('an empty response model falls back to the requested one', async () => {
    const r = await make(
      reply(200, JSON.stringify({ choices: [{ message: { content: 'hi' } }] })),
      { name: 'llama.cpp', model: 'requested-model' },
    ).generate({ systemPrompt: '', userPrompt: '' });
    expect(r.model).toBe('requested-model');
  });
  test('empty choices, non-200 and malformed bodies are errors', async () => {
    const gen = (res: FetchResponse) =>
      make(res, { name: 'llama.cpp' }).generate({ systemPrompt: '', userPrompt: '' });
    await expect(gen(reply(200, '{}'))).rejects.toThrow('no choices');
    await expect(gen(reply(502, ''))).rejects.toThrow('non-OK status');
    await expect(gen(reply(200, 'not json'))).rejects.toThrow('failed to decode response');
  });

  const statuses: [string, number, boolean, string][] = [
    ['a refused key is named by its label', 401, true, 'refused the API key "openai" (401)'],
    ['a forbidden key is named by its label', 403, true, 'refused the API key "openai" (403)'],
    ['a refusal without a key says one is needed', 401, false, 'it needs an API key'],
    ['nothing found names the URL and the model', 404, true, 'for the model "gpt-5" (404)'],
    ['rate limiting says so', 429, true, 'limiting requests'],
    ['anything else gives the status', 502, true, 'non-OK status: 502'],
  ];
  for (const [name, status, withKey, want] of statuses) {
    test(name, async () => {
      const secret = 'sk-proj-abcdefghijklmnop';
      const body = '{"error":{"message":"Incorrect API key provided: sk-proj-***mnop"}}';
      const p = make(reply(status, body), withKey ? { key: { label: 'openai', secret } } : {});
      for (const run of [
        () => p.testConnectivity(),
        () => p.generate({ systemPrompt: '', userPrompt: '' }),
      ]) {
        const err = await run().then(
          () => undefined,
          (e: Error) => e,
        );
        expect(err?.message).toContain(want);
        expect(err?.message).not.toContain('mnop');
        expect(err?.message).not.toContain(secret);
      }
    });
  }

  test('a stream error is repeated only when no key was sent', async () => {
    const sse = reply(
      200,
      'data: {"error":{"message":"key sk-***mnop has no credit"}}\n\n',
      'text/event-stream',
    );
    const withKey = make(sse, { key: { label: 'openai', secret: 'sk-abcdmnop' } });
    const err = await withKey.generate({ systemPrompt: '', userPrompt: '' }).catch((e: Error) => e);
    expect((err as Error).message).toContain('failed while writing');
    expect((err as Error).message).not.toContain('mnop');
    await expect(
      make(sse, { name: 'llama.cpp' }).generate({ systemPrompt: '', userPrompt: '' }),
    ).rejects.toThrow('llama.cpp failed while writing: key sk-***mnop has no credit');
  });
  test('joins an event stream and needs [DONE]', async () => {
    const chunk = (t: string) =>
      `data: ${JSON.stringify({ model: 'm', choices: [{ delta: { content: t } }] })}\n\n`;
    const r = await make(
      reply(
        200,
        chunk('he') + chunk('llo') + 'data: [DONE]\n\n',
        'text/event-stream; charset=utf-8',
      ),
    ).generate({ systemPrompt: '', userPrompt: '' });
    expect(r).toEqual({ content: 'hello', model: 'm' });
    await expect(
      make(reply(200, chunk('he'), 'text/event-stream')).generate({
        systemPrompt: '',
        userPrompt: '',
      }),
    ).rejects.toThrow('ended before the model finished');
  });
  test('names a custom endpoint by its host, a known one by its name', async () => {
    await expect(
      make(reply(401, ''), {
        baseUrl: 'http://127.0.0.1:9/v1',
        key: { label: 'work', secret: 'sk' },
      }).testConnectivity(),
    ).rejects.toThrow('127.0.0.1:9 refused the API key "work" (401)');
    await expect(make(reply(401, ''), { name: 'lmstudio' }).testConnectivity()).rejects.toThrow(
      'lmstudio refused the request (401)',
    );
  });
  test('a network failure does not repeat its message when a key was sent', async () => {
    const p = make(new Error('GET http://h Authorization: Bearer sk-secret'), {
      key: { label: 'k', secret: 'sk-secret' },
    });
    const err = await p.generate({ systemPrompt: '', userPrompt: '' }).catch((e: Error) => e);
    expect((err as Error).message).toBe('failed to call custom'.replace('custom', 'h:1'));
  });
  test('toJsonSchema marks nullable', () => {
    expect(toJsonSchema({ type: 'number', nullable: true })).toEqual({ type: ['number', 'null'] });
  });
});
