// A small HTML to Markdown converter for an HTML resume or saved page: headings, lists, links, bold and italic, line
// breaks. Nothing is fetched or executed, scripts and styles are dropped, and only http(s) and mailto links survive.
import { decodeEntities } from './xml-lite.ts';

const NAMED: Record<string, string> = {
  nbsp: ' ', ndash: '–', mdash: '—', hellip: '…', bull: '•', middot: '·', copy: '©', reg: '®',
  trade: '™', lsquo: '‘', rsquo: '’', ldquo: '“', rdquo: '”', eacute: 'é', euro: '€', times: '×',
};

export function htmlToMarkdown(html: string): string {
  let s = html
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/<(script|style|head|noscript|template|svg)\b[\s\S]*?<\/\1\s*>/gi, '');
  s = s.replace(/<a\b[^>]*?href\s*=\s*("([^"]*)"|'([^']*)')[^>]*>([\s\S]*?)<\/a\s*>/gi, (_m, _q, d: string | undefined, sq: string | undefined, inner: string) => {
    const href = decodeEntities((d ?? sq ?? '').trim());
    const label = inner.replace(/<[^>]*>/g, '').trim();
    if (label === '') return '';
    return /^(https?:\/\/|mailto:)/i.test(href) ? `[${label}](${href.replace(/\)/g, '%29').replace(/\s/g, '%20')})` : label;
  });
  s = s
    .replace(/<h([1-6])\b[^>]*>/gi, (_m, n: string) => `\n\n${'#'.repeat(Number(n))} `)
    .replace(/<\/h[1-6]\s*>/gi, '\n\n')
    .replace(/<li\b[^>]*>/gi, '\n- ')
    .replace(/<br\b[^>]*>/gi, '\n')
    .replace(/<\/(p|div|section|article|header|footer|ul|ol|table|tr|blockquote|pre)\s*>/gi, '\n\n')
    .replace(/<(p|div|section|article|header|footer|ul|ol|table|tr|blockquote|pre)\b[^>]*>/gi, '\n\n')
    .replace(/<\/(td|th)\s*>/gi, ' | ')
    .replace(/<(b|strong)\b[^>]*>([\s\S]*?)<\/\1\s*>/gi, (_m, _t, inner: string) => (inner.trim() === '' ? inner : `**${inner.trim()}**`))
    .replace(/<(i|em)\b[^>]*>([\s\S]*?)<\/\1\s*>/gi, (_m, _t, inner: string) => (inner.trim() === '' ? inner : `*${inner.trim()}*`))
    .replace(/<[^>]*>/g, '');
  s = s.replace(/&([a-zA-Z]{2,6});/g, (m, n: string) => NAMED[n] ?? m);
  s = decodeEntities(s);
  return s
    .split('\n')
    .map((l) => l.replace(/[ \t ]+/g, ' ').replace(/\s*\|\s*$/, '').trim())
    .join('\n')
    .replace(/\n{3,}/g, '\n\n')
    .replace(/^(- .*)\n\n(?=- )/gm, '$1\n')
    .trim();
}
