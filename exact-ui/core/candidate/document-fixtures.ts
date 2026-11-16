// Builders for the tests' own documents (a zip, a Word file, a PDF), so no binary fixture is needed beyond the Go
// core's resume.pdf. Test-only: it uses node:zlib, which the app code never does.
import { deflateRawSync, deflateSync } from 'node:zlib';
import { readFileSync } from 'node:fs';

const enc = (s: string): Uint8Array => new TextEncoder().encode(s);
const goData = new URL('../../../internal/adapters/secondary/documents/testdata/', import.meta.url);
export const goFixture = (name: string): Uint8Array =>
  new Uint8Array(readFileSync(new URL(name, goData)));
export const goFixtureText = (name: string): string => readFileSync(new URL(name, goData), 'utf8');

function crc32(b: Uint8Array): number {
  let c,
    crc = 0xffffffff;
  for (let i = 0; i < b.length; i++) {
    c = (crc ^ b[i]!) & 0xff;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    crc = (crc >>> 8) ^ c;
  }
  return (crc ^ 0xffffffff) >>> 0;
}

export interface ZipSpec {
  name: string;
  data: Uint8Array | string;
  stored?: boolean;
  lieSize?: number;
}

export function makeZip(specs: ZipSpec[]): Uint8Array {
  const chunks: number[][] = [];
  const central: number[] = [];
  let offset = 0;
  const le = (v: number, n: number): number[] =>
    Array.from({ length: n }, (_, i) => (v >>> (8 * i)) & 255);
  for (const s of specs) {
    const raw = typeof s.data === 'string' ? enc(s.data) : s.data;
    const body = s.stored ? raw : new Uint8Array(deflateRawSync(raw));
    const name = Array.from(enc(s.name));
    const local = [
      ...le(0x04034b50, 4),
      ...le(20, 2),
      ...le(0, 2),
      ...le(s.stored ? 0 : 8, 2),
      ...le(0, 4),
      ...le(crc32(raw), 4),
      ...le(body.length, 4),
      ...le(s.lieSize ?? raw.length, 4),
      ...le(name.length, 2),
      ...le(0, 2),
      ...name,
      ...body,
    ];
    central.push(
      ...le(0x02014b50, 4),
      ...le(20, 2),
      ...le(20, 2),
      ...le(0, 2),
      ...le(s.stored ? 0 : 8, 2),
      ...le(0, 4),
      ...le(crc32(raw), 4),
      ...le(body.length, 4),
      ...le(s.lieSize ?? raw.length, 4),
      ...le(name.length, 2),
      ...le(0, 2),
      ...le(0, 2),
      ...le(0, 2),
      ...le(0, 2),
      ...le(0, 4),
      ...le(offset, 4),
      ...name,
    );
    chunks.push(local);
    offset += local.length;
  }
  const end = [
    ...le(0x06054b50, 4),
    ...le(0, 2),
    ...le(0, 2),
    ...le(specs.length, 2),
    ...le(specs.length, 2),
    ...le(central.length, 4),
    ...le(offset, 4),
    ...le(0, 2),
  ];
  return Uint8Array.from([...chunks.flat(), ...central, ...end]);
}

const W =
  'xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"';

export function makeDocx(bodyXml: string, extra: ZipSpec[] = []): Uint8Array {
  return makeZip([
    { name: '[Content_Types].xml', data: '<Types/>' },
    {
      name: 'word/document.xml',
      data: `<?xml version="1.0"?><w:document ${W}><w:body>${bodyXml}</w:body></w:document>`,
    },
    ...extra,
  ]);
}

// A one-font, one-page PDF. Each content string is a page's content stream, written with the given font object body.
export interface PdfSpec {
  content: string;
  font?: string;
  extraObjects?: string[];
  compress?: boolean;
  annots?: string;
  trailerExtra?: string;
}

export function makePdf(spec: PdfSpec): Uint8Array {
  const font =
    spec.font ?? '<</Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding>>';
  const content = spec.compress
    ? new Uint8Array(deflateSync(enc(spec.content)))
    : enc(spec.content);
  const parts: (string | Uint8Array)[] = [];
  const offsets: number[] = [];
  let pos = 0;
  const push = (p: string | Uint8Array): void => {
    parts.push(p);
    pos += typeof p === 'string' ? p.length : p.length;
  };
  const obj = (n: number, body: string): void => {
    offsets[n] = pos;
    push(`${n} 0 obj\n${body}\nendobj\n`);
  };
  push('%PDF-1.4\n');
  obj(1, '<</Type /Catalog /Pages 2 0 R>>');
  obj(2, '<</Type /Pages /Kids [3 0 R] /Count 1>>');
  obj(
    3,
    `<</Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources <</Font <</F1 5 0 R>>>> /Contents 4 0 R ${spec.annots ?? ''}>>`,
  );
  offsets[4] = pos;
  push(
    `4 0 obj\n<</Length ${content.length}${spec.compress ? ' /Filter /FlateDecode' : ''}>>\nstream\n`,
  );
  push(content);
  push('\nendstream\nendobj\n');
  obj(5, font);
  (spec.extraObjects ?? []).forEach((body, i) => obj(6 + i, body));
  push(
    `trailer\n<</Root 1 0 R /Size ${6 + (spec.extraObjects?.length ?? 0)} ${spec.trailerExtra ?? ''}>>\n%%EOF\n`,
  );
  const out = new Uint8Array(pos);
  let at = 0;
  for (const p of parts) {
    const b =
      typeof p === 'string' ? Uint8Array.from(Array.from(p, (c) => c.charCodeAt(0) & 255)) : p;
    out.set(b, at);
    at += b.length;
  }
  return out;
}
