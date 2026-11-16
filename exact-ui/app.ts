import type { Answer, Result, Sources } from './app.contract.d.ts';

// The JobPilot daemon's loopback address (ADR-0012). The daemon picks a free port on every start, so a real
// run sets API_BASE_URL to the address `jobpilot-cli ui` prints; the default is the test server's (server.ts).
import { emptyCriteria, matches } from './core/criteria.ts';
import { fetchRemotive, type SourceJob } from './core/remotive.ts';

export const API: string = import.meta.env.API_BASE_URL ?? 'http://127.0.0.1:4417';
const SOURCE: string = import.meta.env.REMOTIVE_URL ?? 'https://remotive.com/api/remote-jobs';
// ADR-0037: discovery with no daemon. The listing is kept in SQLite on the device for Remotive's own cache
// window (its terms ask for few requests a day), and the search rule runs here, as it does in the Go core.
// A data source cannot read the clock, so the view passes the current hour.
const FRESH_HOURS = 4;
const CACHE = 'app:/data/jobs.db';
export const grants = `net.fetch ${API}\nnet.fetch ${SOURCE}\nsqlite.open ${CACHE}`;

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
  loadApplications: async (): Promise<Result<'loadApplications'>> => {
    const profile = await get('/v1/profile');
    if (!isObject(profile)) return [];
    const list = await get(`/v1/applications?candidate_id=${encodeURIComponent(str(profile.id))}`);
    if (!Array.isArray(list)) throw new Error('The daemon returned an invalid application list.');
    return Promise.all(list.filter(isObject).map(async (a) => {
      const job = toJob(await get(`/v1/jobs/${encodeURIComponent(str(a.job_id))}`));
      return {
        id: str(a.id), applyUrl: str(a.apply_url), jobId: str(a.job_id), title: job?.title ?? 'A job that is no longer stored',
        company: job?.company ?? '', state: STATES[str(a.state)] ?? str(a.state), updated: str(a.updated_at).slice(0, 10),
      };
    }));
  },
  loadScouts: async (): Promise<Result<'loadScouts'>> => {
    const list = await get('/v1/scouts');
    if (!Array.isArray(list)) return [];
    return list.filter(isObject).map((s) => ({
      id: str(s.id), name: str(s.name), enabled: s.is_enabled === true, minScore: num(s.min_match_score),
      ceiling: num(s.max_applications_per_run), lastRun: str(s.last_run_at).slice(0, 10),
    }));
  },
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
discover: async ([query, hour], _store, storage): Promise<Result<'discover'>> => {
  const db = await storage.sqlite.open(CACHE);
  try {
    await db.execute('CREATE TABLE IF NOT EXISTS found (id TEXT PRIMARY KEY, body TEXT NOT NULL)');
    await db.execute('CREATE TABLE IF NOT EXISTS fetched (at INTEGER NOT NULL)');
    const at = Number((await db.query('SELECT MAX(at) FROM fetched')).rows[0]?.[0] ?? 0);
    if (hour - at >= FRESH_HOURS) {
      const listing = await fetchRemotive((url) => fetch(url, { exactTimeout: 15000 }), SOURCE);
      await db.transaction([
        { sql: 'DELETE FROM found' },
        ...listing.map((j) => ({ sql: 'INSERT OR REPLACE INTO found (id, body) VALUES (?, ?)', params: [j.sourceJobId, JSON.stringify(j)] })),
        { sql: 'DELETE FROM fetched' },
        { sql: 'INSERT INTO fetched (at) VALUES (?)', params: [hour] },
      ]);
    }
    const criteria = { ...emptyCriteria, keywords: query };
    return (await db.query('SELECT body FROM found ORDER BY id')).rows
      .map(([body]) => JSON.parse(String(body)) as SourceJob).filter((j) => matches(criteria, j))
      .slice(0, 100).map((j) => ({ id: j.sourceJobId, title: j.title, company: j.companyName, location: j.locations.join(', '), url: j.canonicalUrl }));
  } finally { await db.close(); }
},
  loadHistory: async ([id]): Promise<Result<'loadHistory'>> => {
    const list = await get(`/v1/applications/${encodeURIComponent(id)}/history`);
    if (!Array.isArray(list)) return [];
    return list.filter(isObject).map((s) => ({
      id: str(s.id), from: STATES[str(s.from_state)] ?? str(s.from_state), to: STATES[str(s.to_state)] ?? str(s.to_state),
      reason: str(s.reason), at: str(s.created_at).slice(0, 10),
    }));
  },
};
export const answer: Answer = (source, args, store, storage, native) => sources[source](args, store, storage, native);
