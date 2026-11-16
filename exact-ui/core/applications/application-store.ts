// Applications kept on the device (ADR-0037): the tables and the few reads and writes the app and the scout run share.
// Takes a Db and never reads the clock.
import type { Db, SqlValue } from '../shared/db.ts';
import type { Tracked } from './tracking.ts';

export const applicationSchema: string[] = [
  'CREATE TABLE IF NOT EXISTS applications (id TEXT PRIMARY KEY, job_id TEXT NOT NULL UNIQUE, title TEXT NOT NULL, company TEXT NOT NULL, url TEXT NOT NULL, apply_url TEXT NOT NULL, state TEXT NOT NULL, updated_at INTEGER NOT NULL)',
  'CREATE TABLE IF NOT EXISTS transitions (id TEXT PRIMARY KEY, application_id TEXT NOT NULL, seq INTEGER NOT NULL, from_state TEXT NOT NULL, to_state TEXT NOT NULL, reason TEXT NOT NULL, at INTEGER NOT NULL)',
];
export const ensureApplicationSchema = async (db: Db): Promise<void> => { for (const sql of applicationSchema) await db.execute(sql); };

export const applicationColumns = 'id, job_id, title, company, url, apply_url, state, updated_at';
export const toTracked = (r: SqlValue[]): Tracked => ({ id: String(r[0]), jobId: String(r[1]), title: String(r[2]), company: String(r[3]), url: String(r[4]),
  applyUrl: String(r[5]), state: String(r[6]) as Tracked['state'], updatedAt: Number(r[7]) });

export async function getTracked(db: Db, id: string): Promise<Tracked | undefined> {
  const row = (await db.query(`SELECT ${applicationColumns} FROM applications WHERE id = ?`, [id])).rows[0];
  return row ? toTracked(row) : undefined;
}
export const saveSql = (a: Tracked) => ({ sql: `INSERT OR REPLACE INTO applications (${applicationColumns}) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
  params: [a.id, a.jobId, a.title, a.company, a.url, a.applyUrl, a.state, a.updatedAt] as SqlValue[] });
export const saveTracked = (db: Db, a: Tracked): Promise<unknown> => { const s = saveSql(a); return db.execute(s.sql, s.params); };

export async function trackedJobIds(db: Db): Promise<Set<string>> {
  return new Set((await db.query('SELECT job_id FROM applications')).rows.map((r) => String(r[0])));
}
