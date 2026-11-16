// How a PDF font turns the bytes of a text-showing operator into characters, and how wide each one is.
// A character comes from the font's ToUnicode CMap when it has one (every modern producer writes one), otherwise from
// its simple-font encoding (WinAnsi, MacRoman, a Differences array of glyph names). A font with neither, such as a
// CID font with Identity encoding and no ToUnicode, has no recoverable text: its characters become U+FFFD and are
// counted, so the extractor can say so instead of returning garbage.
import { isDict, isName, PdfDoc, type PdfDict, type PdfValue } from './pdf-parse.ts';
import { decodeUtf16, latin1 } from './utf8.ts';

export interface Glyph { text: string; width: number; space: boolean; unmapped: boolean }

export interface PdfFont {
  // Splits a shown string into glyphs. width is in 1/1000 em.
  decode(bytes: Uint8Array): Glyph[];
  // False when the font declares no widths (the standard 14), so pen positions are only estimates.
  hasWidths: boolean;
}

const MAX_CMAP_ENTRIES = 140000;

export interface CMap { map: Map<number, string>; codeLengths: number[] }

// Parses a ToUnicode CMap: bfchar, bfrange (both destination forms) and codespace ranges.
export function parseToUnicode(src: Uint8Array): CMap {
  const text = latin1(src);
  const map = new Map<number, string>();
  const lengths = new Set<number>();
  const hexRe = /<([0-9a-fA-F\s]*)>/g;
  const hexBytes = (h: string): Uint8Array => {
    const c = h.replace(/\s+/g, '');
    const out = new Uint8Array(Math.ceil(c.length / 2));
    for (let i = 0; i < out.length; i++) out[i] = parseInt(c.substr(i * 2, 2).padEnd(2, '0'), 16);
    return out;
  };
  const code = (h: string): number => { let v = 0; for (const b of hexBytes(h)) v = v * 256 + b; return v; };
  const dest = (h: string): string => decodeUtf16(hexBytes(h), false);
  const section = (begin: string, end: string, fn: (body: string) => void): void => {
    let from = 0;
    for (;;) {
      const s = text.indexOf(begin, from);
      if (s < 0) return;
      const e = text.indexOf(end, s + begin.length);
      const bodyEnd = e < 0 ? text.length : e;
      fn(text.slice(s + begin.length, bodyEnd));
      if (e < 0) return;
      from = e + end.length;
    }
  };
  section('begincodespacerange', 'endcodespacerange', (body) => {
    for (let m = hexRe.exec(body); m; m = hexRe.exec(body)) lengths.add(Math.max(1, Math.ceil(m[1]!.replace(/\s+/g, '').length / 2)));
  });
  section('beginbfchar', 'endbfchar', (body) => {
    const toks = body.match(/<[0-9a-fA-F\s]*>/g) ?? [];
    for (let i = 0; i + 1 < toks.length && map.size < MAX_CMAP_ENTRIES; i += 2) map.set(code(toks[i]!.slice(1, -1)), dest(toks[i + 1]!.slice(1, -1)));
  });
  section('beginbfrange', 'endbfrange', (body) => {
    const re = /<([0-9a-fA-F\s]*)>\s*<([0-9a-fA-F\s]*)>\s*(\[[^\]]*\]|<[0-9a-fA-F\s]*>)/g;
    for (let m = re.exec(body); m && map.size < MAX_CMAP_ENTRIES; m = re.exec(body)) {
      const lo = code(m[1]!), hi = Math.min(code(m[2]!), lo + 65535);
      const d = m[3]!;
      if (d[0] === '[') {
        const items = d.match(/<[0-9a-fA-F\s]*>/g) ?? [];
        for (let i = 0; i < items.length && lo + i <= hi; i++) map.set(lo + i, dest(items[i]!.slice(1, -1)));
      } else {
        const base = dest(d.slice(1, -1));
        if (base === '') continue;
        const last = base.charCodeAt(base.length - 1);
        for (let c = lo; c <= hi && map.size < MAX_CMAP_ENTRIES; c++) map.set(c, base.slice(0, -1) + String.fromCharCode(last + (c - lo)));
      }
    }
  });
  return { map, codeLengths: Array.from(lengths).sort((a, b) => a - b) };
}

// --- simple-font encodings ------------------------------------------------------------------------------------

const ASCII_NAMES = 'space exclam quotedbl numbersign dollar percent ampersand quotesingle parenleft parenright asterisk plus comma hyphen period slash zero one two three four five six seven eight nine colon semicolon less equal greater question at A B C D E F G H I J K L M N O P Q R S T U V W X Y Z bracketleft backslash bracketright asciicircum underscore grave a b c d e f g h i j k l m n o p q r s t u v w x y z braceleft bar braceright asciitilde'.split(' ');
const LATIN1_NAMES = 'space exclamdown cent sterling currency yen brokenbar section dieresis copyright ordfeminine guillemotleft logicalnot hyphen registered macron degree plusminus twosuperior threesuperior acute mu paragraph periodcentered cedilla onesuperior ordmasculine guillemotright onequarter onehalf threequarters questiondown Agrave Aacute Acircumflex Atilde Adieresis Aring AE Ccedilla Egrave Eacute Ecircumflex Edieresis Igrave Iacute Icircumflex Idieresis Eth Ntilde Ograve Oacute Ocircumflex Otilde Odieresis multiply Oslash Ugrave Uacute Ucircumflex Udieresis Yacute Thorn germandbls agrave aacute acircumflex atilde adieresis aring ae ccedilla egrave eacute ecircumflex edieresis igrave iacute icircumflex idieresis eth ntilde ograve oacute ocircumflex otilde odieresis divide oslash ugrave uacute ucircumflex udieresis yacute thorn ydieresis'.split(' ');
const WIN_HIGH = '€�‚ƒ„…†‡ˆ‰Š‹Œ�Ž��‘’“”•–—˜™š›œ�žŸ';
const MAC_HIGH = 'ÄÅÇÉÑÖÜáàâäãåçéèêëíìîïñóòôöõúùûü†°¢£§•¶ß®©™´¨≠ÆØ∞±≤≥¥µ∂∑∏π∫ªºΩæø¿¡¬√ƒ≈∆«»… ÀÃÕŒœ–—“”‘’÷◊ÿŸ⁄€‹›ﬁﬂ‡·‚„‰ÂÊÁËÈÍÎÏÌÓÔÒÚÛÙıˆ˜¯˘˙˚¸˝˛ˇ';

const EXTRA_NAMES: Record<string, string> = {
  Euro: '€', bullet: '•', endash: '–', emdash: '—', quoteleft: '‘', quoteright: '’', quotedblleft: '“',
  quotedblright: '”', quotesinglbase: '‚', quotedblbase: '„', ellipsis: '…', dagger: '†', daggerdbl: '‡',
  perthousand: '‰', florin: 'ƒ', circumflex: 'ˆ', tilde: '˜', OE: 'Œ', oe: 'œ', Scaron: 'Š', scaron: 'š',
  Zcaron: 'Ž', zcaron: 'ž', Ydieresis: 'Ÿ', guilsinglleft: '‹', guilsinglright: '›', trademark: '™',
  fi: 'ﬁ', fl: 'ﬂ', ff: 'ﬀ', ffi: 'ﬃ', ffl: 'ﬄ', minus: '−', fraction: '⁄', dotlessi: 'ı',
  Lslash: 'Ł', lslash: 'ł', nbspace: ' ', sfthyphen: '­', middot: '·', bar: '|', uni00A0: ' ',
};

const NAME_TO_UNICODE = new Map<string, string>();
ASCII_NAMES.forEach((n, i) => NAME_TO_UNICODE.set(n, String.fromCharCode(32 + i)));
LATIN1_NAMES.forEach((n, i) => NAME_TO_UNICODE.set(n, String.fromCharCode(0xa0 + i)));
for (const k of Object.keys(EXTRA_NAMES)) NAME_TO_UNICODE.set(k, EXTRA_NAMES[k]!);
NAME_TO_UNICODE.set('space', ' ');
NAME_TO_UNICODE.set('hyphen', '-');

export function glyphNameToUnicode(name: string): string | undefined {
  const known = NAME_TO_UNICODE.get(name);
  if (known !== undefined) return known;
  let m = /^uni([0-9A-F]{4})$/.exec(name);
  if (m) return String.fromCharCode(parseInt(m[1]!, 16));
  m = /^u([0-9A-F]{4,6})$/.exec(name);
  if (m) { const cp = parseInt(m[1]!, 16); return cp <= 0x10ffff && (cp < 0xd800 || cp >= 0xe000) ? String.fromCodePoint(cp) : undefined; }
  const dot = name.indexOf('.');
  if (dot > 0) return glyphNameToUnicode(name.slice(0, dot));
  return undefined;
}

function baseEncoding(name: string | undefined): string[] {
  const table: string[] = [];
  for (let c = 0; c < 256; c++) {
    if (c >= 32 && c < 127) table.push(String.fromCharCode(c));
    else if (c >= 0xa0) table.push(name === 'MacRomanEncoding' ? MAC_HIGH[c - 128]! : String.fromCharCode(c));
    else if (c >= 128) table.push(name === 'MacRomanEncoding' ? MAC_HIGH[c - 128]! : WIN_HIGH[c - 128]!);
    else table.push('�');
  }
  if (name === 'MacRomanEncoding') table[0xca] = ' ';
  return table;
}

// --- building fonts -------------------------------------------------------------------------------------------

export function buildFont(doc: PdfDoc, fontDict: PdfDict): PdfFont {
  const subtype = doc.name(fontDict.m.get('Subtype'));
  const toUni = doc.dict(fontDict.m.get('ToUnicode'));
  let cmap: CMap | undefined;
  if (toUni?.stream) { try { cmap = parseToUnicode(doc.streamBytes(toUni)); } catch { cmap = undefined; } }

  if (subtype === 'Type0') {
    const desc = doc.dict(doc.arr(fontDict.m.get('DescendantFonts'))?.[0]);
    const dw = (desc && doc.num(desc.m.get('DW'))) ?? 1000;
    const widths = new Map<number, number>();
    const w = desc ? doc.arr(desc.m.get('W')) : undefined;
    if (w) {
      for (let i = 0; i < w.length && widths.size < 70000;) {
        const first = doc.num(w[i]);
        const next = doc.get(w[i + 1]);
        if (first === undefined) { i++; continue; }
        if (Array.isArray(next)) {
          next.forEach((v, k) => { const n = doc.num(v); if (n !== undefined) widths.set(first + k, n); });
          i += 2;
        } else {
          const last = typeof next === 'number' ? next : undefined, width = doc.num(w[i + 2]);
          if (last !== undefined && width !== undefined) for (let c = first; c <= Math.min(last, first + 65535); c++) widths.set(c, width);
          i += 3;
        }
      }
    }
    const lens = cmap && cmap.codeLengths.length > 0 ? cmap.codeLengths : [2];
    const codeLen = lens[lens.length - 1]!;
    return {
      hasWidths: true,
      decode(bytes) {
        const out: Glyph[] = [];
        for (let i = 0; i + codeLen <= bytes.length; i += codeLen) {
          let code = 0;
          for (let k = 0; k < codeLen; k++) code = code * 256 + bytes[i + k]!;
          const t = cmap?.map.get(code);
          out.push({ text: t ?? '�', width: widths.get(code) ?? dw, space: false, unmapped: t === undefined });
        }
        return out;
      },
    };
  }

  // Simple fonts: Type1, MMType1, TrueType, Type3.
  const enc = doc.get(fontDict.m.get('Encoding'));
  let baseName: string | undefined;
  let diffs: PdfValue[] | undefined;
  if (isName(enc)) baseName = enc.v;
  else if (isDict(enc)) { baseName = doc.name(enc.m.get('BaseEncoding')); diffs = doc.arr(enc.m.get('Differences')); }
  const table = baseEncoding(baseName);
  const named = new Set<number>();
  if (diffs) {
    let code = 0;
    for (const d of diffs) {
      const v = doc.get(d);
      if (typeof v === 'number') code = v;
      else if (isName(v)) {
        if (code >= 0 && code < 256) { table[code] = glyphNameToUnicode(v.v) ?? '�'; named.add(code); }
        code++;
      }
    }
  }
  const first = doc.num(fontDict.m.get('FirstChar')) ?? 0;
  const wArr = doc.arr(fontDict.m.get('Widths'));
  const desc = doc.dict(fontDict.m.get('FontDescriptor'));
  // Without a Widths array (the standard 14 fonts) every glyph is estimated at half an em: enough to place link anchors.
  const missing = (desc && doc.num(desc.m.get('MissingWidth'))) ?? (wArr ? 0 : 500);
  const fm = subtype === 'Type3' ? doc.arr(fontDict.m.get('FontMatrix')) : undefined;
  const scale = (fm && doc.num(fm[0])) !== undefined ? (doc.num(fm![0]) ?? 0.001) * 1000 : 1;
  return {
    hasWidths: wArr !== undefined,
    decode(bytes) {
      const out: Glyph[] = [];
      for (const code of bytes) {
        const mapped = cmap?.map.get(code);
        const t = mapped ?? table[code]!;
        const wv = wArr ? doc.num(wArr[code - first]) : undefined;
        out.push({ text: t, width: wv !== undefined ? wv * scale : missing, space: code === 32, unmapped: t === '�' });
      }
      return out;
    },
  };
}
