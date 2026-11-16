// A PDF's object syntax and document structure, for text extraction on the device (ADR-0016's job, without PDFium,
// which is a native library Hermes cannot load). Objects are found by scanning for "N G obj" rather than trusting the
// cross-reference table, which makes damaged and incrementally updated files readable. Everything is bounds-checked:
// the file is untrusted.
import { fail } from './document-error.ts';
import { inflateZlib } from './inflate.ts';
import { latin1 } from './utf8.ts';

export interface PdfName {
  k: 'name';
  v: string;
}
export interface PdfStr {
  k: 'str';
  b: Uint8Array;
}
export interface PdfRef {
  k: 'ref';
  n: number;
}
export interface PdfOp {
  k: 'op';
  v: string;
}
export interface PdfDict {
  k: 'dict';
  m: Map<string, PdfValue>;
  stream?: Uint8Array;
}
export type PdfValue =
  number | boolean | null | PdfName | PdfStr | PdfRef | PdfOp | PdfDict | PdfValue[];

export const MAX_STREAM_BYTES = 32 * 1024 * 1024;
const MAX_DECODED_TOTAL = 96 * 1024 * 1024;
const MAX_VALUE_DEPTH = 64;

const isWs = (c: number): boolean =>
  c === 32 || c === 10 || c === 13 || c === 9 || c === 12 || c === 0;
const isDelim = (c: number): boolean =>
  c === 40 ||
  c === 41 ||
  c === 60 ||
  c === 62 ||
  c === 91 ||
  c === 93 ||
  c === 123 ||
  c === 125 ||
  c === 47 ||
  c === 37;
const isDigit = (c: number): boolean => c >= 48 && c <= 57;

export class Lexer {
  pos: number;
  constructor(
    readonly b: Uint8Array,
    pos = 0,
    private readonly allowRefs = true,
  ) {
    this.pos = pos;
  }

  skipWs(): void {
    const b = this.b;
    while (this.pos < b.length) {
      const c = b[this.pos]!;
      if (isWs(c)) this.pos++;
      else if (c === 37) {
        while (this.pos < b.length && b[this.pos] !== 10 && b[this.pos] !== 13) this.pos++;
      } else break;
    }
  }

  atEnd(): boolean {
    this.skipWs();
    return this.pos >= this.b.length;
  }

  private token(): string {
    const start = this.pos;
    while (this.pos < this.b.length && !isWs(this.b[this.pos]!) && !isDelim(this.b[this.pos]!))
      this.pos++;
    return latin1(this.b, start, this.pos);
  }

  // Next value, or an operator for a bare keyword (content streams). Returns undefined at the end of input or at a
  // closing delimiter that has no opener.
  value(depth = 0): PdfValue | undefined {
    if (depth > MAX_VALUE_DEPTH) fail('corrupt', 'The PDF is nested too deeply.');
    this.skipWs();
    const b = this.b;
    if (this.pos >= b.length) return undefined;
    const c = b[this.pos]!;
    if (c === 47) {
      this.pos++;
      return { k: 'name', v: decodeName(this.token()) };
    }
    if (c === 40) return this.literal();
    if (c === 60) {
      if (b[this.pos + 1] === 60) {
        this.pos += 2;
        return this.dict(depth);
      }
      return this.hex();
    }
    if (c === 91) {
      this.pos++;
      const arr: PdfValue[] = [];
      for (;;) {
        this.skipWs();
        if (this.pos >= b.length) return arr;
        if (b[this.pos] === 93) {
          this.pos++;
          return arr;
        }
        const v = this.value(depth + 1);
        if (v === undefined) {
          this.pos++;
          continue;
        }
        arr.push(v);
      }
    }
    if (c === 93 || c === 62 || c === 41 || c === 123 || c === 125) {
      this.pos++;
      return undefined;
    }
    const start = this.pos;
    const t = this.token();
    if (t === '') {
      this.pos = start + 1;
      return undefined;
    }
    const first = t.charCodeAt(0);
    if (isDigit(first) || first === 43 || first === 45 || first === 46) {
      const n = parseNumber(t);
      if (n !== undefined) {
        if (this.allowRefs && Number.isInteger(n) && n >= 0 && /^\d+$/.test(t)) {
          const save = this.pos;
          this.skipWs();
          const g = this.token();
          if (/^\d{1,5}$/.test(g)) {
            this.skipWs();
            if (
              b[this.pos] === 82 /* R */ &&
              (this.pos + 1 >= b.length || isWs(b[this.pos + 1]!) || isDelim(b[this.pos + 1]!))
            ) {
              this.pos++;
              return { k: 'ref', n };
            }
          }
          this.pos = save;
        }
        return n;
      }
    }
    if (t === 'true') return true;
    if (t === 'false') return false;
    if (t === 'null') return null;
    return { k: 'op', v: t };
  }

  private dict(depth: number): PdfDict {
    const m = new Map<string, PdfValue>();
    const b = this.b;
    for (;;) {
      this.skipWs();
      if (this.pos >= b.length) break;
      if (b[this.pos] === 62 && b[this.pos + 1] === 62) {
        this.pos += 2;
        break;
      }
      const key = this.value(depth + 1);
      if (key === undefined) continue;
      if (typeof key !== 'object' || key === null || Array.isArray(key) || key.k !== 'name')
        continue;
      this.skipWs();
      if (b[this.pos] === 62 && b[this.pos + 1] === 62) {
        m.set(key.v, null);
        this.pos += 2;
        break;
      }
      const v = this.value(depth + 1);
      if (v !== undefined) m.set(key.v, v);
    }
    return { k: 'dict', m };
  }

  private literal(): PdfStr {
    const b = this.b;
    const out: number[] = [];
    let depth = 1;
    this.pos++;
    while (this.pos < b.length) {
      let c = b[this.pos++]!;
      if (c === 92) {
        c = b[this.pos++] ?? 92;
        if (c === 110) out.push(10);
        else if (c === 114) out.push(13);
        else if (c === 116) out.push(9);
        else if (c === 98) out.push(8);
        else if (c === 102) out.push(12);
        else if (c >= 48 && c <= 55) {
          let v = c - 48;
          for (
            let i = 0;
            i < 2 && b[this.pos] !== undefined && b[this.pos]! >= 48 && b[this.pos]! <= 55;
            i++
          )
            v = v * 8 + (b[this.pos++]! - 48);
          out.push(v & 255);
        } else if (c === 13) {
          if (b[this.pos] === 10) this.pos++;
        } else if (c === 10) {
          /* continuation */
        } else out.push(c);
      } else if (c === 40) {
        depth++;
        out.push(c);
      } else if (c === 41) {
        if (--depth === 0) break;
        out.push(c);
      } else out.push(c);
    }
    return { k: 'str', b: Uint8Array.from(out) };
  }

  private hex(): PdfStr {
    const b = this.b;
    const out: number[] = [];
    let hi = -1;
    this.pos++;
    while (this.pos < b.length) {
      const c = b[this.pos++]!;
      if (c === 62) break;
      const v =
        c >= 48 && c <= 57
          ? c - 48
          : c >= 65 && c <= 70
            ? c - 55
            : c >= 97 && c <= 102
              ? c - 87
              : -1;
      if (v < 0) continue;
      if (hi < 0) hi = v;
      else {
        out.push(hi * 16 + v);
        hi = -1;
      }
    }
    if (hi >= 0) out.push(hi * 16);
    return { k: 'str', b: Uint8Array.from(out) };
  }
}

function parseNumber(t: string): number | undefined {
  if (!/^[+-]?(\d+\.?\d*|\.\d+)$/.test(t)) {
    // Tolerate the odd "--5" or "5-3" some writers emit by reading the leading number.
    const m = /^[+-]*(\d+\.?\d*|\.\d+)/.exec(t);
    if (!m) return undefined;
    const v = parseFloat(m[1]!);
    return t.startsWith('-') ? -v : v;
  }
  const v = parseFloat(t);
  return Number.isFinite(v) ? v : undefined;
}

function decodeName(s: string): string {
  return s.indexOf('#') < 0
    ? s
    : s.replace(/#([0-9a-fA-F]{2})/g, (_, h: string) => String.fromCharCode(parseInt(h, 16)));
}

// Typed accessors for untrusted values: a wrong type is "absent", never an exception.
export const isDict = (v: PdfValue | undefined): v is PdfDict =>
  typeof v === 'object' && v !== null && !Array.isArray(v) && v.k === 'dict';
export const isName = (v: PdfValue | undefined, name?: string): v is PdfName =>
  typeof v === 'object' &&
  v !== null &&
  !Array.isArray(v) &&
  v.k === 'name' &&
  (name === undefined || v.v === name);
export const isStr = (v: PdfValue | undefined): v is PdfStr =>
  typeof v === 'object' && v !== null && !Array.isArray(v) && v.k === 'str';
export const isRef = (v: PdfValue | undefined): v is PdfRef =>
  typeof v === 'object' && v !== null && !Array.isArray(v) && v.k === 'ref';

export class PdfDoc {
  readonly data: Uint8Array;
  private readonly offsets = new Map<number, number>();
  private readonly inStream = new Map<number, { stm: number; index: number }>();
  private readonly cache = new Map<number, PdfValue | undefined>();
  private decodedTotal = 0;
  encrypted = false;
  root: PdfValue | undefined;

  constructor(data: Uint8Array) {
    this.data = data;
    const text = latin1(data);
    const re = /(?:^|[\s>\]\d])(\d{1,10})[ \t\r\n]+(\d{1,5})[ \t\r\n]+obj(?![a-zA-Z])/g;
    const objStms: number[] = [];
    const metaObjs: number[] = [];
    for (let m = re.exec(text); m; m = re.exec(text)) {
      const num = parseInt(m[1]!, 10);
      const at = m.index + m[0].length;
      this.offsets.set(num, at);
      const head = text.substr(at, 400);
      if (head.indexOf('/ObjStm') >= 0) objStms.push(num);
      else if (head.indexOf('/XRef') >= 0) metaObjs.push(num);
    }
    const tre = /trailer[ \t\r\n]*<</g;
    for (let m = tre.exec(text); m; m = tre.exec(text)) {
      const d = new Lexer(data, m.index + m[0].length - 2).value();
      this.noteTrailer(d);
    }
    for (const n of metaObjs) this.noteTrailer(this.getObject(n));
    for (const stm of objStms) this.indexObjStm(stm);
    if (!this.encrypted && /\/Encrypt[\s/<\d]/.test(text.slice(Math.max(0, text.length - 4096))))
      this.encrypted = true;
  }

  private noteTrailer(d: PdfValue | undefined): void {
    if (!isDict(d)) return;
    if (d.m.has('Encrypt')) this.encrypted = true;
    if (d.m.has('Root')) this.root = d.m.get('Root');
  }

  private indexObjStm(stmNum: number): void {
    const stm = this.getObject(stmNum);
    if (!isDict(stm) || !stm.stream) return;
    const n = this.num(stm.m.get('N')),
      first = this.num(stm.m.get('First'));
    if (n === undefined || first === undefined || n > 100000) return;
    let bytes: Uint8Array;
    try {
      bytes = this.streamBytes(stm);
    } catch {
      return;
    }
    const lx = new Lexer(bytes, 0, false);
    for (let i = 0; i < n; i++) {
      const num = lx.value(),
        off = lx.value();
      if (typeof num !== 'number' || typeof off !== 'number') break;
      if (!this.offsets.has(num) && !this.inStream.has(num))
        this.inStream.set(num, { stm: stmNum, index: first + off });
    }
  }

  private getObject(num: number): PdfValue | undefined {
    if (this.cache.has(num)) return this.cache.get(num);
    let v: PdfValue | undefined;
    const at = this.offsets.get(num);
    if (at !== undefined) v = this.readObjectAt(at);
    else {
      const s = this.inStream.get(num);
      if (s) {
        const stm = this.getObject(s.stm);
        if (isDict(stm)) {
          try {
            v = new Lexer(this.streamBytes(stm), s.index, true).value();
          } catch {
            v = undefined;
          }
        }
      }
    }
    this.cache.set(num, v);
    return v;
  }

  private readObjectAt(at: number): PdfValue | undefined {
    const lx = new Lexer(this.data, at);
    const v = lx.value();
    if (!isDict(v)) return v;
    lx.skipWs();
    const b = this.data;
    if (b[lx.pos] === 115 && latin1(b, lx.pos, lx.pos + 6) === 'stream') {
      let p = lx.pos + 6;
      if (b[p] === 13 && b[p + 1] === 10) p += 2;
      else if (b[p] === 10 || b[p] === 13) p += 1;
      let end = -1;
      const len = this.num(v.m.get('Length'));
      if (len !== undefined && len >= 0 && p + len <= b.length) {
        const tail = latin1(b, p + len, Math.min(b.length, p + len + 16));
        if (/^\s*endstream/.test(tail)) end = p + len;
      }
      if (end < 0) {
        const text = latin1(b, p, Math.min(b.length, p + MAX_STREAM_BYTES));
        const e = text.indexOf('endstream');
        end = e < 0 ? Math.min(b.length, p + MAX_STREAM_BYTES) : p + e;
        if (e >= 0 && end > p && b[end - 1] === 10) end--;
        if (e >= 0 && end > p && b[end - 1] === 13) end--;
      }
      v.stream = b.subarray(p, end);
    }
    return v;
  }

  // Follows references (with a hop limit, so a reference loop ends).
  get(v: PdfValue | undefined): PdfValue | undefined {
    for (let hops = 0; hops < 16 && isRef(v); hops++) v = this.getObject(v.n);
    return isRef(v) ? undefined : v;
  }
  dict(v: PdfValue | undefined): PdfDict | undefined {
    const d = this.get(v);
    return isDict(d) ? d : undefined;
  }
  num(v: PdfValue | undefined): number | undefined {
    const n = this.get(v);
    return typeof n === 'number' ? n : undefined;
  }
  arr(v: PdfValue | undefined): PdfValue[] | undefined {
    const a = this.get(v);
    return Array.isArray(a) ? a : undefined;
  }
  name(v: PdfValue | undefined): string | undefined {
    const n = this.get(v);
    return isName(n) ? n.v : undefined;
  }

  // All objects, in object-number order, as dictionaries (used when the page tree is unusable).
  allDicts(): PdfDict[] {
    const out: PdfDict[] = [];
    for (const n of Array.from(this.offsets.keys()).sort((a, b) => a - b)) {
      const d = this.dict({ k: 'ref', n });
      if (d) out.push(d);
    }
    return out;
  }

  // A stream's decoded bytes. Supports Flate, ASCIIHex and ASCII85 without predictors; anything else is a typed
  // 'pdf-unsupported' failure that names the filter.
  streamBytes(d: PdfDict): Uint8Array {
    if (!d.stream) return new Uint8Array(0);
    let bytes = d.stream;
    const f = this.get(d.m.get('Filter') ?? d.m.get('F'));
    const filters = Array.isArray(f) ? f.map((x) => this.name(x)) : [this.name(f)];
    const parms = this.get(d.m.get('DecodeParms') ?? d.m.get('DP'));
    const parmList = Array.isArray(parms)
      ? parms.map((p) => this.dict(p))
      : [this.dict(parms as PdfValue)];
    for (let i = 0; i < filters.length; i++) {
      const name = filters[i];
      if (name === undefined) continue;
      const predictor = this.num(parmList[i]?.m.get('Predictor'));
      if (predictor !== undefined && predictor > 1)
        fail(
          'pdf-unsupported',
          'The PDF compresses a stream with a predictor, which the on-device reader does not handle.',
        );
      if (name === 'FlateDecode' || name === 'Fl') bytes = inflateZlib(bytes, MAX_STREAM_BYTES);
      else if (name === 'ASCIIHexDecode' || name === 'AHx') bytes = asciiHex(bytes);
      else if (name === 'ASCII85Decode' || name === 'A85') bytes = ascii85(bytes);
      else
        fail(
          'pdf-unsupported',
          `The PDF uses a ${name} stream filter, which the on-device reader does not handle.`,
        );
    }
    this.decodedTotal += bytes.length;
    if (this.decodedTotal > MAX_DECODED_TOTAL)
      fail('too-large', 'The PDF expands to more than the device will read.');
    return bytes;
  }
}

function asciiHex(b: Uint8Array): Uint8Array {
  const out: number[] = [];
  let hi = -1;
  for (const c of b) {
    if (c === 62) break;
    const v =
      c >= 48 && c <= 57 ? c - 48 : c >= 65 && c <= 70 ? c - 55 : c >= 97 && c <= 102 ? c - 87 : -1;
    if (v < 0) continue;
    if (hi < 0) hi = v;
    else {
      out.push(hi * 16 + v);
      hi = -1;
    }
  }
  if (hi >= 0) out.push(hi * 16);
  return Uint8Array.from(out);
}

function ascii85(b: Uint8Array): Uint8Array {
  const out: number[] = [];
  let group: number[] = [];
  const flush = (n: number): void => {
    let v = 0;
    for (let i = 0; i < 5; i++) v = v * 85 + (group[i] ?? 84);
    for (let i = 0; i < n - 1; i++) out.push(Math.floor(v / 2 ** (24 - 8 * i)) & 255);
  };
  for (let i = 0; i < b.length; i++) {
    const c = b[i]!;
    if (c === 126) break;
    if (isWs(c)) continue;
    if (c === 122 && group.length === 0) {
      out.push(0, 0, 0, 0);
      continue;
    }
    if (c < 33 || c > 117) continue;
    group.push(c - 33);
    if (group.length === 5) {
      flush(5);
      group = [];
    }
  }
  if (group.length > 1) flush(group.length);
  return Uint8Array.from(out);
}
