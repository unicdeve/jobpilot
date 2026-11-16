// A port of the company hiring-page readers (company_boards*.go, ADR-0024), ADR-0037: Greenhouse, Lever, Ashby, Workable
// and SmartRecruiters. Each is a public JSON API; a board's postings become jobs of source 'company-pages'.
// BoardName and ResolveAddress are ported too: the injected fetch may take `redirect: 'manual'` and return `headers`, which is how
// a redirect is seen. A host that cannot (the web: its fetch answers an opaque redirect) fails ResolveAddress with RedirectUnreadableError.
// The host keeps a board's answer for COMPANY_BOARD_TTL_MS and must not cache a failure. Workable limits how often it is asked.
import { BoardBusyError, BoardGoneError, describe, fromUnixMilli, isObject, list, normalizeEmploymentType, num, parseTime, request, sourceJob, str, trimAll, splitLocations, SourceUnreachableError, userAgent, type DoFetch, type FetchResponse, type FullSourceJob } from './job-source.ts';
import { unescapeHtml } from '../search/signals.ts';

export const COMPANY_BOARD_TTL_MS = 60 * 60 * 1000;
export const MAX_BOARD_BODY = 64 << 20;
export const COMPANY_PAGES_SOURCE_ID = 'company-pages';
export const SMARTRECRUITERS_PAGE_SIZE = 100;
export const POSTINGS_AT_ONCE = 4;

export type Provider = 'greenhouse' | 'lever' | 'ashby' | 'workable' | 'smartrecruiters';
export interface Board { provider: Provider; region?: 'eu' | ''; name: string }

export interface BoardEndpoints {
  greenhouseApi: string; greenhouseEuApi: string; leverApi: string; leverEuApi: string; ashbyApi: string; workableApi: string; smartRecruitersApi: string;
}
export const defaultBoardEndpoints: BoardEndpoints = {
  greenhouseApi: 'https://boards-api.greenhouse.io', greenhouseEuApi: 'https://boards-api.eu.greenhouse.io', leverApi: 'https://api.lever.co',
  leverEuApi: 'https://api.eu.lever.co', ashbyApi: 'https://api.ashbyhq.com', workableApi: 'https://apply.workable.com',
  smartRecruitersApi: 'https://api.smartrecruiters.com',
  leverPages: 'https://jobs.lever.co', leverEuPages: 'https://jobs.eu.lever.co', ashbyPages: 'https://jobs.ashbyhq.com', smartRecruitersPages: 'https://careers.smartrecruiters.com',
};

// A fetch that can also be told not to follow redirects, and whose answer may carry headers (`type: 'opaqueredirect'` on the web).
export type BoardFetch = (url: string, init?: { headers?: Record<string, string>; redirect?: 'manual' | 'follow' }) => Promise<FetchResponse & { headers?: { get(name: string): string | null }; type?: string }>;

// The host's fetch cannot show where a redirect leads (a browser answers an opaque redirect with no headers).
export class RedirectUnreadableError extends Error { constructor() { super('this device cannot see where an address redirects to'); this.name = 'RedirectUnreadableError'; } }

export interface BoardContents {
  jobs: FullSourceJob[];
  // The company's name as the provider gives it, when the read does (Workable, SmartRecruiters); '' otherwise.
  name: string;
  // SmartRecruiters' list carries no description: describeJobs reads each posting's.
  undescribed: boolean;
}

const pathEscape = (s: string): string => encodeURIComponent(s).replace(/[!'()*]/g, (c) => `%${c.charCodeAt(0).toString(16).toUpperCase()}`);
// Go's domain.CleanCompanyName: one line, trimmed, at most 100 characters.
export function cleanCompanyName(name: string): string {
  const one = name.split(/\s+/).filter((w) => w !== '').join(' ');
  const chars = [...one];
  return chars.length > 100 ? chars.slice(0, 100).join('').trim() : one;
}

// Go's Board.URL: the board's hiring page.
export function boardUrl(b: Board): string {
  const name = pathEscape(b.name);
  const eu = b.region === 'eu' ? 'eu.' : '';
  switch (b.provider) {
    case 'greenhouse': return `https://job-boards.${eu}greenhouse.io/${name}`;
    case 'lever': return `https://jobs.${eu}lever.co/${name}`;
    case 'ashby': return `https://jobs.ashbyhq.com/${name}`;
    case 'workable': return `https://apply.workable.com/${name}/`;
    case 'smartrecruiters': return `https://careers.smartrecruiters.com/${name}`;
  }
}

async function getBoardText(doFetch: BoardFetch, url: string): Promise<string> {
  const response = await request(doFetch, url, 'the hiring page');
  if (response.status === 404) throw new BoardGoneError();
  if (response.status === 429) throw new BoardBusyError();
  if (!response.ok) throw new Error(`the hiring page answered with status ${response.status}`);
  return readText(response);
}

async function readText(response: FetchResponse): Promise<string> {
  let body: string;
  try {
    body = await response.text();
  } catch {
    throw new Error('failed to read the hiring page');
  }
  if (body.length > MAX_BOARD_BODY) throw new Error("the hiring page's answer is larger than JobPilot reads");
  return body;
}

// Go's getOnce: one request that does not follow a redirect. Returns the page, or where a redirect leads ('' for a page).
async function getOnce(doFetch: BoardFetch, url: string): Promise<{ page: string; location: string }> {
  let response: Awaited<ReturnType<BoardFetch>>;
  try {
    response = await doFetch(url, { headers: { 'User-Agent': userAgent }, redirect: 'manual' });
  } catch (e) {
    throw new SourceUnreachableError('the hiring page', e);
  }
  const status = response.status;
  if (response.type === 'opaqueredirect' || (status === 0 && response.type !== undefined)) throw new RedirectUnreadableError();
  if (status >= 300 && status < 400) {
    const location = response.headers?.get('location') ?? '';
    if (response.headers === undefined) throw new RedirectUnreadableError();
    if (location === '') throw new Error('the hiring page redirected without saying where');
    return { page: '', location };
  }
  if (status === 404) throw new BoardGoneError();
  if (status === 429) throw new BoardBusyError();
  if (!response.ok) throw new Error(`the hiring page answered with status ${status}`);
  return { page: await readText(response), location: '' };
}

// Go's ResolveAddress: follows one redirect from address (a Workable posting's short address leads to the one naming its account).
// A posting that does not exist is BoardGoneError.
export async function resolveAddress(doFetch: BoardFetch, address: string): Promise<string> {
  let from = address.trim();
  if (!from.includes('://')) from = `https://${from}`;
  const { location } = await getOnce(doFetch, from);
  if (location === '') throw new Error('the address did not lead to a company\'s page');
  try {
    return new URL(location, from).toString();
  } catch {
    throw new Error('failed to read where the address leads');
  }
}

const pageTitle = /<title[^>]*>([\s\S]*?)<\/title>/i;
// The organization Ashby's page embeds for its script: "organization":{"organizationId":"…","name":"ClickUp",…}.
const ashbyOrganization = /"organization":\{[^{}]*?"name":("(?:[^"\\]|\\.)*")/;

export function leverPageName(page: string): string {
  const m = pageTitle.exec(page);
  return m ? cleanCompanyName(unescapeHtml(m[1]!)) : '';
}

export function ashbyPageName(page: string): string {
  const m = ashbyOrganization.exec(page);
  if (!m) return '';
  try {
    const name: unknown = JSON.parse(m[1]!);
    return typeof name === 'string' ? cleanCompanyName(name) : '';
  } catch {
    return '';
  }
}

// Go's smartRecruitersPageName: a company that does not exist is redirected to SmartRecruiters' search across every company (BoardGoneError);
// one that does is a page titled "Careers at X", or a redirect to the company's own site, which names nothing.
async function smartRecruitersPageName(doFetch: BoardFetch, board: Board, pages: string): Promise<string> {
  const { page, location } = await getOnce(doFetch, `${pages}/${pathEscape(board.name)}`);
  if (location !== '') {
    let target: URL | undefined;
    try { target = new URL(location); } catch { /* not an address: names nothing */ }
    if (target && target.hostname.toLowerCase() === 'jobs.smartrecruiters.com' && target.pathname.replace(/^\/+|\/+$/g, '') === '') throw new BoardGoneError();
    return '';
  }
  const m = pageTitle.exec(page);
  if (!m) return '';
  const title = unescapeHtml(m[1]!).trim();
  return title.startsWith('Careers at ') ? cleanCompanyName(title.slice('Careers at '.length)) : '';
}

// Go's BoardName: the company's name as its board gives it. Greenhouse's board endpoint; the hosted page for Lever and Ashby;
// the board read itself for Workable and SmartRecruiters. '' when none gives one.
export async function boardName(doFetch: BoardFetch, board: Board, endpoints: BoardEndpoints = defaultBoardEndpoints): Promise<string> {
  const name = pathEscape(board.name);
  const eu = board.region === 'eu';
  switch (board.provider) {
    case 'workable':
    case 'smartrecruiters':
      return (await readBoard(doFetch, board, endpoints)).name; // Workable limits how often it is asked: one read serves both
    case 'greenhouse': {
      const answer = await getBoardJson(doFetch, `${eu ? endpoints.greenhouseEuApi : endpoints.greenhouseApi}/v1/boards/${name}`);
      return isObject(answer) ? cleanCompanyName(str(answer.name)) : '';
    }
    case 'lever': {
      const pages = eu ? endpoints.leverEuPages : endpoints.leverPages;
      if (pages === undefined) return '';
      // Lever's API answers a site that does not exist with an empty list; its page answers 404.
      return leverPageName(await getBoardText(doFetch, `${pages}/${name}`));
    }
    case 'ashby': {
      if (endpoints.ashbyPages === undefined) return '';
      return ashbyPageName(await getBoardText(doFetch, `${endpoints.ashbyPages}/${name}`));
    }
  }
}

async function getBoardJson(doFetch: BoardFetch, url: string): Promise<unknown> {
  const response = await request(doFetch, url, 'the hiring page');
  if (response.status === 404) throw new BoardGoneError();
  if (response.status === 429) throw new BoardBusyError();
  if (!response.ok) throw new Error(`the hiring page answered with status ${response.status}`);
  try {
    return await response.json();
  } catch {
    throw new Error("failed to read the board's answer");
  }
}

const remoteWord = /\bremote\b/i;
const hybridWord = /\bhybrid\b/i;
const onsiteWord = /\b(on-?site|in[- ]office)\b/i;
export const remoteTypeInWords = (location: string): string =>
  remoteWord.test(location) ? 'remote' : hybridWord.test(location) ? 'hybrid' : onsiteWord.test(location) ? 'onsite' : '';
const workplaceType = (value: string): string => {
  switch (value.toLowerCase().replace(/-/g, '')) { case 'remote': return 'remote'; case 'hybrid': return 'hybrid'; case 'onsite': return 'onsite'; }
  return '';
};
const escapeHtml = (s: string): string => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&#34;').replace(/'/g, '&#39;');
const salaryPeriod = (interval: string): string => ['year', 'month', 'week', 'day', 'hour'].find((p) => interval.includes(p)) ?? '';

function boardJob(board: Board, id: string, address: string, title: string, description: string, locations: string[]): FullSourceJob {
  return sourceJob({
    sourceId: COMPANY_PAGES_SOURCE_ID, sourceJobId: `${board.provider}/${board.name}/${id}`, canonicalUrl: address, title: title.trim(),
    companyUrl: boardUrl(board), descriptionMarkdown: description, locations,
  });
}

export function greenhouseToJob(board: Board, item: unknown): FullSourceJob | undefined {
  if (!isObject(item) || str(item.title) === '' || str(item.absolute_url) === '') return undefined;
  const where = isObject(item.location) ? str(item.location.name) : '';
  // The content is HTML escaped once more.
  const job = boardJob(board, String(item.id ?? ''), str(item.absolute_url), str(item.title), describe(unescapeHtml(str(item.content))), splitLocations(where, ';'));
  job.companyName = str(item.company_name).trim();
  job.remoteType = remoteTypeInWords(where);
  job.postedAt = parseTime(str(item.first_published), str(item.updated_at));
  return job;
}

export function leverToJob(board: Board, item: unknown): FullSourceJob | undefined {
  if (!isObject(item) || str(item.text) === '' || str(item.hostedUrl) === '') return undefined;
  let page = str(item.description);
  for (const l of list(item.lists).filter(isObject)) page += `<h3>${escapeHtml(str(l.text))}</h3><ul>${str(l.content)}</ul>`;
  page += str(item.additional);
  const categories = isObject(item.categories) ? item.categories : {};
  const all = list(categories.allLocations).filter((l): l is string => typeof l === 'string');
  // The country code names the office, not where the job can be done from.
  const locations = all.length > 0 ? trimAll(all) : splitLocations(str(categories.location), ';');
  const job = boardJob(board, str(item.id), str(item.hostedUrl), str(item.text), describe(page), locations);
  job.remoteType = workplaceType(str(item.workplaceType));
  job.employmentType = normalizeEmploymentType(str(categories.commitment));
  job.postedAt = fromUnixMilli(num(item.createdAt));
  const s = item.salaryRange;
  if (isObject(s) && (num(s.min) > 0 || num(s.max) > 0)) {
    job.salaryMin = num(s.min); job.salaryMax = num(s.max); job.salaryCurrency = str(s.currency).toUpperCase(); job.salaryPeriod = salaryPeriod(str(s.interval));
  }
  return job;
}

export function ashbyToJob(board: Board, item: unknown): FullSourceJob | undefined {
  if (!isObject(item) || str(item.title) === '' || str(item.jobUrl) === '' || item.isListed === false) return undefined; // unlisted is not the company's to show
  const locations = trimAll([str(item.location), ...list(item.secondaryLocations).filter(isObject).map((s) => str(s.location))]);
  const job = boardJob(board, str(item.id), str(item.jobUrl), str(item.title), describe(str(item.descriptionHtml)), locations);
  job.remoteType = workplaceType(str(item.workplaceType));
  if (job.remoteType === '' && item.isRemote === true) job.remoteType = 'remote';
  job.employmentType = normalizeEmploymentType(str(item.employmentType));
  job.postedAt = parseTime(str(item.publishedAt));
  const compensation = item.compensation;
  if (isObject(compensation)) {
    for (const c of list(compensation.summaryComponents).filter(isObject)) {
      const min = typeof c.minValue === 'number' ? c.minValue : undefined;
      const max = typeof c.maxValue === 'number' ? c.maxValue : undefined;
      if (str(c.compensationType) !== 'Salary' || (min === undefined && max === undefined)) continue;
      job.salaryMin = min ?? 0; job.salaryMax = max ?? 0; job.salaryCurrency = str(c.currencyCode).toUpperCase(); job.salaryPeriod = salaryPeriod(str(c.interval).toLowerCase());
      break;
    }
  }
  return job;
}

export function workableToJob(board: Board, item: unknown): FullSourceJob | undefined {
  if (!isObject(item) || str(item.title).trim() === '' || str(item.shortcode) === '') return undefined;
  const code = str(item.shortcode);
  // The widget's own address does not name the account.
  const address = `https://apply.workable.com/${pathEscape(board.name)}/j/${pathEscape(code)}/`;
  const locations = list(item.locations).filter(isObject).filter((l) => l.hidden !== true) // a hidden location is not published
    .map((l) => trimAll([str(l.city), str(l.region), str(l.country)]).join(', '));
  const job = boardJob(board, code, address, str(item.title), describe(str(item.description)), trimAll(locations));
  if (item.telecommuting === true) job.remoteType = 'remote'; // not remote is all Workable says: hybrid and on-site are not told apart
  job.employmentType = normalizeEmploymentType(str(item.employment_type));
  job.postedAt = parseTime(str(item.published_on), str(item.created_at));
  return job;
}

export function smartRecruitersToJob(board: Board, posting: unknown): FullSourceJob | undefined {
  if (!isObject(posting) || str(posting.id) === '' || str(posting.name).trim() === '') return undefined;
  const id = str(posting.id);
  const location = isObject(posting.location) ? posting.location : {};
  const job = boardJob(board, id, `https://jobs.smartrecruiters.com/${pathEscape(board.name)}/${pathEscape(id)}`, str(posting.name), '', trimAll([str(location.fullLocation)]));
  job.companyName = isObject(posting.company) ? str(posting.company.name).trim() : '';
  if (location.remote === true) job.remoteType = 'remote';
  else if (location.hybrid === true) job.remoteType = 'hybrid';
  job.employmentType = normalizeEmploymentType(isObject(posting.typeOfEmployment) ? str(posting.typeOfEmployment.label) : '');
  job.postedAt = parseTime(str(posting.releasedDate));
  return job;
}

const keep = <T>(items: unknown[], toJob: (item: unknown) => T | undefined): T[] => items.map(toJob).filter((j): j is T => j !== undefined);

export async function readBoard(doFetch: BoardFetch, board: Board, endpoints: BoardEndpoints = defaultBoardEndpoints): Promise<BoardContents> {
  const name = pathEscape(board.name);
  const eu = board.region === 'eu';
  switch (board.provider) {
    case 'greenhouse': {
      const answer = await getBoardJson(doFetch, `${eu ? endpoints.greenhouseEuApi : endpoints.greenhouseApi}/v1/boards/${name}/jobs?content=true`);
      return { jobs: keep(isObject(answer) ? list(answer.jobs) : [], (i) => greenhouseToJob(board, i)), name: '', undescribed: false };
    }
    case 'lever': {
      const answer = await getBoardJson(doFetch, `${eu ? endpoints.leverEuApi : endpoints.leverApi}/v0/postings/${name}?mode=json`);
      return { jobs: keep(list(answer), (i) => leverToJob(board, i)), name: '', undescribed: false };
    }
    case 'ashby': {
      const answer = await getBoardJson(doFetch, `${endpoints.ashbyApi}/posting-api/job-board/${name}?includeCompensation=true`);
      return { jobs: keep(isObject(answer) ? list(answer.jobs) : [], (i) => ashbyToJob(board, i)), name: '', undescribed: false };
    }
    case 'workable': {
      const answer = await getBoardJson(doFetch, `${endpoints.workableApi}/api/v1/widget/accounts/${name}?details=true`);
      const a = isObject(answer) ? answer : {};
      return { jobs: keep(list(a.jobs), (i) => workableToJob(board, i)), name: cleanCompanyName(str(a.name)), undescribed: false };
    }
    case 'smartrecruiters': {
      const postings: unknown[] = [];
      for (let offset = 0; ; offset += SMARTRECRUITERS_PAGE_SIZE) {
        const page = await getBoardJson(doFetch, `${endpoints.smartRecruitersApi}/v1/companies/${name}/postings?limit=${SMARTRECRUITERS_PAGE_SIZE}&offset=${offset}`);
        const p = isObject(page) ? page : {};
        const content = list(p.content);
        postings.push(...content);
        if (content.length === 0 || offset + SMARTRECRUITERS_PAGE_SIZE >= num(p.totalFound)) break;
      }
      // No postings at all is a company that does not exist, or one with none open: its careers page tells them apart.
      if (postings.length === 0) {
        let pageName = '';
        if (endpoints.smartRecruitersPages !== undefined) {
          try {
            pageName = await smartRecruitersPageName(doFetch, board, endpoints.smartRecruitersPages);
          } catch (e) {
            if (!(e instanceof RedirectUnreadableError)) throw e; // a host that cannot see redirects cannot tell: it is a company
          }
        }
        return { jobs: [], name: pageName, undescribed: true };
      }
      const first = postings[0];
      const company = isObject(first) && isObject(first.company) ? cleanCompanyName(str(first.company.name)) : '';
      return { jobs: keep(postings, (i) => smartRecruitersToJob(board, i)), name: company, undescribed: true };
    }
  }
}

export const postingId = (job: FullSourceJob): string => job.sourceJobId.slice(job.sourceJobId.lastIndexOf('/') + 1);

// SmartRecruiters' description of one posting: its sections, each under its title. A posting closed since the list is BoardGoneError.
export async function describeSmartRecruiters(doFetch: DoFetch, board: Board, id: string, endpoints: BoardEndpoints = defaultBoardEndpoints): Promise<string> {
  const details = await getBoardJson(doFetch, `${endpoints.smartRecruitersApi}/v1/companies/${pathEscape(board.name)}/postings/${pathEscape(id)}`);
  const sections = isObject(details) && isObject(details.jobAd) && isObject(details.jobAd.sections) ? details.jobAd.sections : {};
  let page = '';
  for (const key of ['companyDescription', 'jobDescription', 'qualifications', 'additionalInformation']) {
    const s = sections[key];
    if (!isObject(s) || str(s.text).trim() === '') continue;
    if (str(s.title) !== '') page += `<h3>${escapeHtml(str(s.title))}</h3>`;
    page += str(s.text);
  }
  return describe(page);
}

// Go's DescribeJobs: for SmartRecruiters, reads each posting's description (four at once), leaves out a posting gone since the list,
// and fails on any other error. `known` is what the host kept, keyed `${id}@${releasedAt}`, so a described posting is not asked again.
export async function describeJobs(doFetch: DoFetch, board: Board, jobs: FullSourceJob[], endpoints: BoardEndpoints = defaultBoardEndpoints, known: Map<string, string> = new Map()): Promise<FullSourceJob[]> {
  if (board.provider !== 'smartrecruiters') return jobs;
  const out: (FullSourceJob | undefined)[] = new Array(jobs.length).fill(undefined);
  const errors: Error[] = [];
  let next = 0;
  const worker = async (): Promise<void> => {
    while (next < jobs.length) {
      const i = next++;
      const job = jobs[i]!;
      const id = postingId(job);
      const cached = known.get(`${id}@${job.postedAt}`);
      try {
        const description = cached ?? await describeSmartRecruiters(doFetch, board, id, endpoints);
        known.set(`${id}@${job.postedAt}`, description);
        out[i] = { ...job, descriptionMarkdown: description };
      } catch (e) {
        if (!(e instanceof BoardGoneError)) errors.push(e instanceof Error ? e : new Error(String(e)));
      }
    }
  };
  await Promise.all(Array.from({ length: Math.min(POSTINGS_AT_ONCE, jobs.length) }, worker));
  if (errors.length > 0) throw new Error(`failed to read the postings' descriptions: ${errors.map((e) => e.message).join('; ')}`);
  return out.filter((j): j is FullSourceJob => j !== undefined);
}
