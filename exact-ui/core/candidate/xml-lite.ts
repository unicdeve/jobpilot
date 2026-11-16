// A small, safe XML reader for the parts of a Word file the converter reads. It is not a general XML parser:
// DOCTYPE and its entities are skipped and never expanded (so no entity bomb), only the five predefined entities and
// numeric references are decoded, and nesting depth and node count are capped. Namespace prefixes are kept as written
// ("w:p"), because Word always writes the same ones.
import { fail } from './document-error.ts';

export interface XmlNode {
  name: string;
  attrs: Record<string, string>;
  children: XmlNode[];
  text: string;
}

const MAX_DEPTH = 200;
const MAX_NODES = 2_000_000;

const ENTITIES: Record<string, string> = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'" };

export function decodeEntities(s: string): string {
  if (s.indexOf('&') < 0) return s;
  return s.replace(/&(#x[0-9a-fA-F]{1,6}|#[0-9]{1,7}|[a-zA-Z]{2,6});/g, (m, e: string) => {
    if (e[0] === '#') {
      const cp = e[1] === 'x' ? parseInt(e.slice(2), 16) : parseInt(e.slice(1), 10);
      if (!(cp > 0 && cp <= 0x10ffff) || (cp >= 0xd800 && cp < 0xe000)) return '�';
      return String.fromCodePoint(cp);
    }
    return ENTITIES[e] ?? m;
  });
}

// Text nodes are kept in `text` on a synthetic node named '#text'.
export function parseXml(src: string): XmlNode {
  const root: XmlNode = { name: '#root', attrs: {}, children: [], text: '' };
  const stack: XmlNode[] = [root];
  let nodes = 0;
  let i = 0;
  const n = src.length;
  while (i < n) {
    const lt = src.indexOf('<', i);
    const textEnd = lt < 0 ? n : lt;
    if (textEnd > i) {
      const top = stack[stack.length - 1]!;
      top.children.push({
        name: '#text',
        attrs: {},
        children: [],
        text: decodeEntities(src.slice(i, textEnd)),
      });
      if (++nodes > MAX_NODES) fail('too-large', 'The document has too many parts.');
    }
    if (lt < 0) break;
    if (src.startsWith('<!--', lt)) {
      const e = src.indexOf('-->', lt + 4);
      i = e < 0 ? n : e + 3;
      continue;
    }
    if (src.startsWith('<![CDATA[', lt)) {
      const e = src.indexOf(']]>', lt + 9);
      const end = e < 0 ? n : e;
      stack[stack.length - 1]!.children.push({
        name: '#text',
        attrs: {},
        children: [],
        text: src.slice(lt + 9, end),
      });
      i = e < 0 ? n : e + 3;
      continue;
    }
    if (src[lt + 1] === '?') {
      const e = src.indexOf('?>', lt + 2);
      i = e < 0 ? n : e + 2;
      continue;
    }
    if (src[lt + 1] === '!') {
      // A DOCTYPE, possibly with an internal subset in brackets. Skipped whole; nothing in it is expanded.
      let depth = 0,
        j = lt + 2;
      for (; j < n; j++) {
        const c = src[j];
        if (c === '[') depth++;
        else if (c === ']') depth--;
        else if (c === '>' && depth <= 0) break;
      }
      i = j + 1;
      continue;
    }
    // A tag. Find its end, honouring quotes in attribute values.
    let j = lt + 1,
      quote = '';
    for (; j < n; j++) {
      const c = src[j]!;
      if (quote) {
        if (c === quote) quote = '';
      } else if (c === '"' || c === "'") quote = c;
      else if (c === '>') break;
    }
    if (j >= n) fail('corrupt', 'The document XML is cut off.');
    const body = src.slice(lt + 1, j);
    i = j + 1;
    if (body[0] === '/') {
      if (stack.length > 1) stack.pop();
      continue;
    }
    const selfClose = body.endsWith('/');
    const inner = selfClose ? body.slice(0, -1) : body;
    const nameMatch = /^[^\s/>]+/.exec(inner);
    if (!nameMatch) continue;
    const node: XmlNode = { name: nameMatch[0], attrs: {}, children: [], text: '' };
    const attrRe = /([^\s=]+)\s*=\s*("([^"]*)"|'([^']*)')/g;
    const rest = inner.slice(nameMatch[0].length);
    for (let m = attrRe.exec(rest); m; m = attrRe.exec(rest))
      node.attrs[m[1]!] = decodeEntities(m[3] ?? m[4] ?? '');
    if (++nodes > MAX_NODES) fail('too-large', 'The document has too many parts.');
    stack[stack.length - 1]!.children.push(node);
    if (!selfClose) {
      if (stack.length >= MAX_DEPTH) fail('too-large', 'The document is nested too deeply.');
      stack.push(node);
    }
  }
  return root;
}

export function childrenNamed(node: XmlNode, name: string): XmlNode[] {
  return node.children.filter((c) => c.name === name);
}
export function firstChild(node: XmlNode, name: string): XmlNode | undefined {
  return node.children.find((c) => c.name === name);
}
