import { describe, expect, test } from 'bun:test';
import { docxToMarkdown } from './docx.ts';
import { makeDocx, makeZip } from './document-fixtures.ts';
import { readZipEntries, readZipEntry } from './zip-reader.ts';

const p = (text: string, pPr = ''): string =>
  `<w:p>${pPr ? `<w:pPr>${pPr}</w:pPr>` : ''}<w:r><w:t xml:space="preserve">${text}</w:t></w:r></w:p>`;
const code = (f: () => unknown): string => {
  try {
    f();
    return 'no error';
  } catch (e) {
    return (e as { code?: string }).code ?? String(e);
  }
};

describe('zip', () => {
  test('reads stored and deflated entries', () => {
    const z = makeZip([
      { name: 'a.txt', data: 'stored', stored: true },
      { name: 'b.txt', data: 'deflated '.repeat(100) },
    ]);
    const entries = readZipEntries(z);
    expect(new TextDecoder().decode(readZipEntry(z, entries.get('a.txt')!))).toBe('stored');
    expect(new TextDecoder().decode(readZipEntry(z, entries.get('b.txt')!))).toBe(
      'deflated '.repeat(100),
    );
  });
  test('an entry that lies about its size is corrupt', () => {
    const z = makeZip([{ name: 'a.txt', data: 'hello hello hello', lieSize: 5 }]);
    expect(code(() => readZipEntry(z, readZipEntries(z).get('a.txt')!))).toBe('corrupt');
  });
  test('an entry declaring more than the cap is refused before inflating', () => {
    const z = makeZip([{ name: 'a.txt', data: 'x', lieSize: 0x7fffffff }]);
    expect(code(() => readZipEntry(z, readZipEntries(z).get('a.txt')!))).toBe('too-large');
  });
  test('not a zip', () => {
    expect(
      code(() => readZipEntries(new TextEncoder().encode('plainly not a zip archive at all'))),
    ).toBe('corrupt');
  });
});

describe('docx', () => {
  test('headings, paragraphs, bold and italic', () => {
    const md = docxToMarkdown(
      makeDocx(
        p('Ada Lovelace', '<w:pStyle w:val="Heading1"/>') +
          p('Engineer', '') +
          '<w:p><w:r><w:rPr><w:b/></w:rPr><w:t xml:space="preserve">Led </w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t>payments</w:t></w:r><w:r><w:t xml:space="preserve"> and </w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>search</w:t></w:r></w:p>' +
          p('Skills', '<w:pStyle w:val="Heading2"/>'),
      ),
    );
    expect(md).toBe('# Ada Lovelace\n\nEngineer\n\n**Led payments** and *search*\n\n## Skills');
  });

  test('bullet and numbered lists, nested', () => {
    const numbering =
      '<w:numbering xmlns:w="x"><w:abstractNum w:abstractNumId="0"><w:lvl w:ilvl="0"><w:numFmt w:val="bullet"/></w:lvl></w:abstractNum><w:abstractNum w:abstractNumId="1"><w:lvl w:ilvl="0"><w:numFmt w:val="decimal"/></w:lvl></w:abstractNum><w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num><w:num w:numId="2"><w:abstractNumId w:val="1"/></w:num></w:numbering>';
    const item = (t: string, id: number, lvl = 0): string =>
      p(t, `<w:numPr><w:ilvl w:val="${lvl}"/><w:numId w:val="${id}"/></w:numPr>`);
    const md = docxToMarkdown(
      makeDocx(
        item('Built the pipeline', 1) +
          item('Cut latency', 1) +
          item('nested', 1, 1) +
          p('Then') +
          item('first', 2) +
          item('second', 2),
        [{ name: 'word/numbering.xml', data: numbering }],
      ),
    );
    expect(md).toBe(
      '- Built the pipeline\n- Cut latency\n  - nested\n\nThen\n\n1. first\n2. second',
    );
  });

  test('hyperlinks keep their address; internal anchors keep only the text', () => {
    const rels =
      '<Relationships><Relationship Id="rId1" Type="x" Target="https://github.com/adalovelace" TargetMode="External"/><Relationship Id="rId2" Type="x" Target="file:///etc/passwd" TargetMode="External"/></Relationships>';
    const md = docxToMarkdown(
      makeDocx(
        '<w:p><w:hyperlink r:id="rId1"><w:r><w:t>Github</w:t></w:r></w:hyperlink><w:r><w:t xml:space="preserve"> | </w:t></w:r><w:hyperlink r:id="rId2"><w:r><w:t>Local</w:t></w:r></w:hyperlink><w:r><w:t xml:space="preserve"> | </w:t></w:r><w:hyperlink w:anchor="top"><w:r><w:t>Top</w:t></w:r></w:hyperlink></w:p>',
        [{ name: 'word/_rels/document.xml.rels', data: rels }],
      ),
    );
    expect(md).toBe('[Github](https://github.com/adalovelace) | Local | Top');
  });

  test('a table becomes a Markdown table, with pipes escaped', () => {
    const cell = (t: string): string => `<w:tc>${p(t)}</w:tc>`;
    const md = docxToMarkdown(
      makeDocx(
        `<w:tbl><w:tr>${cell('Role')}${cell('Years')}</w:tr><w:tr>${cell('Staff | Lead')}${cell('3')}</w:tr></w:tbl>`,
      ),
    );
    expect(md).toBe('| Role | Years |\n| --- | --- |\n| Staff \\| Lead | 3 |');
  });

  test('tabs, line breaks, text boxes and tracked deletions', () => {
    const md = docxToMarkdown(
      makeDocx(
        '<w:p><w:r><w:t>Berlin</w:t><w:tab/><w:t>Germany</w:t><w:br/><w:t>Second line</w:t></w:r></w:p>' +
          '<w:p><w:del><w:r><w:delText>deleted</w:delText></w:r></w:del><w:ins><w:r><w:t>inserted</w:t></w:r></w:ins></w:p>' +
          '<w:p><w:r><w:txbxContent>' +
          p('In a text box') +
          '</w:txbxContent></w:r></w:p>',
      ),
    );
    expect(md).toBe('Berlin Germany\nSecond line\n\ninserted\n\nIn a text box');
  });

  test('entities are decoded and a DOCTYPE entity is never expanded', () => {
    const xml =
      '<?xml version="1.0"?><!DOCTYPE d [<!ENTITY lol "lol lol lol"><!ENTITY lol2 "&lol;&lol;">]><w:document xmlns:w="x"><w:body><w:p><w:r><w:t>R&amp;D &lt;lead&gt; &#169; &lol2; &#x41;</w:t></w:r></w:p></w:body></w:document>';
    const md = docxToMarkdown(makeZip([{ name: 'word/document.xml', data: xml }]));
    expect(md).toBe('R&D <lead> © &lol2; A');
  });

  test('a zip with no document.xml is not a Word file', () => {
    expect(code(() => docxToMarkdown(makeZip([{ name: 'other.xml', data: '<a/>' }])))).toBe(
      'corrupt',
    );
  });

  test('absurdly deep nesting is refused, not a stack overflow', () => {
    const deep = '<a>'.repeat(5000);
    expect(code(() => docxToMarkdown(makeDocx(deep)))).toBe('too-large');
  });
});
