// Why a file could not be turned into Markdown. Conversion never throws across its public API (see document-converter.ts);
// internal code throws a Failure, and the converter turns it into { ok: false, code, error }.
export type ConvertErrorCode =
  | 'no-extension' // the name does not say what kind of document it is (the Go port's "no extension" error)
  | 'unsupported' // a format with no converter on the device (.doc, .rtf, .odt, images, ...)
  | 'corrupt' // the bytes are not the format they claim to be
  | 'too-large' // past a limit that guards the device against a hostile file
  | 'empty' // converted fine, but there is no text in it
  | 'pdf-encrypted' // password-protected PDF
  | 'pdf-unsupported' // a PDF feature the pure-TypeScript reader does not handle (named in the message)
  | 'pdf-no-text'; // a scan or an image: no text layer

export type ConvertResult =
  { ok: true; markdown: string } | { ok: false; code: ConvertErrorCode; error: string };

export interface Failure extends Error {
  code: ConvertErrorCode;
}

export function fail(code: ConvertErrorCode, message: string): never {
  const e = new Error(message) as Failure;
  e.code = code;
  throw e;
}

export function isFailure(e: unknown): e is Failure {
  return e instanceof Error && typeof (e as Partial<Failure>).code === 'string';
}
