// Storing agent runs and events on the device (ADR-0037). Takes a Db and never reads the clock: `now` is passed in.
// The run recorder is what the scout run loop calls: startRun, appendEvent, finishRun; the Activity screen calls listRuns/getRun.
import type { Db, SqlValue } from '../shared/db.ts';
import {
  agentTaskScoutRun,
  validAgentRunStatus,
  type AgentEvent,
  type AgentRun,
  type AgentRunDetail,
  type AgentRunFilter,
  type AgentRunPage,
  type AgentRunStatus,
  type AgentRunSummary,
  type RunTrigger,
} from './agent-run.ts';

// A run keeps its scout_id after the scout goes (detach); the name is looked up from `scouts` when it exists.
export const agentRunSchema: string[] = [
  `CREATE TABLE IF NOT EXISTS agent_runs (id TEXT PRIMARY KEY, scout_id TEXT, trigger TEXT, task_type TEXT NOT NULL, status TEXT NOT NULL,
    started_at INTEGER NOT NULL, completed_at INTEGER, error TEXT)`,
  `CREATE TABLE IF NOT EXISTS agent_events (id TEXT PRIMARY KEY, run_id TEXT NOT NULL, seq INTEGER NOT NULL, action TEXT NOT NULL, tool TEXT NOT NULL,
    status TEXT NOT NULL, input_summary TEXT, output_summary TEXT, policy_result TEXT, duration_ms INTEGER NOT NULL, created_at INTEGER NOT NULL)`,
  `CREATE INDEX IF NOT EXISTS agent_events_run ON agent_events (run_id, seq)`,
];
export const ensureAgentRunTables = async (db: Db): Promise<void> => {
  for (const s of agentRunSchema) await db.execute(s);
};

const s = (v: SqlValue): string | undefined => (v === null || v === '' ? undefined : String(v));
const runOf = (r: SqlValue[]): AgentRun => ({
  id: String(r[0]),
  scoutId: s(r[1]),
  trigger: s(r[2]) as RunTrigger | undefined,
  taskType: String(r[3]),
  status: String(r[4]) as AgentRunStatus,
  startedAt: Number(r[5]),
  completedAt: r[6] === null ? undefined : Number(r[6]),
  error: s(r[7]),
});
const eventOf = (r: SqlValue[]): AgentEvent => ({
  id: String(r[0]),
  runId: String(r[1]),
  action: String(r[2]),
  tool: String(r[3]),
  status: String(r[4]),
  inputSummary: s(r[5]),
  outputSummary: s(r[6]),
  policyResult: s(r[7]),
  durationMs: Number(r[8]),
  createdAt: Number(r[9]),
});

// Starts a run as running. `id` is supplied by the caller (it cannot be generated without a clock or randomness here).
export async function startRun(
  db: Db,
  run: { id: string; scoutId?: string; trigger?: RunTrigger; taskType?: string },
  now: number,
): Promise<AgentRun> {
  const r: AgentRun = {
    id: run.id,
    scoutId: run.scoutId,
    trigger: run.trigger,
    taskType: run.taskType ?? agentTaskScoutRun,
    status: 'running',
    startedAt: now,
  };
  await db.execute(
    'INSERT INTO agent_runs (id, scout_id, trigger, task_type, status, started_at) VALUES (?,?,?,?,?,?)',
    [r.id, r.scoutId ?? null, r.trigger ?? null, r.taskType, r.status, now],
  );
  return r;
}

export type NewEvent = Omit<AgentEvent, 'id' | 'runId' | 'createdAt'>;

// Appends a step; its id is `${runId}:${seq}` and seq counts from 1, so events list in the order they happened.
export async function appendEvent(
  db: Db,
  runId: string,
  ev: NewEvent,
  now: number,
): Promise<AgentEvent> {
  const { rows } = await db.query(
    'SELECT COALESCE(MAX(seq), 0) FROM agent_events WHERE run_id = ?',
    [runId],
  );
  const seq = Number(rows[0]?.[0] ?? 0) + 1;
  const e: AgentEvent = { ...ev, id: `${runId}:${seq}`, runId, createdAt: now };
  await db.execute(
    'INSERT INTO agent_events (id, run_id, seq, action, tool, status, input_summary, output_summary, policy_result, duration_ms, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)',
    [
      e.id,
      runId,
      seq,
      e.action,
      e.tool,
      e.status,
      e.inputSummary ?? null,
      e.outputSummary ?? null,
      e.policyResult ?? null,
      e.durationMs,
      now,
    ],
  );
  return e;
}

// Finishes a run once: a run already finished keeps its outcome. Returns false then, or when there is no such run.
export async function finishRun(
  db: Db,
  runId: string,
  status: Exclude<AgentRunStatus, 'running'>,
  error: string,
  now: number,
): Promise<boolean> {
  if (!validAgentRunStatus(status) || (status as string) === 'running') return false;
  const { rows } = await db.query('SELECT status FROM agent_runs WHERE id = ?', [runId]);
  if (rows.length === 0 || rows[0][0] !== 'running') return false;
  await db.execute('UPDATE agent_runs SET status = ?, completed_at = ?, error = ? WHERE id = ?', [
    status,
    now,
    status === 'failed' && error ? error : null,
    runId,
  ]);
  return true;
}

const SUMMARY = `SELECT r.id, r.scout_id, r.trigger, r.task_type, r.status, r.started_at, r.completed_at, r.error,
  (SELECT name FROM scouts WHERE scouts.id = r.scout_id), (SELECT COUNT(*) FROM agent_events e WHERE e.run_id = r.id) FROM agent_runs r`;

async function summaries(
  db: Db,
  where: string,
  params: SqlValue[],
  tail = '',
): Promise<AgentRunSummary[]> {
  let rows: SqlValue[][];
  try {
    rows = (await db.query(`${SUMMARY} ${where} ${tail}`, params)).rows;
  } catch {
    // no scouts table yet: names are simply unknown
    rows = (
      await db.query(
        `${SUMMARY.replace('(SELECT name FROM scouts WHERE scouts.id = r.scout_id)', 'NULL')} ${where} ${tail}`,
        params,
      )
    ).rows;
  }
  return rows.map((r) => ({ ...runOf(r), scoutName: s(r[8]), eventCount: Number(r[9]) }));
}

// The activity list, newest first.
export async function listRuns(db: Db, f: AgentRunFilter = {}): Promise<AgentRunPage> {
  const conds: string[] = [];
  const params: SqlValue[] = [];
  if (f.taskType) {
    conds.push('r.task_type = ?');
    params.push(f.taskType);
  }
  if (f.status) {
    conds.push('r.status = ?');
    params.push(f.status);
  }
  const where = conds.length ? `WHERE ${conds.join(' AND ')}` : '';
  const total = Number(
    (await db.query(`SELECT COUNT(*) FROM agent_runs r ${where}`, params)).rows[0][0],
  );
  const limit = f.limit && f.limit > 0 ? f.limit : -1;
  const runs = await summaries(
    db,
    where,
    [...params, limit, f.offset ?? 0],
    'ORDER BY r.started_at DESC, r.id DESC LIMIT ? OFFSET ?',
  );
  return { runs, total };
}

export async function getRun(db: Db, runId: string): Promise<AgentRunDetail | undefined> {
  const [run] = await summaries(db, 'WHERE r.id = ?', [runId]);
  if (!run) return undefined;
  const { rows } = await db.query(
    'SELECT id, run_id, action, tool, status, input_summary, output_summary, policy_result, duration_ms, created_at FROM agent_events WHERE run_id = ? ORDER BY seq',
    [runId],
  );
  return { run, events: rows.map(eventOf) };
}

// Runs interrupted by the app closing stay "running" forever; the app marks them failed at launch.
export async function failInterruptedRuns(db: Db, now: number): Promise<void> {
  await db.execute(
    `UPDATE agent_runs SET status = 'failed', completed_at = ?, error = 'interrupted' WHERE status = 'running'`,
    [now],
  );
}

// "Clear activity" (DeletionTarget activity): every run and its steps.
export async function clearActivity(db: Db): Promise<void> {
  await db.transaction([{ sql: 'DELETE FROM agent_events' }, { sql: 'DELETE FROM agent_runs' }]);
}
