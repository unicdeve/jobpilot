// A port of the We Work Remotely source (weworkremotely.go), ADR-0037.
// WWR's terms: a job keeps its We Work Remotely URL. Five category feeds overlap, so a posting in several is one job.
// The host keeps the answer for WWR_CACHE_MS. The RSS is read with no XML library: a feed is a list of <item> blocks of plain fields.
import { describe, isUnreachable, normalizeEmploymentType, request, sourceJob, type DoFetch, type FullSourceJob } from './job-source.ts';
import { unescapeHtml } from '../search/signals.ts';

export const WWR_URL = 'https://weworkremotely.com/categories';
export const WWR_CACHE_MS = 60 * 60 * 1000;
export const WWR_CATEGORIES = [
  'remote-programming-jobs', 'remote-full-stack-programming-jobs', 'remote-back-end-programming-jobs',
  'remote-front-end-programming-jobs', 'remote-devops-sysadmin-jobs',
];

export interface WwrItem {
  title: string; region: string; country: string; skills: string; type: string; description: string;
  pubDate: string; expiresAt: string; guid: string; link: string;
}

const text = (block: string, tag: string): string => {
  const m = new RegExp(`<${tag}(?:\\s[^>]*)?>([\\s\\S]*?)</${tag}>`).exec(block);
  if (m === null) return '';
  const raw = m[1]!;
  const cdata = /^\s*<!\[CDATA\[([\s\S]*?)\]\]>\s*$/.exec(raw);
  return cdata !== null ? cdata[1]! : unescapeHtml(raw);
};

export function parseFeed(xml: string): WwrItem[] {
  if (!/<rss[\s>]/.test(xml) || !/<channel[\s>]/.test(xml)) throw new Error('not an RSS feed');
  const items: WwrItem[] = [];
  for (const m of xml.matchAll(/<item(?:\s[^>]*)?>([\s\S]*?)<\/item>/g)) {
    const b = m[1]!;
    items.push({
      title: text(b, 'title'), region: text(b, 'region'), country: text(b, 'country'), skills: text(b, 'skills'), type: text(b, 'type'),
      description: text(b, 'description'), pubDate: text(b, 'pubDate'), expiresAt: text(b, 'expires_at'), guid: text(b, 'guid'), link: text(b, 'link'),
    });
  }
  return items;
}

const months = ['jan', 'feb', 'mar', 'apr', 'may', 'jun', 'jul', 'aug', 'sep', 'oct', 'nov', 'dec'];
// Go's time.Parse(time.RFC1123Z, ...): "Thu, 17 Sep 2026 10:51:23 +0000", else ''.
export function parseRfc1123z(value: string): string {
  const m = /^[A-Za-z]{3}, (\d{1,2}) ([A-Za-z]{3}) (\d{4}) (\d\d):(\d\d):(\d\d) ([+-])(\d\d)(\d\d)$/.exec(value.trim());
  if (m === null) return '';
  const month = months.indexOf(m[2]!.toLowerCase());
  if (month < 0) return '';
  const offset = (m[7] === '-' ? -1 : 1) * (Number(m[8]) * 60 + Number(m[9]));
  const t = Date.UTC(Number(m[3]), month, Number(m[1]), Number(m[4]), Number(m[5]), Number(m[6])) - offset * 60000;
  return Number.isNaN(t) ? '' : new Date(t).toISOString();
}

const skillSeparators = /\s*,\s*(?:and\s+)?|\s+and\s+/;

export function toJob(item: WwrItem): FullSourceJob {
  const cut = item.title.indexOf(': ');
  const company = cut >= 0 ? item.title.slice(0, cut) : '';
  const title = cut >= 0 ? item.title.slice(cut + 2) : item.title;
  // The employer's countries, when given, are narrower than the board's region, and truer.
  const location = item.country.trim() !== '' ? item.country.trim() : item.region.trim();
  return sourceJob({
    sourceId: 'weworkremotely', sourceJobId: item.guid, canonicalUrl: item.link, title: title.trim(), companyName: company.trim(),
    descriptionMarkdown: describe(item.description), locations: location === '' ? [] : [location], remoteType: 'remote',
    employmentType: normalizeEmploymentType(item.type), skills: item.skills.trim().split(skillSeparators).map((s) => s.trim()).filter((s) => s !== ''),
    postedAt: parseRfc1123z(item.pubDate), expiresAt: parseRfc1123z(item.expiresAt),
  });
}

async function fetchFeed(doFetch: DoFetch, baseUrl: string, category: string): Promise<WwrItem[]> {
  const response = await request(doFetch, `${baseUrl.replace(/\/$/, '')}/${category}.rss`, 'We Work Remotely');
  if (!response.ok) throw new Error(`the We Work Remotely ${category} feed returned non-OK status: ${response.status}`);
  try {
    return parseFeed(await response.text());
  } catch {
    throw new Error(`failed to decode We Work Remotely's ${category} feed`);
  }
}

// Go's noFeedAnswered: undefined when any feed answered, else the first feed's own failure; one that was reached outranks unreachable.
export function noFeedAnswered(errors: (Error | undefined)[]): Error | undefined {
  let failed: Error | undefined;
  for (const e of errors) {
    if (e === undefined) return undefined;
    if (!isUnreachable(e) && failed === undefined) failed = e;
  }
  return failed ?? errors[0];
}

export async function fetchWeWorkRemotely(doFetch: DoFetch, baseUrl = WWR_URL): Promise<FullSourceJob[]> {
  const feeds: WwrItem[][] = [];
  const errors: (Error | undefined)[] = [];
  await Promise.all(WWR_CATEGORIES.map(async (category, i) => {
    try { feeds[i] = await fetchFeed(doFetch, baseUrl, category); errors[i] = undefined; } catch (e) { feeds[i] = []; errors[i] = e instanceof Error ? e : new Error(String(e)); }
  }));
  const failure = noFeedAnswered(errors);
  if (failure !== undefined) throw failure;
  const seen = new Set<string>();
  const jobs: FullSourceJob[] = [];
  for (const items of feeds) for (const item of items) {
    if (item.link === '' || item.title === '' || seen.has(item.link)) continue;
    seen.add(item.link);
    jobs.push(toJob(item));
  }
  return jobs;
}
