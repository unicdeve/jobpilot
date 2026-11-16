// A port of the Hacker News "Who is hiring?" source (hackernews.go), ADR-0037.
// The latest monthly thread is read through Algolia's HN API, and each top-level comment that follows the convention
// ("Company | Role | Location | ...") and carries a link is a job. The host keeps the answer for HN_CACHE_MS.
// A comment is untrusted text: it is read for its header fields and the first link, and kept as the description.
import { fields as splitFields } from '../search/signals.ts';
import {
  describe,
  getJson,
  isObject,
  list,
  normalizeEmploymentType,
  request,
  sourceJob,
  str,
  type DoFetch,
  type FullSourceJob,
} from './job-source.ts';
import { unescapeHtml } from '../search/signals.ts';

export const HN_URL = 'https://hn.algolia.com/api/v1';
export const HN_CACHE_MS = 60 * 60 * 1000;

export function isHiringThreadTitle(title: string): boolean {
  const lower = title.toLowerCase();
  if (!lower.includes('who is hiring')) return false;
  return !lower.includes('wants to be hired') && !lower.includes('freelancer'); // the two sibling threads are people seeking work
}

const urlPattern = /https?:\/\/[^\s<>"')\]]+/;
const urlPatternAll = /https?:\/\/[^\s<>"')\]]+/g;
const salaryPattern =
  /(\$|€|£|\b(USD|EUR|GBP)\b|\d+\s*k\b|\bsalary\b|\bcompensation\b|\bequity\b)/i;
const remoteWords = /^(remote|hybrid|onsite|on-site|in office|in-office|wfh)$/i;
const markdownLink = /\[([^\]]*)\]\([^)]*\)/g;
const emptyParens = /\(\s*\)|\[\s*\]/g;

// A comment's HTML as text: links keep their address as Markdown ([text](address)), paragraphs become lines.
export function commentText(html: string): string {
  const withLinks = html
    .replace(
      /<a\s[^>]*?href\s*=\s*"([^"]*)"[^>]*>([\s\S]*?)<\/a>/gi,
      (_, href: string, label: string) => `[${label.replace(/<[^>]*>/g, '')}](${href})`,
    )
    .replace(/<p\s*\/?>/gi, '\n\n');
  return unescapeHtml(describe(withLinks));
}

// Go's string length is bytes; the 60 limit is in them.
const utf8Length = (s: string): number => {
  let n = 0;
  for (const ch of s) {
    const c = ch.codePointAt(0)!;
    n += c < 0x80 ? 1 : c < 0x800 ? 2 : c < 0x10000 ? 3 : 4;
  }
  return n;
};

export const plainHeaderField = (field: string): string =>
  splitFields(
    field.replace(markdownLink, '$1').replace(urlPatternAll, '').replace(emptyParens, ''),
  ).join(' ');

export const splitHeaderFields = (header: string): string[] =>
  header.split('|').map(plainHeaderField);

export function remoteTypeFromFields(fields: string[]): string {
  const joined = fields.join(' | ').toLowerCase();
  if (joined.includes('hybrid')) return 'hybrid';
  if (joined.includes('remote')) return 'remote';
  if (['onsite', 'on-site', 'in office', 'in-office'].some((w) => joined.includes(w)))
    return 'onsite';
  return '';
}

export const employmentTypeFromFields = (fields: string[]): string => {
  for (const f of fields) {
    const n = normalizeEmploymentType(f);
    if (n !== '') return n;
  }
  return '';
};

// The first field after company and role that reads as a place: not a salary, a type, a bare arrangement, a link or a shouted note.
export function locationsFromFields(fields: string[]): string[] {
  for (const field of fields.slice(2)) {
    if (field === '' || utf8Length(field) > 60) continue;
    if (salaryPattern.test(field) || normalizeEmploymentType(field) !== '') continue;
    if (remoteWords.test(field)) continue;
    if (field.toLowerCase().includes('http')) continue;
    if (field === field.toUpperCase() && /[A-Z]/.test(field)) continue;
    return [field];
  }
  return [];
}

export function toJob(comment: unknown): FullSourceJob | undefined {
  if (!isObject(comment)) return undefined;
  const text = commentText(str(comment.text));
  if (text.trim() === '') return undefined;
  const header =
    text
      .split('\n')
      .map((l) => l.trim())
      .find((l) => l !== '') ?? '';
  const fields = splitHeaderFields(header);
  if (fields.length < 3) return undefined; // not the convention
  const applyUrl = urlPattern.exec(text)?.[0];
  if (applyUrl === undefined) return undefined; // nothing to apply to
  const [company, title] = [fields[0]!, fields[1]!];
  if (company === '' || title === '') return undefined;
  const id = String(comment.id ?? '');
  return sourceJob({
    sourceId: 'hackernews',
    sourceJobId: id,
    canonicalUrl: applyUrl.replace(/[.,);]+$/, ''),
    title,
    companyName: company,
    descriptionMarkdown: text,
    remoteType: remoteTypeFromFields(fields),
    employmentType: employmentTypeFromFields(fields),
    locations: locationsFromFields(fields),
    postedAt: str(comment.created_at),
  });
}

export async function fetchHackerNews(
  doFetch: DoFetch,
  baseUrl = HN_URL,
): Promise<FullSourceJob[]> {
  const found = await getJson(
    doFetch,
    `${baseUrl}/search_by_date?tags=story,author_whoishiring&hitsPerPage=10`,
    'hacker news',
    {},
  );
  if (!isObject(found)) throw new Error('failed to decode the Hacker News response');
  const hit = list(found.hits)
    .filter(isObject)
    .find((h) => isHiringThreadTitle(str(h.title)));
  if (hit === undefined) throw new Error("no 'Who is hiring?' thread found");
  const response = await request(
    doFetch,
    `${baseUrl}/items/${encodeURIComponent(str(hit.objectID))}`,
    'Hacker News',
    {},
  );
  if (!response.ok) throw new Error(`hacker news returned non-OK status: ${response.status}`);
  let thread: unknown;
  try {
    thread = await response.json();
  } catch {
    throw new Error('failed to decode the Hacker News thread');
  }
  if (!isObject(thread)) throw new Error('failed to decode the Hacker News thread');
  return list(thread.children)
    .map(toJob)
    .filter((j): j is FullSourceJob => j !== undefined);
}
