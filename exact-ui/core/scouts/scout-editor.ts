// Creating and editing a scout with its saved search on the device (ADR-0037). The form is plain text so a view can hold it:
// the search is chosen from a template, an existing saved search, or typed keywords. Takes a Db and never reads the clock.
import { emptyCriteria, type SearchCriteria } from '../search/criteria.ts';
import type { Db } from '../shared/db.ts';
import {
  availableName,
  createSavedSearch,
  getSavedSearch,
  listSavedSearches,
  type Result,
} from './saved-search.ts';
import { searchTemplateGroups, findSearchTemplate } from '../search/search-template.ts';
import { createScout, getScout, updateScout, type StoredScout } from './scout-store.ts';
import { validateScout, type Scout } from './scout.ts';

export interface ScoutForm {
  name: string;
  // '' for typed keywords, `template:<id>` for a template, `search:<id>` for a saved search.
  choice: string;
  keywords: string;
  // Text, as typed: parsed and checked on save.
  minScore: string;
  ceiling: string;
  enabled: boolean;
}
export const newScoutForm: ScoutForm = {
  name: '',
  choice: '',
  keywords: '',
  minScore: '70',
  ceiling: '10',
  enabled: true,
};
export const newScoutId = 'new';

// The next id in a sequence of `${prefix}-N`: ids on the device are short and stable, so a test and a person can name them.
async function nextId(db: Db, table: 'scouts' | 'saved_searches', prefix: string): Promise<string> {
  const { rows } = await db.query(`SELECT id FROM ${table}`);
  const top = rows.reduce((n, r) => {
    const m = new RegExp(`^${prefix}-(\\d+)$`).exec(String(r[0]));
    return m === null ? n : Math.max(n, Number(m[1]));
  }, 0);
  return `${prefix}-${top + 1}`;
}

export interface SearchChoice {
  value: string;
  label: string;
}

export async function searchChoices(db: Db): Promise<SearchChoice[]> {
  const saved = (await listSavedSearches(db)).map((s) => ({
    value: `search:${s.id}`,
    label: `Saved: ${s.name}`,
  }));
  const templates = searchTemplateGroups().flatMap((g) =>
    g.templates.map((t) => ({ value: `template:${t.id}`, label: `Template: ${t.name}` })),
  );
  return [{ value: '', label: 'Keywords I type' }, ...saved, ...templates];
}

export const parseWhole = (text: string): number | undefined =>
  /^\s*\d{1,4}\s*$/.test(text) ? Number(text) : undefined;
const sentence = (message: string): string =>
  message === ''
    ? ''
    : `${message[0]!.toUpperCase()}${message.slice(1)}${/[.!?]$/.test(message) ? '' : '.'}`;

// Criteria that only carry keywords are the typed kind; any other is shown as the saved search it is.
const onlyKeywords = (c: SearchCriteria): boolean =>
  JSON.stringify({ ...c, keywords: '' }) === JSON.stringify({ ...emptyCriteria });

// A short line for a search's criteria, for lists.
export function describeCriteria(c: SearchCriteria): string {
  const parts = [
    c.title.trim() !== '' && `title ${c.title.trim()}`,
    c.keywords.trim() !== '' && `keywords ${c.keywords.trim()}`,
    c.location.trim() !== '' && c.location.trim(),
    c.remotePreference !== '' && c.remotePreference,
    c.employmentType !== '' && c.employmentType,
    c.salaryMin > 0 && `from ${c.salaryMin}`,
  ].filter((p): p is string => typeof p === 'string');
  return parts.length === 0 ? 'No criteria' : parts.join(' · ');
}

export async function loadScoutForm(db: Db, id: string): Promise<ScoutForm | undefined> {
  if (id === newScoutId) return newScoutForm;
  const scout = await getScout(db, id);
  if (scout === undefined) return undefined;
  const search = await getSavedSearch(db, scout.savedSearchId);
  const typed =
    search !== undefined && search.criteria.keywords.trim() !== '' && onlyKeywords(search.criteria);
  return {
    name: scout.name,
    choice: typed ? '' : `search:${scout.savedSearchId}`,
    keywords: typed ? search.criteria.keywords : '',
    minScore: String(scout.minMatchScore),
    ceiling: String(scout.maxApplicationsPerRun),
    enabled: scout.isEnabled,
  };
}

async function resolveCriteria(
  db: Db,
  form: ScoutForm,
): Promise<Result<{ searchId: string; criteria: SearchCriteria }>> {
  if (form.choice.startsWith('search:')) {
    const search = await getSavedSearch(db, form.choice.slice(7));
    return search === undefined
      ? { ok: false, error: 'That saved search is no longer stored.' }
      : { ok: true, value: { searchId: search.id, criteria: search.criteria } };
  }
  if (form.choice.startsWith('template:')) {
    const template = findSearchTemplate(form.choice.slice(9));
    return template === undefined
      ? { ok: false, error: 'That template does not exist.' }
      : { ok: true, value: { searchId: '', criteria: template.criteria } };
  }
  const keywords = form.keywords.trim().replace(/\s+/g, ' ');
  if (keywords === '')
    return {
      ok: false,
      error: 'Type the keywords to look for, or choose a template or a saved search.',
    };
  return { ok: true, value: { searchId: '', criteria: { ...emptyCriteria, keywords } } };
}

// Saves a new scout (`id` is newScoutId; `newId` makes its id) or changes one. A typed or template search becomes a saved search of
// its own, named after the scout, unless the scout already runs one with the same criteria. The schedule is left as it was.
export async function saveScoutForm(
  db: Db,
  id: string,
  form: ScoutForm,
  now: number,
): Promise<Result<StoredScout>> {
  const min = parseWhole(form.minScore);
  const ceiling = parseWhole(form.ceiling);
  if (min === undefined)
    return { ok: false, error: 'The minimum match must be a whole number from 0 to 100.' };
  if (ceiling === undefined)
    return { ok: false, error: 'The most applications per run must be a whole number.' };
  const prior = id === newScoutId ? undefined : await getScout(db, id);
  if (id !== newScoutId && prior === undefined)
    return { ok: false, error: 'This scout is no longer stored.' };
  const scout: Scout = {
    name: form.name.trim(),
    savedSearchId: prior?.savedSearchId ?? 'pending',
    minMatchScore: min,
    maxApplicationsPerRun: ceiling,
    source: prior?.source ?? 'sources',
    isEnabled: form.enabled,
    schedule: prior?.schedule ?? { kind: 'off', everyHours: 0, dailyAt: 0 },
  };
  const invalid = validateScout(scout);
  if (invalid !== '') return { ok: false, error: sentence(invalid) };

  const chosen = await resolveCriteria(db, form);
  if (!chosen.ok) return chosen;
  let searchId = chosen.value.searchId;
  if (searchId === '') {
    const current = prior === undefined ? undefined : await getSavedSearch(db, prior.savedSearchId);
    if (
      current !== undefined &&
      JSON.stringify(current.criteria) === JSON.stringify(chosen.value.criteria)
    )
      searchId = current.id;
    else {
      const taken = new Set((await listSavedSearches(db)).map((s) => s.name.toLowerCase()));
      const created = await createSavedSearch(
        db,
        await nextId(db, 'saved_searches', 'search'),
        availableName(scout.name, taken),
        chosen.value.criteria,
        now,
      );
      if (!created.ok) return { ok: false, error: sentence(created.error) };
      searchId = created.value.id;
    }
  }
  const input = { ...scout, savedSearchId: searchId };
  const saved =
    prior === undefined
      ? await createScout(db, await nextId(db, 'scouts', 'scout'), input, now)
      : await updateScout(db, id, input, now);
  return saved.ok ? saved : { ok: false, error: sentence(saved.error) };
}
