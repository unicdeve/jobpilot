// Resumes in the device's SQLite (ADR-0037), the schema of the Go core's resumes table.
// Times are epoch milliseconds, passed in by the caller.
import type { Db, SqlValue } from '../shared/db.ts';
import type { Resume } from './resume.ts';

export const resumeSchema = `CREATE TABLE IF NOT EXISTS resumes (
  id TEXT PRIMARY KEY,
  candidate_id TEXT NOT NULL,
  name TEXT NOT NULL,
  original_file_path TEXT NOT NULL DEFAULT '',
  markdown_content TEXT NOT NULL DEFAULT '',
  parsed_metadata TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
)`;

export const ensureResumeSchema = (db: Db): Promise<unknown> => db.execute(resumeSchema);

const columns =
  'id, candidate_id, name, original_file_path, markdown_content, parsed_metadata, created_at, updated_at';

const fromRow = (r: SqlValue[]): Resume => ({
  id: String(r[0]),
  candidateId: String(r[1]),
  name: String(r[2]),
  originalFilePath: String(r[3] ?? ''),
  markdownContent: String(r[4] ?? ''),
  parsedMetadata: String(r[5] ?? ''),
  createdAt: Number(r[6]),
  updatedAt: Number(r[7]),
});

export async function saveResume(db: Db, r: Resume): Promise<void> {
  await db.execute(`INSERT INTO resumes (${columns}) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, [
    r.id,
    r.candidateId,
    r.name,
    r.originalFilePath,
    r.markdownContent,
    r.parsedMetadata,
    r.createdAt,
    r.updatedAt,
  ]);
}

export async function getResume(db: Db, id: string): Promise<Resume | undefined> {
  const { rows } = await db.query(`SELECT ${columns} FROM resumes WHERE id = ?`, [id]);
  return rows[0] ? fromRow(rows[0]) : undefined;
}

// Oldest first.
export async function resumesOf(db: Db, candidateId: string): Promise<Resume[]> {
  const { rows } = await db.query(
    `SELECT ${columns} FROM resumes WHERE candidate_id = ? ORDER BY created_at, rowid`,
    [candidateId],
  );
  return rows.map(fromRow);
}

export const deleteResume = (db: Db, id: string): Promise<unknown> =>
  db.execute('DELETE FROM resumes WHERE id = ?', [id]);
