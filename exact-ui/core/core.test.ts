import { expect, test } from 'bun:test';
import { emptyCriteria, matches, searchTerms, type Job } from './criteria.ts';
import { fetchRemotive, stripHtml, toJob } from './remotive.ts';

const job: Job = { title: 'Engineer, Backend (Go)', companyName: 'Acme', descriptionMarkdown: 'Build services.', locations: ['Berlin'],
  remoteType: 'remote', employmentType: 'full_time', salaryMin: 0, salaryMax: 0 };
const c = (over: Partial<typeof emptyCriteria>) => ({ ...emptyCriteria, ...over });

test('terms are matched in any order, as the Go rule does', () => {
  expect(searchTerms('  Backend  Engineer ')).toEqual(['backend', 'engineer']);
  expect(matches(c({ title: 'backend engineer' }), job)).toBe(true);
  expect(matches(c({ title: 'frontend' }), job)).toBe(false);
});
test('keywords read the description too', () => {
  expect(matches(c({ keywords: 'services' }), job)).toBe(true);
  expect(matches(c({ keywords: 'kubernetes' }), job)).toBe(false);
});
test('an unset criterion excludes nothing; a set one compares case-insensitively', () => {
  expect(matches(emptyCriteria, job)).toBe(true);
  expect(matches(c({ remotePreference: 'REMOTE' }), job)).toBe(true);
  expect(matches(c({ remotePreference: 'onsite' }), job)).toBe(false);
});
test('a job with no salary survives a salary bound, one with a salary is judged', () => {
  expect(matches(c({ salaryMin: 100000 }), job)).toBe(true);
  expect(matches(c({ salaryMin: 100000 }), { ...job, salaryMin: 50000, salaryMax: 60000 })).toBe(false);
  expect(matches(c({ salaryMin: 55000 }), { ...job, salaryMin: 50000, salaryMax: 60000 })).toBe(true);
  expect(matches(c({ salaryMax: 40000 }), { ...job, salaryMin: 50000, salaryMax: 60000 })).toBe(false);
});
test('an excluded company is left out, ignoring case and spaces', () => {
  expect(matches(c({ excludedCompanies: [' acme '] }), job)).toBe(false);
});

const item = { id: 7, url: 'https://remotive.com/j/7', title: 'Go Dev', company_name: 'Acme', tags: ['go'], job_type: 'full_time',
  publication_date: '2026-09-07T01:10:43', candidate_required_location: 'Europe', description: '<p>Build &amp; ship</p>' };
test('a Remotive item becomes a job', () => {
  expect(toJob(item)).toMatchObject({ sourceId: 'remotive', sourceJobId: '7', remoteType: 'remote', locations: ['Europe'],
    descriptionMarkdown: 'Build & ship', postedAt: '2026-09-07T01:10:43Z', skills: ['go'] });
  expect(toJob({ title: 'no id' })).toBeUndefined();
  expect(stripHtml('a<br/>b')).toBe('a\nb');
});
test('the listing is fetched whole, and a bad answer is an error', async () => {
  const ok = async () => ({ ok: true, status: 200, json: async () => ({ jobs: [item, { nope: 1 }] }) });
  expect(await fetchRemotive(ok)).toHaveLength(1);
  await expect(fetchRemotive(async () => ({ ok: false, status: 503, json: async () => ({}) }))).rejects.toThrow('503');
  await expect(fetchRemotive(async () => ({ ok: true, status: 200, json: async () => ({}) }))).rejects.toThrow('decode');
});
