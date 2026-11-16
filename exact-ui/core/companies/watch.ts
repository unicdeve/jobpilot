// A port of CompanyWatchService (internal/core/services/companywatch.go, ADR-0024, ADR-0037): watching a company, the starter
// suggestions, and the watched companies as one job source, "Company pages". Reads go through the injected fetch; storage takes a Db
// and the time is passed in. Nothing here reaches an employer: only the providers' public hiring-page APIs are read.
import { mayMatch, type SearchCriteria } from '../search/criteria.ts';
import type { Db } from '../shared/db.ts';
import type { DiscoverySource } from '../sources/discovery.ts';
import {
  boardName,
  boardUrl,
  cleanCompanyName,
  COMPANY_PAGES_SOURCE_ID,
  describeJobs,
  POSTINGS_AT_ONCE,
  readBoard,
  resolveAddress,
  type Board,
  type BoardEndpoints,
  type BoardFetch,
  defaultBoardEndpoints,
} from '../sources/company-boards.ts';
import {
  BoardGoneError,
  PartialAnswerError,
  SourceUnreachableError,
  type FullSourceJob,
} from '../sources/job-source.ts';
import {
  boardOf,
  defaultName,
  parseBoard,
  sameBoard,
  UnsupportedAddressError,
  WorkablePostingAddressError,
} from './company.ts';
import { starterCompanies, type StarterCompany } from './starter-companies.ts';
import {
  createCompany,
  getByBoard,
  listCompanies,
  recordCheck,
  type BoardCheck,
  type WatchedCompany,
} from './watch-store.ts';

export class JobNotOnBoardError extends Error {
  constructor() {
    super('this job is not on a Greenhouse, Lever, Ashby, Workable or SmartRecruiters hiring page');
    this.name = 'JobNotOnBoardError';
  }
}

export interface Watcher {
  db: Db;
  fetch: BoardFetch;
  endpoints?: BoardEndpoints;
  newId(): string;
}

// Go's Watch: adds the company whose hiring page `address` is on, once its board has answered (a mistyped address is refused, not
// watched). A Workable posting's short address is followed to the account that names it. `created` is false for a board already watched.
export async function watch(
  w: Watcher,
  address: string,
  now: number,
): Promise<{ company: WatchedCompany; created: boolean }> {
  let board: Board;
  try {
    board = parseBoard(address);
  } catch (e) {
    if (!(e instanceof WorkablePostingAddressError)) throw e;
    let resolved: string;
    try {
      resolved = await resolveAddress(w.fetch, address);
    } catch (re) {
      throw new Error(`could not find whose posting ${address} is: ${(re as Error).message}`);
    }
    board = parseBoard(resolved);
  }
  return watchBoard(w, board, address, now);
}

// Go's WatchJob: the company whose hiring page a job's posting is on.
export async function watchJob(
  w: Watcher,
  job: { canonicalUrl: string; descriptionMarkdown: string },
  now: number,
): Promise<{ company: WatchedCompany; created: boolean }> {
  const board = boardOf(job);
  if (board === undefined) throw new JobNotOnBoardError();
  return watchBoard(w, board, job.canonicalUrl, now);
}

async function watchBoard(
  w: Watcher,
  board: Board,
  addedFrom: string,
  now: number,
): Promise<{ company: WatchedCompany; created: boolean }> {
  const existing = await getByBoard(w.db, board);
  if (existing !== undefined) return { company: existing, created: false };
  let contents;
  try {
    contents = await readBoard(w.fetch, board, w.endpoints);
  } catch (e) {
    throw new Error(`could not read ${boardUrl(board)}: ${(e as Error).message}`);
  }
  let name = '';
  try {
    name = await boardName(w.fetch, board, w.endpoints);
  } catch (e) {
    if (e instanceof BoardGoneError)
      throw new Error(`could not read ${boardUrl(board)}: ${e.message}`);
    // The name is a convenience; the board answered, which is what watching needs.
  }
  name = cleanCompanyName(name || contents.name);
  if (name === '') name = defaultName(board);
  const company = await createCompany(w.db, {
    id: w.newId(),
    board,
    name,
    addedFrom,
    addedAt: now,
  });
  await recordCheck(w.db, company.id, now, 'ok', contents.jobs.length);
  const stored = await getByBoard(w.db, board);
  return { company: stored ?? company, created: true };
}

// Go's Suggestions: the starter list, less the companies already watched.
export async function suggestions(db: Db): Promise<StarterCompany[]> {
  const watched = await listCompanies(db);
  return starterCompanies.filter(
    (s) => !watched.some((c) => sameBoard(c.board, { provider: s.provider, name: s.board })),
  );
}

export const starterBoard = (s: StarterCompany): Board => ({ provider: s.provider, name: s.board });

export interface JobCompany {
  board?: Board;
  url?: string;
  company?: WatchedCompany;
}
// Go's CompanyForJob: the hiring page a job is on and the watched company it is, if any.
export async function companyForJob(
  db: Db,
  job: { canonicalUrl: string; descriptionMarkdown: string },
): Promise<JobCompany> {
  const board = boardOf(job);
  if (board === undefined) return {};
  return { board, url: boardUrl(board), company: await getByBoard(db, board) };
}

const checkOf = (e: unknown): BoardCheck =>
  e instanceof BoardGoneError
    ? 'gone'
    : e instanceof SourceUnreachableError
      ? 'unreachable'
      : 'failed';

interface BoardRead {
  jobs?: FullSourceJob[];
  openRoles: number;
  error?: unknown;
}

// Reads one company's board, names the postings that have no company, and describes only the postings the criteria may match when
// the board lists them without descriptions (SmartRecruiters).
async function readCompany(
  w: Watcher,
  company: WatchedCompany,
  criteria: SearchCriteria,
): Promise<BoardRead> {
  try {
    const contents = await readBoard(w.fetch, company.board, w.endpoints);
    let jobs: FullSourceJob[] = [];
    for (const job of contents.jobs) {
      const named = job.companyName === '' ? { ...job, companyName: company.name } : job;
      if (contents.undescribed && !mayMatch(criteria, named)) continue;
      jobs.push(named);
    }
    if (contents.undescribed && jobs.length > 0)
      jobs = await describeJobs(w.fetch, company.board, jobs, w.endpoints);
    return { jobs, openRoles: contents.jobs.length };
  } catch (error) {
    return { error, openRoles: 0 };
  }
}

// Go's companyPagesSource: reads every watched company's page, four at a time, and records how each went. A company whose page did
// not answer is named, and the others' postings are kept. Discovery applies the criteria in full afterwards. `now` stamps each check.
export function companyPagesSource(w: Watcher, now: number): DiscoverySource {
  return {
    id: COMPANY_PAGES_SOURCE_ID,
    name: 'Company pages',
    canSearch: true,
    filtersServerSide: false,
    async search(criteria) {
      const companies = await listCompanies(w.db);
      const reads: BoardRead[] = new Array(companies.length);
      let next = 0;
      const worker = async (): Promise<void> => {
        while (next < companies.length) {
          const i = next++;
          reads[i] = await readCompany(w, companies[i]!, criteria);
        }
      };
      await Promise.all(
        Array.from({ length: Math.min(POSTINGS_AT_ONCE, companies.length) }, worker),
      );

      const jobs: FullSourceJob[] = [];
      const failing: string[] = [];
      let unreachable = 0;
      for (const [i, company] of companies.entries()) {
        const read = reads[i]!;
        if (read.error !== undefined) {
          failing.push(company.name);
          const check = checkOf(read.error);
          if (check === 'unreachable') unreachable++;
          // A check not recorded is only a stale list: it is never the search's failure.
          try {
            await recordCheck(
              w.db,
              company.id,
              now,
              check,
              check === 'gone' ? 0 : company.openRoles,
            );
          } catch {
            /* ignored */
          }
          continue;
        }
        try {
          await recordCheck(w.db, company.id, now, 'ok', read.openRoles);
        } catch {
          /* ignored */
        }
        jobs.push(...read.jobs!);
      }
      if (failing.length === 0) return jobs;
      if (failing.length < companies.length) throw new PartialAnswerError(failing, undefined, jobs);
      throw new PartialAnswerError(
        failing,
        unreachable === companies.length
          ? new SourceUnreachableError(
              'the watched companies',
              new Error("no watched company's page answered"),
            )
          : new Error("no watched company's page answered"),
      );
    },
  };
}

export { UnsupportedAddressError };
