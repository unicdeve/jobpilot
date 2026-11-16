import { describe, expect, test } from 'bun:test';
import { memoryDb } from '../shared/test-db.ts';
import { apiKeyRef, hostedAt, isLoopbackHost, validateLLMSettings } from './llm-settings.ts';
import { ensureSettingsTable, hasAPIKey, loadDiscoverySettings, loadLLMSettings, loadNotificationSettings, saveLLMSettings, saveNotificationSettings } from './llm-settings-store.ts';

describe('llm settings', () => {
  test('isLoopbackHost', () => {
    const want: Record<string, boolean> = { localhost: true, LOCALHOST: true, 'localhost.': true, 'app.localhost': true, '127.0.0.1': true, '127.1.2.3': true,
      '::1': true, '[::1]': true, '0.0.0.0': false, '192.168.1.10': false, 'api.openai.com': false, 'localhost.evil': false, '127.0.0.1.nip.io': false, '': false };
    for (const [h, w] of Object.entries(want)) expect(isLoopbackHost(h)).toBe(w);
  });
  test('hostedAt', () => {
    const want: Record<string, string> = { 'https://api.openai.com/v1': 'api.openai.com', 'https://openrouter.ai/api/v1': 'openrouter.ai',
      'http://192.168.1.20:8080/v1': '192.168.1.20', 'http://localhost:11434': '', 'http://127.0.0.1:8080/v1': '', 'http://[::1]:1234/v1': '', '': '', 'not a url': '' };
    for (const [u, w] of Object.entries(want)) expect(hostedAt({ provider: 'x', model: 'm', baseUrl: u })).toBe(w);
  });
  test('apiKeyRef', () => {
    expect(apiKeyRef({ provider: 'p', model: 'm', baseUrl: '' })).toBeNull();
    expect(apiKeyRef({ provider: 'p', model: 'm', baseUrl: '', apiKeyLabel: 'or' })).toEqual({ source: 'llm', label: 'or' });
  });
  test('validate', () => {
    const ok = { provider: 'openai', model: 'm', baseUrl: 'https://api.openai.com/v1' };
    expect(validateLLMSettings(ok)).toBeNull();
    expect(validateLLMSettings({ ...ok, provider: '' })).toContain('provider');
    expect(validateLLMSettings({ ...ok, model: '' })).toContain('model');
    expect(validateLLMSettings({ ...ok, baseUrl: 'ftp://x' })).toContain('http or https');
    expect(validateLLMSettings({ ...ok, baseUrl: 'nope' })).toContain('absolute');
    expect(validateLLMSettings({ ...ok, provider: 'ollama', apiKeyLabel: 'k' })).toContain('Ollama');
    expect(validateLLMSettings({ ...ok, baseUrl: 'http://example.com', apiKeyLabel: 'k' })).toContain('https');
    expect(validateLLMSettings({ ...ok, baseUrl: 'http://localhost:8080', apiKeyLabel: 'k' })).toBeNull();
  });
  test('store round trip', async () => {
    const db = memoryDb(); await ensureSettingsTable(db);
    expect(await loadLLMSettings(db)).toBeUndefined();
    expect(await saveLLMSettings(db, { provider: '', model: '', baseUrl: '' })).not.toBeNull();
    const v = { provider: 'openai', model: 'm', baseUrl: 'https://x.io/v1', apiKeyLabel: 'k' };
    expect(await saveLLMSettings(db, v)).toBeNull();
    expect(await loadLLMSettings(db)).toEqual(v);
    expect(hasAPIKey(v)).toBe(true);
    expect((await loadDiscoverySettings(db)).refreshOnLaunch).toBe(true);
    expect(await saveNotificationSettings(db, { enabled: true, strongMatchScore: 0 })).not.toBeNull();
    expect(await saveNotificationSettings(db, { enabled: false, strongMatchScore: 70 })).toBeNull();
    expect(await loadNotificationSettings(db)).toEqual({ enabled: false, strongMatchScore: 70 });
  });
});
