// A port of the watched-company rules in internal/core/domain/company.go (ADR-0024, ADR-0037): a board is named by its provider,
// its region and its name, read from an address without a request. `boardUrl`, `cleanCompanyName` and `Board` are in
// sources/company-boards.ts and are used here, not repeated.
import { boardUrl, cleanCompanyName, type Board, type Provider } from '../sources/company-boards.ts';

export const providerName = (p: Provider): string =>
  ({ greenhouse: 'Greenhouse', lever: 'Lever', ashby: 'Ashby', workable: 'Workable', smartrecruiters: 'SmartRecruiters' })[p];

// Two boards are one when provider and region are equal and the names are equal ignoring case.
export const sameBoard = (a: Board, b: Board): boolean => a.provider === b.provider && (a.region ?? '') === (b.region ?? '') && a.name.toLowerCase() === b.name.toLowerCase();

// What a company is called when its board does not say: the board's name, `-` and `_` as spaces, the first letter capitalised.
export function defaultName(b: Board): string {
  const chars = [...b.name.replace(/[-_]/g, ' ')];
  if (chars.length > 0) chars[0] = chars[0]!.toUpperCase();
  return chars.join('');
}

export class UnsupportedAddressError extends Error {
  constructor(address: string) {
    super(`${JSON.stringify(address)} is not a hiring page JobPilot can read; use a Greenhouse (boards.greenhouse.io/…, job-boards.greenhouse.io/…), Lever (jobs.lever.co/…), Ashby (jobs.ashbyhq.com/…), Workable (apply.workable.com/…) or SmartRecruiters (careers.smartrecruiters.com/…, jobs.smartrecruiters.com/…) address`);
    this.name = 'UnsupportedAddressError';
  }
}
// A Workable posting's short address, apply.workable.com/j/{code}, names no account: Workable redirects it to the account's own.
export class WorkablePostingAddressError extends Error {
  constructor(address: string) { super(`${JSON.stringify(address)}: a Workable posting's short address does not name its company`); this.name = 'WorkablePostingAddressError'; }
}

const boardNamePattern = /^[\p{L}\p{N}][\p{L}\p{N} ._-]*$/u;
// Workable's own subdomains, which are not accounts: jobs.workable.com searches every account.
const workableHosts = new Set(['apply', 'jobs', 'www', 'help', 'resources', 'api', 'app', 'careers', 'status', 'developers']);
// SmartRecruiters' own pages on its job hosts, which are not companies.
const smartRecruitersPaths = new Set(['external-referrals', 'oneclick-ui', 'sr-jobs', 'ni', 'search', 'public', 'embed']);

function boardAt(host: string, segments: string[], query: URLSearchParams): Board | undefined {
  const first = segments[0] ?? '';
  switch (host) {
    case 'boards.greenhouse.io': case 'job-boards.greenhouse.io': return greenhouse('', first, query);
    case 'boards.eu.greenhouse.io': case 'job-boards.eu.greenhouse.io': return greenhouse('eu', first, query);
    case 'jobs.lever.co': return first !== '' ? { provider: 'lever', name: first } : undefined;
    case 'jobs.eu.lever.co': return first !== '' ? { provider: 'lever', region: 'eu', name: first } : undefined;
    case 'jobs.ashbyhq.com': return first !== '' ? { provider: 'ashby', name: first } : undefined;
    case 'apply.workable.com': return first !== '' ? { provider: 'workable', name: first } : undefined;
    case 'careers.smartrecruiters.com': case 'jobs.smartrecruiters.com':
      return first !== '' && !smartRecruitersPaths.has(first.toLowerCase()) ? { provider: 'smartrecruiters', name: first } : undefined;
  }
  // A Workable account's older address is its own subdomain: sumble-inc.workable.com.
  if (host.endsWith('.workable.com')) {
    const account = host.slice(0, -'.workable.com'.length);
    if (!account.includes('.') && !workableHosts.has(account)) return { provider: 'workable', name: account };
  }
  return undefined;
}

function greenhouse(region: '' | 'eu', first: string, query: URLSearchParams): Board | undefined {
  const name = first === 'embed' ? (query.get('for') ?? '') : first;
  return name !== '' ? { provider: 'greenhouse', ...(region === 'eu' ? { region } : {}), name } : undefined;
}

// Go's ParseBoard: the board an address names, the address of a hiring page or of any posting on it. Fetches nothing.
export function parseBoard(input: string): Board {
  let address = input.trim();
  if (address !== '' && !address.includes('://')) address = `https://${address}`;
  let url: URL;
  try { url = new URL(address); } catch { throw new UnsupportedAddressError(address); }
  if (url.hostname === '') throw new UnsupportedAddressError(address);
  const host = url.hostname.toLowerCase().replace(/^www\./, '');
  const segments = url.pathname.split('/').filter((s) => s !== '').map((s) => { try { return decodeURIComponent(s); } catch { return s; } });
  if (host === 'apply.workable.com' && segments[0] === 'j') throw new WorkablePostingAddressError(address);
  const board = boardAt(host, segments, url.searchParams);
  if (board === undefined || !boardNamePattern.test(board.name)) throw new UnsupportedAddressError(address);
  return board;
}

const addressInText = /https?:\/\/(?:www\.)?(?:(?:job-)?boards(?:\.eu)?\.greenhouse\.io|jobs(?:\.eu)?\.lever\.co|jobs\.ashbyhq\.com|[a-z0-9-]+\.workable\.com|(?:careers|jobs)\.smartrecruiters\.com)\/[^\s)\]>"'<]+/gi;

// Go's BoardOf: the board a job's posting is on, from its own address or else the first supported address in its description
// (where a Hacker News posting links to the company's page).
export function boardOf(job: { canonicalUrl: string; descriptionMarkdown: string }): Board | undefined {
  try { return parseBoard(job.canonicalUrl); } catch { /* the description is looked at next */ }
  for (const address of job.descriptionMarkdown.match(addressInText) ?? []) {
    try { return parseBoard(address); } catch { /* the next address */ }
  }
  return undefined;
}

export { boardUrl, cleanCompanyName };
