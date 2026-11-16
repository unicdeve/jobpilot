// What the ported job sources share (ADR-0037): the fetch the host injects, the job shape beyond remotive.ts's, and the
// errors a source raises. Every fetched value is untrusted data: it is read into strings and numbers and never run.
import type { SourceJob } from './remotive.ts';
import { stripHtml } from './remotive.ts';

export type { SourceJob };

// Sent where Go sends it: RemoteOK rejects a request without one, and the others ask who is calling.
export const userAgent = 'JobPilot (local job search assistant)';

export interface FetchResponse {
  ok: boolean;
  status: number;
  json(): Promise<unknown>;
  text(): Promise<string>;
}
export type DoFetch = (url: string, init?: { headers?: Record<string, string> }) => Promise<FetchResponse>;

// SourceJob (remotive.ts) with what the other sources add. Absent salary is 0 and '', never "unpaid" (Go's rule).
export interface FullSourceJob extends SourceJob {
  salaryCurrency: string;
  salaryPeriod: string;
  expiresAt: string;
  companyUrl: string;
}

export const sourceJob = (j: Partial<FullSourceJob> & Pick<FullSourceJob, 'sourceId' | 'sourceJobId' | 'canonicalUrl'>): FullSourceJob => ({
  title: '', companyName: '', descriptionMarkdown: '', locations: [], remoteType: '', employmentType: '', salaryMin: 0, salaryMax: 0,
  skills: [], postedAt: '', salaryCurrency: '', salaryPeriod: '', expiresAt: '', companyUrl: '', ...j,
});

// Go's ports.ErrSourceUnreachable: no answer at all (offline, no name, timeout). A status or a bad body is not it.
export class SourceUnreachableError extends Error {
  constructor(what: string, cause: unknown) {
    super(`failed to reach ${what}: ${cause instanceof Error ? cause.message : String(cause)}`);
    this.name = 'SourceUnreachableError';
  }
}
// ports.ErrBoardGone and ErrBoardBusy: a hiring page the provider says is not there, and one that asked to be left alone.
export class BoardGoneError extends Error { constructor() { super('no hiring page at this address'); this.name = 'BoardGoneError'; } }
export class BoardBusyError extends Error { constructor() { super('the hiring page asked JobPilot to wait before asking again'); this.name = 'BoardBusyError'; } }
// ports.PartialAnswer: a source made of parts that answered for some. `error` is unset when some part answered.
export class PartialAnswerError extends Error {
  constructor(public failing: string[], public error?: Error, public jobs: FullSourceJob[] = []) { super(error?.message ?? `${failing.length} parts did not answer`); this.name = 'PartialAnswerError'; }
}
export const isUnreachable = (e: unknown): boolean => e instanceof SourceUnreachableError;

export async function request(doFetch: DoFetch, url: string, what: string, headers: Record<string, string> = { 'User-Agent': userAgent }): Promise<FetchResponse> {
  try {
    return await doFetch(url, { headers });
  } catch (e) {
    throw new SourceUnreachableError(what, e);
  }
}

export async function getJson(doFetch: DoFetch, url: string, what: string, headers?: Record<string, string>): Promise<unknown> {
  const response = await request(doFetch, url, what, headers);
  if (!response.ok) throw new Error(`${what} returned non-OK status: ${response.status}`);
  try {
    return await response.json();
  } catch {
    throw new Error(`failed to decode the ${what} response`);
  }
}

export const isObject = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === 'object' && !Array.isArray(v);
export const str = (v: unknown): string => (typeof v === 'string' ? v : '');
export const num = (v: unknown): number => (typeof v === 'number' && Number.isFinite(v) ? v : 0);
export const strings = (v: unknown): string[] => (Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : []);
export const list = (v: unknown): unknown[] => (Array.isArray(v) ? v : []);
export const trimmed = (v: string): string => v.trim();
export const trimAll = (values: string[]): string[] => values.map((v) => v.trim()).filter((v) => v !== '');
export const splitLocations = (value: string, separator: string): string[] => trimAll(value.split(separator));
export const describe = (html: string): string => stripHtml(html);

// Go's time.Unix(n, 0) as the ISO string a SourceJob carries; '' for 0.
export const fromUnix = (seconds: number): string => (seconds > 0 ? new Date(seconds * 1000).toISOString() : '');
export const fromUnixMilli = (ms: number): string => (ms > 0 ? new Date(ms).toISOString() : '');

// Go's parseTime: RFC 3339 (with an offset) or a bare date, else ''. The first value that reads wins.
export function parseTime(...values: string[]): string {
  for (const v of values) {
    if (/^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?(Z|[+-]\d\d:\d\d)$/.test(v)) {
      const t = Date.parse(v);
      if (!Number.isNaN(t)) return new Date(t).toISOString();
    }
    if (/^\d{4}-\d\d-\d\d$/.test(v)) return `${v}T00:00:00.000Z`;
  }
  return '';
}

// Go's normalizeEmploymentType: the vocabulary's word for what a source wrote, or '' for one nothing can compare.
export function normalizeEmploymentType(value: string): string {
  const simplify = (s: string) => s.replace(/[ _-]/g, '');
  const simplified = simplify(value.trim().toLowerCase());
  for (const known of ['full-time', 'part-time', 'contract', 'internship', 'temporary']) if (simplify(known) === simplified) return known;
  switch (simplified) {
    case 'fulltimeemployee': case 'permanent': return 'full-time';
    case 'intern': return 'internship';
    case 'freelance': case 'contractor': return 'contract';
  }
  return '';
}

// A URL query as Go's url.Values.Encode writes it: keys sorted, values form-encoded.
export function encodeQuery(query: Record<string, string>): string {
  const enc = (s: string) => encodeURIComponent(s).replace(/%20/g, '+').replace(/[!'()*]/g, (c) => `%${c.charCodeAt(0).toString(16).toUpperCase()}`);
  return Object.keys(query).sort().map((k) => `${enc(k)}=${enc(query[k]!)}`).join('&');
}
