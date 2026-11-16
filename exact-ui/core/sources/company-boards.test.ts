// company_boards_test.go / company_boards_more_test.go: BoardName and ResolveAddress, against fake fetches.
import { expect, test } from 'bun:test';
import { BoardGoneError, isUnreachable } from './job-source.ts';
import { boardName, cleanCompanyName, readBoard, resolveAddress, RedirectUnreadableError, type Board, type BoardEndpoints, type BoardFetch } from './company-boards.ts';

interface Reply { status?: number; body?: string; location?: string; type?: string; noHeaders?: boolean }
function net(route: (url: string) => Reply | undefined) {
  const asked: { url: string; redirect?: string }[] = [];
  const doFetch: BoardFetch = async (url, init) => {
    asked.push({ url, redirect: init?.redirect });
    const r = route(url);
    if (r === undefined) throw new Error('connection refused');
    const status = r.status ?? 200;
    const headers = r.noHeaders ? undefined : { get: (n: string) => (n.toLowerCase() === 'location' ? (r.location ?? null) : null) };
    return { ok: status >= 200 && status < 300, status, type: r.type, headers, json: async () => JSON.parse(r.body ?? ''), text: async () => r.body ?? '' };
  };
  return { doFetch, asked };
}
const endpoints: BoardEndpoints = {
  greenhouseApi: 'http://x/greenhouse', greenhouseEuApi: 'http://x/greenhouse-eu', leverApi: 'http://x/lever', leverEuApi: 'http://x/lever-eu',
  ashbyApi: 'http://x/ashby', workableApi: 'http://x/workable', smartRecruitersApi: 'http://x/sr',
  leverPages: 'http://x/lever-pages', leverEuPages: 'http://x/lever-eu-pages', ashbyPages: 'http://x/ashby-pages', smartRecruitersPages: 'http://x/sr-pages',
};
const b = (provider: Board['provider'], name: string): Board => ({ provider, name });

const pages: Record<string, Reply> = {
  'http://x/greenhouse/v1/boards/gitlab': { body: '{"name":"GitLab","content":""}' },
  'http://x/lever-pages/spotify': { body: '<html><head><title>Spotify</title></head><body></body></html>' },
  'http://x/ashby-pages/railway': { body: '<html><head><title>Railway Jobs</title></head><body><script>window.__appData = {"organization":{"organizationId":"2f86","name":"Railway & Co","publicWebsite":"https://railway.com/"}};</script></body></html>' },
  'http://x/ashby-pages/nameless': { body: '<html><title>Jobs</title></html>' },
};

test('BoardName reads each provider where it says the name', async () => {
  const n = net((u) => pages[u] ?? { status: 404 });
  expect(await boardName(n.doFetch, b('greenhouse', 'gitlab'), endpoints)).toBe('GitLab');
  expect(await boardName(n.doFetch, b('lever', 'spotify'), endpoints)).toBe('Spotify');
  expect(await boardName(n.doFetch, b('ashby', 'railway'), endpoints)).toBe('Railway & Co');
  expect(await boardName(n.doFetch, b('ashby', 'nameless'), endpoints)).toBe('');
  await expect(boardName(n.doFetch, b('lever', 'kraken'), endpoints)).rejects.toBeInstanceOf(BoardGoneError); // its page says 404 where its API says []
  expect(isUnreachable(await boardName(net(() => undefined).doFetch, b('lever', 'spotify'), endpoints).catch((e) => e))).toBe(true);
});

test('BoardName reads EU Lever pages and skips pages the endpoints do not give', async () => {
  const n = net(() => ({ body: '<title>N26 &amp; Co</title>' }));
  expect(await boardName(n.doFetch, { provider: 'lever', region: 'eu', name: 'n26' }, endpoints)).toBe('N26 & Co');
  expect(n.asked[0]!.url).toBe('http://x/lever-eu-pages/n26');
  const { leverPages: _l, ashbyPages: _a, ...bare } = endpoints;
  expect(await boardName(n.doFetch, b('lever', 'spotify'), bare)).toBe('');
});

test('BoardName for Workable and SmartRecruiters is the name the board read gives', async () => {
  const n = net((u) => {
    if (u === 'http://x/workable/api/v1/widget/accounts/huggingface?details=true') return { body: '{"name":"  Hugging\\n Face ","jobs":[]}' };
    if (u.startsWith('http://x/sr/v1/companies/Sectigo/postings?')) return { body: '{"totalFound":1,"content":[{"id":"1","name":"Eng","company":{"name":"Sectigo"}}]}' };
    return undefined;
  });
  expect(await boardName(n.doFetch, b('workable', 'huggingface'), endpoints)).toBe('Hugging Face');
  expect(await boardName(n.doFetch, b('smartrecruiters', 'Sectigo'), endpoints)).toBe('Sectigo');
});

test('a SmartRecruiters company with no postings is named by its careers page', async () => {
  const n = net((u) => {
    if (u.includes('/postings?')) return { body: '{"offset":0,"limit":100,"totalFound":0,"content":[]}' };
    if (u === 'http://x/sr-pages/Visa') return { body: '<html><head><title>Careers at Visa</title></head></html>' };
    if (u === 'http://x/sr-pages/Bosch') return { status: 302, location: 'https://jobs.bosch.com/en' };
    if (u === 'http://x/sr-pages/nobody') return { status: 302, location: 'https://jobs.smartrecruiters.com' };
    return undefined;
  });
  const visa = await readBoard(n.doFetch, b('smartrecruiters', 'Visa'), endpoints);
  expect(visa.jobs).toEqual([]);
  expect(visa.name).toBe('Visa');
  expect(await boardName(n.doFetch, b('smartrecruiters', 'Visa'), endpoints)).toBe('Visa');
  expect((await readBoard(n.doFetch, b('smartrecruiters', 'Bosch'), endpoints)).name).toBe(''); // the company's own site is still a company
  await expect(readBoard(n.doFetch, b('smartrecruiters', 'nobody'), endpoints)).rejects.toBeInstanceOf(BoardGoneError);
  expect(n.asked.filter((a) => a.url.includes('sr-pages')).every((a) => a.redirect === 'manual')).toBe(true);
  // A host that cannot see redirects cannot tell: the board is still read.
  const web = net((u) => (u.includes('/postings?') ? { body: '{"totalFound":0,"content":[]}' } : { status: 0, type: 'opaqueredirect', noHeaders: true }));
  expect((await readBoard(web.doFetch, b('smartrecruiters', 'nobody'), endpoints)).name).toBe('');
});

test('ResolveAddress follows one redirect, read against the address', async () => {
  const n = net((u) => (u === 'http://host/workable/j/F4C096B22E' ? { status: 301, location: '/huggingface/j/F4C096B22E' } : { status: 404 }));
  expect(await resolveAddress(n.doFetch, 'http://host/workable/j/F4C096B22E')).toBe('http://host/huggingface/j/F4C096B22E');
  expect(n.asked[0]!.redirect).toBe('manual');
  await expect(resolveAddress(n.doFetch, 'http://host/workable/j/0000000000')).rejects.toBeInstanceOf(BoardGoneError);
});

test('ResolveAddress adds a scheme, and fails clearly where it cannot see a redirect', async () => {
  const n = net(() => ({ status: 302, location: 'https://b.example/x' }));
  expect(await resolveAddress(n.doFetch, ' a.example/j/1 ')).toBe('https://b.example/x');
  expect(n.asked[0]!.url).toBe('https://a.example/j/1');
  await expect(resolveAddress(net(() => ({ body: 'page' })).doFetch, 'https://a.example/j/1')).rejects.toThrow('did not lead');
  await expect(resolveAddress(net(() => ({ status: 302 })).doFetch, 'https://a.example/j/1')).rejects.toThrow('without saying where');
  const web = net(() => ({ status: 0, type: 'opaqueredirect', noHeaders: true }));
  await expect(resolveAddress(web.doFetch, 'https://a.example/j/1')).rejects.toBeInstanceOf(RedirectUnreadableError);
  await expect(resolveAddress(net(() => ({ status: 429 })).doFetch, 'https://a.example/j/1')).rejects.toThrow('wait');
  await expect(resolveAddress(net(() => ({ status: 500 })).doFetch, 'https://a.example/j/1')).rejects.toThrow('status 500');
  expect(isUnreachable(await resolveAddress(net(() => undefined).doFetch, 'https://a.example/j/1').catch((e) => e))).toBe(true);
});

test('CleanCompanyName makes one trimmed line of at most 100 characters', () => {
  expect(cleanCompanyName('  Acme \n  Corp ')).toBe('Acme Corp');
  expect(cleanCompanyName('é'.repeat(150))).toBe('é'.repeat(100));
});
