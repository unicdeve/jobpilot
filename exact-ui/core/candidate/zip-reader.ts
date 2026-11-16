// Reads entries out of a zip (a .docx is one). Only the central directory is trusted for where entries are, and
// every offset and size from the file is checked against the file's own length and a ceiling, because the archive is
// untrusted: a zip bomb is a few kilobytes that expand to gigabytes.
import { fail } from './document-error.ts';
import { inflateRaw } from './inflate.ts';

export const MAX_ENTRY_BYTES = 16 * 1024 * 1024;
const MAX_ENTRIES = 10000;

export interface ZipEntry { name: string; method: number; compressedSize: number; size: number; offset: number }

const u16 = (b: Uint8Array, p: number): number => b[p]! | (b[p + 1]! << 8);
const u32 = (b: Uint8Array, p: number): number => (b[p]! | (b[p + 1]! << 8) | (b[p + 2]! << 16) | (b[p + 3]! << 24)) >>> 0;

export function isZip(data: Uint8Array): boolean {
  return data.length >= 4 && data[0] === 0x50 && data[1] === 0x4b && (data[2] === 3 || data[2] === 5) && (data[3] === 4 || data[3] === 6);
}

export function readZipEntries(data: Uint8Array): Map<string, ZipEntry> {
  // The end-of-central-directory record is within the last 64 KiB + 22 bytes.
  let eocd = -1;
  for (let p = data.length - 22; p >= Math.max(0, data.length - 22 - 65535); p--) {
    if (data[p] === 0x50 && data[p + 1] === 0x4b && data[p + 2] === 5 && data[p + 3] === 6) { eocd = p; break; }
  }
  if (eocd < 0) fail('corrupt', 'This is not a valid zip archive.');
  const count = u16(data, eocd + 10);
  const dirSize = u32(data, eocd + 12);
  const dirStart = u32(data, eocd + 16);
  if (count > MAX_ENTRIES || dirStart + dirSize > data.length) fail('corrupt', 'The archive directory is invalid.');
  const entries = new Map<string, ZipEntry>();
  let p = dirStart;
  for (let i = 0; i < count; i++) {
    if (p + 46 > data.length || u32(data, p) !== 0x02014b50) fail('corrupt', 'The archive directory is invalid.');
    const nameLen = u16(data, p + 28), extraLen = u16(data, p + 30), commentLen = u16(data, p + 32);
    if (p + 46 + nameLen > data.length) fail('corrupt', 'The archive directory is invalid.');
    const flags = u16(data, p + 8);
    if ((flags & 1) !== 0) fail('unsupported', 'The file is encrypted.');
    let name = '';
    for (let k = 0; k < nameLen; k++) name += String.fromCharCode(data[p + 46 + k]!);
    // Names are UTF-8 (flag bit 11) or CP437; only ASCII names matter for the parts read here.
    entries.set(name, { name, method: u16(data, p + 10), compressedSize: u32(data, p + 20), size: u32(data, p + 24), offset: u32(data, p + 42) });
    p += 46 + nameLen + extraLen + commentLen;
  }
  return entries;
}

export function readZipEntry(data: Uint8Array, entry: ZipEntry): Uint8Array {
  if (entry.size > MAX_ENTRY_BYTES) fail('too-large', `${entry.name} is larger than the device will read.`);
  const p = entry.offset;
  if (p + 30 > data.length || u32(data, p) !== 0x04034b50) fail('corrupt', `${entry.name} is damaged.`);
  const start = p + 30 + u16(data, p + 26) + u16(data, p + 28);
  if (start + entry.compressedSize > data.length) fail('corrupt', `${entry.name} is damaged.`);
  const raw = data.subarray(start, start + entry.compressedSize);
  let out: Uint8Array;
  if (entry.method === 0) out = raw.slice();
  else if (entry.method === 8) {
    try { out = inflateRaw(raw, 0, entry.size + 1); } catch (e) {
      // Inflating past the declared size means the directory lied about it, which is damage, not a big file.
      if ((e as { code?: string }).code === 'too-large') fail('corrupt', `${entry.name} is damaged.`);
      throw e;
    }
  }
  else return fail('unsupported', `${entry.name} uses a compression method the device cannot read.`);
  // The declared size is a claim; the inflated size must agree, or the entry is lying.
  if (out.length !== entry.size) fail('corrupt', `${entry.name} is damaged.`);
  return out;
}
