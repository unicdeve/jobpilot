// The Go sources' tests (internal/adapters/secondary/discovery/*_test.go), run against the same fixtures with an injected fetch.
import { expect, test } from 'bun:test';
import {
  BoardBusyError,
  BoardGoneError,
  SourceUnreachableError,
  isUnreachable,
  normalizeEmploymentType,
  parseTime,
  encodeQuery,
  type DoFetch,
} from './job-source.ts';
import { fetchArbeitnow } from './arbeitnow.ts';
import { fetchRemoteOK } from './remoteok.ts';
import {
  fetchHimalayas,
  himalayasQuery,
  HIMALAYAS_MAX_PAGES,
  HIMALAYAS_MAX_SEARCH_PAGES,
} from './himalayas.ts';
import { fetchJobicy, jobicyFeeds } from './jobicy.ts';
import {
  fetchWeWorkRemotely,
  noFeedAnswered,
  WWR_CATEGORIES,
  parseRfc1123z,
} from './we-work-remotely.ts';
import {
  fetchHackerNews,
  isHiringThreadTitle,
  locationsFromFields,
  plainHeaderField,
} from './hacker-news.ts';
import {
  describeJobs,
  readBoard,
  boardUrl,
  type Board,
  type BoardEndpoints,
} from './company-boards.ts';

type Reply = { status?: number; body: string };
// A fake network: `route` answers a URL, and every URL asked is recorded. A throw is a network that did not answer.
function network(route: (url: string, headers: Record<string, string>) => Reply | undefined) {
  const asked: string[] = [];
  const doFetch: DoFetch = async (url, init) => {
    asked.push(url);
    const reply = route(url, init?.headers ?? {});
    if (reply === undefined) throw new Error('connection refused');
    const status = reply.status ?? 200;
    return {
      ok: status >= 200 && status < 300,
      status,
      json: async () => JSON.parse(reply.body),
      text: async () => reply.body,
    };
  };
  return { doFetch, asked };
}
const ok = (body: string): Reply => ({ body });
const status = (code: number): Reply => ({ status: code, body: '' });

// ---- Arbeitnow
const arbeitnowFixture = `{"data":[
  {"slug":"senior-go-engineer-berlin-1","company_name":"Acme GmbH","title":"Senior Go Engineer","description":"<p>We use <strong>Go</strong> and SQLite.</p>","remote":true,
   "url":"https://www.arbeitnow.com/jobs/companies/acme/senior-go-engineer-berlin-1","tags":["Go","SQLite"],"job_types":["Full-time"],"location":"Berlin; Remote","created_at":1700000000},
  {"slug":"frontend-dev-2","company_name":"Widget Co","title":"Frontend Developer","description":"<p>React role.</p>","remote":false,
   "url":"https://www.arbeitnow.com/jobs/companies/widget/frontend-dev-2","tags":["React"],"job_types":[],"location":"Lagos","created_at":1700001000}],
  "links":{"first":"","last":null,"prev":null,"next":null},"meta":{"current_page":1}}`;

test('Arbeitnow maps fields and returns the listing whole', async () => {
  const n = network(() => ok(arbeitnowFixture));
  const jobs = await fetchArbeitnow(n.doFetch);
  expect(jobs).toHaveLength(2);
  expect(jobs[0]).toMatchObject({
    sourceId: 'arbeitnow',
    sourceJobId: 'senior-go-engineer-berlin-1',
    title: 'Senior Go Engineer',
    companyName: 'Acme GmbH',
    canonicalUrl: 'https://www.arbeitnow.com/jobs/companies/acme/senior-go-engineer-berlin-1',
    remoteType: 'remote',
    locations: ['Berlin', 'Remote'],
    skills: ['Go', 'SQLite'],
    employmentType: 'Full-time',
    postedAt: '2023-11-14T22:13:20.000Z',
  });
  expect(jobs[0]!.descriptionMarkdown).toContain('Go');
  expect(jobs[0]!.descriptionMarkdown).not.toContain('<strong>');
  expect(jobs[1]).toMatchObject({ remoteType: 'onsite', locations: ['Lagos'], employmentType: '' });
  expect(n.asked).toHaveLength(1);
});

test('Arbeitnow errors', async () => {
  await expect(fetchArbeitnow(network(() => status(500)).doFetch)).rejects.toThrow('non-OK status');
  await expect(fetchArbeitnow(network(() => ok('{}')).doFetch)).rejects.toThrow('decode');
  expect(
    isUnreachable(await fetchArbeitnow(network(() => undefined).doFetch).catch((e) => e)),
  ).toBe(true);
});

// ---- RemoteOK
const remoteOKFixture = `[
  {"legal":"API Terms of Service: Please link back to Remote OK","last_updated":1790208007},
  {"id":"1137421","slug":"remote-senior-go-engineer","position":"Senior Go Engineer ","company":"Acme ","description":"<p>We use <strong>Go</strong> and SQLite every day.</p>",
   "location":"Germany","tags":["golang","backend"],"url":"https://remoteOK.com/remote-jobs/remote-senior-go-engineer-1137421","date":"2026-09-23T00:00:10+00:00","salary_min":120000,"salary_max":160000},
  {"id":"1137422","position":"Product Designer","company":"Widget","description":"<p>Design role.</p>","location":"","url":"https://remoteOK.com/remote-jobs/product-designer-1137422",
   "date":"2026-09-22T00:00:10+00:00","salary_min":0,"salary_max":0},
  {"id":"9","position":"","url":"https://remoteOK.com/x"}]`;

test('RemoteOK skips the notice, trims, and keeps an unstated salary unstated', async () => {
  const n = network((_, h) => {
    expect(h['User-Agent']).not.toBe('');
    return ok(remoteOKFixture);
  });
  const jobs = await fetchRemoteOK(n.doFetch);
  expect(jobs).toHaveLength(2);
  expect(jobs[0]).toMatchObject({
    title: 'Senior Go Engineer',
    companyName: 'Acme',
    sourceId: 'remoteok',
    sourceJobId: '1137421',
    locations: ['Germany'],
    remoteType: 'remote',
    skills: ['golang', 'backend'],
    postedAt: '2026-09-23T00:00:10.000Z',
    salaryMin: 120000,
    salaryMax: 160000,
    salaryCurrency: 'USD',
    salaryPeriod: 'yearly',
  });
  expect(jobs[0]!.descriptionMarkdown).not.toContain('<strong>');
  expect(jobs[1]).toMatchObject({ salaryMin: 0, salaryCurrency: '', locations: [] });
  await expect(fetchRemoteOK(network(() => status(429)).doFetch)).rejects.toThrow('429');
});

// ---- Himalayas
const himalayasPage = `{"totalCount":100632,"jobs":[
  {"guid":"job-1","title":"Staff Platform Engineer","companyName":"iFIT","description":"<p>Run the <strong>platform</strong>.</p>","excerpt":"Run the platform.",
   "applicationLink":"https://himalayas.app/companies/ifit/jobs/staff-platform-engineer","locationRestrictions":["United States"],"employmentType":"Full Time",
   "minSalary":155000,"maxSalary":190000,"currency":"usd","salaryPeriod":"Yearly","categories":["Engineering"],"pubDate":1790203312,"expiryDate":1792795312},
  {"guid":"job-2","title":"Support Lead","companyName":"Widget","description":"","excerpt":"Lead support.","applicationLink":"https://himalayas.app/companies/widget/jobs/support-lead",
   "locationRestrictions":[],"employmentType":"Sabbatical","minSalary":0,"maxSalary":0},
  {"guid":"job-3","title":"No link","applicationLink":""}]}`;
const noCriteria = { title: '', keywords: '' };

test('Himalayas maps a job and drops one with no application link', async () => {
  let calls = 0;
  const n = network(() => ok(++calls === 1 ? himalayasPage : '{"totalCount":0,"jobs":[]}'));
  const jobs = await fetchHimalayas(n.doFetch, noCriteria);
  expect(jobs).toHaveLength(2);
  expect(n.asked).toHaveLength(2); // stops at an empty page, not four
  expect(jobs[0]).toMatchObject({
    title: 'Staff Platform Engineer',
    sourceId: 'himalayas',
    sourceJobId: 'job-1',
    locations: ['United States'],
    remoteType: 'remote',
    employmentType: 'full-time',
    salaryMin: 155000,
    salaryCurrency: 'USD',
    salaryPeriod: 'yearly',
  });
  expect(jobs[0]!.postedAt).not.toBe('');
  expect(jobs[0]!.expiresAt).not.toBe('');
  expect(jobs[0]!.descriptionMarkdown).not.toContain('<strong>');
  expect(jobs[1]).toMatchObject({
    locations: ['Worldwide'],
    descriptionMarkdown: 'Lead support.',
    employmentType: '',
    salaryMin: 0,
  });
});

test('Himalayas pages through a bounded slice of the board', async () => {
  const n = network((url) => {
    const o = new URL(url).searchParams.get('offset');
    return ok(
      `{"jobs":[{"guid":"g${o}","title":"Engineer","companyName":"Acme","applicationLink":"https://himalayas.app/j/${o}"}]}`,
    );
  });
  const jobs = await fetchHimalayas(n.doFetch, noCriteria);
  expect(jobs).toHaveLength(HIMALAYAS_MAX_PAGES);
  expect(n.asked.map((u) => new URL(u).searchParams.get('offset'))).toEqual([
    '0',
    '20',
    '40',
    '60',
  ]);
});

test('Himalayas searches for the criteria, dedupes and stops at an empty page', async () => {
  const n = network((url) => {
    const page = new URL(url).searchParams.get('page');
    if (page === '3') return ok('{"totalCount":41,"jobs":[]}');
    return ok(`{"jobs":[{"guid":"a","title":"Backend Engineer","companyName":"Acme","applicationLink":"https://himalayas.app/j/a"},
      {"guid":"p${page}","title":"Backend Engineer","companyName":"Acme","applicationLink":"https://himalayas.app/j/p${page}"}]}`);
  });
  const jobs = await fetchHimalayas(n.doFetch, {
    title: 'backend',
    keywords: '  engineer ',
    workFrom: 'ng',
  });
  expect(n.asked.map((u) => u.replace('https://himalayas.app/jobs/api', ''))).toEqual([
    '/search?country=NG&page=1&q=backend+engineer',
    '/search?country=NG&page=2&q=backend+engineer',
    '/search?country=NG&page=3&q=backend+engineer',
  ]);
  expect(jobs).toHaveLength(3); // a posting on two pages is one job
  expect(himalayasQuery({ title: 'frontend', keywords: '' })).toEqual({ q: 'frontend' }); // no country is sent when none is set
});

test('Himalayas reads at most five search pages', async () => {
  const n = network((url) => {
    const p = new URL(url).searchParams.get('page');
    return ok(
      `{"jobs":[{"guid":"g${p}","title":"Engineer","companyName":"Acme","applicationLink":"https://himalayas.app/j/${p}"}]}`,
    );
  });
  expect(await fetchHimalayas(n.doFetch, { title: '', keywords: 'go' })).toHaveLength(
    HIMALAYAS_MAX_SEARCH_PAGES,
  );
  expect(n.asked).toHaveLength(5);
});

test('Himalayas keeps what it fetched when a later page fails, and fails on the first', async () => {
  let calls = 0;
  const jobs = await fetchHimalayas(
    network(() => (++calls === 1 ? ok(himalayasPage) : status(500))).doFetch,
    noCriteria,
  );
  expect(jobs).toHaveLength(2);
  await expect(fetchHimalayas(network(() => status(503)).doFetch, noCriteria)).rejects.toThrow(
    '503',
  );
});

test('employment types are the vocabulary or nothing', () => {
  const want: Record<string, string> = {
    'Full Time': 'full-time',
    full_time: 'full-time',
    'FULL-TIME': 'full-time',
    Permanent: 'full-time',
    Contract: 'contract',
    Freelance: 'contract',
    Intern: 'internship',
    Internship: 'internship',
    Sabbatical: '',
    '': '',
    'something else': '',
  };
  for (const [written, expected] of Object.entries(want))
    expect(normalizeEmploymentType(written)).toBe(expected);
});

// ---- Jobicy
const jobicyLocations = `{"locations":[{"geoName":"Anywhere","geoSlug":"anywhere"},{"geoName":"APAC","geoSlug":"apac"},{"geoName":"EMEA","geoSlug":"emea"},
  {"geoName":"Europe","geoSlug":"europe"},{"geoName":"Germany","geoSlug":"germany"},{"geoName":"USA","geoSlug":"usa"}]}`;
function jobicyNetwork() {
  return network((url) => {
    const q = new URL(url).searchParams;
    if (q.get('get') === 'locations') return ok(jobicyLocations);
    if (q.get('geo') === 'anywhere') {
      return ok(`{"jobs":[{"id":1,"url":"https://jobicy.com/jobs/1-go","jobTitle":"Go Engineer","companyName":"Acme","jobIndustry":["Engineering"],"jobType":["Full-Time"],"jobGeo":"Anywhere",
        "jobDescription":"<p>Build it.</p>","pubDate":"2026-09-30T08:18:51+00:00","salaryMin":148000,"salaryMax":216000,"salaryCurrency":"usd","salaryPeriod":"Yearly"}]}`);
    }
    return ok(`{"jobs":[{"id":1,"url":"https://jobicy.com/jobs/1-go","jobTitle":"Go Engineer","companyName":"Acme","jobGeo":"Anywhere"},
      {"id":2,"url":"https://jobicy.com/jobs/2-eu","jobTitle":"Account Executive","companyName":"Nash","jobGeo":"Europe,  UK","jobType":["Contract"]}]}`);
  });
}

test('Jobicy asks the feeds for the country and dedupes across them', async () => {
  const n = jobicyNetwork();
  const jobs = await fetchJobicy(n.doFetch, 'NG');
  const geos = n.asked
    .filter((u) => !u.includes('get=locations'))
    .map((u) => new URL(u).searchParams.get('geo'));
  expect(geos).toEqual(['anywhere', 'emea']); // Nigeria is in EMEA and in none of Jobicy's other locations
  expect(jobs).toHaveLength(2); // a job in two feeds is one job
  expect(jobs[0]).toMatchObject({
    canonicalUrl: 'https://jobicy.com/jobs/1-go',
    sourceId: 'jobicy',
    sourceJobId: '1',
    locations: ['Anywhere'],
    employmentType: 'full-time',
    salaryMin: 148000,
    salaryCurrency: 'USD',
    salaryPeriod: 'yearly',
  });
  expect(jobs[0]!.postedAt).toBe('2026-09-30T08:18:51.000Z');
  expect(jobs[1]!.locations).toEqual(['Europe', 'UK']);
  expect(
    jobicyFeeds(
      'DE',
      JSON.parse(jobicyLocations).locations.map((l: { geoName: string; geoSlug: string }) => ({
        name: l.geoName,
        slug: l.geoSlug,
      })),
    ),
  ).toEqual(['anywhere', 'emea', 'europe', 'germany']); // Germany adds its own feeds
});

test('Jobicy without a country reads the whole board, and fails with its status', async () => {
  const n = jobicyNetwork();
  await fetchJobicy(n.doFetch);
  expect(n.asked).toHaveLength(1);
  expect(new URL(n.asked[0]!).searchParams.has('geo')).toBe(false);
  expect(new URL(n.asked[0]!).searchParams.get('count')).toBe('200');
  await expect(fetchJobicy(network(() => status(429)).doFetch)).rejects.toThrow('429');
});

// ---- We Work Remotely
const wwrFeed = (...items: string[]) =>
  `<?xml version="1.0" encoding="UTF-8"?>\n<rss version="2.0"><channel><title>We Work Remotely</title>${items.join('')}</channel></rss>`;
const wwrWorldwide = `<item><title>Twikey: Senior Java Developer</title><region>Anywhere in the World</region><skills>Java, PostgreSQL, and Spring</skills><type>Full-Time</type>
  <description>&lt;p&gt;Build &lt;strong&gt;payments&lt;/strong&gt;.&lt;/p&gt;</description><pubDate>Thu, 17 Sep 2026 10:51:23 +0000</pubDate>
  <expires_at>Sat, 17 Oct 2026 10:51:23 +0000</expires_at><guid>https://weworkremotely.com/remote-jobs/twikey-senior-java-developer</guid>
  <link>https://weworkremotely.com/remote-jobs/twikey-senior-java-developer</link></item>`;
const wwrUSOnly = `<item><title>Glean: Application Security Engineer</title><region>Anywhere in the World</region><country>🇺🇸 United States of America</country><type>Contract</type>
  <description>Remote.</description><guid>https://weworkremotely.com/remote-jobs/glean-application-security-engineer</guid>
  <link>https://weworkremotely.com/remote-jobs/glean-application-security-engineer</link></item>`;

test('We Work Remotely merges its category feeds into one job per posting', async () => {
  const n = network((url) =>
    ok(
      wwrFeed(
        wwrWorldwide,
        ...(url.endsWith('/remote-back-end-programming-jobs.rss') ? [wwrUSOnly] : []),
      ),
    ),
  );
  const jobs = await fetchWeWorkRemotely(n.doFetch);
  expect(jobs).toHaveLength(2);
  expect(n.asked).toHaveLength(WWR_CATEGORIES.length);
  const java = jobs.find((j) => j.title === 'Senior Java Developer')!;
  expect(java).toMatchObject({
    companyName: 'Twikey',
    sourceId: 'weworkremotely',
    canonicalUrl: 'https://weworkremotely.com/remote-jobs/twikey-senior-java-developer',
    locations: ['Anywhere in the World'],
    remoteType: 'remote',
    employmentType: 'full-time',
    skills: ['Java', 'PostgreSQL', 'Spring'],
    postedAt: '2026-09-17T10:51:23.000Z',
    expiresAt: '2026-10-17T10:51:23.000Z',
  });
  expect(java.descriptionMarkdown).toContain('payments');
  expect(java.descriptionMarkdown).not.toContain('<strong>');
  expect(jobs.find((j) => j.title === 'Application Security Engineer')!.locations).toEqual([
    '🇺🇸 United States of America',
  ]); // narrower than the region, and truer
});

test('We Work Remotely: one feed failing is not the source failing', async () => {
  const jobs = await fetchWeWorkRemotely(
    network((url) =>
      url.endsWith('/remote-programming-jobs.rss') ? status(500) : ok(wwrFeed(wwrWorldwide)),
    ).doFetch,
  );
  expect(jobs).toHaveLength(1);
});

test('We Work Remotely: no feed answering reports the status, which is not unreachable', async () => {
  const err = await fetchWeWorkRemotely(network(() => status(403)).doFetch).catch((e) => e);
  expect(err.message).toContain('403');
  expect(isUnreachable(err)).toBe(false);
});

test('noFeedAnswered prefers a feed that was reached', () => {
  const unreachable = new SourceUnreachableError('feed', new Error('offline'));
  expect(noFeedAnswered([undefined, new Error('x')])).toBeUndefined();
  const failed = new Error('status 500');
  expect(noFeedAnswered([unreachable, failed])).toBe(failed); // one feed reached means the connection is not the reason
  expect(noFeedAnswered([unreachable, unreachable])).toBe(unreachable);
  expect(parseRfc1123z('Thu, 17 Sep 2026 10:51:23 +0200')).toBe('2026-09-17T08:51:23.000Z');
  expect(parseRfc1123z('nonsense')).toBe('');
});

test('We Work Remotely: every feed unreachable is unreachable', async () => {
  const err = await fetchWeWorkRemotely(network(() => undefined).doFetch).catch((e) => e);
  expect(isUnreachable(err)).toBe(true);
});

// ---- Hacker News
const hnStories = `{"hits":[
  {"objectID":"1","title":"Ask HN: Who wants to be hired? (September 2026)","created_at":"2026-09-01T15:00:00Z"},
  {"objectID":"2","title":"Ask HN: Freelancer? Seeking freelancer? (September 2026)","created_at":"2026-09-01T15:00:00Z"},
  {"objectID":"3","title":"Ask HN: Who is hiring? (September 2026)","created_at":"2026-09-01T15:00:00Z"}]}`;
const hnThread = `{"id":3,"title":"Ask HN: Who is hiring? (September 2026)","children":[
  {"id":11,"created_at":"2026-09-01T16:00:00Z","text":"Modash.io | Senior Product Engineer | Remote (Europe) | Full-time | &euro;75k&ndash;110k | https://modash.io<p>We help brands find creators.</p>"},
  {"id":12,"created_at":"2026-09-01T17:00:00Z","text":"Open Education / Neon | Platform &amp; DevOps Engineer | Utrecht, The Netherlands | HYBRID | DUTCH REQUIRED | https://neon.example/jobs"},
  {"id":13,"created_at":"2026-09-01T18:00:00Z","text":"Just a comment with no structure at all, wishing everyone luck."},
  {"id":14,"created_at":"2026-09-01T19:00:00Z","text":"NoLink Corp | Backend Engineer | Berlin | Full-time"},
  {"id":15,"created_at":"2026-09-01T20:00:00Z","text":""}]}`;
const hnNetwork = (stories = hnStories, thread = hnThread) =>
  network((url) =>
    url.includes('search_by_date')
      ? ok(stories)
      : url.includes('/items/')
        ? ok(thread)
        : status(404),
  );

test('Hacker News reads the postings that follow the convention and carry a link', async () => {
  const jobs = await fetchHackerNews(hnNetwork().doFetch);
  expect(jobs).toHaveLength(2);
  expect(jobs[0]).toMatchObject({
    companyName: 'Modash.io',
    title: 'Senior Product Engineer',
    canonicalUrl: 'https://modash.io',
    sourceId: 'hackernews',
    sourceJobId: '11',
    remoteType: 'remote',
    employmentType: 'full-time',
    locations: ['Remote (Europe)'],
  }); // "EUR" must not match inside "Europe"
  expect(jobs[1]).toMatchObject({
    title: 'Platform & DevOps Engineer',
    remoteType: 'hybrid',
    locations: ['Utrecht, The Netherlands'],
  });
  expect(jobs.map((j) => j.title)).not.toContain('Backend Engineer');
  for (const j of jobs) {
    expect(j.canonicalUrl).not.toBe('');
    expect(j.companyName).not.toBe('');
  }
});

test('Hacker News picks the hiring thread, not its siblings', async () => {
  expect(isHiringThreadTitle('Ask HN: Who is hiring? (September 2026)')).toBe(true);
  expect(isHiringThreadTitle('Ask HN: Who wants to be hired? (September 2026)')).toBe(false);
  expect(isHiringThreadTitle('Ask HN: Freelancer? Seeking freelancer? (September 2026)')).toBe(
    false,
  );
  expect(isHiringThreadTitle('Ask HN: Anything else?')).toBe(false);
  const n = hnNetwork();
  await fetchHackerNews(n.doFetch);
  expect(n.asked[1]).toContain('/items/3');
  await expect(fetchHackerNews(hnNetwork('{"hits":[]}').doFetch)).rejects.toThrow(
    "no 'Who is hiring?' thread found",
  );
});

test('locationsFromFields', () => {
  const cases: [string, string[], string[]][] = [
    ['a plain place', ['Acme', 'Engineer', 'Berlin, Germany'], ['Berlin, Germany']],
    ['remote with a region', ['Acme', 'Engineer', 'Remote (Europe)'], ['Remote (Europe)']],
    ['a bare arrangement is not a place', ['Acme', 'Engineer', 'HYBRID', 'Utrecht'], ['Utrecht']],
    ['a shouted note is not a place', ['Acme', 'Engineer', 'DUTCH REQUIRED'], []],
    ['a salary is not a place', ['Acme', 'Engineer', '$150k - $210k'], []],
    ['an employment type is not a place', ['Acme', 'Engineer', 'Full-time', 'Lisbon'], ['Lisbon']],
    ['a URL is not a place', ['Acme', 'Engineer', 'https://acme.example'], []],
    ['nothing to go on', ['Acme', 'Engineer'], []],
  ];
  for (const [name, fields, want] of cases) expect(locationsFromFields(fields), name).toEqual(want);
});

test('plainHeaderField drops a homepage written beside the name', () => {
  const cases: [string, string][] = [
    ['Freeform ([http://freeform.co](http://freeform.co))', 'Freeform'],
    ['Snout [https://snout.com/](https://snout.com/)', 'Snout'],
    [
      'Cerity Partners ([https://ceritypartners.com](https://ceritypartners.com))',
      'Cerity Partners',
    ],
    ['Acme (https://acme.example)', 'Acme'],
    ['[Acme](https://acme.example)', 'Acme'],
    ['  Remote (Europe)  ', 'Remote (Europe)'],
    ['Open Education / Neon', 'Open Education / Neon'],
  ];
  for (const [input, want] of cases) expect(plainHeaderField(input), input).toBe(want);
});

test('Hacker News: a company written with its homepage link', async () => {
  const thread = `{"id":3,"children":[{"id":21,"created_at":"2026-09-01T16:00:00Z",
    "text":"Freeform (<a href=\\"http://freeform.co\\">http://freeform.co</a>) | Software Engineers | Hawthorne, CA | Onsite | https://freeform.co/careers"}]}`;
  const jobs = await fetchHackerNews(hnNetwork(hnStories, thread).doFetch);
  expect(jobs).toHaveLength(1);
  expect(jobs[0]).toMatchObject({ companyName: 'Freeform', title: 'Software Engineers' });
});

// ---- Company boards
const greenhouseJobs = `{"jobs":[{"absolute_url":"https://job-boards.greenhouse.io/gitlab/jobs/8556658002","location":{"name":"Remote, Canada; Remote, United Kingdom; Remote, United States"},
  "id":8556658002,"updated_at":"2026-09-29T10:38:50-04:00","title":"AI Engineer","company_name":"GitLab","first_published":"2026-05-22T09:16:29-04:00",
  "content":"&lt;p&gt;GitLab is the &lt;strong&gt;intelligent&lt;/strong&gt; platform.&lt;/p&gt;"},
  {"absolute_url":"https://job-boards.greenhouse.io/gitlab/jobs/2","location":{"name":"Denver, CO - Hybrid"},"id":2,"title":"Sales","company_name":"GitLab","content":""},
  {"absolute_url":"","id":3,"title":"No address"}]}`;
const leverJobs = `[{"id":"2193db3f","text":"Android Engineer - Experience","hostedUrl":"https://jobs.lever.co/spotify/2193db3f","description":"<div><p>We design Spotify’s consumer experience.</p></div>",
  "lists":[{"text":"What You'll Do","content":"<li>Develop Kotlin code</li>"}],"additional":"<div>Spotify is an equal opportunity employer.</div>",
  "categories":{"commitment":"Permanent","location":"London","allLocations":["London","Stockholm"]},"country":"GB","workplaceType":"hybrid","createdAt":1782214185805,
  "salaryRange":{"currency":"gbp","interval":"per-year-salary","min":70000,"max":90000}},
  {"id":"x","text":"Unspecified","hostedUrl":"https://jobs.lever.co/spotify/x","categories":{"location":"Remote"},"workplaceType":"unspecified"}]`;
const ashbyJobs = `{"jobs":[{"id":"1e8c7f3d","title":"Strategic Solutions Engineer","employmentType":"FullTime","location":"Global","secondaryLocations":[{"location":"EMEA"}],
  "publishedAt":"2026-09-21T02:50:22.578+00:00","isListed":true,"isRemote":true,"workplaceType":"Remote","jobUrl":"https://jobs.ashbyhq.com/railway/1e8c7f3d","descriptionHtml":"<p>Help customers.</p>",
  "compensation":{"summaryComponents":[{"compensationType":"EquityPercentage","interval":"NONE"},{"compensationType":"Salary","interval":"1 YEAR","currencyCode":"USD","minValue":200000,"maxValue":250000}]}},
  {"id":"hidden","title":"Unlisted","jobUrl":"https://jobs.ashbyhq.com/railway/hidden","isListed":false},
  {"id":"old","title":"No workplace","jobUrl":"https://jobs.ashbyhq.com/railway/old","location":"Remote, AMER","isRemote":true}]}`;
const endpoints: BoardEndpoints = {
  greenhouseApi: 'http://x/greenhouse',
  greenhouseEuApi: 'http://x/greenhouse-eu',
  leverApi: 'http://x/lever',
  leverEuApi: 'http://x/lever-eu',
  ashbyApi: 'http://x/ashby',
  workableApi: 'http://x/workable',
  smartRecruitersApi: 'http://x/sr',
  leverPages: 'http://x/lever-pages',
  leverEuPages: 'http://x/lever-eu-pages',
  ashbyPages: 'http://x/ashby-pages',
};

test('a Greenhouse board', async () => {
  const n = network((url) =>
    url === 'http://x/greenhouse/v1/boards/gitlab/jobs?content=true'
      ? ok(greenhouseJobs)
      : url.endsWith('/broken/jobs?content=true')
        ? status(500)
        : status(404),
  );
  const c = await readBoard(n.doFetch, { provider: 'greenhouse', name: 'gitlab' }, endpoints);
  expect(c.jobs).toHaveLength(2); // a posting without an address is dropped
  expect(c.jobs[0]).toMatchObject({
    canonicalUrl: 'https://job-boards.greenhouse.io/gitlab/jobs/8556658002',
    sourceId: 'company-pages',
    sourceJobId: 'greenhouse/gitlab/8556658002',
    companyName: 'GitLab',
    companyUrl: 'https://job-boards.greenhouse.io/gitlab',
    locations: ['Remote, Canada', 'Remote, United Kingdom', 'Remote, United States'],
    remoteType: 'remote',
    postedAt: '2026-05-22T13:16:29.000Z',
  });
  expect(c.jobs[0]!.descriptionMarkdown).toContain('intelligent'); // the content is HTML escaped once more
  expect(c.jobs[0]!.descriptionMarkdown).not.toContain('<');
  expect(c.jobs[1]!.remoteType).toBe('hybrid');
  await expect(
    readBoard(n.doFetch, { provider: 'greenhouse', name: 'closed' }, endpoints),
  ).rejects.toBeInstanceOf(BoardGoneError);
  const broken = await readBoard(
    n.doFetch,
    { provider: 'greenhouse', name: 'broken' },
    endpoints,
  ).catch((e) => e);
  expect(broken).toBeInstanceOf(Error);
  expect(broken).not.toBeInstanceOf(BoardGoneError);
  expect(isUnreachable(broken)).toBe(false);
  expect(
    isUnreachable(
      await readBoard(
        network(() => undefined).doFetch,
        { provider: 'greenhouse', name: 'gitlab' },
        endpoints,
      ).catch((e) => e),
    ),
  ).toBe(true);
});

test('a Lever board', async () => {
  const c = await readBoard(
    network(() => ok(leverJobs)).doFetch,
    { provider: 'lever', name: 'spotify' },
    endpoints,
  );
  expect(c.jobs).toHaveLength(2);
  const job = c.jobs[0]!;
  expect(job).toMatchObject({
    canonicalUrl: 'https://jobs.lever.co/spotify/2193db3f',
    locations: ['London', 'Stockholm'],
    remoteType: 'hybrid',
    employmentType: 'full-time',
    salaryMin: 70000,
    salaryMax: 90000,
    salaryCurrency: 'GBP',
    salaryPeriod: 'year',
    companyName: '',
  });
  for (const part of [
    'consumer experience',
    "What You'll Do",
    'Develop Kotlin code',
    'equal opportunity',
  ])
    expect(job.descriptionMarkdown).toContain(part);
  expect(c.jobs[1]).toMatchObject({ remoteType: '', locations: ['Remote'] });
});

test('an Ashby board', async () => {
  const n = network((url) => {
    expect(url).toContain('includeCompensation=true');
    return ok(ashbyJobs);
  });
  const c = await readBoard(n.doFetch, { provider: 'ashby', name: 'railway' }, endpoints);
  expect(c.jobs).toHaveLength(2); // an unlisted posting is not the company's to show
  expect(c.jobs[0]).toMatchObject({
    canonicalUrl: 'https://jobs.ashbyhq.com/railway/1e8c7f3d',
    locations: ['Global', 'EMEA'],
    remoteType: 'remote',
    employmentType: 'full-time',
    salaryMin: 200000,
    salaryMax: 250000,
    salaryCurrency: 'USD',
    salaryPeriod: 'year',
    postedAt: '2026-09-21T02:50:22.578Z',
  });
  expect(c.jobs[1]!.remoteType).toBe('remote'); // isRemote stands in for a missing workplace type
});

test('EU boards are read from the EU servers', async () => {
  const n = network(() => ok('{"jobs":[]}'));
  await readBoard(n.doFetch, { provider: 'greenhouse', region: 'eu', name: 'n26' }, endpoints);
  const l = network(() => ok('[]'));
  await readBoard(l.doFetch, { provider: 'lever', region: 'eu', name: 'acme' }, endpoints);
  expect([...n.asked, ...l.asked]).toEqual([
    'http://x/greenhouse-eu/v1/boards/n26/jobs?content=true',
    'http://x/lever-eu/v0/postings/acme?mode=json',
  ]);
  expect(boardUrl({ provider: 'lever', region: 'eu', name: 'acme' })).toBe(
    'https://jobs.eu.lever.co/acme',
  );
});

const workableWidget = `{"name":"Hugging Face","description":"Here at Hugging Face…","jobs":[
  {"title":"Low-level Senior Software Engineer, Xet Storage - EMEA Remote","shortcode":"F4C096B22E","employment_type":"Full-time","telecommuting":true,"published_on":"2026-07-30","created_at":"2026-07-30",
   "locations":[{"country":"France","countryCode":"FR","city":"Paris","region":"Île-de-France","hidden":true}],
   "description":"<p>At Hugging Face, we're on a journey to democratize <strong>good AI</strong>.</p><p><strong>What we're looking for</strong></p><ul><li>Rust</li></ul>"},
  {"title":"Open-Source Machine Learning Engineer - EMEA Remote","shortcode":"81B46579FE","employment_type":"Full-time","telecommuting":true,"published_on":"2026-05-29","created_at":"2026-05-29",
   "locations":[{"country":"France","countryCode":"FR","city":"Paris","region":"Île-de-France","hidden":false}],"description":"<p>Open source.</p>"},
  {"title":"Office Manager","shortcode":"0FF1CE0000","employment_type":"Part-time","telecommuting":false,"published_on":"2026-09-01",
   "locations":[{"country":"United Kingdom","countryCode":"GB","city":"London","region":"England","hidden":false}],"description":"<p>In the office.</p>"},
  {"title":"","shortcode":"NOTITLE000","description":""}]}`;

test('a Workable board', async () => {
  const n = network((url) =>
    url.includes('/accounts/huggingface?')
      ? ok(workableWidget)
      : url.includes('/accounts/busy?')
        ? status(429)
        : status(404),
  );
  const c = await readBoard(n.doFetch, { provider: 'workable', name: 'huggingface' }, endpoints);
  expect(c.undescribed).toBe(false);
  expect(c.name).toBe('Hugging Face');
  expect(c.jobs).toHaveLength(3); // a posting without a title is dropped
  expect(c.jobs[0]).toMatchObject({
    canonicalUrl: 'https://apply.workable.com/huggingface/j/F4C096B22E/',
    sourceJobId: 'workable/huggingface/F4C096B22E',
    companyUrl: 'https://apply.workable.com/huggingface/',
    locations: [],
    remoteType: 'remote',
    employmentType: 'full-time',
    postedAt: '2026-07-30T00:00:00.000Z',
    companyName: '',
  });
  expect(c.jobs[0]!.descriptionMarkdown).toContain('good AI');
  expect(c.jobs[0]!.descriptionMarkdown).toContain('Rust');
  expect(c.jobs[1]!.locations).toEqual(['Paris, Île-de-France, France']);
  expect(c.jobs[2]).toMatchObject({ remoteType: '', employmentType: 'part-time' });
  await expect(
    readBoard(n.doFetch, { provider: 'workable', name: 'nobody' }, endpoints),
  ).rejects.toBeInstanceOf(BoardGoneError);
  const busy = await readBoard(n.doFetch, { provider: 'workable', name: 'busy' }, endpoints).catch(
    (e) => e,
  );
  expect(busy).toBeInstanceOf(BoardBusyError);
  expect(busy).not.toBeInstanceOf(BoardGoneError);
});

const srPosting = (
  id: string,
  name: string,
  remote: boolean,
  hybrid: boolean,
) => `{"id":"${id}","name":"${name}","company":{"identifier":"Sectigo","name":"Sectigo"},
  "releasedDate":"2026-10-02T12:15:46.155Z","location":{"city":"Paris","region":"IDF","country":"fr","remote":${remote},"hybrid":${hybrid},"fullLocation":"Paris, IDF, France"},
  "typeOfEmployment":{"id":"permanent","label":"Full-time"}}`;
const srDetails = `{"id":"3743990015853896","name":"Enterprise Onboarding Engineer","jobAd":{"sections":{
  "companyDescription":{"title":"Company Description","text":"<p><strong>Sectigo</strong> is the most innovative provider of certificate lifecycle management.</p>"},
  "jobDescription":{"title":"Job Description","text":"<p>Onboard enterprise customers onto Kubernetes.</p>"},
  "qualifications":{"title":"Qualifications","text":"<ul><li>Bachelor’s degree.</li></ul>"},
  "additionalInformation":{"title":"Additional Information","text":"<p>Sectigo is an Equal Opportunity Employer.</p>"},
  "videos":{"title":"Videos To Watch","urls":["https://www.youtube.com/watch?v=R2zvMKayp8I"]}}}}`;

test('a SmartRecruiters board pages by a hundred and is described apart', async () => {
  const postings = Array.from({ length: 230 }, (_, i) =>
    srPosting(
      String(3743990015853896n + BigInt(i)),
      i === 0 ? 'Enterprise Onboarding Engineer' : `Role ${i}`,
      i % 3 === 0,
      i % 5 === 4,
    ),
  );
  const n = network((url) => {
    const u = new URL(url);
    if (u.pathname === '/sr/v1/companies/Sectigo/postings') {
      const offset = Number(u.searchParams.get('offset'));
      expect(u.searchParams.get('limit')).toBe('100');
      return ok(
        `{"offset":${offset},"limit":100,"totalFound":230,"content":[${postings.slice(offset, offset + 100).join(',')}]}`,
      );
    }
    if (
      u.pathname === '/sr/v1/companies/Sectigo/postings/3743990015853896' ||
      u.pathname === '/sr/v1/companies/Sectigo/postings/3743990015853897'
    )
      return ok(srDetails);
    return status(404);
  });
  const board: Board = { provider: 'smartrecruiters', name: 'Sectigo' };
  const c = await readBoard(n.doFetch, board, endpoints);
  expect(c.undescribed).toBe(true);
  expect(c.jobs).toHaveLength(230);
  expect(n.asked).toHaveLength(3);
  expect(c.name).toBe('Sectigo');
  expect(c.jobs[0]).toMatchObject({
    canonicalUrl: 'https://jobs.smartrecruiters.com/Sectigo/3743990015853896',
    sourceJobId: 'smartrecruiters/Sectigo/3743990015853896',
    companyUrl: 'https://careers.smartrecruiters.com/Sectigo',
    companyName: 'Sectigo',
    locations: ['Paris, IDF, France'],
    remoteType: 'remote',
    employmentType: 'full-time',
    descriptionMarkdown: '',
  });
  expect(c.jobs[4]!.remoteType).toBe('hybrid');
  expect(c.jobs[1]!.remoteType).toBe('');

  const before = n.asked.length;
  const known = new Map<string, string>();
  const full = await describeJobs(n.doFetch, board, c.jobs.slice(0, 3), endpoints, known);
  expect(full).toHaveLength(2); // a posting closed since the list is left out
  for (const part of ['Company Description', 'Kubernetes', 'Equal Opportunity'])
    expect(full[0]!.descriptionMarkdown).toContain(part);
  expect(n.asked.length - before).toBe(3); // two described, and one asked for and gone
  await describeJobs(n.doFetch, board, c.jobs.slice(0, 2), endpoints, known);
  expect(n.asked.length - before).toBe(3); // a posting described before is not asked for again
  // Other providers' jobs are left as they are.
  const same = [c.jobs[0]!];
  expect(
    await describeJobs(n.doFetch, { provider: 'greenhouse', name: 'gitlab' }, same, endpoints),
  ).toBe(same);
});

test('a SmartRecruiters company with no postings reads as an empty board', async () => {
  const c = await readBoard(
    network(() => ok('{"offset":0,"limit":100,"totalFound":0,"content":[]}')).doFetch,
    { provider: 'smartrecruiters', name: 'Visa' },
    endpoints,
  );
  expect(c.jobs).toEqual([]);
});

test('helpers', () => {
  expect(parseTime('2026-09-29T10:38:50-04:00')).toBe('2026-09-29T14:38:50.000Z');
  expect(parseTime('nope', '2026-07-30')).toBe('2026-07-30T00:00:00.000Z');
  expect(parseTime('')).toBe('');
  expect(encodeQuery({ q: 'backend engineer', country: 'NG' })).toBe(
    'country=NG&q=backend+engineer',
  );
});
