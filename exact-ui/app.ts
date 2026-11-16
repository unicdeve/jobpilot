import type { Answer, Database, Result, Sources, Storage } from './app.contract.d.ts';

// The JobPilot daemon's loopback address (ADR-0012). The daemon picks a free port on every start, so a real
// run sets API_BASE_URL to the address `jobpilot-cli ui` prints; the default is the test server's (server.ts).
import { emptyCriteria } from './core/search/criteria.ts';
import type { Db } from './core/shared/db.ts';
import { ensureAgentRunTables } from './core/activity/agent-run-store.ts';
import { applicationColumns, ensureApplicationSchema, getTracked, saveSql, toTracked, trackedJobIds } from './core/applications/application-store.ts';
import { foundId, searchBoard, sourceUrls } from './core/sources/discover-sources.ts';
import type { DoFetch } from './core/sources/job-source.ts';
import { ensureSavedSearchSchema, listSavedSearches } from './core/scouts/saved-search.ts';
import { describeCriteria, loadScoutForm, saveScoutForm, searchChoices } from './core/scouts/scout-editor.ts';
import { runScoutNow } from './core/scouts/scout-manual.ts';
import { ensureScoutSchema, listRunEvents, listRuns, listScouts } from './core/scouts/scout-store.ts';
import { choices, day, isLocal, move, newApplication, setApplyLink, applicationId } from './core/applications/tracking.ts';

export const API: string = import.meta.env.API_BASE_URL ?? 'http://127.0.0.1:4417';
// ADR-0037: discovery with no daemon. Every ported source is searched here, each kept in SQLite on the device for its own cache
// window (core/discover-sources.ts), and the search rule runs here, as it does in the Go core. REMOTIVE_URL and SOURCES_BASE_URL
// point the sources at the stand-ins in server.ts for `test` and `verify`; a real build leaves them unset.
const SOURCE: string = import.meta.env.REMOTIVE_URL ?? 'https://remotive.com/api/remote-jobs';
// The default is RemoteOK's own origin, so the grant below is always a real address; it means "no stand-ins".
const REAL_SOURCES = 'https://remoteok.com';
const SOURCES_BASE: string = import.meta.env.SOURCES_BASE_URL ?? 'https://remoteok.com';
const URLS = sourceUrls(SOURCES_BASE === REAL_SOURCES ? '' : SOURCES_BASE, SOURCE);
// A data source cannot read the clock: the view passes the current quarter hour, the finest a source's cache window needs.
const SLOT_MS = 15 * 60 * 1000;
const CACHE = 'app:/data/jobs.db';
// Applications, scouts, saved searches and their runs kept on the device, with history (ADR-0037).
const KEPT = 'app:/data/applications.db';
export const grants = `net.fetch ${API}
net.fetch ${SOURCE}
net.fetch ${SOURCES_BASE}
net.fetch https://remoteok.com
net.fetch https://www.arbeitnow.com
net.fetch https://himalayas.app
net.fetch https://jobicy.com
net.fetch https://weworkremotely.com
net.fetch https://hn.algolia.com
net.fetch https://boards-api.greenhouse.io
net.fetch https://boards-api.eu.greenhouse.io
net.fetch https://api.lever.co
net.fetch https://api.eu.lever.co
net.fetch https://api.ashbyhq.com
net.fetch https://apply.workable.com
net.fetch https://api.smartrecruiters.com
sqlite.open ${CACHE}
sqlite.open ${KEPT}`;

type Json = Record<string, unknown>;
const isObject = (v: unknown): v is Json => v !== null && typeof v === 'object' && !Array.isArray(v);
const str = (v: unknown): string => (typeof v === 'string' ? v : '');
const num = (v: unknown): number => (typeof v === 'number' && Number.isFinite(v) ? v : 0);
const strings = (v: unknown): string[] => (Array.isArray(v) ? v.filter((s): s is string => typeof s === 'string') : []);

// A failed read throws, so Exact keeps the previous answer and the view can offer Try Again.
async function get(path: string): Promise<unknown> {
  const response = await fetch(`${API}${path}`, { exactTimeout: 10000 });
  if (response.status === 404) return null;
  if (!response.ok) throw new Error(`The daemon answered ${response.status} for ${path}.`);
  return response.json();
}

function salary(job: Json): string {
  const min = num(job.salary_min), max = num(job.salary_max), currency = str(job.salary_currency);
  if (min === 0 && max === 0) return '';
  const k = (n: number) => (n >= 1000 ? `${Math.round(n / 1000)}k` : String(n));
  const range = max > min ? `${k(min)}–${k(max)}` : k(Math.max(min, max));
  const period = str(job.salary_period);
  return `${currency} ${range}${period && period !== 'year' ? ` / ${period}` : ''}`.trim();
}

const REMOTE: Record<string, string> = { remote: 'Remote', hybrid: 'Hybrid', onsite: 'On-site' };

function toJob(v: unknown): Result<'loadJobs'>[number] | undefined {
  if (!isObject(v) || str(v.id) === '') return undefined;
  return {
    id: str(v.id),
    title: str(v.title),
    company: str(v.company_name),
    location: strings(v.locations).join(', '),
    remote: REMOTE[str(v.remote_type)] ?? '',
    salary: salary(v),
    posted: str(v.posted_at).slice(0, 10),
    skills: strings(v.skills),
    description: str(v.description_markdown),
    url: str(v.canonical_url),
  };
}

const STATES: Record<string, string> = {
  DISCOVERED: 'Discovered', MATCHED: 'Matched', APPROVED: 'Approved', SUBMITTED: 'Sent',
  INTERVIEWING: 'Interviewing', OFFERED: 'Offer', NO_RESPONSE: 'No response', REJECTED: 'Rejected', WITHDRAWN: 'Withdrawn',
};

async function daemonApplications(): Promise<Result<'loadApplications'>> {
  const profile = await get('/v1/profile');
  if (!isObject(profile)) return [];
  const list = await get(`/v1/applications?candidate_id=${encodeURIComponent(str(profile.id))}`);
  if (!Array.isArray(list)) throw new Error('The daemon returned an invalid application list.');
  return Promise.all(list.filter(isObject).map(async (a) => {
    const job = toJob(await get(`/v1/jobs/${encodeURIComponent(str(a.job_id))}`));
    return {
      id: str(a.id), applyUrl: str(a.apply_url), jobId: str(a.job_id), title: job?.title ?? 'A job that is no longer stored',
      company: job?.company ?? '', state: STATES[str(a.state)] ?? str(a.state), updated: str(a.updated_at).slice(0, 10),
      local: false, next: [],
    };
  }));
}


// One connection at a time per file: a refresh and a write that overlap would find the file busy.
const turns = new Map<string, Promise<unknown>>();
function opened<T>(storage: Storage, file: string, prepare: (db: Db) => Promise<void>, work: (db: Database) => Promise<T>): Promise<T> {
  const go = async (): Promise<T> => {
    const db = await storage.sqlite.open(file);
    try { await prepare(db as unknown as Db); return await work(db); } finally { await db.close(); }
  };
  const run = (turns.get(file) ?? Promise.resolve()).then(go, go);
  turns.set(file, run.catch(() => undefined));
  return run;
}
const prepareKept = async (db: Db): Promise<void> => {
  await ensureApplicationSchema(db);
  await ensureSavedSearchSchema(db);
  await ensureScoutSchema(db);
  await ensureAgentRunTables(db);
};
const kept = <T>(storage: Storage, work: (db: Db) => Promise<T>): Promise<T> => opened(storage, KEPT, prepareKept, (db) => work(db as unknown as Db));
// Each answer has a budget for one turn: ending it between jobs keeps a long search inside it.
declare const scheduler: { yield(): Promise<void> } | undefined;
const turn = async (): Promise<void> => { if (typeof scheduler !== 'undefined') await scheduler.yield(); };
const doFetch: DoFetch = (url, init) => fetch(url, { exactTimeout: 15000, headers: init?.headers });
const SOURCE_NAMES: Record<string, string> = { remotive: 'Remotive', remoteok: 'RemoteOK', arbeitnow: 'Arbeitnow', himalayas: 'Himalayas', jobicy: 'Jobicy',
  weworkremotely: 'We Work Remotely', hackernews: 'Hacker News' };
const sourceName = (id: string): string => SOURCE_NAMES[id] ?? id;
const label = (code: string) => STATES[code] ?? code;

const sources: Sources = {
  loadJobs: async (): Promise<Result<'loadJobs'>> => {
    const page = await get('/v1/jobs?limit=100');
    if (!isObject(page) || !Array.isArray(page.jobs)) throw new Error('The daemon returned an invalid job list.');
    return page.jobs.map(toJob).filter((j): j is NonNullable<typeof j> => j !== undefined);
  },
  loadMatch: async ([jobId]): Promise<Result<'loadMatch'>> => {
    const m = await get(`/v1/jobs/${encodeURIComponent(jobId)}/match`);
    if (!isObject(m)) return { jobId, score: -1, rationale: '', strengths: [], gaps: [] };
    return { jobId, score: Math.round(num(m.score)), rationale: str(m.rationale), strengths: strings(m.strengths), gaps: strings(m.gaps) };
  },
  loadApplications: async (_args, _store, storage): Promise<Result<'loadApplications'>> => {
    const local = await kept(storage, async (db) => (await db.query(`SELECT ${applicationColumns} FROM applications ORDER BY updated_at DESC, id`)).rows.map((r) => {
      const a = toTracked(r);
      return { id: a.id, applyUrl: a.applyUrl, jobId: a.jobId, title: a.title, company: a.company, state: label(a.state), updated: day(a.updatedAt),
        local: true, next: choices(a.state).map((code) => ({ code, label: label(code) })) };
    }));
    // The daemon is optional now: when it is not running, the applications on the device still show.
    let remote: Result<'loadApplications'> = [];
    try { remote = await daemonApplications(); } catch { remote = []; }
    return [...local, ...remote];
  },
  // Scouts on the device, then the daemon's when it answers (the daemon is optional, as with applications).
  loadScouts: async (_args, _store, storage): Promise<Result<'loadScouts'>> => {
    const local = await kept(storage, async (db) => {
      const searches = new Map((await listSavedSearches(db)).map((x) => [x.id, x]));
      return (await listScouts(db)).map((x) => ({
        id: x.id, name: x.name, enabled: x.isEnabled, minScore: x.minMatchScore, ceiling: x.maxApplicationsPerRun,
        lastRun: x.lastRunAt === null ? '' : day(x.lastRunAt), local: true,
        search: describeCriteria(searches.get(x.savedSearchId)?.criteria ?? emptyCriteria),
      }));
    });
    let remote: Result<'loadScouts'> = [];
    try {
      const list = await get('/v1/scouts');
      if (Array.isArray(list)) remote = list.filter(isObject).map((x) => ({
        id: str(x.id), name: str(x.name), enabled: x.is_enabled === true, minScore: num(x.min_match_score), ceiling: num(x.max_applications_per_run),
        lastRun: str(x.last_run_at).slice(0, 10), local: false, search: '',
      }));
    } catch { remote = []; }
    return [...local, ...remote];
  },
  loadSavedSearches: async (_args, _store, storage): Promise<Result<'loadSavedSearches'>> =>
    kept(storage, async (db) => (await listSavedSearches(db)).map((x) => ({ id: x.id, name: x.name, summary: describeCriteria(x.criteria) }))),
  loadSearchChoices: async (_args, _store, storage): Promise<Result<'loadSearchChoices'>> =>
    kept(storage, async (db) => (await searchChoices(db)).map((c) => ({ code: c.value, label: c.label }))),
  loadScoutForm: async ([id], _store, storage): Promise<Result<'loadScoutForm'>> => {
    const form = await kept(storage, (db) => loadScoutForm(db, id));
    return form ?? { name: '', choice: '', keywords: '', minScore: '70', ceiling: '10', enabled: true };
  },
  loadScoutRuns: async ([scoutId], _store, storage): Promise<Result<'loadScoutRuns'>> => kept(storage, async (db) => {
    const runs = (await listRuns(db, 100)).filter((r) => r.scoutId === scoutId).slice(0, 20);
    return Promise.all(runs.map(async (r) => {
      const finished = (await listRunEvents(db, r.id)).find((e) => e.action === 'scout_finished');
      return { id: r.id, status: r.status, started: day(r.startedAt), error: r.error,
        summary: finished === undefined ? '' : `${finished.inputSummary}, ${finished.outputSummary}` };
    }));
  }),
  loadRunTrail: async ([runId], _store, storage): Promise<Result<'loadRunTrail'>> =>
    kept(storage, async (db) => (await listRunEvents(db, runId)).map((e) => ({
      id: e.id, action: e.action.replaceAll('_', ' '), status: e.status, policy: e.policyResult,
      detail: [e.inputSummary, e.outputSummary].filter((x) => x !== '').join(' — '),
    }))),
  loadProfile: async (): Promise<Result<'loadProfile'>> => {
    const p = await get('/v1/profile');
    if (!isObject(p)) return { name: '', email: '', phone: '', location: '' };
    const place = [str(p.location_city), str(p.location_region), str(p.location_country)].filter((s) => s !== '');
    return { name: `${str(p.first_name)} ${str(p.last_name)}`.trim(), email: str(p.email), phone: str(p.phone), location: place.join(', ') };
  },
  loadResumes: async (): Promise<Result<'loadResumes'>> => {
    const profile = await get('/v1/profile');
    if (!isObject(profile)) return [];
    const list = await get(`/v1/resumes?candidate_id=${encodeURIComponent(str(profile.id))}`);
    if (!Array.isArray(list)) throw new Error('The daemon returned an invalid resume list.');
    return list.filter(isObject).map((r) => ({ id: str(r.id), name: str(r.name), updated: str(r.updated_at).slice(0, 10) }));
  },
  loadRuns: async (): Promise<Result<'loadRuns'>> => {
    const page = await get('/v1/agent-runs?limit=50');
    if (!isObject(page) || !Array.isArray(page.runs)) throw new Error('The daemon returned an invalid activity list.');
    return page.runs.filter(isObject).map((r) => ({
      id: str(r.id), title: str(r.scout_name) || str(r.task_type), status: str(r.status),
      started: str(r.started_at).slice(0, 10), events: num(r.event_count),
    }));
  },
  loadModel: async (): Promise<Result<'loadModel'>> => {
    const s = await get('/v1/settings/llm');
    if (!isObject(s)) return { provider: '', model: '', address: '' };
    return { provider: str(s.provider), model: str(s.model), address: str(s.base_url) };
  },
  // Search every ported source at once (core/discover-sources.ts): one that fails is named, never fatal; the rest dedupe by URL.
  discover: async ([query, slot], _store, storage): Promise<Result<'discover'>> => {
    if (query.trim() === '') return { jobs: [], failed: [], error: '' };
    const trackedJobs = await kept(storage, (db) => trackedJobIds(db));
    const board = await opened(storage, CACHE, async () => undefined, (db) =>
      searchBoard(db as unknown as Db, doFetch, URLS, slot * SLOT_MS, { ...emptyCriteria, keywords: query }, turn));
    return {
      error: board.error, failed: board.sources.filter((x) => x.status !== 'ok').map((x) => x.sourceName),
      jobs: board.jobs.slice(0, 100).map((j) => ({ id: foundId(j), title: j.title, company: j.companyName, location: j.locations.join(', '), url: j.canonicalUrl,
        tracked: trackedJobs.has(foundId(j)), source: sourceName(j.sourceId) })),
    };
  },
  loadHistory: async ([id], _store, storage): Promise<Result<'loadHistory'>> => {
    if (isLocal(id)) {
      return kept(storage, async (db) => (await db.query('SELECT id, from_state, to_state, reason, at FROM transitions WHERE application_id = ? ORDER BY seq DESC', [id])).rows
        .map((r) => ({ id: String(r[0]), from: label(String(r[1])), to: label(String(r[2])), reason: String(r[3]), at: day(Number(r[4])) })));
    }
    const list = await get(`/v1/applications/${encodeURIComponent(id)}/history`);
    if (!Array.isArray(list)) return [];
    return list.filter(isObject).map((s) => ({
      id: str(s.id), from: STATES[str(s.from_state)] ?? str(s.from_state), to: STATES[str(s.to_state)] ?? str(s.to_state),
      reason: str(s.reason), at: str(s.created_at).slice(0, 10),
    }));
  },
  // ADR-0037: track a found job. One application per job; a second track is a no-op, never a duplicate.
  trackJob: async ([jobId, title, company, url, now], _store, storage): Promise<Result<'trackJob'>> => kept(storage, async (db) => {
    if (await getTracked(db, applicationId(jobId))) return { ok: true, error: '' };
    const created = newApplication({ id: jobId, title, company, url }, now);
    if (!created) return { ok: false, error: 'This job has no title, so it cannot be tracked.' };
    await db.execute(saveSql(created).sql, saveSql(created).params);
    return { ok: true, error: '' };
  }),
  moveApplication: async ([id, to, reason, now], _store, storage): Promise<Result<'moveApplication'>> => kept(storage, async (db) => {
    const app = await getTracked(db, id);
    if (!app) return { ok: false, error: 'This application is no longer stored.' };
    const seq = Number((await db.query('SELECT COALESCE(MAX(seq), 0) FROM transitions WHERE application_id = ?', [id])).rows[0]?.[0] ?? 0) + 1;
    const moved = move(app, to, reason, now, seq);
    if (!moved.ok) return moved;
    const t = moved.step;
    await db.transaction([saveSql(moved.application),
      { sql: 'INSERT INTO transitions (id, application_id, seq, from_state, to_state, reason, at) VALUES (?, ?, ?, ?, ?, ?, ?)', params: [t.id, id, seq, t.from, t.to, t.reason, t.at] }]);
    return { ok: true, error: '' };
  }),
  setApplyLink: async ([id, url, now], _store, storage): Promise<Result<'setApplyLink'>> => kept(storage, async (db) => {
    const app = await getTracked(db, id);
    if (!app) return { ok: false, error: 'This application is no longer stored.' };
    const linked = setApplyLink(app, url, now);
    if (!linked.ok) return linked;
    await db.execute(saveSql(linked.application).sql, saveSql(linked.application).params);
    return { ok: true, error: '' };
  }),
  saveScout: async ([id, name, choice, keywords, minScore, ceiling, enabled, now], _store, storage): Promise<Result<'saveScout'>> =>
    kept(storage, async (db) => {
      const saved = await saveScoutForm(db, id, { name, choice, keywords, minScore, ceiling, enabled }, now);
      return { ok: saved.ok, error: saved.ok ? '' : saved.error };
    }),
  // A manual run (ADR-0037 section 3). No model is chosen yet (section 4), so no score is given and no application is created.
  runScout: async ([id, now], _store, storage): Promise<Result<'runScout'>> =>
    runScoutNow({
      withDb: (work) => kept(storage, work), now,
      search: (criteria) => opened(storage, CACHE, async () => undefined, (db) => searchBoard(db as unknown as Db, doFetch, URLS, Math.floor(now / SLOT_MS) * SLOT_MS, criteria, turn)),
    }, id),
};
export const answer: Answer = (source, args, store, storage, native) => sources[source](args, store, storage, native);
