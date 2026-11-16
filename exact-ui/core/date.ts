// A port of the Go core's calendar date (internal/core/domain/date.go, plus ParseDraftDate from profile_draft.go), ADR-0037.
// It has no Exact, network or storage dependency, so it runs the same in the app's JS engine and under `bun test`.
// A date is a day, not an instant: nothing here reads a clock or a time zone.
// Month is 1 to 12, as Go's time.Month is.
// Not ported: Format(layout) and the Go-only SQL Scan of a time.Time (a stored value is text here).

export interface DateValue {
  year: number;
  month: number;
  day: number;
}

export const zeroDate: DateValue = { year: 0, month: 0, day: 0 };

export const newDate = (year: number, month: number, day: number): DateValue => ({ year, month, day });

export const isZeroDate = (d: DateValue): boolean => d.year === 0 && d.month === 0 && d.day === 0;

const pad = (n: number, width: number): string => String(n).padStart(width, '0');

const isLeap = (y: number): boolean => (y % 4 === 0 && y % 100 !== 0) || y % 400 === 0;

const daysIn = (y: number, m: number): number => [31, isLeap(y) ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][m - 1] ?? 0;

// Go's time.Parse rejects a day or month outside the calendar.
const build = (y: number, m: number, d: number): DateValue | null =>
  m >= 1 && m <= 12 && d >= 1 && d <= daysIn(y, m) ? { year: y, month: m, day: d } : null;

// The time part of "2006-01-02T15:04:05" and RFC 3339: the clock must be real, but the date is the day as written.
const clockOk = (h: string, mi: string, s: string): boolean => +h < 24 && +mi < 60 && +s < 60;

const ISO_DAY = /^(\d{4})-(\d{2})-(\d{2})$/;
const ISO_LOCAL = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})$/;
const RFC3339 = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(?:Z|[+-](\d{2}):(\d{2}))$/;
const ISO_MONTH = /^(\d{4})-(\d{2})$/;
const ISO_YEAR = /^(\d{4})$/;

// ParseDate reads a date, reporting null when it cannot.
// A timestamp is read as the day it is written on, whatever its offset (Go's DateOf keeps the parsed location).
export const parseDate = (value: string): DateValue | null => {
  const v = value.trim();
  if (v === '') return null;
  let m = ISO_DAY.exec(v);
  if (m) return build(+m[1]!, +m[2]!, +m[3]!);
  m = RFC3339.exec(v);
  if (m && clockOk(m[4]!, m[5]!, m[6]!) && (m[7] === undefined || (+m[7] < 24 && +m[8]! < 60))) return build(+m[1]!, +m[2]!, +m[3]!);
  m = ISO_LOCAL.exec(v);
  if (m && clockOk(m[4]!, m[5]!, m[6]!)) return build(+m[1]!, +m[2]!, +m[3]!);
  m = ISO_MONTH.exec(v);
  if (m) return build(+m[1]!, +m[2]!, 1);
  m = ISO_YEAR.exec(v);
  if (m) return build(+m[1]!, 1, 1);
  return null;
};

// Compare as a tuple, which is the order of midnight UTC on each date without Date's two-digit-year quirk.
const compare = (a: DateValue, b: DateValue): number => a.year - b.year || a.month - b.month || a.day - b.day;

export const dateBefore = (a: DateValue, b: DateValue): boolean => compare(a, b) < 0;

export const dateAfter = (a: DateValue, b: DateValue): boolean => compare(a, b) > 0;

// The only spelling a date is written in: YYYY-MM-DD, or '' for no date.
export const dateString = (d: DateValue): string => (isZeroDate(d) ? '' : `${pad(d.year, 4)}-${pad(d.month, 2)}-${pad(d.day, 2)}`);

// Reads a JSON value: '' is no date, null is no date (null), a non-string or unreadable string throws with Go's messages.
export const dateFromJSON = (value: unknown): DateValue | null => {
  if (value === null || value === undefined) return null;
  if (typeof value !== 'string') throw new Error('a date must be a string like 2006-01-02');
  if (value.trim() === '') return zeroDate;
  const parsed = parseDate(value);
  if (!parsed) throw new Error(`${JSON.stringify(value)} is not a date; write it as 2006-01-02`);
  return parsed;
};

export const dateToJSON = (d: DateValue): string => dateString(d);

// SQLite holds the text, which orders correctly as a string; an unset date is NULL.
export const dateToSql = (d: DateValue): string | null => (isZeroDate(d) ? null : dateString(d));

export const dateFromSql = (stored: string | null | undefined): DateValue => {
  if (stored === null || stored === undefined || stored.trim() === '') return zeroDate;
  const parsed = parseDate(stored);
  if (!parsed) throw new Error(`stored value ${JSON.stringify(stored)} is not a date`);
  return parsed;
};

const MONTHS_LONG = ['january', 'february', 'march', 'april', 'may', 'june', 'july', 'august', 'september', 'october', 'november', 'december'];
const MONTHS_SHORT = MONTHS_LONG.map((n) => n.slice(0, 3));

const PRESENT = new Set(['present', 'current', 'now', 'ongoing', 'to date', 'presently']);

const monthNamed = (name: string, names: string[]): number => names.indexOf(name.toLowerCase()) + 1;

// ParseDraftDate reads how a resume writes a date; null is a date that is absent, "Present", or unreadable.
// A guessed date would do harm, so anything else stays unread.
export const parseDraftDate = (value: string): DateValue | null => {
  const v = value.trim();
  if (v === '' || PRESENT.has(v.toLowerCase())) return null;
  const iso = parseDate(v);
  if (iso) return iso;
  let m: RegExpExecArray | null;
  // "January 2006", "Jan 2006"
  if ((m = /^([A-Za-z]+) (\d{4})$/.exec(v))) {
    const mo = monthNamed(m[1]!, MONTHS_LONG) || monthNamed(m[1]!, MONTHS_SHORT);
    return mo ? build(+m[2]!, mo, 1) : null;
  }
  // "01/2006", "2006/01"
  if ((m = /^(\d{2})\/(\d{4})$/.exec(v))) return build(+m[2]!, +m[1]!, 1);
  if ((m = /^(\d{4})\/(\d{2})$/.exec(v))) return build(+m[1]!, +m[2]!, 1);
  // "January 2, 2006", "Jan 2, 2006"
  if ((m = /^([A-Za-z]+) (\d{1,2}), (\d{4})$/.exec(v))) {
    const mo = monthNamed(m[1]!, MONTHS_LONG) || monthNamed(m[1]!, MONTHS_SHORT);
    return mo ? build(+m[3]!, mo, +m[2]!) : null;
  }
  // "02/01/2006" is day/month/year
  if ((m = /^(\d{2})\/(\d{2})\/(\d{4})$/.exec(v))) return build(+m[3]!, +m[2]!, +m[1]!);
  return null;
};
