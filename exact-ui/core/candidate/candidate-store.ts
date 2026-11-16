// The candidate profile in the device's SQLite (ADR-0037), the schema of the Go core's candidates, candidate_links,
// candidate_work_authorizations, candidate_experiences and candidate_skills tables. Times are epoch milliseconds,
// dates are YYYY-MM-DD text, and the caller passes `now`: nothing here reads the clock. Saving a candidate writes it
// and its children in one transaction, because a half-saved profile would answer a form with half a person.
import { dateFromSql, dateToSql } from '../shared/date.ts';
import type { Db, SqlValue } from '../shared/db.ts';
import { isLinkKind, isRemotePreference, normalizeTimeZone, type Candidate, type RemotePreference } from './candidate.ts';

export const candidateSchema: string[] = [
  `CREATE TABLE IF NOT EXISTS candidates (
    id TEXT PRIMARY KEY,
    first_name TEXT NOT NULL DEFAULT '', last_name TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '', phone TEXT NOT NULL DEFAULT '',
    location_city TEXT NOT NULL DEFAULT '', location_region TEXT NOT NULL DEFAULT '', location_country TEXT NOT NULL DEFAULT '',
    time_zone TEXT NOT NULL DEFAULT '',
    notice_period_days INTEGER, earliest_start_date TEXT,
    summary TEXT NOT NULL DEFAULT '', remote_preference TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
  )`,
  `CREATE TABLE IF NOT EXISTS candidate_links (
    id TEXT PRIMARY KEY, candidate_id TEXT NOT NULL REFERENCES candidates(id) ON DELETE CASCADE,
    kind TEXT NOT NULL, url TEXT NOT NULL, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
    UNIQUE (candidate_id, kind)
  )`,
  `CREATE TABLE IF NOT EXISTS candidate_work_authorizations (
    id TEXT PRIMARY KEY, candidate_id TEXT NOT NULL REFERENCES candidates(id) ON DELETE CASCADE,
    country TEXT NOT NULL, is_authorized INTEGER NOT NULL DEFAULT 0, requires_sponsorship INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
    UNIQUE (candidate_id, country)
  )`,
  `CREATE TABLE IF NOT EXISTS candidate_experiences (
    id TEXT PRIMARY KEY, candidate_id TEXT NOT NULL REFERENCES candidates(id) ON DELETE CASCADE,
    company TEXT NOT NULL DEFAULT '', title TEXT NOT NULL DEFAULT '',
    start_date TEXT, end_date TEXT, is_current INTEGER NOT NULL DEFAULT 0, description TEXT NOT NULL DEFAULT '',
    position INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
  )`,
  `CREATE TABLE IF NOT EXISTS candidate_skills (
    id TEXT PRIMARY KEY, candidate_id TEXT NOT NULL REFERENCES candidates(id) ON DELETE CASCADE,
    name TEXT NOT NULL, years_experience REAL,
    position INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
    UNIQUE (candidate_id, name)
  )`,
  'CREATE INDEX IF NOT EXISTS idx_candidate_links_candidate ON candidate_links (candidate_id)',
  'CREATE INDEX IF NOT EXISTS idx_candidate_work_auth_candidate ON candidate_work_authorizations (candidate_id)',
  'CREATE INDEX IF NOT EXISTS idx_candidate_experiences_candidate ON candidate_experiences (candidate_id)',
  'CREATE INDEX IF NOT EXISTS idx_candidate_skills_candidate ON candidate_skills (candidate_id)',
];

export async function ensureCandidateSchema(db: Db): Promise<void> {
  for (const sql of candidateSchema) await db.execute(sql);
}

const num = (v: SqlValue | undefined): number | null => (v === null || v === undefined ? null : Number(v));
const text = (v: SqlValue | undefined): string => (v === null || v === undefined ? '' : String(v));

export async function getCandidate(db: Db, id: string): Promise<Candidate | undefined> {
  const head = await db.query(
    `SELECT id, first_name, last_name, email, phone, location_city, location_region, location_country, time_zone,
      notice_period_days, earliest_start_date, summary, remote_preference, created_at, updated_at FROM candidates WHERE id = ?`, [id]);
  const r = head.rows[0];
  if (!r) return undefined;
  const [links, auths, exps, skills] = await Promise.all([
    db.query('SELECT id, kind, url FROM candidate_links WHERE candidate_id = ? ORDER BY created_at, rowid', [id]),
    db.query('SELECT id, country, is_authorized, requires_sponsorship FROM candidate_work_authorizations WHERE candidate_id = ? ORDER BY created_at, rowid', [id]),
    db.query('SELECT id, company, title, start_date, end_date, is_current, description FROM candidate_experiences WHERE candidate_id = ? ORDER BY position, rowid', [id]),
    db.query('SELECT id, name, years_experience FROM candidate_skills WHERE candidate_id = ? ORDER BY position, rowid', [id]),
  ]);
  const date = (v: SqlValue | undefined) => (v === null || v === undefined || String(v) === '' ? null : dateFromSql(String(v)));
  return {
    id: text(r[0]), firstName: text(r[1]), lastName: text(r[2]), email: text(r[3]), phone: text(r[4]),
    locationCity: text(r[5]), locationRegion: text(r[6]), locationCountry: text(r[7]), timeZone: text(r[8]),
    noticePeriodDays: num(r[9]), earliestStartDate: date(r[10]), summary: text(r[11]),
    remotePreference: text(r[12]) as RemotePreference, createdAt: Number(r[13]), updatedAt: Number(r[14]),
    links: links.rows.map((l) => ({ id: text(l[0]), kind: text(l[1]) as Candidate['links'][number]['kind'], url: text(l[2]) })),
    workAuthorizations: auths.rows.map((a) => ({ id: text(a[0]), country: text(a[1]), isAuthorized: Number(a[2]) !== 0, requiresSponsorship: Number(a[3]) !== 0 })),
    experiences: exps.rows.map((e) => ({
      id: text(e[0]), company: text(e[1]), title: text(e[2]), startDate: date(e[3]), endDate: date(e[4]), isCurrent: Number(e[5]) !== 0, description: text(e[6]),
    })),
    skills: skills.rows.map((s) => ({ id: text(s[0]), name: text(s[1]), yearsExperience: num(s[2]) })),
  };
}

// The user's profile: JobPilot is local and has one candidate, the oldest.
export async function firstCandidate(db: Db): Promise<Candidate | undefined> {
  const { rows } = await db.query('SELECT id FROM candidates ORDER BY created_at, rowid LIMIT 1');
  return rows[0] ? getCandidate(db, String(rows[0][0])) : undefined;
}

export type Saved = { ok: true; candidate: Candidate } | { ok: false; error: string };

function validate(c: Candidate): string | undefined {
  if (c.id.trim() === '') return 'A profile needs an id.';
  if (!isRemotePreference(c.remotePreference)) return `"${c.remotePreference}" is not a remote preference.`;
  if (c.noticePeriodDays !== null && (!Number.isInteger(c.noticePeriodDays) || c.noticePeriodDays < 0)) return 'The notice period must be a whole number of days.';
  const kinds = new Set<string>();
  for (const l of c.links) {
    if (!isLinkKind(l.kind)) return `"${l.kind}" is not a kind of link.`;
    if (kinds.has(l.kind)) return `There is more than one ${l.kind} link.`;
    kinds.add(l.kind);
  }
  const countries = new Set<string>();
  for (const a of c.workAuthorizations) {
    const key = a.country.trim().toLowerCase();
    if (key === '') return 'A work authorization needs a country.';
    if (countries.has(key)) return `${a.country.trim()} is listed twice.`;
    countries.add(key);
  }
  const names = new Set<string>();
  for (const s of c.skills) {
    const key = s.name.trim();
    if (key === '') return 'A skill needs a name.';
    if (names.has(key)) return `${key} is listed twice.`;
    names.add(key);
  }
  return undefined;
}

// Writes the whole profile, replacing its children, in one transaction. Created rows keep their created_at on a
// re-save of the same profile. `newId` names any link, authorization, experience or skill saved without an id.
export async function saveCandidate(db: Db, input: Candidate, now: number, newId: () => string): Promise<Saved> {
  const tz = normalizeTimeZone(input.timeZone);
  if (!tz.ok) return tz;
  const c: Candidate = {
    ...input, timeZone: tz.zone, updatedAt: now,
    links: input.links.map((l) => ({ ...l, id: l.id || newId(), url: l.url.trim() })),
    workAuthorizations: input.workAuthorizations.map((a) => ({ ...a, id: a.id || newId(), country: a.country.trim() })),
    skills: input.skills.map((s) => ({ ...s, id: s.id || newId(), name: s.name.trim() })),
    experiences: input.experiences.map((e) => ({ ...e, id: e.id || newId() })),
  };
  const error = validate(c);
  if (error) return { ok: false, error };

  const existing = await db.query('SELECT created_at FROM candidates WHERE id = ?', [c.id]);
  const createdAt = existing.rows[0] ? Number(existing.rows[0][0]) : now;
  c.createdAt = createdAt;
  const id = c.id;
  const st: { sql: string; params?: SqlValue[] }[] = [
    { sql: `INSERT INTO candidates (id, first_name, last_name, email, phone, location_city, location_region, location_country, time_zone,
        notice_period_days, earliest_start_date, summary, remote_preference, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(id) DO UPDATE SET first_name = excluded.first_name, last_name = excluded.last_name, email = excluded.email,
          phone = excluded.phone, location_city = excluded.location_city, location_region = excluded.location_region,
          location_country = excluded.location_country, time_zone = excluded.time_zone, notice_period_days = excluded.notice_period_days,
          earliest_start_date = excluded.earliest_start_date, summary = excluded.summary, remote_preference = excluded.remote_preference,
          updated_at = excluded.updated_at`,
      params: [id, c.firstName.trim(), c.lastName.trim(), c.email.trim(), c.phone.trim(), c.locationCity.trim(), c.locationRegion.trim(),
        c.locationCountry.trim(), c.timeZone, c.noticePeriodDays, c.earliestStartDate ? dateToSql(c.earliestStartDate) : null,
        c.summary.trim(), c.remotePreference, createdAt, now] },
    { sql: 'DELETE FROM candidate_links WHERE candidate_id = ?', params: [id] },
    { sql: 'DELETE FROM candidate_work_authorizations WHERE candidate_id = ?', params: [id] },
    { sql: 'DELETE FROM candidate_experiences WHERE candidate_id = ?', params: [id] },
    { sql: 'DELETE FROM candidate_skills WHERE candidate_id = ?', params: [id] },
  ];
  for (const l of c.links) st.push({ sql: 'INSERT INTO candidate_links (id, candidate_id, kind, url, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)', params: [l.id, id, l.kind, l.url, now, now] });
  for (const a of c.workAuthorizations) st.push({ sql: 'INSERT INTO candidate_work_authorizations (id, candidate_id, country, is_authorized, requires_sponsorship, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)', params: [a.id, id, a.country, a.isAuthorized ? 1 : 0, a.requiresSponsorship ? 1 : 0, now, now] });
  c.experiences.forEach((e, i) => st.push({ sql: 'INSERT INTO candidate_experiences (id, candidate_id, company, title, start_date, end_date, is_current, description, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)', params: [e.id, id, e.company.trim(), e.title.trim(), e.startDate ? dateToSql(e.startDate) : null, e.isCurrent || !e.endDate ? null : dateToSql(e.endDate), e.isCurrent ? 1 : 0, e.description.trim(), i, now, now] }));
  c.skills.forEach((s, i) => st.push({ sql: 'INSERT INTO candidate_skills (id, candidate_id, name, years_experience, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)', params: [s.id, id, s.name, s.yearsExperience, i, now, now] }));
  await db.transaction(st);
  return { ok: true, candidate: (await getCandidate(db, id))! };
}
