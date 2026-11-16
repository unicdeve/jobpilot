// A port of the application state machine (internal/core/services/application.go), ADR-0037.
// It is the explicit rule table, never a model's call (AGENTS.md). SUBMITTED is reachable only from MATCHED and
// APPROVED, so nothing else can mark an application sent (ADR-0034 §3).

export type ApplicationState =
  | 'DISCOVERED'
  | 'MATCHED'
  | 'APPROVED'
  | 'SUBMITTED'
  | 'INTERVIEWING'
  | 'OFFERED'
  | 'NO_RESPONSE'
  | 'REJECTED'
  | 'WITHDRAWN';

export const allowedTransitions: Record<ApplicationState, ApplicationState[]> = {
  DISCOVERED: ['MATCHED', 'WITHDRAWN'],
  // APPROVED is "ready to send", which a user may skip by recording that they sent it.
  MATCHED: ['APPROVED', 'SUBMITTED', 'WITHDRAWN'],
  APPROVED: ['SUBMITTED', 'MATCHED', 'WITHDRAWN'],
  // Everything below SUBMITTED is an outcome the user reports, never one the application works out itself.
  SUBMITTED: ['INTERVIEWING', 'OFFERED', 'REJECTED', 'NO_RESPONSE', 'WITHDRAWN'],
  INTERVIEWING: ['OFFERED', 'REJECTED', 'NO_RESPONSE', 'WITHDRAWN'],
  OFFERED: ['REJECTED', 'WITHDRAWN'],
  // A late reply reopens an application the user had given up on.
  NO_RESPONSE: ['INTERVIEWING', 'REJECTED'],
  REJECTED: [],
  WITHDRAWN: [],
};

// The states to offer next, in order. An unknown state offers nothing.
export const nextStates = (from: string): ApplicationState[] => [
  ...(allowedTransitions[from as ApplicationState] ?? []),
];

export const canTransition = (from: string, to: string): boolean =>
  nextStates(from).includes(to as ApplicationState);

// The link is opened in the user's browser, so only a web address qualifies (SetApplyURL). Empty clears it.
export function validApplyUrl(raw: string): boolean {
  const url = raw.trim();
  if (url === '') return true;
  try {
    const parsed = new URL(url);
    return parsed.host !== '' && (parsed.protocol === 'http:' || parsed.protocol === 'https:');
  } catch {
    return false;
  }
}
