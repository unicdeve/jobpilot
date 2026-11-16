// The scout run's `score` on a phone (ADR-0038): the model the user named, the key from the one Keychain secret, and the
// profile the user entered. Nothing here invents a fact: a profile with no summary, skills or experience is scored by nobody,
// because a model asked to rate a candidate who said nothing would be rating the model's own guess (AGENTS.md principle 6).
import type { Candidate } from '../candidate/candidate.ts';
import type { RunJob } from '../scouts/scout-run.ts';
import { analyzeJob, type MatchCandidate } from './match.ts';
import type { LLMFetch, LLMProvider } from './llm-provider.ts';
import { parseKeys } from './model-settings.ts';
import type { LLMSettings } from './llm-settings.ts';
import { ollamaProvider } from './providers/ollama.ts';
import { openAICompatProvider, PROVIDER_CUSTOM } from './providers/openai-compat.ts';

const pad = (n: number): string => String(n).padStart(2, '0');

// The candidate as the prompt reads them: only what the user entered.
export function matchCandidateOf(c: Candidate): MatchCandidate {
  return {
    summary: c.summary,
    skills: c.skills.map((s) => ({ name: s.name })),
    experiences: c.experiences.map((e) => ({
      title: e.title,
      company: e.company,
      description: e.description,
      isCurrent: e.isCurrent,
      startDate:
        e.startDate === null || e.startDate.year === 0
          ? null
          : `${e.startDate.year}-${pad(e.startDate.month)}-${pad(e.startDate.day)}`,
      endDate:
        e.endDate === null || e.endDate.year === 0
          ? null
          : `${e.endDate.year}-${pad(e.endDate.month)}-${pad(e.endDate.day)}`,
    })),
  };
}

export const hasProfileToScore = (m: MatchCandidate): boolean =>
  m.summary.trim() !== '' || m.skills.length > 0 || m.experiences.length > 0;

// The provider the settings name. `keys` is the raw `llm-keys` secret; the key is read for this one provider and kept nowhere else.
export function providerOf(
  settings: LLMSettings,
  keys: string | null,
  doFetch: LLMFetch,
): LLMProvider {
  if (settings.provider === 'ollama')
    return ollamaProvider(doFetch, settings.baseUrl, settings.model);
  const label = settings.apiKeyLabel ?? '';
  const secret = label === '' ? '' : (parseKeys(keys)[label] ?? '');
  return openAICompatProvider(doFetch, {
    name: settings.provider === '' ? PROVIDER_CUSTOM : settings.provider,
    baseUrl: settings.baseUrl,
    model: settings.model,
    ...(label !== '' ? { key: { label, secret } } : {}),
  });
}

// A `score` for a run, or undefined when there is no model or nothing about the candidate to score against.
export function makeScorer(
  settings: LLMSettings | undefined,
  keys: string | null,
  candidate: Candidate | undefined,
  doFetch: LLMFetch,
  signal?: AbortSignal,
): ((job: RunJob) => Promise<number | undefined>) | undefined {
  if (
    settings === undefined ||
    settings.model === '' ||
    settings.baseUrl === '' ||
    candidate === undefined
  )
    return undefined;
  const profile = matchCandidateOf(candidate);
  if (!hasProfileToScore(profile)) return undefined;
  const provider = providerOf(settings, keys, doFetch);
  return async (job) =>
    (
      await analyzeJob(
        provider,
        profile,
        {
          title: job.title,
          companyName: job.companyName,
          descriptionMarkdown: job.descriptionMarkdown,
        },
        undefined,
        signal,
      )
    ).score;
}
