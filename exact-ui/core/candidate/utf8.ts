// UTF-8 and UTF-16 decoding without TextDecoder, which a Hermes host does not promise. Malformed input becomes U+FFFD.
export function decodeUtf8(b: Uint8Array): string {
  let s = '';
  let chunk: number[] = [];
  const flush = (): void => { s += String.fromCharCode.apply(null, chunk); chunk = []; };
  const push = (cp: number): void => {
    if (cp >= 0x10000) { cp -= 0x10000; chunk.push(0xd800 + (cp >> 10), 0xdc00 + (cp & 0x3ff)); } else chunk.push(cp);
    if (chunk.length >= 4096) flush();
  };
  let i = 0;
  if (b.length >= 3 && b[0] === 0xef && b[1] === 0xbb && b[2] === 0xbf) i = 3;
  while (i < b.length) {
    const c = b[i]!;
    let need = 0, cp = 0, min = 0;
    if (c < 0x80) { push(c); i++; continue; }
    else if (c >= 0xc2 && c < 0xe0) { need = 1; cp = c & 0x1f; min = 0x80; }
    else if (c >= 0xe0 && c < 0xf0) { need = 2; cp = c & 0x0f; min = 0x800; }
    else if (c >= 0xf0 && c < 0xf5) { need = 3; cp = c & 0x07; min = 0x10000; }
    else { push(0xfffd); i++; continue; }
    let j = 1;
    for (; j <= need; j++) {
      const n = b[i + j];
      if (n === undefined || (n & 0xc0) !== 0x80) break;
      cp = (cp << 6) | (n & 0x3f);
    }
    if (j <= need || cp < min || cp > 0x10ffff || (cp >= 0xd800 && cp < 0xe000)) { push(0xfffd); i += Math.max(1, j); continue; }
    push(cp);
    i += need + 1;
  }
  flush();
  return s;
}

export function decodeUtf16(b: Uint8Array, littleEndian: boolean, from = 0): string {
  let s = '';
  for (let i = from; i + 1 < b.length; i += 2) {
    s += String.fromCharCode(littleEndian ? b[i]! | (b[i + 1]! << 8) : (b[i]! << 8) | b[i + 1]!);
  }
  return s;
}

// Latin-1 bytes to a string (PDF names and literal strings in PDFDocEncoding are close enough for URLs and keys).
export function latin1(b: Uint8Array, from = 0, to = b.length): string {
  let s = '';
  for (let i = from; i < to; i += 4096) s += String.fromCharCode.apply(null, Array.from(b.subarray(i, Math.min(to, i + 4096))));
  return s;
}
