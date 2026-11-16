// Resume file conversion on the device: the TypeScript port of the Go core's ports.DocumentConverter
// (internal/adapters/secondary/documents, ADR-0016). It takes a file's bytes and name and returns Markdown, so the
// app's file picker reads the file (storage.fs.readFile) and hands the bytes here; no path and no native library.
//
// What it reads, as the Go ConvertBytes does: a PDF is recognised by its bytes, never its name; anything else is
// chosen by the file's extension. Word (.docx), plain text and Markdown, and HTML are converted. Everything else is a
// typed 'unsupported' result naming the format. The file is untrusted: nothing in it is executed or fetched, sizes are
// capped, and a damaged file is a 'corrupt' result, never an exception.
//
// PDF is read by our own pure-TypeScript extractor (pdf-extract.ts). It covers the PDFs word processors and
// browsers write; the ones it cannot read come back as 'pdf-encrypted', 'pdf-unsupported' or 'pdf-no-text' with the
// reason, so the screen can say what to do (paste the text instead) rather than store a mangled resume.
import { fail, isFailure, type ConvertResult } from './document-error.ts';
import { docxToMarkdown } from './docx.ts';
import { htmlToMarkdown } from './html-to-markdown.ts';
import { extractPdf } from './pdf-extract.ts';
import { decodeUtf16, decodeUtf8 } from './utf8.ts';
import { isZip, readZipEntries } from './zip-reader.ts';

export const MAX_DOCUMENT_BYTES = 25 * 1024 * 1024;

// Go's documentExtension: a dot and a few letters or digits, nothing a client-sent name could use to look like a path.
const EXTENSION = /^\.[a-z0-9]{1,10}$/;

const TEXT = new Set(['.txt', '.text', '.md', '.markdown', '.mdown']);
const HTML = new Set(['.html', '.htm', '.xhtml']);
const WORD = new Set(['.docx', '.docm', '.dotx']);

// The file picker's `accept` list for a resume: what convertDocument can read.
export const RESUME_ACCEPT = ['.pdf', '.docx', '.txt', '.md', '.markdown', '.html', '.htm'];

// A PDF by signature, allowing a little junk before the header as readers do (the Go isPDF).
export function isPdf(data: Uint8Array): boolean {
  const limit = Math.min(data.length, 1024);
  for (let i = 0; i < limit && i + 5 <= data.length; i++) {
    if (data[i] === 0x25 && data[i + 1] === 0x50 && data[i + 2] === 0x44 && data[i + 3] === 0x46 && data[i + 4] === 0x2d) return true;
  }
  return false;
}

function extensionOf(name: string): string {
  const base = name.replace(/\\/g, '/').split('/').pop() ?? '';
  const dot = base.lastIndexOf('.');
  return dot < 0 ? '' : base.slice(dot).toLowerCase();
}

function textOf(data: Uint8Array): string {
  let s: string;
  if (data.length >= 2 && data[0] === 0xff && data[1] === 0xfe) s = decodeUtf16(data, true, 2);
  else if (data.length >= 2 && data[0] === 0xfe && data[1] === 0xff) s = decodeUtf16(data, false, 2);
  else s = decodeUtf8(data);
  return s.replace(/\r\n?/g, '\n').replace(/\u0000/g, '').trim();
}

export function convertDocument(name: string, data: Uint8Array): ConvertResult {
  const label = name.replace(/\\/g, '/').split('/').pop() || 'The file';
  try {
    if (data.length === 0) fail('empty', `${label} is empty.`);
    if (data.length > MAX_DOCUMENT_BYTES) fail('too-large', `${label} is larger than the device will read.`);
    let markdown: string;
    if (isPdf(data)) markdown = extractPdf(data);
    else {
      const ext = extensionOf(name);
      if (ext === '' || !EXTENSION.test(ext)) fail('no-extension', `"${label}" has no extension that says what kind of document it is.`);
      if (WORD.has(ext) || (isZip(data) && readZipEntries(data).has('word/document.xml'))) markdown = docxToMarkdown(data);
      else if (HTML.has(ext)) markdown = htmlToMarkdown(textOf(data));
      else if (TEXT.has(ext)) markdown = textOf(data);
      else if (ext === '.doc' || ext === '.rtf' || ext === '.odt' || ext === '.pages') fail('unsupported', `${label} is a ${ext.slice(1).toUpperCase()} file, which the device cannot read. Save it as .docx or PDF, or paste the text.`);
      else fail('unsupported', `${label} is a ${ext} file, which the device cannot read. A PDF, .docx, text or Markdown file works.`);
    }
    markdown = markdown.trim();
    if (markdown === '') fail('empty', `${label} has no text.`);
    return { ok: true, markdown };
  } catch (e) {
    if (isFailure(e)) return { ok: false, code: e.code, error: e.message };
    // A bug or an engine limit (such as a stack overflow on hostile nesting) must still not escape as a throw.
    return { ok: false, code: 'corrupt', error: `${label} could not be read.` };
  }
}
