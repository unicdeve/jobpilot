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
  '/v1/health': () => send({ status: 'ok' }),
  '/v1/profile': () => send({ id: 'c1', first_name: 'Ada', last_name: 'Lovelace' }),
  '/v1/jobs': () => send({ jobs: JOBS, total: JOBS.length }),
  '/v1/jobs/:id': (req) => { const j = JOBS.find((x) => x.id === req.params.id); return j ? send(j) : send({ error: 'not found' }, 404); },
  '/v1/jobs/:id/match': (req) => { const m = MATCHES[req.params.id]; return m ? send({ id: 'm1', job_id: req.params.id, ...m }) : send({ error: 'not found' }, 404); },
  '/v1/applications/:id/history': (req) => send(HISTORY.filter((h) => h.application_id === req.params.id)),
  '/v1/applications': () => send(APPLICATIONS),
} });
console.log(`server.ts: ${API}`);
