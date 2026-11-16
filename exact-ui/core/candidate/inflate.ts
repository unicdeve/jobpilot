// DEFLATE (RFC 1951) in plain TypeScript, for Word files (a zip) and PDF streams (FlateDecode). Hermes has no
// DecompressionStream and no zlib, and ADR-0037 adds no dependency. The decoder follows zlib's puff.c: canonical
// Huffman codes decoded a bit at a time. It is small and slow, which suits a resume. The input is untrusted, so the
// output is capped and every read is bounds-checked.
import { fail } from './document-error.ts';

interface Huffman { count: Uint16Array; symbol: Uint16Array }

const LEN_BASE = [3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 15, 17, 19, 23, 27, 31, 35, 43, 51, 59, 67, 83, 99, 115, 131, 163, 195, 227, 258];
const LEN_EXTRA = [0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5, 0];
const DIST_BASE = [1, 2, 3, 4, 5, 7, 9, 13, 17, 25, 33, 49, 65, 97, 129, 193, 257, 385, 513, 769, 1025, 1537, 2049, 3073, 4097, 6145, 8193, 12289, 16385, 24577];
const DIST_EXTRA = [0, 0, 0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13, 13];
const CL_ORDER = [16, 17, 18, 0, 8, 7, 9, 6, 10, 5, 11, 4, 12, 3, 13, 2, 14, 1, 15];

function build(lengths: ArrayLike<number>, n: number): Huffman {
  const count = new Uint16Array(16);
  for (let i = 0; i < n; i++) count[lengths[i]!]!++;
  const offs = new Uint16Array(16);
  for (let len = 1; len < 15; len++) offs[len + 1] = offs[len]! + count[len]!;
  const symbol = new Uint16Array(n);
  for (let i = 0; i < n; i++) if (lengths[i] !== 0) symbol[offs[lengths[i]!]!++] = i;
  return { count, symbol };
}

let fixedTables: { lit: Huffman; dist: Huffman } | undefined;
function fixed(): { lit: Huffman; dist: Huffman } {
  if (fixedTables) return fixedTables;
  const l = new Uint8Array(288);
  for (let i = 0; i < 144; i++) l[i] = 8;
  for (let i = 144; i < 256; i++) l[i] = 9;
  for (let i = 256; i < 280; i++) l[i] = 7;
  for (let i = 280; i < 288; i++) l[i] = 8;
  const d = new Uint8Array(30).fill(5);
  fixedTables = { lit: build(l, 288), dist: build(d, 30) };
  return fixedTables;
}

// Inflates a raw DEFLATE stream that starts at `start`. Fails with 'corrupt' on bad data and 'too-large' past maxOut.
export function inflateRaw(data: Uint8Array, start: number, maxOut: number): Uint8Array {
  let pos = start;
  let bitBuf = 0;
  let bitCnt = 0;
  let out = new Uint8Array(Math.min(Math.max(data.length * 4, 1024), maxOut));
  let outLen = 0;

  const bits = (need: number): number => {
    let val = bitBuf;
    while (bitCnt < need) {
      if (pos >= data.length) fail('corrupt', 'The compressed data ends too soon.');
      val |= data[pos++]! << bitCnt;
      bitCnt += 8;
    }
    bitBuf = val >>> need;
    bitCnt -= need;
    return val & ((1 << need) - 1);
  };
  const decode = (h: Huffman): number => {
    let code = 0, first = 0, index = 0;
    for (let len = 1; len <= 15; len++) {
      code |= bits(1);
      const count = h.count[len]!;
      if (code - count < first) return h.symbol[index + (code - first)]!;
      index += count;
      first += count;
      first <<= 1;
      code <<= 1;
    }
    return fail('corrupt', 'The compressed data has an invalid code.');
  };
  const put = (b: number): void => {
    if (outLen >= out.length) {
      if (out.length >= maxOut) fail('too-large', 'The file expands to more than the device will read.');
      const bigger = new Uint8Array(Math.min(out.length * 2, maxOut));
      bigger.set(out);
      out = bigger;
    }
    out[outLen++] = b;
  };

  let last = 0;
  do {
    last = bits(1);
    const type = bits(2);
    if (type === 0) {
      bitBuf = 0; bitCnt = 0;
      if (pos + 4 > data.length) fail('corrupt', 'The compressed data ends too soon.');
      const len = data[pos]! | (data[pos + 1]! << 8);
      const nlen = data[pos + 2]! | (data[pos + 3]! << 8);
      if ((len ^ 0xffff) !== nlen) fail('corrupt', 'The compressed data has a bad block length.');
      pos += 4;
      if (pos + len > data.length) fail('corrupt', 'The compressed data ends too soon.');
      for (let i = 0; i < len; i++) put(data[pos + i]!);
      pos += len;
    } else if (type === 1 || type === 2) {
      let lit: Huffman, dist: Huffman;
      if (type === 1) ({ lit, dist } = fixed());
      else {
        const nlen = bits(5) + 257, ndist = bits(5) + 1, ncode = bits(4) + 4;
        if (nlen > 286 || ndist > 30) fail('corrupt', 'The compressed data has a bad table.');
        const cl = new Uint8Array(19);
        for (let i = 0; i < ncode; i++) cl[CL_ORDER[i]!] = bits(3);
        const clh = build(cl, 19);
        const lengths = new Uint8Array(nlen + ndist);
        let i = 0;
        while (i < nlen + ndist) {
          const sym = decode(clh);
          if (sym < 16) lengths[i++] = sym;
          else {
            let prev = 0, rep: number;
            if (sym === 16) {
              if (i === 0) fail('corrupt', 'The compressed data has a bad table.');
              prev = lengths[i - 1]!;
              rep = 3 + bits(2);
            } else if (sym === 17) rep = 3 + bits(3);
            else rep = 11 + bits(7);
            if (i + rep > nlen + ndist) fail('corrupt', 'The compressed data has a bad table.');
            while (rep--) lengths[i++] = prev;
          }
        }
        lit = build(lengths.subarray(0, nlen), nlen);
        dist = build(lengths.subarray(nlen), ndist);
      }
      for (;;) {
        const sym = decode(lit);
        if (sym < 256) put(sym);
        else if (sym === 256) break;
        else {
          const s = sym - 257;
          if (s >= 29) fail('corrupt', 'The compressed data has an invalid length.');
          const len = LEN_BASE[s]! + bits(LEN_EXTRA[s]!);
          const ds = decode(dist);
          if (ds >= 30) fail('corrupt', 'The compressed data has an invalid distance.');
          const d = DIST_BASE[ds]! + bits(DIST_EXTRA[ds]!);
          if (d > outLen) fail('corrupt', 'The compressed data points before its own start.');
          for (let i = 0; i < len; i++) put(out[outLen - d]!);
        }
      }
    } else fail('corrupt', 'The compressed data has an invalid block.');
  } while (!last);
  return out.slice(0, outLen);
}

// A zlib stream (PDF FlateDecode): a two-byte header, then DEFLATE. The Adler-32 trailer is not checked, as
// readers tolerate a truncated one and the text is checked downstream.
export function inflateZlib(data: Uint8Array, maxOut: number): Uint8Array {
  if (data.length < 2 || (data[0]! & 0x0f) !== 8 || ((data[0]! << 8) | data[1]!) % 31 !== 0 || (data[1]! & 0x20) !== 0) {
    fail('corrupt', 'The compressed data has no valid header.');
  }
  return inflateRaw(data, 2, maxOut);
}
