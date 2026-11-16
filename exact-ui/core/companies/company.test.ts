// company_test.go: ParseBoard, Board.URL, Same, BoardOf, DefaultName, CleanCompanyName.
import { expect, test } from 'bun:test';
import {
  boardOf,
  boardUrl,
  cleanCompanyName,
  defaultName,
  parseBoard,
  sameBoard,
  UnsupportedAddressError,
  WorkablePostingAddressError,
} from './company.ts';
import type { Board } from '../sources/company-boards.ts';

const gh = (name: string): Board => ({ provider: 'greenhouse', name });

test('parseBoard reads a board from every supported address', () => {
  const cases: Record<string, Board> = {
    'https://boards.greenhouse.io/gitlab': gh('gitlab'),
    'https://job-boards.greenhouse.io/gitlab/jobs/8556658002': gh('gitlab'),
    'job-boards.greenhouse.io/gitlab': gh('gitlab'),
    '  https://www.boards.greenhouse.io/gitlab?gh_src=abc  ': gh('gitlab'),
    'https://boards.greenhouse.io/embed/job_board?for=grafanalabs': gh('grafanalabs'),
    'https://boards.greenhouse.io/embed/job_app?for=gitlab&token=8556658002': gh('gitlab'),
    'https://job-boards.eu.greenhouse.io/n26/jobs/1': {
      provider: 'greenhouse',
      region: 'eu',
      name: 'n26',
    },
    'https://jobs.lever.co/spotify/2193db3f-77c5-43b8-b030-8f92c9882bf1/apply': {
      provider: 'lever',
      name: 'spotify',
    },
    'https://jobs.eu.lever.co/acme': { provider: 'lever', region: 'eu', name: 'acme' },
    'https://jobs.ashbyhq.com/clickup/1e8c7f3d-3cd6-4393-b17f-6a858f480ba9': {
      provider: 'ashby',
      name: 'clickup',
    },
    'HTTPS://JOBS.ASHBYHQ.COM/ClickUp': { provider: 'ashby', name: 'ClickUp' },
    'https://jobs.ashbyhq.com/The%20Browser%20Company': {
      provider: 'ashby',
      name: 'The Browser Company',
    },
    'https://apply.workable.com/seeq/j/CDBF1AE73C': { provider: 'workable', name: 'seeq' },
    'apply.workable.com/longshot-systems-ltd': {
      provider: 'workable',
      name: 'longshot-systems-ltd',
    },
    'https://apply.workable.com/huggingface/j/F4C096B22E/apply': {
      provider: 'workable',
      name: 'huggingface',
    },
    'https://sumble-inc.workable.com/': { provider: 'workable', name: 'sumble-inc' },
    'https://careers.smartrecruiters.com/Sectigo': { provider: 'smartrecruiters', name: 'Sectigo' },
    'https://jobs.smartrecruiters.com/Sectigo/3743990015862586-revenue-accountant-': {
      provider: 'smartrecruiters',
      name: 'Sectigo',
    },
    'https://jobs.smartrecruiters.com/BoschGroup/744000082736493-ai-engineer?oga=true': {
      provider: 'smartrecruiters',
      name: 'BoschGroup',
    },
  };
  for (const [address, want] of Object.entries(cases))
    expect(parseBoard(address), address).toEqual(want);
});

test('parseBoard refuses what is not a supported hiring page', () => {
  for (const address of [
    '',
    'gitlab',
    'https://gitlab.com/jobs',
    'https://www.linkedin.com/company/gitlab',
    'https://boards.greenhouse.io/',
    'https://boards.greenhouse.io/embed/job_board',
    'https://jobs.lever.co',
    'https://jobs.ashbyhq.com/..',
    'https://greenhouse.io.evil.example/gitlab',
    'https://jobs.ashbyhq.com/%3Cscript%3E',
    'https://jobs.workable.com/view/abc',
    'https://www.workable.com/',
    'https://apply.workable.com/',
    'https://jobs.smartrecruiters.com/external-referrals/company/Sectigo/publication/eede6bb6',
    'https://jobs.smartrecruiters.com',
    'https://evil.workable.com.example/acme',
  ])
    expect(() => parseBoard(address), address).toThrow(UnsupportedAddressError);
});

test("a Workable posting's short address is its own error, not a company called j", () => {
  let error: unknown;
  try {
    parseBoard('https://apply.workable.com/j/F4C096B22E');
  } catch (e) {
    error = e;
  }
  expect(error).toBeInstanceOf(WorkablePostingAddressError);
  expect(error).not.toBeInstanceOf(UnsupportedAddressError);
});

test("a board's address names the board", () => {
  expect(boardUrl(gh('gitlab'))).toBe('https://job-boards.greenhouse.io/gitlab');
  expect(boardUrl({ provider: 'ashby', name: 'The Browser Company' })).toBe(
    'https://jobs.ashbyhq.com/The%20Browser%20Company',
  );
  for (const board of [
    { provider: 'greenhouse', region: 'eu', name: 'n26' },
    { provider: 'ashby', name: 'The Browser Company' },
    { provider: 'workable', name: 'huggingface' },
    { provider: 'smartrecruiters', name: 'BoschGroup' },
  ] as Board[]) {
    expect(parseBoard(boardUrl(board))).toEqual(board);
  }
});

test('sameBoard ignores case in the name only', () => {
  expect(sameBoard(gh('gitlab'), gh('GitLab'))).toBe(true);
  expect(sameBoard(gh('gitlab'), { provider: 'greenhouse', region: 'eu', name: 'gitlab' })).toBe(
    false,
  );
  expect(sameBoard(gh('gitlab'), { provider: 'lever', name: 'gitlab' })).toBe(false);
});

test("boardOf reads a job's own address, then the first board in its description", () => {
  expect(
    boardOf({ canonicalUrl: 'https://jobs.lever.co/spotify/2193db3f', descriptionMarkdown: '' })
      ?.name,
  ).toBe('spotify');
  expect(
    boardOf({
      canonicalUrl: 'https://news.ycombinator.com/item?id=1',
      descriptionMarkdown:
        'Acme | Remote\n\nSee https://acme.example/careers or apply at [Ashby](https://jobs.ashbyhq.com/acme/123/application).',
    }),
  ).toEqual({ provider: 'ashby', name: 'acme' });
  expect(
    boardOf({
      canonicalUrl: 'https://news.ycombinator.com/item?id=2',
      descriptionMarkdown:
        'Seeq | Remote\n\nApply: https://apply.workable.com/j/FD4C7FD4F3 or https://apply.workable.com/seeq/j/CDBF1AE73C',
    }),
  ).toEqual({ provider: 'workable', name: 'seeq' });
  expect(
    boardOf({
      canonicalUrl: 'https://remoteok.com/remote-jobs/1',
      descriptionMarkdown: 'Apply by email.',
    }),
  ).toBeUndefined();
});

test('names: cleaned to one line of at most 100 characters, or the board name capitalised', () => {
  expect(cleanCompanyName('  GitLab\n\tInc. ')).toBe('GitLab Inc.');
  expect([...cleanCompanyName('é'.repeat(150))]).toHaveLength(100);
  expect(defaultName({ provider: 'greenhouse', name: 'grafanalabs' })).toBe('Grafanalabs');
  expect(defaultName({ provider: 'greenhouse', name: 'cockroach-labs' })).toBe('Cockroach labs');
});
