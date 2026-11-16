// The watched companies in the device's SQLite (ADR-0024 section 2, ADR-0037): the schema of Go's watched_companies table.
// Times are epoch milliseconds, passed in by the caller. A name is compared ignoring case, as Go's unique index does.
import type { Db, SqlValue } from '../shared/db.ts';
import { boardUrl, type Board, type Provider } from '../sources/company-boards.ts';

export type BoardCheck = 'ok' | 'gone' | 'unreachable' | 'failed';
export interface WatchedCompany {
  id: string;
  board: Board;
  name: string;
  url: string;
  addedFrom: string;
  addedAt: number;
  checkedAt?: number;
  // How the last check went; '' before the first.
  check: BoardCheck | '';
  openRoles: number;
}

export const watchSchema = [
  `CREATE TABLE IF NOT EXISTS watched_companies (
  id TEXT PRIMARY KEY, provider TEXT NOT NULL, region TEXT NOT NULL DEFAULT '', board TEXT NOT NULL, name TEXT NOT NULL,
  added_from TEXT NOT NULL, added_at INTEGER NOT NULL, checked_at INTEGER, check_status TEXT NOT NULL DEFAULT '', open_roles INTEGER NOT NULL DEFAULT 0)`,
  'CREATE UNIQUE INDEX IF NOT EXISTS idx_watched_companies_board ON watched_companies(provider, region, board COLLATE NOCASE)',
];
export async function ensureWatchSchema(db: Db): Promise<void> {
  for (const s of watchSchema) await db.execute(s);
}

const columns =
  'id, provider, region, board, name, added_from, added_at, checked_at, check_status, open_roles';
function fromRow(r: SqlValue[]): WatchedCompany {
  const board: Board = {
    provider: String(r[1]) as Provider,
    ...(String(r[2]) === 'eu' ? { region: 'eu' as const } : {}),
    name: String(r[3]),
  };
  return {
    id: String(r[0]),
    board,
    name: String(r[4]),
    url: boardUrl(board),
    addedFrom: String(r[5]),
    addedAt: Number(r[6]),
    ...(r[7] === null ? {} : { checkedAt: Number(r[7]) }),
    check: String(r[8]) as BoardCheck | '',
    openRoles: Number(r[9]),
  };
}

export async function getByBoard(db: Db, b: Board): Promise<WatchedCompany | undefined> {
  const { rows } = await db.query(
    `SELECT ${columns} FROM watched_companies WHERE provider = ? AND region = ? AND board = ? COLLATE NOCASE`,
    [b.provider, b.region ?? '', b.name],
  );
  return rows[0] ? fromRow(rows[0]) : undefined;
}
export async function getCompany(db: Db, id: string): Promise<WatchedCompany | undefined> {
  const { rows } = await db.query(`SELECT ${columns} FROM watched_companies WHERE id = ?`, [id]);
  return rows[0] ? fromRow(rows[0]) : undefined;
}

// Stores a company, or returns the one already watching its board.
export async function createCompany(
  db: Db,
  c: { id: string; board: Board; name: string; addedFrom: string; addedAt: number },
): Promise<WatchedCompany> {
  await db.execute(
    'INSERT INTO watched_companies (id, provider, region, board, name, added_from, added_at) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(provider, region, board COLLATE NOCASE) DO NOTHING',
    [c.id, c.board.provider, c.board.region ?? '', c.board.name, c.name, c.addedFrom, c.addedAt],
  );
  return (await getByBoard(db, c.board))!;
}

// By name, ignoring case.
export async function listCompanies(db: Db): Promise<WatchedCompany[]> {
  const { rows } = await db.query(
    `SELECT ${columns} FROM watched_companies ORDER BY name COLLATE NOCASE, id`,
  );
  return rows.map(fromRow);
}
export const renameCompany = (db: Db, id: string, name: string): Promise<unknown> =>
  db.execute('UPDATE watched_companies SET name = ? WHERE id = ?', [name, id]);
export const recordCheck = (
  db: Db,
  id: string,
  at: number,
  check: BoardCheck,
  openRoles: number,
): Promise<unknown> =>
  db.execute(
    'UPDATE watched_companies SET checked_at = ?, check_status = ?, open_roles = ? WHERE id = ?',
    [at, check, openRoles, id],
  );
export const deleteCompany = (db: Db, id: string): Promise<unknown> =>
  db.execute('DELETE FROM watched_companies WHERE id = ?', [id]);
