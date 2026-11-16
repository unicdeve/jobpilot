// The slice of Exact's storage.sqlite database that the on-device core uses (ADR-0037).
// Core storage modules take a Db, so they run in the app and, through test-db.ts, under `bun test`.
// A data source cannot read the clock, so every time a module stores is passed in by the caller.
export type SqlValue = string | number | null;

export interface Db {
  execute(sql: string, params?: SqlValue[]): Promise<unknown>;
  query(sql: string, params?: SqlValue[]): Promise<{ rows: SqlValue[][] }>;
  transaction(statements: { sql: string; params?: SqlValue[] }[]): Promise<unknown>;
}
