// What the candidate said about themselves, on the device (ADR-0037): the About you answers and the answers they kept
// (Go's AboutYouService and personal_answers table). Only the candidate writes an answer (ADR-0034 section 5); nothing
// here writes one. A kept answer is stored from the text the candidate wrote to an application question; the caller
// reads that question. Times are epoch milliseconds, passed in.
import type { Db, SqlValue } from '../shared/db.ts';
import {
  isStandingKey,
  standingQuestions,
  type PersonalAnswer,
  type PersonalAnswerKey,
  type StandingQuestion,
} from './personal-answer.ts';

export const personalAnswerSchema: string[] = [
  `CREATE TABLE IF NOT EXISTS personal_answers (
    id TEXT PRIMARY KEY, candidate_id TEXT NOT NULL, key TEXT NOT NULL,
    question TEXT NOT NULL DEFAULT '', answer TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
  )`,
  'CREATE INDEX IF NOT EXISTS idx_personal_answers_candidate ON personal_answers(candidate_id)',
];

export async function ensurePersonalAnswerSchema(db: Db): Promise<void> {
  for (const sql of personalAnswerSchema) await db.execute(sql);
}

export interface AboutYouAnswer extends StandingQuestion {
  answer: string;
}
export interface AboutYou {
  questions: AboutYouAnswer[];
  kept: PersonalAnswer[];
}

const fromRow = (r: SqlValue[]): PersonalAnswer => ({
  id: String(r[0]),
  candidateId: String(r[1]),
  key: String(r[2]) as PersonalAnswerKey,
  question: String(r[3] ?? ''),
  answer: String(r[4] ?? ''),
  updatedAt: Number(r[5]),
});

// Oldest first.
export async function listAnswers(db: Db, candidateId: string): Promise<PersonalAnswer[]> {
  const { rows } = await db.query(
    'SELECT id, candidate_id, key, question, answer, updated_at FROM personal_answers WHERE candidate_id = ? ORDER BY created_at, rowid',
    [candidateId],
  );
  return rows.map(fromRow);
}

export function aboutYouFrom(rows: PersonalAnswer[]): AboutYou {
  const byKey = new Map<string, string>();
  const kept: PersonalAnswer[] = [];
  for (const row of rows) {
    if (row.key === 'kept') kept.push(row);
    else byKey.set(row.key ?? '', row.answer);
  }
  return {
    questions: standingQuestions.map((q) => ({ ...q, answer: byKey.get(q.key) ?? '' })),
    kept,
  };
}

export async function getAboutYou(db: Db, candidateId: string): Promise<AboutYou> {
  return aboutYouFrom(await listAnswers(db, candidateId));
}

export type SavedAbout = { ok: true; about: AboutYou } | { ok: false; error: string };

// A question left out of `answers` is unchanged; an empty answer clears it (ADR-0022 section 2).
// Every key is checked before anything is written, so an unknown one changes nothing.
export async function saveAboutYou(
  db: Db,
  candidateId: string,
  answers: Record<string, string>,
  now: number,
  newId: () => string,
): Promise<SavedAbout> {
  for (const key of Object.keys(answers))
    if (!isStandingKey(key)) return { ok: false, error: `No such question: "${key}".` };
  const rows = await listAnswers(db, candidateId);
  const existing = new Map(rows.filter((r) => r.key !== 'kept').map((r) => [r.key, r]));
  const st: { sql: string; params?: SqlValue[] }[] = [];
  for (const q of standingQuestions) {
    const answer = answers[q.key];
    if (answer === undefined) continue;
    const row = existing.get(q.key);
    if (answer.trim() === '') {
      if (row) st.push({ sql: 'DELETE FROM personal_answers WHERE id = ?', params: [row.id] });
    } else if (row) {
      st.push({
        sql: 'UPDATE personal_answers SET question = ?, answer = ?, updated_at = ? WHERE id = ?',
        params: [q.question, answer, now, row.id],
      });
    } else {
      st.push({
        sql: 'INSERT INTO personal_answers (id, candidate_id, key, question, answer, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)',
        params: [newId(), candidateId, q.key, q.question, answer, now, now],
      });
    }
  }
  if (st.length > 0) await db.transaction(st);
  return { ok: true, about: await getAboutYou(db, candidateId) };
}

export type Kept = { ok: true; answer: PersonalAnswer } | { ok: false; error: string };

// Keeps the answer the candidate wrote to a question, so a similar question later is offered it. Keeping the same
// question again (case-insensitively) replaces what was kept for it.
export async function keepAnswer(
  db: Db,
  candidateId: string,
  label: string,
  answer: string,
  now: number,
  newId: () => string,
): Promise<Kept> {
  if (answer.trim() === '') return { ok: false, error: 'Only an answered question can be kept.' };
  const question = label.trim();
  const rows = await listAnswers(db, candidateId);
  const same = rows.find(
    (r) => r.key === 'kept' && r.question.trim().toLowerCase() === question.toLowerCase(),
  );
  if (same) {
    await db.execute(
      'UPDATE personal_answers SET question = ?, answer = ?, updated_at = ? WHERE id = ?',
      [question, answer, now, same.id],
    );
    return { ok: true, answer: { ...same, question, answer, updatedAt: now } };
  }
  const id = newId();
  await db.execute(
    'INSERT INTO personal_answers (id, candidate_id, key, question, answer, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)',
    [id, candidateId, 'kept', question, answer, now, now],
  );
  return { ok: true, answer: { id, candidateId, key: 'kept', question, answer, updatedAt: now } };
}

// Forgets a kept answer. One that is not the candidate's kept answer is refused; one already gone is not an error.
export async function deleteKept(
  db: Db,
  candidateId: string,
  id: string,
): Promise<{ ok: true } | { ok: false; error: string }> {
  const { rows } = await db.query('SELECT candidate_id, key FROM personal_answers WHERE id = ?', [
    id,
  ]);
  const row = rows[0];
  if (row && (row[1] !== 'kept' || row[0] !== candidateId))
    return { ok: false, error: 'That is not one of your kept answers.' };
  await db.execute('DELETE FROM personal_answers WHERE id = ?', [id]);
  return { ok: true };
}

export async function keptAnswers(db: Db, candidateId: string): Promise<PersonalAnswer[]> {
  return (await getAboutYou(db, candidateId)).kept;
}

// What a draft's claims may rest on (draft-check): the About you answers, then the kept answers.
export async function ownWords(db: Db, candidateId: string): Promise<string[]> {
  const about = await getAboutYou(db, candidateId);
  return [
    ...about.questions.filter((q) => q.answer !== '').map((q) => q.answer),
    ...about.kept.map((a) => a.answer),
  ];
}
