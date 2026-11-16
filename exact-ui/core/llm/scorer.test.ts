import { expect, test } from 'bun:test';
import { emptyCandidate } from '../candidate/candidate.ts';
import type { RunJob } from '../scouts/scout-run.ts';
import { makeScorer, matchCandidateOf } from './scorer.ts';
import type { LLMFetch } from './llm-provider.ts';

const REPLY =
  '{"score":77,"confidence":0.8,"strengths":[],"gaps":[],"concerns":[],"matched_skills":[],"missing_skills":[],"rationale":"ok"}';
const job = {
  id: 'remotive:1',
  title: 'Go dev',
  companyName: 'Acme',
  descriptionMarkdown: 'Go.',
} as unknown as RunJob;
const withProfile = () => ({
  ...emptyCandidate('me', 0),
  summary: 'Backend engineer.',
  skills: [{ id: 's', name: 'Go', yearsExperience: null }],
});
const asked: { url: string; headers?: Record<string, string>; body?: string }[] = [];
const fetchOf =
  (content: string): LLMFetch =>
  async (url, init) => {
    asked.push({ url, headers: init?.headers, body: init?.body });
    const body = JSON.stringify({ choices: [{ message: { content } }], model: 'm' });
    return { ok: true, status: 200, text: async () => body };
  };

test('no model, no profile, or a profile that says nothing is scored by nobody', () => {
  const settings = {
    provider: 'custom',
    baseUrl: 'https://api.openai.com/v1',
    model: 'gpt',
    apiKeyLabel: 'k',
  };
  expect(makeScorer(undefined, null, withProfile(), fetchOf(REPLY))).toBeUndefined();
  expect(makeScorer(settings, null, undefined, fetchOf(REPLY))).toBeUndefined();
  expect(makeScorer(settings, null, emptyCandidate('me', 0), fetchOf(REPLY))).toBeUndefined();
});

test("a job is scored by the user's model with the key from llm-keys", async () => {
  asked.length = 0;
  const settings = {
    provider: 'custom',
    baseUrl: 'https://api.openai.com/v1',
    model: 'gpt',
    apiKeyLabel: 'work',
  };
  const score = makeScorer(
    settings,
    JSON.stringify({ work: 'sk-secret' }),
    withProfile(),
    fetchOf(REPLY),
  )!;
  expect(await score(job)).toBe(77);
  expect(asked[0]!.url).toStartWith('https://api.openai.com/v1');
  expect(JSON.stringify(asked[0]!.headers)).toContain('sk-secret');
  expect(asked[0]!.body).not.toContain('sk-secret');
});

test('a reply that cannot be read is a job that could not be scored, never a score', async () => {
  const settings = {
    provider: 'custom',
    baseUrl: 'https://api.openai.com/v1',
    model: 'gpt',
    apiKeyLabel: 'work',
  };
  const score = makeScorer(settings, '{"work":"k"}', withProfile(), fetchOf('no json here'))!;
  await expect(score(job)).rejects.toThrow('failed to parse match response');
});

test('the profile reaches the prompt as entered, dates as YYYY-MM-DD', () => {
  const c = {
    ...withProfile(),
    experiences: [
      {
        id: 'e',
        company: 'Acme',
        title: 'Eng',
        startDate: { year: 2019, month: 3, day: 1 },
        endDate: null,
        isCurrent: true,
        description: 'x',
      },
    ],
  };
  expect(matchCandidateOf(c).experiences[0]).toMatchObject({
    startDate: '2019-03-01',
    endDate: null,
  });
});
