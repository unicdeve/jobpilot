// Storing the non-secret settings on the device (ADR-0037) in a key/value table. Takes a Db; never reads the clock.
// The API key is NOT here: only `apiKeyLabel`, which names a credential in secure storage (see llm-settings.ts).
import type { Db } from '../shared/db.ts';
import { defaultDiscoverySettings, validateLLMSettings, type DiscoverySettings, type LLMSettings } from './llm-settings.ts';
import { defaultNotificationSettings, validateNotificationSettings, type NotificationSettings } from '../activity/notification.ts';

export const settingsSchema = 'CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)';
export const ensureSettingsTable = (db: Db): Promise<unknown> => db.execute(settingsSchema);

async function read<T>(db: Db, key: string): Promise<T | undefined> {
  const { rows } = await db.query('SELECT value FROM settings WHERE key = ?', [key]);
  if (rows.length === 0) return undefined;
  try { return JSON.parse(String(rows[0][0])) as T; } catch { return undefined; }
}
const write = (db: Db, key: string, v: unknown) =>
  db.execute('INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value', [key, JSON.stringify(v)]);

export const loadLLMSettings = (db: Db): Promise<LLMSettings | undefined> => read<LLMSettings>(db, 'llm');
// Returns the validation message, or null once saved.
export async function saveLLMSettings(db: Db, v: LLMSettings): Promise<string | null> {
  const err = validateLLMSettings(v);
  if (err) return err;
  await write(db, 'llm', v);
  return null;
}
export const hasAPIKey = (v: LLMSettings | undefined): boolean => !!v?.apiKeyLabel;

export const loadDiscoverySettings = async (db: Db): Promise<DiscoverySettings> => ({ ...defaultDiscoverySettings(), ...(await read<DiscoverySettings>(db, 'discovery')) });
export const saveDiscoverySettings = (db: Db, v: DiscoverySettings) => write(db, 'discovery', v);

export const loadNotificationSettings = async (db: Db): Promise<NotificationSettings> => ({ ...defaultNotificationSettings(), ...(await read<NotificationSettings>(db, 'notifications')) });
export async function saveNotificationSettings(db: Db, v: NotificationSettings): Promise<string | null> {
  const err = validateNotificationSettings(v);
  if (err) return err;
  await write(db, 'notifications', v);
  return null;
}
