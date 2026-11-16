// A stand-in for the JobPilot daemon's read API, for `test` and `verify` only: it answers the few routes app.ts
// reads with fixed data, where the address in app.ts says. The real daemon is internal/adapters/primary/httpapi.
import { API } from './app.ts';

const JOBS = [
  { id: 'j1', title: 'Senior Go Engineer', company_name: 'Acme', locations: ['Berlin', 'Remote (EU)'], remote_type: 'hybrid',
    salary_min: 90000, salary_max: 120000, salary_currency: 'EUR', salary_period: 'year', posted_at: '2026-11-10T09:00:00Z',
    skills: ['Go', 'SQLite'], description_markdown: 'Build a local-first daemon.', canonical_url: 'https://acme.example/jobs/1' },
  { id: 'j2', title: 'Product Designer', company_name: 'Globex', locations: ['Lagos'], remote_type: 'remote',
    salary_min: 0, salary_max: 0, salary_currency: '', salary_period: '', posted_at: '2026-11-12T09:00:00Z',
    skills: ['Figma'], description_markdown: 'Design calm tools.', canonical_url: 'https://globex.example/jobs/2' },
  { id: 'j3', title: 'Platform Engineer', company_name: 'Initech', locations: ['London'], remote_type: 'onsite',
    salary_min: 70000, salary_max: 70000, salary_currency: 'GBP', salary_period: 'year', posted_at: '2026-11-01T09:00:00Z',
    skills: ['Kubernetes'], description_markdown: 'Keep the lights on.', canonical_url: 'https://initech.example/jobs/3' },
];
const MATCHES: Record<string, object> = {
  j1: { score: 86, rationale: 'Strong Go and SQLite background.', strengths: ['Go for six years', 'Local-first work'], gaps: ['No Berlin presence'] },
};
const APPLICATIONS = [
  { id: 'a1', job_id: 'j1', candidate_id: 'c1', state: 'SUBMITTED', apply_url: 'https://acme.example/apply', updated_at: '2026-11-13T10:00:00Z' },
  { id: 'a2', job_id: 'j3', candidate_id: 'c1', state: 'INTERVIEWING', updated_at: '2026-11-14T10:00:00Z' },
];
const REMOTIVE = { jobs: [
  { id: 1, url: 'https://remotive.com/j/1', title: 'Go Developer', company_name: 'Acme', tags: ['go'], job_type: 'full_time',
    publication_date: '2026-11-10T09:00:00', candidate_required_location: 'Europe', description: '<p>Build services in Go.</p>' },
  { id: 2, url: 'https://remotive.com/j/2', title: 'Product Designer', company_name: 'Globex', tags: [], job_type: 'full_time',
    publication_date: '2026-11-11T09:00:00', candidate_required_location: 'Worldwide', description: '<p>Design calm tools.</p>' },
] };
// The other sources, under /sources (SOURCES_BASE_URL): two answer, Jobicy answers with nothing, and the rest fail, so a test sees one
// source fail without stopping the search.
const REMOTEOK = [{ legal: 'RemoteOK terms' }, { id: '77', position: 'Go Platform Engineer', company: 'Initech ', url: 'https://remoteok.com/remote-jobs/77',
  description: '<p>Go services.</p>', location: 'Worldwide', tags: ['go'], date: '2026-11-12T09:00:00+00:00' }];
const ARBEITNOW = { data: [{ slug: 'go-dev-berlin', url: 'https://www.arbeitnow.com/jobs/go-dev-berlin', title: 'Go Backend Developer', company_name: 'Hooli',
  description: '<p>Go in Berlin.</p>', location: 'Berlin', remote: false, job_types: ['full time'], tags: ['go'], created_at: 1731400000 }] };
// A Greenhouse board, "acme", for watching a company: its name and one opening.
const GREENHOUSE = { name: 'Acme Robotics' };
const ACME_JOBS = { jobs: [{ id: 5, title: 'Go Systems Engineer', absolute_url: 'https://job-boards.greenhouse.io/acme/jobs/5', location: { name: 'Remote, Worldwide' },
  content: '<p>Go services.</p>', updated_at: '2026-11-10T09:00:00Z' }] };
const SCOUTS = [
  { id: 'sc1', name: 'Go roles in Europe', is_enabled: true, min_match_score: 70, max_applications_per_run: 5, last_run_at: '2026-11-14T08:00:00Z' },
  { id: 'sc2', name: 'Design, remote', is_enabled: false, min_match_score: 80, max_applications_per_run: 3 },
];
const RESUMES = [{ id: 'r1', candidate_id: 'c1', name: 'Main resume', updated_at: '2026-11-02T10:00:00Z' }];
const RUNS = [{ id: 'run1', scout_id: 'sc1', scout_name: 'Go roles in Europe', task_type: 'scout', status: 'completed', started_at: '2026-11-14T08:00:00Z', event_count: 7 }];
const HISTORY = [
  { id: 's1', application_id: 'a1', from_state: '', to_state: 'DISCOVERED', reason: '', created_at: '2026-11-11T10:00:00Z' },
  { id: 's2', application_id: 'a1', from_state: 'DISCOVERED', to_state: 'SUBMITTED', reason: 'Sent from my own browser', created_at: '2026-11-13T10:00:00Z' },
];

// The web build's page is another origin; the real daemon refuses one (ADR-0035 §3), so a browser run needs
// the daemon to serve this build, and this server only allows the test page.
const CORS = { 'access-control-allow-origin': '*' };
const send = (body: unknown, status = 200) => Response.json(body, { status, headers: CORS });

const { hostname, port } = new URL(API);
Bun.serve({ hostname, port: Number(port), routes: {
  '/remotive': () => send(REMOTIVE),
  '/sources/remoteok': () => send(REMOTEOK),
  '/sources/arbeitnow': () => send(ARBEITNOW),
  '/sources/jobicy': () => send({ jobs: [] }),
  '/sources/himalayas': () => send({ error: 'down' }, 500),
  '/sources/himalayas/*': () => send({ error: 'down' }, 500),
  '/sources/wwr/*': () => send({ error: 'down' }, 500),
  '/sources/greenhouse/v1/boards/acme': () => send(GREENHOUSE),
  '/sources/greenhouse/v1/boards/acme/jobs': () => send(ACME_JOBS),
  '/sources/greenhouse/v1/boards/:name': () => send({ status: 404, error: 'Job not found' }, 404),
  '/sources/greenhouse/v1/boards/:name/jobs': () => send({ status: 404, error: 'Job not found' }, 404),
  '/sources/hn/*': () => send({ hits: [] }),
  '/v1/health': () => send({ status: 'ok' }),
  '/v1/profile': () => send({ id: 'c1', first_name: 'Ada', last_name: 'Lovelace', email: 'ada@example.com', phone: '', location_city: 'London', location_region: '', location_country: 'UK' }),
  '/v1/jobs': () => send({ jobs: JOBS, total: JOBS.length }),
  '/v1/jobs/:id': (req) => { const j = JOBS.find((x) => x.id === req.params.id); return j ? send(j) : send({ error: 'not found' }, 404); },
  '/v1/jobs/:id/match': (req) => { const m = MATCHES[req.params.id]; return m ? send({ id: 'm1', job_id: req.params.id, ...m }) : send({ error: 'not found' }, 404); },
  '/v1/applications/:id/history': (req) => send(HISTORY.filter((h) => h.application_id === req.params.id)),
  '/v1/scouts': () => send(SCOUTS),
  '/v1/resumes': () => send(RESUMES),
  '/v1/agent-runs': () => send({ runs: RUNS, total: RUNS.length }),
  '/v1/settings/llm': () => send({ provider: 'ollama', base_url: 'http://127.0.0.1:11434', model: 'llama3.2' }),
  '/v1/applications': () => send(APPLICATIONS),
} });
console.log(`server.ts: ${API}`);
