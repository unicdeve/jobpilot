import { describe, expect, test } from 'bun:test';
import { deflateRawSync, deflateSync } from 'node:zlib';
import { inflateRaw, inflateZlib } from './inflate.ts';

const text = (s: string): Uint8Array => new TextEncoder().encode(s);

describe('inflate', () => {
  const samples: [string, Uint8Array][] = [
    ['empty', new Uint8Array(0)],
    ['short text', text('Ada Lovelace, engineer')],
    ['repetitive text', text('the quick brown fox '.repeat(5000))],
    ['all byte values', Uint8Array.from({ length: 4096 }, (_, i) => (i * 7919) & 255)],
    ['a long run', new Uint8Array(100000)],
  ];
  for (const [name, data] of samples) {
    for (const level of [0, 1, 6, 9]) {
      test(`${name}, level ${level}`, () => {
        expect(inflateRaw(new Uint8Array(deflateRawSync(data, { level })), 0, 1 << 24)).toEqual(data);
        expect(inflateZlib(new Uint8Array(deflateSync(data, { level })), 1 << 24)).toEqual(data);
      });
    }
  }

  test('a fixed-Huffman block (strategy FIXED)', () => {
    const data = text('fixed codes only, fixed codes only');
    expect(inflateRaw(new Uint8Array(deflateRawSync(data, { strategy: 4 })), 0, 1 << 20)).toEqual(data);
  });

  test('truncated and corrupt data fail with a typed code, not a throw of another kind', () => {
    const good = new Uint8Array(deflateRawSync(text('hello hello hello hello hello')));
    expect(() => inflateRaw(good.subarray(0, good.length - 3), 0, 1000)).toThrow();
    try { inflateRaw(Uint8Array.from([0xff, 0xff, 0xff]), 0, 1000); } catch (e) { expect((e as { code: string }).code).toBe('corrupt'); }
    try { inflateZlib(text('not zlib'), 1000); } catch (e) { expect((e as { code: string }).code).toBe('corrupt'); }
  });

  test('a decompression bomb stops at the cap', () => {
    const bomb = new Uint8Array(deflateRawSync(new Uint8Array(20 * 1024 * 1024), { level: 9 }));
    expect(bomb.length).toBeLessThan(100000);
    try { inflateRaw(bomb, 0, 1024 * 1024); throw new Error('should have thrown'); } catch (e) { expect((e as { code: string }).code).toBe('too-large'); }
  });
});
