import { expect, test } from 'bun:test';
import {
  allowedTransitions,
  canTransition,
  nextStates,
  validApplyUrl,
  type ApplicationState,
} from './application.ts';

// The cases are the Go test's (internal/core/services/application_test.go), so the two cores cannot drift silently.
const valid: [string, string][] = [
  ['DISCOVERED', 'MATCHED'],
  ['MATCHED', 'APPROVED'],
  ['MATCHED', 'SUBMITTED'],
  ['MATCHED', 'WITHDRAWN'],
  ['APPROVED', 'SUBMITTED'],
  ['APPROVED', 'MATCHED'],
  ['APPROVED', 'WITHDRAWN'],
  ['SUBMITTED', 'REJECTED'],
  ['DISCOVERED', 'WITHDRAWN'],
  ['SUBMITTED', 'INTERVIEWING'],
  ['SUBMITTED', 'OFFERED'],
  ['SUBMITTED', 'NO_RESPONSE'],
  ['INTERVIEWING', 'OFFERED'],
  ['INTERVIEWING', 'REJECTED'],
  ['INTERVIEWING', 'NO_RESPONSE'],
  ['OFFERED', 'REJECTED'],
  ['NO_RESPONSE', 'INTERVIEWING'],
  ['NO_RESPONSE', 'REJECTED'],
  ['SUBMITTED', 'WITHDRAWN'],
  ['INTERVIEWING', 'WITHDRAWN'],
  ['OFFERED', 'WITHDRAWN'],
];
const invalid: [string, string][] = [
  ['DISCOVERED', 'SUBMITTED'],
  ['DISCOVERED', 'APPROVED'],
  ['SUBMITTED', 'APPROVED'],
  ['REJECTED', 'MATCHED'],
  ['WITHDRAWN', 'MATCHED'],
  ['MATCHED', 'PREPARING'],
  ['MATCHED', 'READY_FOR_REVIEW'],
  ['MATCHED', 'FAILED'],
  ['INTERVIEWING', 'SUBMITTED'],
  ['NO_RESPONSE', 'SUBMITTED'],
  ['OFFERED', 'INTERVIEWING'],
  ['OFFERED', 'NO_RESPONSE'],
  ['INTERVIEWING', 'MATCHED'],
  ['APPROVED', 'INTERVIEWING'],
  ['MATCHED', 'OFFERED'],
];

test.each(valid)('%s -> %s is allowed', (from, to) => expect(canTransition(from, to)).toBe(true));
test.each(invalid)('%s -> %s is rejected', (from, to) =>
  expect(canTransition(from, to)).toBe(false),
);

test('a submitted application offers its outcomes in order; a closed one offers nothing', () => {
  expect(nextStates('SUBMITTED')).toEqual([
    'INTERVIEWING',
    'OFFERED',
    'REJECTED',
    'NO_RESPONSE',
    'WITHDRAWN',
  ]);
  expect(nextStates('REJECTED')).toEqual([]);
  expect(nextStates('WITHDRAWN')).toEqual([]);
  expect(nextStates('NOPE')).toEqual([]);
});
test('SUBMITTED is reachable only from MATCHED and APPROVED', () => {
  const into = (Object.keys(allowedTransitions) as ApplicationState[]).filter((s) =>
    allowedTransitions[s].includes('SUBMITTED'),
  );
  expect(into).toEqual(['MATCHED', 'APPROVED']);
});
test('a copy is returned, so a caller cannot change the table', () => {
  nextStates('MATCHED').pop();
  expect(nextStates('MATCHED')).toHaveLength(3);
});
test('an apply link must be a web address, or empty to clear it', () => {
  expect(validApplyUrl('')).toBe(true);
  expect(validApplyUrl(' https://acme.example/apply ')).toBe(true);
  expect(validApplyUrl('http://acme.example')).toBe(true);
  expect(validApplyUrl('javascript:alert(1)')).toBe(false);
  expect(validApplyUrl('file:///etc/passwd')).toBe(false);
  expect(validApplyUrl('acme.example/apply')).toBe(false);
  expect(validApplyUrl('https://')).toBe(false);
});
