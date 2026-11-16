// Saved searches on the device (ADR-0037), ported from SavedSearchService and the SQLite repository.
// Every function takes a Db and never reads the clock: the caller passes `now` (epoch ms) and, for a new row, its `id`.
import type { Db, SqlValue } from '../shared/db.ts';
import { countryByCode } from '../search/geo.ts';
import { emptyCriteria, maxHoursShift, type SearchCriteria } from '../search/criteria.ts';
import { criteriaIsEmpty, findSearchTemplate } from '../search/search-template.ts';

export interface SavedSearch {
  id: string;
  name: string;
  criteria: SearchCriteria;
  isDefault: boolean;
  // When the search last ran against the job sources and one answered; null is never.
  lastRunAt: number | null;
  createdAt: number;
  updatedAt: number;
}

export type Result<T> = { ok: true; value: T } | { ok: false; error: string };
const fail = <T>(error: string): Result<T> => ({ ok: false, error });
const ok = <T>(value: T): Result<T> => ({ ok: true, value });

// SearchRefreshInterval: a search is stale when it never ran or its last run is at least this old.
export const searchRefreshInterval = 6 * 60 * 60 * 1000;
export const isStale = (s: SavedSearch, now: number): boolean => s.lastRunAt === null || now - s.lastRunAt >= searchRefreshInterval;

// Names are unique without regard to case, as the Go store compares them (COLLATE NOCASE).
const nameKey = (name: string): string => name.trim().toLowerCase();

export const savedSearchSchema: string[] = [
  `CREATE TABLE IF NOT EXISTS saved_searches (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    name_key TEXT NOT NULL UNIQUE,
    criteria TEXT NOT NULL,
    is_default INTEGER NOT NULL DEFAULT 0,
    last_run_at INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
  )`,
];
export const ensureSavedSearchSchema = async (db: Db): Promise<void> => {
  for (const sql of savedSearchSchema) await db.execute(sql);
};

const columns = 'id, name, criteria, is_default, last_run_at, created_at, updated_at';

function parseCriteria(json: string): SearchCriteria {
  try { return { ...emptyCriteria, ...(JSON.parse(json) as Partial<SearchCriteria>) }; } catch { return { ...emptyCriteria }; }
}

function toSearch(r: SqlValue[]): SavedSearch {
  return {
    id: String(r[0]), name: String(r[1]), criteria: parseCriteria(String(r[2])), isDefault: Number(r[3]) === 1,
    lastRunAt: r[4] === null ? null : Number(r[4]), createdAt: Number(r[5]), updatedAt: Number(r[6]),
  };
}

// Default first, then oldest first, as List does.
export async function listSavedSearches(db: Db): Promise<SavedSearch[]> {
  const { rows } = await db.query(`SELECT ${columns} FROM saved_searches ORDER BY is_default DESC, created_at ASC, rowid ASC`);
  return rows.map(toSearch);
}

export async function getSavedSearch(db: Db, id: string): Promise<SavedSearch | undefined> {
  const { rows } = await db.query(`SELECT ${columns} FROM saved_searches WHERE id = ?`, [id]);
  return rows.length === 0 ? undefined : toSearch(rows[0]);
}

export async function defaultSavedSearch(db: Db): Promise<SavedSearch | undefined> {
  return (await listSavedSearches(db)).find((s) => s.isDefault);
}

// The searches due to run again, in List order.
export async function staleSavedSearches(db: Db, now: number): Promise<SavedSearch[]> {
  return (await listSavedSearches(db)).filter((s) => isStale(s, now));
}

// normalizeWorkFrom: an upper-case country code, or an error when it is not a country.
export function normalizeWorkFrom(workFrom: string): Result<string> {
  const code = workFrom.trim().toUpperCase();
  if (code === '') return ok('');
  if (countryByCode(code) === undefined) return fail(`"${code}" is not a country code`);
  return ok(code);
}

// The rules that need no storage: work-from, hours shift, a name, criteria that narrow something.
function checked(name: string, criteria: SearchCriteria): Result<{ name: string; criteria: SearchCriteria }> {
  const trimmed = name.trim();
  const workFrom = normalizeWorkFrom(criteria.workFrom ?? '');
  if (!workFrom.ok) return workFrom;
  const shift = criteria.hoursShift ?? 0;
  if (shift < 0 || shift > maxHoursShift) return fail(`hours shift must be between 0 and ${maxHoursShift}`);
  if (trimmed === '') return fail('a saved search needs a name');
  if (criteriaIsEmpty(criteria)) {
    return fail('a saved search needs at least one criterion: searching with none would import every job on every board, not the jobs you want');
  }
  return ok({ name: trimmed, criteria: { ...criteria, workFrom: workFrom.value } });
}

async function nameTaken(db: Db, name: string, exceptId: string): Promise<boolean> {
  const { rows } = await db.query('SELECT id FROM saved_searches WHERE name_key = ?', [nameKey(name)]);
  return rows.length > 0 && String(rows[0][0]) !== exceptId;
}

// The first search created becomes the default, so a user with one never has to nominate it.
export async function createSavedSearch(db: Db, id: string, name: string, criteria: SearchCriteria, now: number): Promise<Result<SavedSearch>> {
  const c = checked(name, criteria);
  if (!c.ok) return c;
  if (await nameTaken(db, c.value.name, '')) return fail(`a saved search named "${c.value.name}" already exists`);
  const { rows } = await db.query('SELECT COUNT(*) FROM saved_searches');
  const search: SavedSearch = { id, name: c.value.name, criteria: c.value.criteria, isDefault: Number(rows[0][0]) === 0, lastRunAt: null, createdAt: now, updatedAt: now };
  await db.execute(
    `INSERT INTO saved_searches (${columns.replace('criteria', 'name_key, criteria')}) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
    [id, search.name, nameKey(search.name), JSON.stringify(search.criteria), search.isDefault ? 1 : 0, null, now, now],
  );
  return ok(search);
}

// Replaces the name and criteria, leaving default status alone (that is setDefaultSavedSearch's job).
export async function updateSavedSearch(db: Db, id: string, name: string, criteria: SearchCriteria, now: number): Promise<Result<SavedSearch>> {
  const c = checked(name, criteria);
  if (!c.ok) return c;
  if (await nameTaken(db, c.value.name, id)) return fail(`a saved search named "${c.value.name}" already exists`);
  const stored = await getSavedSearch(db, id);
  if (stored === undefined) return fail('saved search not found');
  const search = { ...stored, name: c.value.name, criteria: c.value.criteria, updatedAt: now };
  await db.execute('UPDATE saved_searches SET name = ?, name_key = ?, criteria = ?, updated_at = ? WHERE id = ?',
    [search.name, nameKey(search.name), JSON.stringify(search.criteria), now, id]);
  return ok(search);
}

// Records that a search ran and at least one source answered.
export async function markSavedSearchRun(db: Db, id: string, now: number): Promise<void> {
  await db.execute('UPDATE saved_searches SET last_run_at = ?, updated_at = ? WHERE id = ?', [now, now, id]);
}

// Promotes one search and demotes the previous holder, together.
export async function setDefaultSavedSearch(db: Db, id: string, now: number): Promise<Result<SavedSearch>> {
  if ((await getSavedSearch(db, id)) === undefined) return fail('saved search not found');
  await db.transaction([
    { sql: 'UPDATE saved_searches SET is_default = 0, updated_at = ? WHERE is_default = 1 AND id <> ?', params: [now, id] },
    { sql: 'UPDATE saved_searches SET is_default = 1, updated_at = ? WHERE id = ?', params: [now, id] },
  ]);
  return ok((await getSavedSearch(db, id))!);
}

// Removes a search and, with it, the scouts that run it and their runs (ADR-0018). Deleting the default promotes the next search.
// Returns how many scouts went with it, so the app can refresh what it shows. Requires the scout tables (scout-store.ts).
export async function deleteSavedSearch(db: Db, id: string, now: number): Promise<Result<{ scoutsDeleted: number }>> {
  const stored = await getSavedSearch(db, id);
  if (stored === undefined) return fail('saved search not found');
  const { rows } = await db.query('SELECT COUNT(*) FROM scouts WHERE saved_search_id = ?', [id]);
  const scouts = 'SELECT id FROM scouts WHERE saved_search_id = ?';
  const runs = `SELECT id FROM scout_runs WHERE scout_id IN (${scouts})`;
  const statements: { sql: string; params?: SqlValue[] }[] = [
    { sql: `DELETE FROM scout_run_events WHERE run_id IN (${runs})`, params: [id] },
    { sql: `DELETE FROM scout_runs WHERE scout_id IN (${scouts})`, params: [id] },
    { sql: 'DELETE FROM scouts WHERE saved_search_id = ?', params: [id] },
    { sql: 'DELETE FROM saved_searches WHERE id = ?', params: [id] },
  ];
  if (stored.isDefault) {
    statements.push({
      sql: `UPDATE saved_searches SET is_default = 1, updated_at = ? WHERE id = (SELECT id FROM saved_searches ORDER BY created_at ASC, rowid ASC LIMIT 1)`,
      params: [now],
    });
  }
  await db.transaction(statements);
  return ok({ scoutsDeleted: Number(rows[0][0]) });
}

// A second copy of a template is numbered rather than refused.
export function availableName(base: string, taken: Set<string>): string {
  if (!taken.has(base.toLowerCase())) return base;
  for (let n = 2; ; n++) {
    const candidate = `${base} ${n}`;
    if (!taken.has(candidate.toLowerCase())) return candidate;
  }
}

// ImportTemplates: each template becomes an ordinary saved search. Every id is resolved before anything is written.
// `newId` makes the id of each search created.
export async function importTemplates(db: Db, templateIds: string[], workFrom: string, now: number, newId: () => string): Promise<Result<SavedSearch[]>> {
  if (templateIds.length === 0) return fail('no templates chosen');
  const where = normalizeWorkFrom(workFrom);
  if (!where.ok) return where;
  const templates = [];
  const seen = new Set<string>();
  for (const id of templateIds) {
    if (seen.has(id)) continue;
    seen.add(id);
    const t = findSearchTemplate(id);
    if (t === undefined) return fail(`unknown search template "${id}"`);
    templates.push(t);
  }
  const taken = new Set((await listSavedSearches(db)).map((s) => s.name.toLowerCase()));
  const created: SavedSearch[] = [];
  for (const t of templates) {
    const name = availableName(t.name, taken);
    taken.add(name.toLowerCase());
    const made = await createSavedSearch(db, newId(), name, { ...t.criteria, workFrom: where.value }, now);
    if (!made.ok) return fail(`failed to import template "${t.id}": ${made.error}`);
    created.push(made.value);
  }
  return ok(created);
}
