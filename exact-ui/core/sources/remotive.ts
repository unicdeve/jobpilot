// A port of the Remotive job source (internal/adapters/secondary/discovery/remotive.go), ADR-0037.
// Remotive acts on none of its query parameters, so the whole listing is fetched and matches() filters it.
import type { Job } from '../search/criteria.ts';

export const REMOTIVE_URL = 'https://remotive.com/api/remote-jobs';

export interface SourceJob extends Job {
  sourceId: string;
  sourceJobId: string;
  canonicalUrl: string;
  skills: string[];
  postedAt: string;
}

type Fetch = (url: string) => Promise<{ ok: boolean; status: number; json(): Promise<unknown> }>;
const isObject = (v: unknown): v is Record<string, unknown> =>
  v !== null && typeof v === 'object' && !Array.isArray(v);
const str = (v: unknown): string => (typeof v === 'string' ? v : '');

// The Go side converts the HTML to Markdown; this keeps the text only, until a converter is chosen (ADR-0037).
export const stripHtml = (html: string): string =>
  html
    .replace(/<(br|\/p|\/li|\/h\d)\s*\/?>/gi, '\n')
    .replace(/<[^>]*>/g, '')
    .replace(/&nbsp;/g, ' ')
    .replace(/&amp;/g, '&')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/\n{3,}/g, '\n\n')
    .trim();

export function toJob(item: unknown): SourceJob | undefined {
  if (!isObject(item) || item.id === undefined) return undefined;
  const where = str(item.candidate_required_location);
  const posted = str(item.publication_date); // "2026-09-07T01:10:43", no offset: Remotive's are UTC
  return {
    sourceId: 'remotive',
    sourceJobId: String(item.id),
    canonicalUrl: str(item.url),
    title: str(item.title),
    companyName: str(item.company_name),
    descriptionMarkdown: stripHtml(str(item.description)),
    locations: where === '' ? [] : [where],
    remoteType: 'remote',
    employmentType: str(item.job_type),
    salaryMin: 0,
    salaryMax: 0,
    skills: Array.isArray(item.tags)
      ? item.tags.filter((t): t is string => typeof t === 'string')
      : [],
    postedAt: posted === '' ? '' : `${posted}Z`,
  };
}

export async function fetchRemotive(doFetch: Fetch, url = REMOTIVE_URL): Promise<SourceJob[]> {
  const response = await doFetch(url);
  if (!response.ok) throw new Error(`remotive returned non-OK status: ${response.status}`);
  const body = await response.json();
  if (!isObject(body) || !Array.isArray(body.jobs))
    throw new Error('failed to decode remotive response');
  return body.jobs.map(toJob).filter((j): j is SourceJob => j !== undefined);
}
