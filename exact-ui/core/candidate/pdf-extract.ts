// Text and hyperlinks out of a PDF, in pure TypeScript (ADR-0016's behaviour without PDFium, which Hermes cannot
// load). Like the Go converter it reads each page's text in content-stream order, one line per baseline, and appends
// each link annotation's address next to the text it sits on, so a resume's "Github" label arrives with its real URL
// and a project's or employer's link is not mistaken for the candidate's own.
import { fail } from './document-error.ts';
import { buildFont, type PdfFont } from './pdf-font.ts';
import { isDict, isName, isStr, Lexer, PdfDoc, type PdfDict, type PdfValue } from './pdf-parse.ts';
import { decodeUtf16, decodeUtf8 } from './utf8.ts';

const MAX_PAGES = 100;
const MAX_LINKS = 64; // the Go converter's maxRecoveredLinks
const MAX_OPERATORS = 4_000_000;
const MAX_FORM_DEPTH = 6;
const MAX_FORMS = 1000;

type Matrix = [number, number, number, number, number, number];
const IDENTITY: Matrix = [1, 0, 0, 1, 0, 0];
// Row-vector convention as in the PDF spec: result = a x b applies a first.
const mul = (a: Matrix, b: Matrix): Matrix => [
  a[0] * b[0] + a[1] * b[2],
  a[0] * b[1] + a[1] * b[3],
  a[2] * b[0] + a[3] * b[2],
  a[2] * b[1] + a[3] * b[3],
  a[4] * b[0] + a[5] * b[2] + b[4],
  a[4] * b[1] + a[5] * b[3] + b[5],
];

interface PlacedChar {
  text: string;
  x: number;
  y: number;
}
interface Page {
  text: string;
  chars: PlacedChar[];
  unmapped: number;
  mapped: number;
  dict: PdfDict;
}
export interface RecoveredLink {
  anchor: string;
  url: string;
}

export function extractPdf(data: Uint8Array): string {
  const doc = new PdfDoc(data);
  if (doc.encrypted)
    fail(
      'pdf-encrypted',
      'The PDF is password-protected or encrypted, which the on-device reader cannot open.',
    );
  const pages = pageDicts(doc);
  if (pages.length === 0) fail('corrupt', 'The PDF could not be read: it has no pages.');

  const fonts = new Map<PdfDict, PdfFont>();
  const body: string[] = [];
  const links: RecoveredLink[] = [];
  const seen = new Set<string>();
  let unmapped = 0,
    mapped = 0;
  for (const { dict, resources } of pages.slice(0, MAX_PAGES)) {
    const page = readPage(doc, dict, resources, fonts);
    unmapped += page.unmapped;
    mapped += page.mapped;
    const t = page.text.trim();
    if (t !== '') body.push(t);
    for (const link of pageLinks(doc, dict, page.chars)) {
      if (seen.has(link.url) || links.length >= MAX_LINKS) continue;
      seen.add(link.url);
      links.push(link);
    }
  }
  const markdown = body.join('\n\n').trim();
  if (markdown === '' || (mapped === 0 && unmapped > 0)) {
    if (unmapped > 0) {
      fail(
        'pdf-unsupported',
        'The PDF has text, but its fonts carry no map back to characters (a CID font without a ToUnicode table), so the on-device reader cannot recover it.',
      );
    }
    if (links.length === 0)
      fail('pdf-no-text', 'No text could be read from the PDF; it may be a scan or an image.');
  } else if (unmapped > 0 && unmapped > mapped * 0.2) {
    fail(
      'pdf-unsupported',
      'Much of the PDF text uses fonts with no map back to characters, so the on-device reader cannot recover it reliably.',
    );
  }
  return markdown + linksSection(links);
}

// Same wording as the Go converter's linksSection, so the model reads the same thing on either core.
export function linksSection(links: RecoveredLink[]): string {
  if (links.length === 0) return '';
  let s = '\n\n## Hyperlinks in this document\n\n';
  s += 'Each line is the text the reader sees, then the address it links to. ';
  s +=
    'Some of these belong to employers, projects or certificates rather than to the candidate.\n\n';
  for (const l of links) s += `- ${l.anchor !== '' ? `${l.anchor} -> ` : ''}${l.url}\n`;
  return s;
}

// Pages in document order, each with the Resources it inherits.
function pageDicts(doc: PdfDoc): { dict: PdfDict; resources: PdfDict | undefined }[] {
  const out: { dict: PdfDict; resources: PdfDict | undefined }[] = [];
  const visited = new Set<PdfDict>();
  const walk = (
    node: PdfValue | undefined,
    inherited: PdfDict | undefined,
    depth: number,
  ): void => {
    const d = doc.dict(node);
    if (!d || visited.has(d) || depth > 32 || out.length >= MAX_PAGES + 1) return;
    visited.add(d);
    const resources = doc.dict(d.m.get('Resources')) ?? inherited;
    const kids = doc.arr(d.m.get('Kids'));
    if (kids && doc.name(d.m.get('Type')) !== 'Page')
      for (const k of kids) walk(k, resources, depth + 1);
    else out.push({ dict: d, resources });
  };
  const root = doc.dict(doc.root);
  if (root) walk(root.m.get('Pages'), undefined, 0);
  if (out.length === 0) {
    for (const d of doc.allDicts())
      if (doc.name(d.m.get('Type')) === 'Page')
        out.push({ dict: d, resources: doc.dict(d.m.get('Resources')) });
  }
  return out;
}

interface Interp {
  doc: PdfDoc;
  fonts: Map<PdfDict, PdfFont>;
  text: string;
  chars: PlacedChar[];
  unmapped: number;
  mapped: number;
  ops: number;
  forms: number;
  // Pen state for deciding line and word breaks.
  haveLast: boolean;
  lastY: number;
  lastEndX: number;
  lastSize: number;
  pendingSpace: boolean;
}

function readPage(
  doc: PdfDoc,
  page: PdfDict,
  resources: PdfDict | undefined,
  fonts: Map<PdfDict, PdfFont>,
): Page {
  const st: Interp = {
    doc,
    fonts,
    text: '',
    chars: [],
    unmapped: 0,
    mapped: 0,
    ops: 0,
    forms: 0,
    haveLast: false,
    lastY: 0,
    lastEndX: 0,
    lastSize: 0,
    pendingSpace: false,
  };
  const contents = doc.get(page.m.get('Contents'));
  const streams: PdfDict[] = [];
  if (Array.isArray(contents)) {
    for (const c of contents) {
      const d = doc.dict(c);
      if (d) streams.push(d);
    }
  } else if (isDict(contents)) streams.push(contents);
  const parts = streams.map((s) => doc.streamBytes(s));
  let total = 0;
  for (const p of parts) total += p.length + 1;
  const joined = new Uint8Array(total);
  let at = 0;
  for (const p of parts) {
    joined.set(p, at);
    at += p.length;
    joined[at++] = 10;
  }
  runContent(st, joined, resources, IDENTITY, 0);
  const text = st.text
    .split('\n')
    .map((l) => l.replace(/[ \t ]+/g, ' ').trim())
    .join('\n')
    .replace(/\n{2,}/g, '\n');
  return { text, chars: st.chars, unmapped: st.unmapped, mapped: st.mapped, dict: page };
}

function runContent(
  st: Interp,
  bytes: Uint8Array,
  resources: PdfDict | undefined,
  startCtm: Matrix,
  depth: number,
): void {
  const doc = st.doc;
  const lx = new Lexer(bytes, 0, false);
  let ctm = startCtm;
  const stack: Matrix[] = [];
  let tm: Matrix = IDENTITY,
    tlm: Matrix = IDENTITY;
  let font: PdfFont | undefined;
  let fs = 0,
    tc = 0,
    tw = 0,
    tz = 1,
    tl = 0;
  let operands: PdfValue[] = [];
  const fontRes = resources ? doc.dict(resources.m.get('Font')) : undefined;

  const num = (i: number): number => {
    const v = operands[i];
    return typeof v === 'number' ? v : 0;
  };
  const move = (tx: number, ty: number): void => {
    tlm = mul([1, 0, 0, 1, tx, ty], tlm);
    tm = tlm;
  };

  const show = (s: Uint8Array): void => {
    if (!font) return;
    for (const g of font.decode(s)) {
      const m = mul(tm, ctm);
      const sizeY = fs * Math.hypot(m[2], m[3]);
      const sizeX = Math.hypot(m[0], m[1]);
      const x = m[4],
        y = m[5];
      const advance = ((g.width / 1000) * fs + tc + (g.space ? tw : 0)) * tz;
      const devAdvance = advance * sizeX;
      if (st.haveLast) {
        const ref = Math.max(0.1, Math.min(st.lastSize, sizeY) || st.lastSize);
        if (Math.abs(y - st.lastY) > ref * 0.5) {
          st.text += '\n';
          st.pendingSpace = false;
        } else if (font.hasWidths && x - st.lastEndX > ref * 0.25) st.pendingSpace = true;
      }
      if (st.pendingSpace && !g.space && !/[ \n]$/.test(st.text) && st.text !== '') st.text += ' ';
      st.pendingSpace = false;
      st.text += g.text;
      if (g.unmapped) st.unmapped++;
      else if (g.text.trim() !== '') st.mapped++;
      st.chars.push({ text: g.text, x: x + devAdvance / 2, y: y + sizeY * 0.3 });
      st.haveLast = true;
      st.lastY = y;
      st.lastEndX = x + devAdvance;
      st.lastSize = sizeY > 0 ? sizeY : st.lastSize;
      tm = mul([1, 0, 0, 1, advance, 0], tm);
    }
  };
  const showArray = (arr: PdfValue[]): void => {
    for (const item of arr) {
      if (isStr(item)) show(item.b);
      else if (typeof item === 'number') {
        const tx = (-item / 1000) * fs * tz;
        tm = mul([1, 0, 0, 1, tx, 0], tm);
        if (-item > 250) st.pendingSpace = true;
      }
    }
  };

  for (;;) {
    const v = lx.value();
    if (v === undefined) {
      if (lx.atEnd()) break;
      continue;
    }
    if (typeof v !== 'object' || v === null || Array.isArray(v) || v.k !== 'op') {
      operands.push(v);
      if (operands.length > 512) operands = operands.slice(-64);
      continue;
    }
    if (++st.ops > MAX_OPERATORS)
      fail('too-large', 'The PDF has more drawing operations than the device will read.');
    switch (v.v) {
      case 'q':
        if (stack.length < 64) stack.push(ctm);
        break;
      case 'Q':
        ctm = stack.pop() ?? ctm;
        break;
      case 'cm':
        ctm = mul([num(0), num(1), num(2), num(3), num(4), num(5)], ctm);
        break;
      case 'BT':
        tm = IDENTITY;
        tlm = IDENTITY;
        break;
      case 'Tf': {
        const n = operands[0];
        fs = num(1);
        const fd = fontRes && isName(n) ? doc.dict(fontRes.m.get(n.v)) : undefined;
        if (fd) {
          let f = st.fonts.get(fd);
          if (!f) {
            f = buildFont(doc, fd);
            st.fonts.set(fd, f);
          }
          font = f;
        } else font = undefined;
        break;
      }
      case 'Tc':
        tc = num(0);
        break;
      case 'Tw':
        tw = num(0);
        break;
      case 'Tz':
        tz = num(0) / 100;
        break;
      case 'TL':
        tl = num(0);
        break;
      case 'Td':
        move(num(0), num(1));
        break;
      case 'TD':
        tl = -num(1);
        move(num(0), num(1));
        break;
      case 'Tm':
        tlm = [num(0), num(1), num(2), num(3), num(4), num(5)];
        tm = tlm;
        break;
      case 'T*':
        move(0, -tl);
        break;
      case 'Tj': {
        const s = operands[0];
        if (isStr(s)) show(s.b);
        break;
      }
      case 'TJ': {
        const a = operands[0];
        if (Array.isArray(a)) showArray(a);
        break;
      }
      case "'": {
        move(0, -tl);
        const s = operands[0];
        if (isStr(s)) show(s.b);
        break;
      }
      case '"': {
        tw = num(0);
        tc = num(1);
        move(0, -tl);
        const s = operands[2];
        if (isStr(s)) show(s.b);
        break;
      }
      case 'Do': {
        const n = operands[0];
        const xo = resources ? doc.dict(resources.m.get('XObject')) : undefined;
        const form = xo && isName(n) ? doc.dict(xo.m.get(n.v)) : undefined;
        if (
          form &&
          form.stream &&
          doc.name(form.m.get('Subtype')) === 'Form' &&
          depth < MAX_FORM_DEPTH &&
          ++st.forms <= MAX_FORMS
        ) {
          const mArr = doc.arr(form.m.get('Matrix'));
          const fm: Matrix =
            mArr && mArr.length >= 6
              ? ([0, 1, 2, 3, 4, 5].map((i) => doc.num(mArr[i]) ?? 0) as Matrix)
              : IDENTITY;
          runContent(
            st,
            doc.streamBytes(form),
            doc.dict(form.m.get('Resources')) ?? resources,
            mul(fm, ctm),
            depth + 1,
          );
        }
        break;
      }
      case 'BI': {
        // Inline image: skip to "EI" after the data, which is binary and must not be parsed as operators.
        const b = bytes;
        let p = lx.pos;
        for (; p + 1 < b.length; p++)
          if (
            b[p] === 73 &&
            b[p + 1] === 68 &&
            (p + 2 >= b.length || b[p + 2] === 32 || b[p + 2] === 10 || b[p + 2] === 13)
          )
            break;
        for (p += 2; p + 1 < b.length; p++) {
          if (
            b[p] === 69 &&
            b[p + 1] === 73 &&
            (b[p - 1] === 10 || b[p - 1] === 13 || b[p - 1] === 32) &&
            (p + 2 >= b.length || b[p + 2] === 10 || b[p + 2] === 13 || b[p + 2] === 32)
          ) {
            p += 2;
            break;
          }
        }
        lx.pos = Math.min(p, b.length);
        break;
      }
      default:
        break;
    }
    operands = [];
  }
}

function pageLinks(doc: PdfDoc, page: PdfDict, chars: PlacedChar[]): RecoveredLink[] {
  const out: RecoveredLink[] = [];
  const annots = doc.arr(page.m.get('Annots'));
  if (!annots) return out;
  for (const a of annots.slice(0, 2000)) {
    const d = doc.dict(a);
    if (!d || doc.name(d.m.get('Subtype')) !== 'Link') continue;
    const action = doc.dict(d.m.get('A'));
    if (!action || doc.name(action.m.get('S')) !== 'URI') continue;
    const u = doc.get(action.m.get('URI'));
    if (!isStr(u)) continue;
    // A PDF string is bytes: UTF-16 with a byte-order mark, or (in practice) ASCII/UTF-8. Control characters are never
    // part of an address and are dropped, because the address is untrusted input headed for a model's context.
    const raw = u.b[0] === 0xfe && u.b[1] === 0xff ? decodeUtf16(u.b, false, 2) : decodeUtf8(u.b);
    const url = raw
      .replace(/[\u0000-\u001f\u007f]/g, '')
      .trim()
      .slice(0, 2048);
    if (url === '') continue;
    const r = doc.arr(d.m.get('Rect'));
    let anchor = '';
    if (r && r.length >= 4) {
      const n = [0, 1, 2, 3].map((i) => doc.num(r[i]) ?? 0);
      const left = Math.min(n[0]!, n[2]!),
        right = Math.max(n[0]!, n[2]!),
        bottom = Math.min(n[1]!, n[3]!),
        top = Math.max(n[1]!, n[3]!);
      anchor = chars
        .filter((c) => c.x >= left && c.x <= right && c.y >= bottom && c.y <= top)
        .map((c) => c.text)
        .join('')
        .split(/\s+/)
        .filter((w) => w !== '')
        .join(' ');
    }
    out.push({ anchor, url });
  }
  return out;
}
