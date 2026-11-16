import { expect, test } from 'bun:test';
import {
  checkModelForm,
  dataNotice,
  hasKey,
  parseKeys,
  withKey,
  withoutKey,
} from './model-settings.ts';

const ok = {
  provider: 'openai',
  baseUrl: 'https://api.openai.com/v1',
  model: 'gpt',
  keyLabel: 'work',
  apiKey: 'sk-1',
};

test('a key is refused over plain http unless the host is this device', () => {
  expect(checkModelForm({ ...ok, baseUrl: 'http://models.example.com/v1' })).toMatchObject({
    ok: false,
  });
  expect(checkModelForm({ ...ok, baseUrl: 'http://192.168.1.5:11434' })).toMatchObject({
    ok: false,
  });
  expect(checkModelForm({ ...ok, baseUrl: 'http://127.0.0.1:8080/v1' })).toMatchObject({
    ok: true,
  });
  expect(checkModelForm(ok)).toMatchObject({ ok: true, newKey: 'sk-1' });
});
test('a typed key needs a label, Ollama takes none, and the basics are required', () => {
  expect(checkModelForm({ ...ok, keyLabel: '' })).toMatchObject({ ok: false });
  expect(
    checkModelForm({ ...ok, provider: 'ollama', baseUrl: 'http://127.0.0.1:11434' }),
  ).toMatchObject({ ok: false });
  expect(checkModelForm({ ...ok, model: '' })).toMatchObject({ ok: false });
  expect(checkModelForm({ ...ok, keyLabel: '', apiKey: '' })).toMatchObject({ ok: true });
});
test('keys live in one JSON object by label, and a bad secret reads as empty', () => {
  const a = withKey(null, 'work', 'k1');
  const b = withKey(a, 'home', 'k2');
  expect(parseKeys(b)).toEqual({ work: 'k1', home: 'k2' });
  expect(hasKey(b, 'home')).toBe(true);
  expect(parseKeys(withoutKey(b, 'work'))).toEqual({ home: 'k2' });
  expect(parseKeys('not json')).toEqual({});
  expect(hasKey(null, 'work')).toBe(false);
});
test('the notice names a host that is not this device', () => {
  expect(dataNotice({ baseUrl: 'https://api.openai.com/v1' })).toContain('api.openai.com');
  expect(dataNotice({ baseUrl: 'http://localhost:11434' })).toBe('');
});
