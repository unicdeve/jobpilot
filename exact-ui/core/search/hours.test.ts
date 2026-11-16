import { expect, test } from 'bun:test';
import { formatOffset, hoursKnown, readHours, shiftFor, windowText } from './hours.ts';
import { sentences } from './signals.ts';

// The cases are the Go tests' (internal/core/domain/hours_test.go), so the two cores cannot drift silently.
const requirements: Record<string, [number, number]> = {
  'To create the best experience for our new hire, this role requires you to be based within UTC−6 to UTC+3.': [-6, 3],
  'To create the best experience for our new hire, this role requires you to be based within +3 / -7 UTC.': [-7, 3],
  'To create the best experience for our new hire, this role requires you to be based within UTC -4 / -7 .': [-7, -4],
  'as long as your main location is between UTC-4 and UTC+4.': [-4, 4],
  'UTC +3 to UTC -5': [-5, 3],
  'Located in CET timezone (+/- 3 hours), we are unable to consider applications from candidates in other time zones.': [-2, 4],
  'Time zone: CET (+/- 3 hours).': [-2, 4],
  'Able to overlap at least 4 hours a day with US Eastern Time': [-9, -1],
  'Overlap: 6hs with East Coast time zone, USA': [-7, -3],
  'At least four hours of weekday overlap with Bangkok (GMT+7)': [3, 11],
  'Able to provide at least 7 hours of overlap with the 9 am-5 pm EST workday (Monday - Friday).': [-6, -4],
  'To create the best experience for our new hire, this role requires at least 3-4 hours of overlap with Central European Time (CET/CEST) business hours to support collaboration with the team.': [-4, 6],
  'Mandatory availability from 9:00 AM – 12:00 PM PST for daily team syncs and real-time collaboration.': [-8, -3],
  'Our team meetings are scheduled between 11:00 and 16:00 UTC.': [-2, 1],
  'This position requires working hours in CEST.': [1, 1],
  'in LATAM working EST hours.': [-5, -5],
  'Full-time availability during EST or PST working hours.': [-8, -5],
  'The ability to operate in ET, CT, MT, or PT timezones is required.': [-8, -5],
  'Must be located in East or Central timezones in the US.': [-6, -5],
  '100% Remote but ONLY residents of countries with US time zones (EST-PST)': [-8, -5],
  'These roles are home based in the EMEA time zone.': [-1, 4],
  'This is a remote role in Americas time zones.': [-10, -3],
  'This role can be filled in European, Middle East and African time zones.': [-1, 4],
  'This role may be held anywhere in European, Middle Eastern, African or Americas regions or time zones.': [-10, 4],
  'This role requires strong working overlap with Americas time zones due to production support and on-call responsibilities.': [-10, -3],
  "This is a remote position and we're open to considering candidates located across EMEA and AMER time zones.": [-10, 4],
};

test('readHours reads requirements', () => {
  for (const [sentence, want] of Object.entries(requirements)) {
    const hours = readHours(sentences(sentence));
    expect([sentence, hours.status]).toEqual([sentence, 'required']);
    expect([sentence, hours.from, hours.to]).toEqual([sentence, want[0], want[1]]);
    expect(hours.evidence).toBeTruthy();
  }
});

test('readHours reads preferences', () => {
  const preferences: Record<string, [number, number]> = {
    'EST time zone is preferred for this position.': [-5, -5],
    'However, to facilitate working with the Finance team, we prefer candidates based in GMT+0 to GMT+3.': [0, 3],
    'We have a preference for candidates in EMEA time zones.': [-1, 4],
    'Prefer location in PST, MST or CST timezone alignment.': [-8, -6],
  };
  for (const [sentence, want] of Object.entries(preferences)) {
    const hours = readHours(sentences(sentence));
    expect([sentence, hours.status]).toEqual([sentence, 'preferred']);
    expect([sentence, hours.from, hours.to]).toEqual([sentence, want[0], want[1]]);
  }
});

test('readHours asks nothing of sentences about spread, benefits and rotas', () => {
  for (const sentence of [
    "We're backend, frontend, and AI specialists working asynchronously across time zones, using issues, merge requests, and documentation as our main collaboration tools.",
    'Location: we have remote roles open in every time zone.',
    'We are growing teams in EMEA, Americas and APAC time zones, so can accommodate candidates from almost any country.',
    'A typical first-level software engineering team is based in a single time zone, such as EMEA or Americas, or APAC, with an Engineering Manager.',
    'Flexible working hours (fully flexible, as in most cases you only have to be on a couple of meetings weekly);',
    'Please note: applications will be open until Friday 2nd October 2026 at 08.00 CEST.',
    'Collaborate with enablement counterparts in the Americas and EMEA, and join the weekly team sync at 4pm CEST at least once per week.',
    'It’s a full-time position with 4 10-hour shifts, permanently covering weekends during EMEA Hours: 08:00-18:00 UTC',
    'As a 24 x 7 team, SOC analysts are expected to work Friday - Tuesday, with the daily shift being 0000 - 0800 UTC - (9am - 6pm AEST)',
    'You’re comfortable participating in a paid on-call rotation to help support urgent customer issues outside standard working hours.',
    'Develop a deep understanding of silicon partner strategies and identify overlap with Canonical’s current and future products.',
    'Canonical est une entreprise mondiale axée sur le travail à distance et elle est rentable.',
    'Join us in building the future.',
  ]) {
    const hours = readHours(sentences(sentence));
    expect([sentence, hoursKnown(hours)]).toEqual([sentence, false]);
  }
});

test('readHours joins windows of a kind and ranks a requirement over a preference', () => {
  const joined = readHours(['8:00 AM - 4:00 PM EST', '11:00 AM - 7:00 PM EST']);
  expect([joined.from, joined.to]).toEqual([-7, -4]);
  expect(joined.evidence).toBe('8:00 AM - 4:00 PM EST');
  const ranked = readHours(['EST time zone is preferred.', 'This role is home-based in EMEA time zones.']);
  expect(ranked.status).toBe('required');
  expect([ranked.from, ranked.to]).toEqual([-1, 4]);
});

test('shiftFor is measured around the clock', () => {
  const cet = { status: 'required' as const, from: -2, to: 4 };
  expect(shiftFor(cet, 1)).toBe(0);
  expect(shiftFor(cet, -3)).toBe(1);
  expect(shiftFor(cet, 5.5)).toBe(1.5);
  expect(shiftFor({ status: 'required', from: -9, to: -1 }, 1)).toBe(2);
  const pacific = { status: 'required' as const, from: -8, to: -8 };
  expect(shiftFor(pacific, 12)).toBe(4);
  expect(shiftFor(pacific, 1)).toBe(9);
  expect(shiftFor({ status: 'required', from: 5, to: 13 }, -11)).toBe(0);
});

test('formatOffset writes an offset as a person reads it', () => {
  const cases: [number, string][] = [[0, 'UTC'], [1, 'UTC+1'], [-5, 'UTC−5'], [5.5, 'UTC+5:30'], [5.75, 'UTC+5:45'], [-3.5, 'UTC−3:30'], [15, 'UTC−9']];
  for (const [offset, want] of cases) expect(formatOffset(offset)).toBe(want);
  expect(windowText({ status: 'required', from: -2, to: 4 })).toBe('UTC−2 to UTC+4');
  expect(windowText({ status: 'required', from: -5, to: -5 })).toBe('UTC−5');
});
