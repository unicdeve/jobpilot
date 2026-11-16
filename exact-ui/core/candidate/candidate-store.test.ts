import { expect, test } from 'bun:test';
import { emptyCandidate, type Candidate } from './candidate.ts';
import { ensureCandidateSchema, firstCandidate, getCandidate, saveCandidate } from './candidate-store.ts';
import { newDate } from '../shared/date.ts';
import { aboutYouFrom, deleteKept, ensurePersonalAnswerSchema, getAboutYou, keepAnswer, keptAnswers, ownWords, saveAboutYou } from './answers-store.ts';
import { ensureResumeSchema, getResume, resumesOf, saveResume } from './resume-store.ts';
import { newResume, resumeName } from './resume.ts';
import { memoryDb } from '../shared/test-db.ts';

const ids = () => { let n = 0; return () => `id${++n}`; };

async function setup() {
  const db = memoryDb();
  await ensureCandidateSchema(db);
  await ensurePersonalAnswerSchema(db);
  await ensureResumeSchema(db);
  return db;
}

const ada = (): Candidate => ({
  ...emptyCandidate('c1', 0), firstName: ' Ada ', lastName: 'Lovelace', email: 'ada@example.com', timeZone: 'africa/lagos',
  noticePeriodDays: 0, earliestStartDate: newDate(2026, 12, 1), remotePreference: 'remote',
  links: [{ id: '', kind: 'github', url: ' https://github.com/ada ' }],
  workAuthorizations: [{ id: '', country: 'Nigeria', isAuthorized: true, requiresSponsorship: false }],
  skills: [{ id: '', name: 'Go', yearsExperience: 4 }, { id: '', name: 'TypeScript', yearsExperience: null }],
  experiences: [
    { id: '', company: 'Acme', title: 'Engineer', startDate: newDate(2022, 7, 1), endDate: newDate(2023, 1, 1), isCurrent: true, description: 'Payments.' },
    { id: '', company: 'Old Co', title: 'Dev', startDate: null, endDate: newDate(2022, 6, 1), isCurrent: false, description: '' },
  ],
});

test('a profile round-trips with its children, and nothing is invented', async () => {
  const db = await setup();
  expect(await firstCandidate(db)).toBeUndefined();
  const saved = await saveCandidate(db, ada(), 100, ids());
  expect(saved.ok).toBe(true);
  const c = (await getCandidate(db, 'c1'))!;
  expect(c.firstName).toBe('Ada');
  expect(c.timeZone).toBe('Africa/Lagos');
  expect(c.noticePeriodDays).toBe(0);
  expect(c.earliestStartDate).toEqual(newDate(2026, 12, 1));
  expect(c.links[0]!.url).toBe('https://github.com/ada');
  expect(c.skills.map((s) => [s.name, s.yearsExperience])).toEqual([['Go', 4], ['TypeScript', null]]);
  expect(c.experiences[0]!.endDate).toBeNull();
  expect(c.experiences[1]!.startDate).toBeNull();
  expect(c.createdAt).toBe(100);
  expect((await firstCandidate(db))!.id).toBe('c1');
  const bare = (await saveCandidate(db, emptyCandidate('c2', 5), 5, ids()));
  expect(bare.ok && bare.candidate.noticePeriodDays).toBeNull();
});

test('a re-save replaces children and keeps created_at', async () => {
  const db = await setup();
  await saveCandidate(db, ada(), 100, ids());
  const next = { ...(await getCandidate(db, 'c1'))!, skills: [{ id: '', name: 'Rust', yearsExperience: null }], links: [] };
  const r = await saveCandidate(db, next, 200, ids());
  expect(r.ok).toBe(true);
  const c = (await getCandidate(db, 'c1'))!;
  expect(c.skills.map((s) => s.name)).toEqual(['Rust']);
  expect(c.links).toEqual([]);
  expect([c.createdAt, c.updatedAt]).toEqual([100, 200]);
});

test('an invalid profile is refused and writes nothing', async () => {
  const db = await setup();
  const bad = async (patch: Partial<Candidate>) => { const r = await saveCandidate(db, { ...ada(), ...patch }, 1, ids()); expect(r.ok).toBe(false); };
  await bad({ timeZone: 'Mars/Base' });
  await bad({ links: [{ id: '', kind: 'github', url: 'a.dev' }, { id: '', kind: 'github', url: 'b.dev' }] });
  await bad({ skills: [{ id: '', name: 'Go', yearsExperience: null }, { id: '', name: 'Go', yearsExperience: 1 }] });
  await bad({ workAuthorizations: [{ id: '', country: '  ', isAuthorized: true, requiresSponsorship: false }] });
  await bad({ noticePeriodDays: -1 });
  await bad({ remotePreference: 'moon' as never });
  expect(await firstCandidate(db)).toBeUndefined();
});

test('about you: save, clear, keep and forget', async () => {
  const db = await setup();
  const id = ids();
  expect((await saveAboutYou(db, 'c1', { nope: 'x' }, 1, id)).ok).toBe(false);
  expect((await getAboutYou(db, 'c1')).questions.every((q) => q.answer === '')).toBe(true);

  let r = await saveAboutYou(db, 'c1', { values: 'Honesty', looking_for: 'Platform work' }, 10, id);
  expect(r.ok && r.about.questions.find((q) => q.key === 'values')!.answer).toBe('Honesty');
  r = await saveAboutYou(db, 'c1', { values: '', looking_for: 'Infra' }, 20, id);
  expect(r.ok && r.about.questions.map((q) => q.answer).filter((a) => a !== '')).toEqual(['Infra']);

  expect((await keepAnswer(db, 'c1', 'Why us?', '  ', 30, id)).ok).toBe(false);
  const first = await keepAnswer(db, 'c1', ' Why us? ', 'Because.', 30, id);
  const again = await keepAnswer(db, 'c1', 'why us?', 'Because of the people.', 40, id);
  expect(first.ok && again.ok && first.answer.id === again.answer.id).toBe(true);
  const kept = await keptAnswers(db, 'c1');
  expect(kept.map((k) => [k.question, k.answer, k.updatedAt])).toEqual([['why us?', 'Because of the people.', 40]]);
  expect(await ownWords(db, 'c1')).toEqual(['Infra', 'Because of the people.']);

  expect((await deleteKept(db, 'other', kept[0]!.id)).ok).toBe(false);
  expect((await deleteKept(db, 'c1', kept[0]!.id)).ok).toBe(true);
  expect((await deleteKept(db, 'c1', kept[0]!.id)).ok).toBe(true);
  expect(await keptAnswers(db, 'c1')).toEqual([]);
  expect(aboutYouFrom([]).questions.length).toBe(5);
});

test('resumes store the text and metadata, not the file', async () => {
  const db = await setup();
  expect(resumeName('C:\\docs\\cv.pdf')).toBe('cv.pdf');
  expect(newResume({ id: 'r', candidateId: 'c1', fileName: '/', markdown: 'x' }, 1).ok).toBe(false);
  expect(newResume({ id: 'r', candidateId: 'c1', fileName: 'cv.pdf', markdown: '  ' }, 1).ok).toBe(false);
  const a = newResume({ id: 'r1', candidateId: 'c1', fileName: '/home/ada/cv.pdf', markdown: '# Ada', originalFilePath: '/home/ada/cv.pdf' }, 1);
  const b = newResume({ id: 'r2', candidateId: 'c1', fileName: 'cv2.md', markdown: '# Ada 2', parsedMetadata: '{}' }, 2);
  if (!a.ok || !b.ok) throw new Error('resume');
  await saveResume(db, b.resume);
  await saveResume(db, a.resume);
  expect(a.resume.name).toBe('cv.pdf');
  expect((await getResume(db, 'r1'))).toEqual(a.resume);
  expect((await resumesOf(db, 'c1')).map((r) => r.id)).toEqual(['r1', 'r2']);
  expect(await getResume(db, 'nope')).toBeUndefined();
});
