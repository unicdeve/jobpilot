// A port of the LLM provider port (internal/core/ports/llm.go and schema.go), ADR-0037.
// Which model backs it is not decided here: a provider is anything that satisfies LLMProvider.
// No key is stored in this layer; a caller passes the API key in, read from secure storage.

// The shape of a JSON response, for backends that can constrain a model to it (ADR-0017).
// Like the Go type it describes shape and never meaning: no enum, pattern, minimum or maximum.
export type SchemaType = 'object' | 'array' | 'string' | 'integer' | 'number' | 'boolean';

export interface Schema {
  type: SchemaType;
  description?: string;
  nullable?: boolean;
  properties?: Record<string, Schema>;
  items?: Schema;
}

export const objectSchema = (properties: Record<string, Schema>): Schema => ({
  type: 'object',
  properties,
});
export const stringSchema = (description = ''): Schema => ({ type: 'string', description });
export const integerSchema = (description = ''): Schema => ({ type: 'integer', description });
export const numberSchema = (description = ''): Schema => ({ type: 'number', description });
export const booleanSchema = (description = ''): Schema => ({ type: 'boolean', description });
export const arraySchema = (description: string, items: Schema): Schema => ({
  type: 'array',
  description,
  items,
});

// The schema as a JSON Schema document, with every property required and no extra ones, as Schema.MarshalJSON does.
export function toJsonSchema(schema: Schema): Record<string, unknown> {
  const doc: Record<string, unknown> = {
    type: schema.nullable ? [schema.type, 'null'] : schema.type,
  };
  if (schema.description) doc.description = schema.description;
  if (schema.type === 'object') {
    const names = Object.keys(schema.properties ?? {}).sort();
    const properties: Record<string, unknown> = {};
    for (const name of names) properties[name] = toJsonSchema(schema.properties![name]!);
    doc.properties = properties;
    doc.required = names;
    doc.additionalProperties = false;
  } else if (schema.type === 'array' && schema.items) {
    doc.items = toJsonSchema(schema.items);
  }
  return doc;
}

export interface LLMRequest {
  systemPrompt: string;
  userPrompt: string;
  temperature?: number;
  maxTokens?: number;
  // A request, not a guarantee: a backend that cannot apply it ignores it, so callers still validate the reply.
  responseSchema?: Schema;
}

export interface LLMResponse {
  content: string;
  model: string;
}

export interface LLMProvider {
  generate(req: LLMRequest, signal?: AbortSignal): Promise<LLMResponse>;
  testConnectivity(signal?: AbortSignal): Promise<void>;
  // A short identifier for the backend (e.g. "ollama"), recorded with the result.
  provider(): string;
}

// The slice of fetch the providers use; the app wires net.fetch (or global fetch) in.
export interface FetchInit {
  method?: string;
  headers?: Record<string, string>;
  body?: string;
  signal?: AbortSignal;
}
export interface FetchResponse {
  ok: boolean;
  status: number;
  headers?: { get(name: string): string | null };
  text(): Promise<string>;
}
export type LLMFetch = (url: string, init?: FetchInit) => Promise<FetchResponse>;

export const isObject = (v: unknown): v is Record<string, unknown> =>
  v !== null && typeof v === 'object' && !Array.isArray(v);
export const errorMessage = (e: unknown): string => (e instanceof Error ? e.message : String(e));
