// A Db over bun:sqlite, for tests only (the app uses Exact's storage.sqlite). Not imported by app code.
import { Database } from 'bun:sqlite';
import type { Db, SqlValue } from './db.ts';

export function memoryDb(): Db {
  const sqlite = new Database(':memory:', { safeIntegers: true });
  return {
    async execute(sql, params = []) { sqlite.query(sql).run(...params); return undefined; },
    async query(sql, params = []) { return { rows: sqlite.query(sql).values(...params) as SqlValue[][] }; },
    async transaction(statements) {
      sqlite.exec('BEGIN');
      try {
        for (const s of statements) sqlite.query(s.sql).run(...(s.params ?? []));
        sqlite.exec('COMMIT');
      } catch (e) { sqlite.exec('ROLLBACK'); throw e; }
      return undefined;
    },
  };
}
