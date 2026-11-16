import { describe, expect, test } from 'bun:test';
import { convertDocument, isPdf, RESUME_ACCEPT } from './document-converter.ts';
import { goFixture, goFixtureText, makeDocx, makePdf, makeZip } from './document-fixtures.ts';

const bytes = (s: string): Uint8Array => new TextEncoder().encode(s);
const failure = (name: string, data: Uint8Array): { code: string; error: string } => {
  const r = convertDocument(name, data);
  if (r.ok) throw new Error('expected a failure');
  return r;
};

// The cases follow the Go converter's tests (converter_test.go): routing by bytes, the extension rule, and HTML.
describe('convertDocument', () => {
  test('a PDF is recognised by its bytes, whatever it is called', () => {
    for (const name of ['resume', 'resume.txt', 'resume.docx', 'Ada Lovelace CV.PDF']) {
      const r = convertDocument(name, goFixture('resume.pdf'));
      expect(r.ok && r.markdown.includes('Ada Lovelace')).toBe(true);
    }
  });

  test('the PDF result is the Go golden, trimmed and with LF line ends', () => {
    const r = convertDocument('resume.pdf', goFixture('resume.pdf'));
    expect(r).toEqual({
      ok: true,
      markdown: goFixtureText('resume.golden.md').replace(/\r\n/g, '\n').trim(),
    });
  });

  test('plain text and Markdown pass through, with UTF-8 and UTF-16 handled', () => {
    expect(convertDocument('notes.txt', bytes('Ada Lovelace\r\nEngineer\r\n'))).toEqual({
      ok: true,
      markdown: 'Ada Lovelace\nEngineer',
    });
    expect(convertDocument('cv.md', bytes('\ufeff# Ada\n\n- Go\n'))).toEqual({
      ok: true,
      markdown: '# Ada\n\n- Go',
    });
    const utf16 = Uint8Array.from([
      0xff,
      0xfe,
      ...Array.from('Ada \u00e9', (c) => [c.charCodeAt(0) & 255, c.charCodeAt(0) >> 8]).flat(),
    ]);
    expect(convertDocument('cv.TXT', utf16)).toEqual({ ok: true, markdown: 'Ada \u00e9' });
  });

  test('a file that is not a PDF is not treated as one', () => {
    expect(convertDocument('notes.txt', bytes('I have attached my PDF resume'))).toEqual({
      ok: true,
      markdown: 'I have attached my PDF resume',
    });
    expect(isPdf(bytes('%PDF-1.7\n'))).toBe(true);
    expect(isPdf(Uint8Array.from([...new Array(200).fill(0), ...bytes('%PDF-1.4')]))).toBe(true);
    expect(isPdf(bytes('Ada Lovelace, engineer'))).toBe(false);
    expect(isPdf(new Uint8Array(0))).toBe(false);
    expect(isPdf(Uint8Array.from([...new Array(1100).fill(0), ...bytes('%PDF-1.4')]))).toBe(false);
  });

  test('anything else needs an extension to say what it is', () => {
    for (const name of [
      'resume',
      'resume.',
      'resume.with spaces',
      'resume.t/xt',
      'resume.waytoolongextension',
    ]) {
      expect(failure(name, bytes('Ada Lovelace')).code).toBe('no-extension');
    }
  });

  test('a Windows path names the file by its last part', () => {
    expect(convertDocument('C:\\Users\\ada\\cv.txt', bytes('hi'))).toEqual({
      ok: true,
      markdown: 'hi',
    });
  });

  test('HTML converts to Markdown without markup (Go: resume.html)', () => {
    const r = convertDocument('resume.html', goFixture('resume.html'));
    expect(r.ok).toBe(true);
    if (!r.ok) return;
    expect(r.markdown).toContain('Ada Lovelace');
    expect(r.markdown).toContain('PROFESSIONAL SUMMARY');
    expect(r.markdown).toContain('[Github](https://github.com/adalovelace)');
    expect(r.markdown).not.toContain('<');
  });

  test('HTML scripts, styles and unsafe links do not reach the text', () => {
    const r = convertDocument(
      'a.html',
      bytes(
        '<style>p{}</style><script>alert(1)</script><p>Hi <a href="javascript:alert(1)">there</a> &amp; <b>you</b></p>',
      ),
    );
    expect(r).toEqual({ ok: true, markdown: 'Hi there & **you**' });
  });

  test('Word files convert, by extension or by content', () => {
    const doc = makeDocx('<w:p><w:r><w:t>Ada Lovelace</w:t></w:r></w:p>');
    expect(convertDocument('cv.docx', doc)).toEqual({ ok: true, markdown: 'Ada Lovelace' });
    expect(convertDocument('cv.dat', doc)).toEqual({ ok: true, markdown: 'Ada Lovelace' });
  });

  test('typed errors', () => {
    expect(failure('a.txt', new Uint8Array(0)).code).toBe('empty');
    expect(failure('a.txt', bytes('  \n ')).code).toBe('empty');
    expect(failure('a.doc', bytes('x')).error).toContain('DOC');
    expect(failure('a.rtf', bytes('x')).code).toBe('unsupported');
    expect(failure('a.png', bytes('x')).code).toBe('unsupported');
    expect(failure('a.docx', bytes('not a zip')).code).toBe('corrupt');
    expect(failure('a.docx', makeZip([{ name: 'x', data: 'y' }])).code).toBe('corrupt');
    expect(
      failure('a.pdf', makePdf({ content: 'BT ET', trailerExtra: '/Encrypt 9 0 R' })).code,
    ).toBe('pdf-encrypted');
    expect(failure('a.pdf', makePdf({ content: 'q Q' })).code).toBe('pdf-no-text');
    expect(failure('a.pdf', bytes('%PDF-1.4 junk')).code).toBe('corrupt');
  });

  test('a file past the size ceiling is refused without reading it', () => {
    expect(failure('big.txt', new Uint8Array(26 * 1024 * 1024)).code).toBe('too-large');
  });

  test('hostile bytes never throw', () => {
    let seed = 7;
    const rnd = (): number => {
      seed = (seed * 1103515245 + 12345) & 0x7fffffff;
      return seed;
    };
    const base = goFixture('resume.pdf');
    for (let i = 0; i < 40; i++) {
      const mutated = base.slice(0, 20000 + (rnd() % 40000));
      for (let k = 0; k < 30; k++) mutated[rnd() % mutated.length] = rnd() & 255;
      expect(() => convertDocument('x.pdf', mutated)).not.toThrow();
    }
    const docx = makeDocx('<w:p><w:r><w:t>Hi</w:t></w:r></w:p>');
    for (let i = 0; i < 40; i++) {
      const mutated = docx.slice();
      for (let k = 0; k < 5; k++) mutated[rnd() % mutated.length] = rnd() & 255;
      expect(() => convertDocument('x.docx', mutated)).not.toThrow();
    }
  });

  test('the picker list names only what the converter reads', () => {
    for (const ext of RESUME_ACCEPT) {
      const r = convertDocument(
        `a${ext}`,
        ext === '.docx'
          ? makeDocx('<w:p><w:r><w:t>x</w:t></w:r></w:p>')
          : ext === '.pdf'
            ? makePdf({ content: 'BT /F1 9 Tf (x) Tj ET' })
            : bytes('x'),
      );
      expect(r.ok).toBe(true);
    }
  });
});
