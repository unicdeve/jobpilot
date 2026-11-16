import { expect, test } from 'bun:test';
import { fetchRemotive, stripHtml, toJob } from './remotive.ts';

const item = {
  id: 7,
  url: 'https://remotive.com/j/7',
  title: 'Go Dev',
  company_name: 'Acme',
  tags: ['go'],
  job_type: 'full_time',
  publication_date: '2026-09-07T01:10:43',
  candidate_required_location: 'Europe',
  description: '<p>Build &amp; ship</p>',
};
test('a Remotive item becomes a job', () => {
  expect(toJob(item)).toMatchObject({
    sourceId: 'remotive',
    sourceJobId: '7',
    remoteType: 'remote',
    locations: ['Europe'],
    descriptionMarkdown: 'Build & ship',
    postedAt: '2026-09-07T01:10:43Z',
    skills: ['go'],
  });
  expect(toJob({ title: 'no id' })).toBeUndefined();
  expect(stripHtml('a<br/>b')).toBe('a\nb');
});
test('the listing is fetched whole, and a bad answer is an error', async () => {
  const ok = async () => ({
    ok: true,
    status: 200,
    json: async () => ({ jobs: [item, { nope: 1 }] }),
  });
  expect(await fetchRemotive(ok)).toHaveLength(1);
  await expect(
    fetchRemotive(async () => ({ ok: false, status: 503, json: async () => ({}) })),
  ).rejects.toThrow('503');
  await expect(
    fetchRemotive(async () => ({ ok: true, status: 200, json: async () => ({}) })),
  ).rejects.toThrow('decode');
});
