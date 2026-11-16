// Scouts and their run trail on the device (ADR-0037), ported from ScoutService's create/update/delete and the SQLite repositories.
// Every function takes a Db and never reads the clock: the caller passes `now` (epoch ms) and a new row's `id`.
// The schedule is stored and validated but nothing here runs it: on a phone a scout runs when the user asks (ADR-0037 section 3).
import type { Db, SqlValue } from '../shared/db.ts';
import { isScheduled, normalizedSchedule, type ScoutSchedule } from './scout-schedule.ts';
import { validateScout, type Scout } from './scout.ts';
import { getSavedSearch, type Result } from './saved-search.ts';

export interface StoredScout extends Scout {
  id: string;
  // When the schedule was last set or changed, or null while unscheduled.
  scheduleSetAt: number | null;
  lastRunAt: number | null;
  createdAt: number;
  updatedAt: number;
}

export type RunTrigger = 'manual' | 'schedule';
export type RunStatus = 'running' | 'completed' | 'failed';

export interface ScoutRun {
  id: string;
  scoutId: string;
  trigger: RunTrigger | string;
  status: RunStatus;
  error: string;
  startedAt: number;
  completedAt: number | null;
}

// One step of a run's trail. policyResult is 'allowed' or 'denied' when an eligibility rule decided it, else ''.
export interface RunEvent {
  id: string;
  runId: string;
  seq: number;
  action: string;
  status: string;
  inputSummary: string;
  outputSummary: string;
  policyResult: string;
  createdAt: number;
}

export const scoutSchema: string[] = [
  `CREATE TABLE IF NOT EXISTS scouts (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    saved_search_id TEXT NOT NULL,
    min_match_score INTEGER NOT NULL,
    max_applications_per_run INTEGER NOT NULL,
    source TEXT NOT NULL,
    is_enabled INTEGER NOT NULL DEFAULT 1,
    schedule_kind TEXT NOT NULL DEFAULT 'off',
    schedule_every_hours INTEGER NOT NULL DEFAULT 0,
    schedule_daily_at INTEGER NOT NULL DEFAULT 0,
    schedule_set_at INTEGER,
    last_run_at INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
  )`,
  `CREATE TABLE IF NOT EXISTS scout_runs (
    id TEXT PRIMARY KEY,
    scout_id TEXT NOT NULL,
    trigger TEXT NOT NULL,
    status TEXT NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    started_at INTEGER NOT NULL,
    completed_at INTEGER
  )`,
  `CREATE TABLE IF NOT EXISTS scout_run_events (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    seq INTEGER NOT NULL,
    action TEXT NOT NULL,
    status TEXT NOT NULL,
    input_summary TEXT NOT NULL DEFAULT '',
    output_summary TEXT NOT NULL DEFAULT '',
    policy_result TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
  )`,
];
export const ensureScoutSchema = async (db: Db): Promise<void> => {
  for (const sql of scoutSchema) await db.execute(sql);
};

const scoutColumns =
  'id, name, saved_search_id, min_match_score, max_applications_per_run, source, is_enabled, schedule_kind, schedule_every_hours, schedule_daily_at, schedule_set_at, last_run_at, created_at, updated_at';
const nullable = (v: SqlValue): number | null => (v === null ? null : Number(v));

function toScout(r: SqlValue[]): StoredScout {
  return {
    id: String(r[0]),
    name: String(r[1]),
    savedSearchId: String(r[2]),
    minMatchScore: Number(r[3]),
    maxApplicationsPerRun: Number(r[4]),
    source: String(r[5]),
    isEnabled: Number(r[6]) === 1,
    schedule: { kind: String(r[7]), everyHours: Number(r[8]), dailyAt: Number(r[9]) },
    scheduleSetAt: nullable(r[10]),
    lastRunAt: nullable(r[11]),
    createdAt: Number(r[12]),
    updatedAt: Number(r[13]),
  };
}

export async function listScouts(db: Db): Promise<StoredScout[]> {
  const { rows } = await db.query(
    `SELECT ${scoutColumns} FROM scouts ORDER BY created_at ASC, rowid ASC`,
  );
  return rows.map(toScout);
}

export async function getScout(db: Db, id: string): Promise<StoredScout | undefined> {
  const { rows } = await db.query(`SELECT ${scoutColumns} FROM scouts WHERE id = ?`, [id]);
  return rows.length === 0 ? undefined : toScout(rows[0]);
}

const sameSchedule = (a: ScoutSchedule, b: ScoutSchedule): boolean =>
  a.kind === b.kind && a.everyHours === b.everyHours && a.dailyAt === b.dailyAt;
const scoutValues = (s: StoredScout): SqlValue[] => [
  s.name,
  s.savedSearchId,
  s.minMatchScore,
  s.maxApplicationsPerRun,
  s.source,
  s.isEnabled ? 1 : 0,
  s.schedule.kind,
  s.schedule.everyHours,
  s.schedule.dailyAt,
  s.scheduleSetAt,
];

// Fills the defaults a caller may omit (source local, ceiling 10), validates, and checks the saved search exists.
export async function createScout(
  db: Db,
  id: string,
  input: Scout,
  now: number,
): Promise<Result<StoredScout>> {
  const scout: Scout = {
    ...input,
    source: input.source === '' ? 'local' : input.source,
    schedule: normalizedSchedule(input.schedule),
  };
  if (scout.maxApplicationsPerRun === 0) scout.maxApplicationsPerRun = 10;
  const error = validateScout(scout);
  if (error !== '') return { ok: false, error };
  if ((await getSavedSearch(db, scout.savedSearchId)) === undefined)
    return { ok: false, error: "the scout's saved search could not be found" };
  const stored: StoredScout = {
    ...scout,
    id,
    scheduleSetAt: isScheduled(scout.schedule) ? now : null,
    lastRunAt: null,
    createdAt: now,
    updatedAt: now,
  };
  await db.execute(
    `INSERT INTO scouts (id, name, saved_search_id, min_match_score, max_applications_per_run, source, is_enabled, schedule_kind, schedule_every_hours, schedule_daily_at, schedule_set_at, last_run_at, created_at, updated_at)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)`,
    [id, ...scoutValues(stored), now, now],
  );
  return { ok: true, value: stored };
}

// What the user cannot change is carried over: when it last ran, and when its schedule was set unless the schedule is what changed.
export async function updateScout(
  db: Db,
  id: string,
  input: Scout,
  now: number,
): Promise<Result<StoredScout>> {
  const scout: Scout = { ...input, schedule: normalizedSchedule(input.schedule) };
  const error = validateScout(scout);
  if (error !== '') return { ok: false, error };
  if ((await getSavedSearch(db, scout.savedSearchId)) === undefined)
    return { ok: false, error: "the scout's saved search could not be found" };
  const prior = await getScout(db, id);
  if (prior === undefined) return { ok: false, error: 'scout not found' };
  let scheduleSetAt = prior.scheduleSetAt;
  if (!isScheduled(scout.schedule)) scheduleSetAt = null;
  else if (
    !sameSchedule(scout.schedule, normalizedSchedule(prior.schedule)) ||
    scheduleSetAt === null
  )
    scheduleSetAt = now;
  const stored: StoredScout = {
    ...scout,
    id,
    scheduleSetAt,
    lastRunAt: prior.lastRunAt,
    createdAt: prior.createdAt,
    updatedAt: now,
  };
  await db.execute(
    `UPDATE scouts SET name = ?, saved_search_id = ?, min_match_score = ?, max_applications_per_run = ?, source = ?, is_enabled = ?,
    schedule_kind = ?, schedule_every_hours = ?, schedule_daily_at = ?, schedule_set_at = ?, updated_at = ? WHERE id = ?`,
    [...scoutValues(stored), now, id],
  );
  return { ok: true, value: stored };
}

export async function markScoutRun(db: Db, id: string, at: number): Promise<void> {
  await db.execute('UPDATE scouts SET last_run_at = ?, updated_at = ? WHERE id = ?', [at, at, id]);
}

// Removes a scout with its runs and their trail.
export async function deleteScout(db: Db, id: string): Promise<Result<true>> {
  if ((await getScout(db, id)) === undefined) return { ok: false, error: 'scout not found' };
  await db.transaction([
    {
      sql: 'DELETE FROM scout_run_events WHERE run_id IN (SELECT id FROM scout_runs WHERE scout_id = ?)',
      params: [id],
    },
    { sql: 'DELETE FROM scout_runs WHERE scout_id = ?', params: [id] },
    { sql: 'DELETE FROM scouts WHERE id = ?', params: [id] },
  ]);
  return { ok: true, value: true };
}

const runColumns = 'id, scout_id, trigger, status, error, started_at, completed_at';
const toRun = (r: SqlValue[]): ScoutRun => ({
  id: String(r[0]),
  scoutId: String(r[1]),
  trigger: String(r[2]),
  status: String(r[3]) as RunStatus,
  error: String(r[4]),
  startedAt: Number(r[5]),
  completedAt: nullable(r[6]),
});

export async function startRun(
  db: Db,
  run: { id: string; scoutId: string; trigger: RunTrigger },
  now: number,
): Promise<ScoutRun> {
  await db.execute(
    `INSERT INTO scout_runs (${runColumns}) VALUES (?, ?, ?, 'running', '', ?, NULL)`,
    [run.id, run.scoutId, run.trigger, now],
  );
  return { ...run, status: 'running', error: '', startedAt: now, completedAt: null };
}

export async function finishRun(
  db: Db,
  id: string,
  status: RunStatus,
  error: string,
  now: number,
): Promise<void> {
  await db.execute('UPDATE scout_runs SET status = ?, error = ?, completed_at = ? WHERE id = ?', [
    status,
    error,
    now,
    id,
  ]);
}

export async function getRun(db: Db, id: string): Promise<ScoutRun | undefined> {
  const { rows } = await db.query(`SELECT ${runColumns} FROM scout_runs WHERE id = ?`, [id]);
  return rows.length === 0 ? undefined : toRun(rows[0]);
}

// Recent runs, newest first.
export async function listRuns(db: Db, limit: number): Promise<ScoutRun[]> {
  const { rows } = await db.query(
    `SELECT ${runColumns} FROM scout_runs ORDER BY started_at DESC, rowid DESC LIMIT ?`,
    [limit],
  );
  return rows.map(toRun);
}

// Appends one step; the event's id is the run's id and its sequence number.
export async function addEvent(db: Db, event: Omit<RunEvent, 'id'>): Promise<void> {
  await db.execute(
    `INSERT INTO scout_run_events (id, run_id, seq, action, status, input_summary, output_summary, policy_result, created_at)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
    [
      `${event.runId}:${event.seq}`,
      event.runId,
      event.seq,
      event.action,
      event.status,
      event.inputSummary,
      event.outputSummary,
      event.policyResult,
      event.createdAt,
    ],
  );
}

// What one run did, in order.
export async function listRunEvents(db: Db, runId: string): Promise<RunEvent[]> {
  const { rows } = await db.query(
    'SELECT id, run_id, seq, action, status, input_summary, output_summary, policy_result, created_at FROM scout_run_events WHERE run_id = ? ORDER BY seq ASC',
    [runId],
  );
  return rows.map((r) => ({
    id: String(r[0]),
    runId: String(r[1]),
    seq: Number(r[2]),
    action: String(r[3]),
    status: String(r[4]),
    inputSummary: String(r[5]),
    outputSummary: String(r[6]),
    policyResult: String(r[7]),
    createdAt: Number(r[8]),
  }));
}
