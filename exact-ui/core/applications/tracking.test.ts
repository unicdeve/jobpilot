import { expect, test } from 'bun:test';
import { applicationId, move, newApplication, setApplyLink } from './tracking.ts';

const job = { id: '7', title: ' Backend ', company: 'Acme ', url: 'https://acme.test/7' };

test('a new application starts DISCOVERED, one per job', () => {
  const a = newApplication(job, 1000)!;
  expect(a).toMatchObject({ id: applicationId('7'), state: 'DISCOVERED', title: 'Backend', company: 'Acme', applyUrl: 'https://acme.test/7' });
  expect(newApplication(job, 2000)!.id).toBe(a.id);
  expect(newApplication({ ...job, title: ' ' }, 1)).toBeUndefined();
});
test('a job page that is not a web address is not kept as the link', () => {
  expect(newApplication({ ...job, url: 'javascript:alert(1)' }, 1)!.applyUrl).toBe('');
});
test('a move follows the rule table and records its reason', () => {
  const a = newApplication(job, 1)!;
  const m = move(a, 'MATCHED', ' fits ', 5, 1);
  expect(m.ok && m.application.state).toBe('MATCHED');
  expect(m.ok && m.step).toMatchObject({ from: 'DISCOVERED', to: 'MATCHED', reason: 'fits', at: 5 });
  const bad = move(a, 'SUBMITTED', '', 5, 1);
  expect(bad.ok).toBe(false);
});
test('the apply link must be a web address; empty clears it', () => {
  const a = newApplication(job, 1)!;
  expect(setApplyLink(a, 'ftp://x', 2).ok).toBe(false);
  expect(setApplyLink(a, 'nonsense', 2).ok).toBe(false);
  const ok = setApplyLink(a, ' https://jobs.test/apply ', 2);
  expect(ok.ok && ok.application.applyUrl).toBe('https://jobs.test/apply');
  const cleared = setApplyLink(a, '', 2);
  expect(cleared.ok && cleared.application.applyUrl).toBe('');
});
