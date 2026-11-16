// A port of the aggregation rules of JobDiscoveryService.Search (internal/core/services/discovery.go), ADR-0037:
// ask every searchable source, isolate each source's failure, read signals, filter what a source did not, and dedupe by URL.
// Pure: sources are injected and storage is the host's, so what Go does against repositories is returned as data.
// Rate and ToS rules live with each source (their *_CACHE_MS constants, the User-Agent, attribution via canonicalUrl); a data source
// cannot read the clock (ADR-0037), so the host keeps an answer in SQLite for that long and asks again after.
import { isEmpty, matchesFor, noSeeker, type SearchCriteria, type Seeker } from '../search/criteria.ts';
import * as geo from '../search/geo.ts';
import { isUnreachable, PartialAnswerError, type FullSourceJob } from './job-source.ts';
import { readSignals, type Signals } from '../search/signals.ts';

export interface DiscoverySource {
  id: string;
  name: string;
  canSearch: boolean;
  // A source that cannot filter server-side (none of the public ones can) is filtered here, never trusted.
  filtersServerSide: boolean;
  search(criteria: SearchCriteria): Promise<FullSourceJob[]>;
}

export type SourceStatus = 'ok' | 'failed' | 'unreachable';
export interface SourceOutcome {
  sourceId: string;
  sourceName: string;
  status: SourceStatus;
  // How many jobs the source gave that matched.
  found: number;
  // The parts of a source made of parts that did not answer (a watched company whose board has gone).
  failing: string[];
  error?: string;
}

export type FoundJob = FullSourceJob & Signals;
export interface DiscoveryResult { jobs: FoundJob[]; sources: SourceOutcome[] }

export class DiscoveryError extends Error {
  constructor(public code: 'empty-criteria' | 'invalid-work-from' | 'no-searchable-source' | 'sources-unreachable' | 'all-sources-failed', message: string) {
    super(message);
    this.name = 'DiscoveryError';
  }
}
export const errEmptyCriteria = 'a job search needs at least one criterion: neither public source filters server-side, so an unrestricted search imports every posting on the board rather than the jobs you want';
export const errSourcesUnreachable = 'could not reach any job source: finding new jobs needs internet access, and the jobs already found are still available';

// A country in upper case, '' for none; refuses what is not a two-letter ISO code JobPilot knows.
export function normalizeWorkFrom(workFrom: string): string {
  const code = workFrom.trim().toUpperCase();
  if (code === '') return '';
  if (geo.countryByCode(code) === undefined) {
    throw new DiscoveryError('invalid-work-from', `"${code}" is not a country JobPilot knows; use its two-letter ISO code, such as NG or DE`);
  }
  return code;
}

// The error for a search no source answered: unreachable when none could be reached, else naming how each failed.
export function noAnswerError(outcomes: SourceOutcome[]): DiscoveryError {
  if (outcomes.every((o) => o.status === 'unreachable')) return new DiscoveryError('sources-unreachable', errSourcesUnreachable);
  const reasons = outcomes.map((o) => (o.status === 'unreachable' ? `${o.sourceName} could not be reached` : `${o.sourceName} answered with an error`));
  return new DiscoveryError('all-sources-failed', `no job source answered: ${reasons.join('; ')}`);
}

// The sources a search asks, in order: only one that can search is one.
export const searchable = (sources: DiscoverySource[]): DiscoverySource[] => sources.filter((s) => s.canSearch);

// Searches every source at once. A source that throws is one failed source, never a failed search (AC-008).
// Only when no source answers is the search an error, and then nothing should be written.
// `seeker` is the candidate, read only when the criteria use them (work-from with relocation, or fit-hours).
// `turn` lets a host that budgets each turn (Exact's data sources) end one between jobs; it never changes the answer.
export async function searchSources(
  sources: DiscoverySource[], criteria: SearchCriteria, seeker: Seeker = noSeeker, turn: () => Promise<void> = async () => {},
): Promise<DiscoveryResult> {
  if (isEmpty(criteria)) throw new DiscoveryError('empty-criteria', errEmptyCriteria);
  const asked = { ...criteria, workFrom: normalizeWorkFrom(criteria.workFrom ?? '') };
  const targets = searchable(sources);
  if (targets.length === 0) throw new DiscoveryError('no-searchable-source', 'no job source that can search is configured');

  const answers = await Promise.all(targets.map(async (source): Promise<{ jobs: FullSourceJob[]; error?: unknown }> => {
    try { return { jobs: await source.search(asked) }; } catch (error) { return { jobs: [], error }; }
  }));

  const outcomes: SourceOutcome[] = [];
  const matched: FoundJob[] = [];
  let answered = false;
  for (const [i, source] of targets.entries()) {
    const answer = answers[i]!;
    const outcome: SourceOutcome = { sourceId: source.id, sourceName: source.name, status: 'ok', found: 0, failing: [] };
    let error = answer.error;
    // A source made of parts answers for the parts that answered, and names the others (ADR-0024 section 5).
    if (error instanceof PartialAnswerError) { outcome.failing = error.failing; error = error.error; }
    if (error !== undefined) {
      outcome.status = isUnreachable(error) ? 'unreachable' : 'failed';
      outcome.error = error instanceof Error ? error.message : String(error);
      outcomes.push(outcome);
      continue;
    }
    answered = true;
    // Read before filtering: where a job can be done from is one of the criteria (ADR-0023).
    const read: FoundJob[] = [];
    // Reading a job is the heaviest step (several regex passes over its description), so each is a turn of its own.
    for (const job of answer.jobs) {
      await turn();
      read.push({ ...job, ...readSignals(job) });
    }
    const kept: FoundJob[] = [];
    for (const job of read) {
      if (!source.filtersServerSide) await turn();
      if (source.filtersServerSide || matchesFor(asked, job, seeker)) kept.push(job);
    }
    outcome.found = kept.length;
    outcomes.push(outcome);
    matched.push(...kept);
  }
  if (!answered) throw noAnswerError(outcomes);
  return { jobs: matched, sources: outcomes };
}

// What the host stores of a search, in the order the sources are registered, so a posting two sources both list is stored once and
// by the same source on every run. `known` is the stored jobs by canonical URL: a job the user ruled out is left out of the result.
export interface Known { notInterested: boolean }
export function dedupe(matched: FoundJob[], known: ReadonlyMap<string, Known>): { fresh: FoundJob[]; existingUrls: string[] } {
  const fresh: FoundJob[] = [];
  const existingUrls: string[] = [];
  const seen = new Set<string>();
  for (const job of matched) {
    if (seen.has(job.canonicalUrl)) continue;
    seen.add(job.canonicalUrl);
    const existing = known.get(job.canonicalUrl);
    if (existing === undefined) fresh.push(job);
    else if (!existing.notInterested) existingUrls.push(job.canonicalUrl);
  }
  return { fresh, existingUrls };
}
