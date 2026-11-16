import { expect, test } from 'bun:test';
import { dateAfter, dateBefore, dateFromJSON, dateFromSql, dateString, dateToJSON, dateToSql, isZeroDate, newDate, parseDate, parseDraftDate, zeroDate } from './date.ts';

// The cases are the Go test's (internal/core/domain/date_test.go), so the two cores cannot drift silently.
test('reads what an HTML date input sends', () => {
  expect(dateFromJSON('2022-07-01')).toEqual({ year: 2022, month: 7, day: 1 });
});

test('an empty string is no date, not an error', () => {
  const d = dateFromJSON('');
  expect(d).not.toBeNull();
  expect(isZeroDate(d!)).toBe(true);
});

test('null is no date', () => {
  expect(dateFromJSON(null)).toBeNull();
});

test('a timestamp is read as the day it falls on', () => {
  expect(dateString(dateFromJSON('2022-07-01T00:00:00Z')!)).toBe('2022-07-01');
  expect(dateString(dateFromJSON('2022-07-01T23:30:00-05:00')!)).toBe('2022-07-01');
});

test('writes YYYY-MM-DD, never an instant', () => {
  expect(dateToJSON(newDate(2022, 7, 1))).toBe('2022-07-01');
  expect(dateToJSON(zeroDate)).toBe('');
});

test('a value that is not a date says so usefully', () => {
  expect(() => dateFromJSON('last summer')).toThrow('last summer');
  expect(() => dateFromJSON('last summer')).toThrow('2006-01-02');
  expect(() => dateFromJSON(5)).toThrow('a date must be a string');
});

test('SQL stores text SQLite can order, and NULL for no date', () => {
  expect(dateToSql(newDate(2022, 7, 1))).toBe('2022-07-01');
  expect(dateToSql(zeroDate)).toBeNull();
});

test('SQL reads back what earlier rows stored', () => {
  for (const stored of ['2022-07-01', '2022-07-01T00:00:00Z']) expect(dateString(dateFromSql(stored))).toBe('2022-07-01');
  expect(isZeroDate(dateFromSql(null))).toBe(true);
  expect(() => dateFromSql('nope')).toThrow('stored value');
});

const parseCases: [string, string, boolean][] = [
  ['2022-07-01', '2022-07-01', true],
  ['2022-07', '2022-07-01', true],
  ['2022', '2022-01-01', true],
  ['2022-07-01T09:30:00Z', '2022-07-01', true],
  ['  2022-07-01  ', '2022-07-01', true],
  ['', '', false],
  ['July 2022', '', false],
  ['not a date', '', false],
  ['2022-02-30', '', false],
];
for (const [input, want, ok] of parseCases) {
  test(`parseDate(${JSON.stringify(input)})`, () => {
    const got = parseDate(input);
    expect(got !== null).toBe(ok);
    if (ok) expect(dateString(got!)).toBe(want);
  });
}

test('orders dates', () => {
  expect(dateBefore(newDate(2022, 6, 30), newDate(2022, 7, 1))).toBe(true);
  expect(dateAfter(newDate(2022, 7, 1), newDate(2022, 6, 30))).toBe(true);
  expect(dateAfter(newDate(2022, 7, 1), newDate(2022, 7, 1))).toBe(false);
});

test('parseDraftDate reads how resumes write', () => {
  const want: Record<string, string> = {
    'Jan 2022': '2022-01-01', 'January 2022': '2022-01-01', '03/2019': '2019-03-01', '2019/03': '2019-03-01', '2022-07': '2022-07-01',
  };
  for (const [spelling, iso] of Object.entries(want)) expect(dateString(parseDraftDate(spelling)!)).toBe(iso);
  for (const unreadable of ['', 'Present', 'current', 'sometime in 2019']) expect(parseDraftDate(unreadable)).toBeNull();
});
