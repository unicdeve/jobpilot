// A Word (.docx) file to Markdown. A .docx is a zip whose body is word/document.xml; the Go side reads it through
// Go MarkItDown (mammoth-style: headings, lists, tables, bold and italic, hyperlinks), and this does the same by
// reading the XML directly. The file is untrusted: nothing in it is executed, fetched or expanded.
// Headers, footers, footnotes and comments are not read, as in the Go converter.
import { fail } from './document-error.ts';
import { childrenNamed, firstChild, parseXml, type XmlNode } from './xml-lite.ts';
import { readZipEntries, readZipEntry } from './zip-reader.ts';
import { decodeUtf8 } from './utf8.ts';

interface Seg { text: string; bold: boolean; italic: boolean }
type NumFormats = Map<string, string>; // "numId:ilvl" -> numFmt

const attr = (n: XmlNode | undefined, name: string): string | undefined => n?.attrs[name];
const flagOn = (rPr: XmlNode | undefined, tag: string): boolean => {
  const n = rPr ? firstChild(rPr, tag) : undefined;
  if (!n) return false;
  const v = n.attrs['w:val'];
  return !(v === '0' || v === 'false' || v === 'off' || v === 'none');
};

export function docxToMarkdown(data: Uint8Array): string {
  const entries = readZipEntries(data);
  const part = (name: string): XmlNode | undefined => {
    const e = entries.get(name);
    return e ? parseXml(decodeUtf8(readZipEntry(data, e))) : undefined;
  };
  const doc = part('word/document.xml');
  if (!doc) fail('corrupt', 'This is not a Word document: it has no word/document.xml.');

  const links = new Map<string, string>();
  const rels = part('word/_rels/document.xml.rels');
  for (const r of rels ? descendants(rels, 'Relationship') : []) {
    const id = r.attrs['Id'], target = r.attrs['Target'];
    if (id && target && r.attrs['TargetMode'] === 'External' && /^(https?:|mailto:)/i.test(target)) links.set(id, target);
  }
  const formats = numberingFormats(part('word/numbering.xml'));

  const body = descendants(doc, 'w:body')[0];
  if (!body) fail('corrupt', 'The Word document has no body.');
  const blocks: string[] = [];
  const counters = new Map<string, number>();
  walkBlocks(body, { links, formats, counters }, blocks);
  return blocks.join('\n\n').replace(/\n{3,}/g, '\n\n').trim();
}

interface Ctx { links: Map<string, string>; formats: NumFormats; counters: Map<string, number> }

function descendants(node: XmlNode, name: string, out: XmlNode[] = []): XmlNode[] {
  for (const c of node.children) {
    if (c.name === name) out.push(c);
    if (c.children.length > 0) descendants(c, name, out);
  }
  return out;
}

function numberingFormats(numbering: XmlNode | undefined): NumFormats {
  const formats: NumFormats = new Map();
  if (!numbering) return formats;
  const abstract = new Map<string, XmlNode>();
  for (const a of descendants(numbering, 'w:abstractNum')) abstract.set(a.attrs['w:abstractNumId'] ?? '', a);
  for (const num of descendants(numbering, 'w:num')) {
    const a = abstract.get(attr(firstChild(num, 'w:abstractNumId'), 'w:val') ?? '');
    if (!a) continue;
    for (const lvl of childrenNamed(a, 'w:lvl')) {
      formats.set(`${num.attrs['w:numId'] ?? ''}:${lvl.attrs['w:ilvl'] ?? '0'}`, attr(firstChild(lvl, 'w:numFmt'), 'w:val') ?? 'bullet');
    }
  }
  return formats;
}

function walkBlocks(parent: XmlNode, ctx: Ctx, out: string[]): void {
  for (const c of parent.children) {
    if (c.name === 'w:p') paragraph(c, ctx, out);
    else if (c.name === 'w:tbl') { const t = table(c, ctx); if (t) out.push(t); }
    else if (c.name === 'w:sdt') { const content = firstChild(c, 'w:sdtContent'); if (content) walkBlocks(content, ctx, out); }
    else if (c.name === 'mc:AlternateContent') { const choice = firstChild(c, 'mc:Choice') ?? firstChild(c, 'mc:Fallback'); if (choice) walkBlocks(choice, ctx, out); }
    else if (c.name === 'w:ins' || c.name === 'w:customXml' || c.name === 'w:smartTag') walkBlocks(c, ctx, out);
  }
}

function paragraph(p: XmlNode, ctx: Ctx, out: string[]): void {
  const segs: Seg[] = [];
  const extra: string[] = [];
  inlineRuns(p, ctx, segs, extra, undefined);
  const text = render(segs).trim();
  const pPr = firstChild(p, 'w:pPr');
  const style = attr(firstChild(pPr ?? p, 'w:pStyle'), 'w:val') ?? '';
  const heading = /^heading\s*([1-9])$/i.exec(style);
  const numPr = pPr ? firstChild(pPr, 'w:numPr') : undefined;
  if (text !== '') {
    if (heading) out.push(`${'#'.repeat(Number(heading[1]))} ${stripEmphasis(text)}`);
    else if (/^title$/i.test(style)) out.push(`# ${stripEmphasis(text)}`);
    else if (/^subtitle$/i.test(style)) out.push(`## ${stripEmphasis(text)}`);
    else if (numPr && attr(firstChild(numPr, 'w:numId'), 'w:val') !== '0') {
      const numId = attr(firstChild(numPr, 'w:numId'), 'w:val') ?? '';
      const ilvl = Number(attr(firstChild(numPr, 'w:ilvl'), 'w:val') ?? '0') || 0;
      const fmt = ctx.formats.get(`${numId}:${ilvl}`) ?? 'bullet';
      const indent = '  '.repeat(Math.min(ilvl, 8));
      let marker = '-';
      if (fmt !== 'bullet' && fmt !== 'none') {
        const key = `${numId}:${ilvl}`;
        const next = (ctx.counters.get(key) ?? 0) + 1;
        ctx.counters.set(key, next);
        marker = `${next}.`;
      }
      // List items stay together: no blank line between them.
      const line = `${indent}${marker} ${text}`;
      const last = out.length - 1;
      if (last >= 0 && /^ *(-|\d+\.) /.test(out[last]!) && !out[last]!.includes('\n\n') && listBlock(out[last]!)) out[last] += `\n${line}`;
      else out.push(line);
    } else out.push(text);
  }
  for (const e of extra) if (e !== '') out.push(e);
}

const listBlock = (s: string): boolean => s.split('\n').every((l) => /^ *(-|\d+\.) /.test(l));
const stripEmphasis = (s: string): string => s.replace(/\*+/g, '');

function inlineRuns(parent: XmlNode, ctx: Ctx, segs: Seg[], extra: string[], link: { url: string } | undefined): void {
  for (const c of parent.children) {
    if (c.name === 'w:r') run(c, ctx, segs, extra);
    else if (c.name === 'w:hyperlink') {
      const url = ctx.links.get(c.attrs['r:id'] ?? '');
      if (url) {
        const inner: Seg[] = [];
        inlineRuns(c, ctx, inner, extra, undefined);
        const label = render(inner).trim();
        if (label !== '') segs.push({ text: `[${label.replace(/\]/g, '\\]')}](${url.replace(/\)/g, '%29').replace(/ /g, '%20')})`, bold: false, italic: false });
      } else inlineRuns(c, ctx, segs, extra, link);
    } else if (c.name === 'w:ins' || c.name === 'w:smartTag' || c.name === 'w:fldSimple' || c.name === 'w:sdt' || c.name === 'w:sdtContent' || c.name === 'w:customXml') {
      inlineRuns(c, ctx, segs, extra, link);
    } else if (c.name === 'mc:AlternateContent') {
      const choice = firstChild(c, 'mc:Choice') ?? firstChild(c, 'mc:Fallback');
      if (choice) inlineRuns(choice, ctx, segs, extra, link);
    }
  }
}

function run(r: XmlNode, ctx: Ctx, segs: Seg[], extra: string[]): void {
  const rPr = firstChild(r, 'w:rPr');
  const bold = flagOn(rPr, 'w:b'), italic = flagOn(rPr, 'w:i');
  let text = '';
  const flush = (): void => { if (text !== '') segs.push({ text, bold, italic }); text = ''; };
  const visit = (n: XmlNode): void => {
    for (const c of n.children) {
      if (c.name === 'w:t') text += c.children.map((t) => t.text).join('');
      else if (c.name === 'w:tab') text += ' ';
      else if (c.name === 'w:br' || c.name === 'w:cr') text += attr(c, 'w:type') === 'page' ? '' : '\n';
      else if (c.name === 'w:noBreakHyphen') text += '-';
      else if (c.name === 'w:txbxContent') {
        const nested: string[] = [];
        walkBlocks(c, ctx, nested);
        extra.push(...nested);
      } else if (c.name === 'mc:AlternateContent') {
        const choice = firstChild(c, 'mc:Choice') ?? firstChild(c, 'mc:Fallback');
        if (choice) visit(choice);
      } else if (c.name !== 'w:rPr' && c.children.length > 0) visit(c);
    }
  };
  visit(r);
  flush();
}

// Joins runs into Markdown, merging neighbours of one format and keeping the markers off surrounding spaces.
function render(segs: Seg[]): string {
  const merged: Seg[] = [];
  for (const s of segs) {
    const last = merged[merged.length - 1];
    if (last && last.bold === s.bold && last.italic === s.italic) last.text += s.text;
    else merged.push({ ...s });
  }
  let out = '';
  for (const s of merged) {
    const m = /^(\s*)([\s\S]*?)(\s*)$/.exec(s.text)!;
    if ((!s.bold && !s.italic) || m[2] === '') { out += s.text; continue; }
    const mark = (s.bold ? '**' : '') + (s.italic ? '*' : '');
    out += `${m[1]}${mark}${m[2]}${mark.split('').reverse().join('')}${m[3]}`;
  }
  return out.replace(/[ \t]*\n[ \t]*/g, '\n').replace(/[ \t]{2,}/g, ' ');
}

function table(tbl: XmlNode, ctx: Ctx): string {
  const rows: string[][] = [];
  for (const tr of childrenNamed(tbl, 'w:tr')) {
    const cells: string[] = [];
    for (const tc of childrenNamed(tr, 'w:tc')) {
      const blocks: string[] = [];
      walkBlocks(tc, ctx, blocks);
      cells.push(blocks.join(' ').replace(/\s*\n\s*/g, ' ').replace(/\|/g, '\\|').trim());
    }
    if (cells.some((c) => c !== '')) rows.push(cells);
  }
  if (rows.length === 0) return '';
  const width = Math.max(...rows.map((r) => r.length));
  const line = (r: string[]): string => `| ${Array.from({ length: width }, (_, i) => r[i] ?? '').join(' | ')} |`;
  return [line(rows[0]!), `| ${Array.from({ length: width }, () => '---').join(' | ')} |`, ...rows.slice(1).map(line)].join('\n');
}
