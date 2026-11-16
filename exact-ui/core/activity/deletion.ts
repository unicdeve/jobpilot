// What a deletion reaches (ADR-0037), ported from internal/core/domain/deletion.go (the rules and targets, not the preview).
export type RecordKind = 'candidate' | 'candidate_detail' | 'resume' | 'job' | 'match' | 'application' | 'question' | 'cover_letter'
  | 'state_transition' | 'agent_run' | 'agent_step' | 'scout' | 'saved_search' | 'credential_name' | 'settings' | 'personal_answer'
  | 'watched_company' | 'notification';
export type DeletionEffect = 'delete' | 'detach';
export interface DeletionRule { referencing: RecordKind; referenced: RecordKind; effect: DeletionEffect }

export const deletionRules: DeletionRule[] = [
  { referencing: 'candidate_detail', referenced: 'candidate', effect: 'delete' },
  { referencing: 'resume', referenced: 'candidate', effect: 'delete' },
  { referencing: 'match', referenced: 'job', effect: 'delete' },
  { referencing: 'match', referenced: 'candidate', effect: 'delete' },
  { referencing: 'application', referenced: 'job', effect: 'delete' },
  { referencing: 'application', referenced: 'candidate', effect: 'delete' },
  { referencing: 'application', referenced: 'resume', effect: 'detach' },
  { referencing: 'application', referenced: 'cover_letter', effect: 'detach' },
  { referencing: 'question', referenced: 'application', effect: 'delete' },
  { referencing: 'cover_letter', referenced: 'application', effect: 'delete' },
  { referencing: 'state_transition', referenced: 'application', effect: 'delete' },
  { referencing: 'agent_run', referenced: 'scout', effect: 'detach' },
  { referencing: 'agent_step', referenced: 'agent_run', effect: 'delete' },
  { referencing: 'scout', referenced: 'saved_search', effect: 'delete' },
  { referencing: 'personal_answer', referenced: 'candidate', effect: 'delete' },
  { referencing: 'notification', referenced: 'scout', effect: 'detach' },
  { referencing: 'notification', referenced: 'agent_run', effect: 'detach' },
];

export const rulesReferencing = (kind: RecordKind): DeletionRule[] => deletionRules.filter((r) => r.referenced === kind);

export type DeletionTarget = 'job' | 'application' | 'cover_letter' | 'resume' | 'profile' | 'activity' | 'jobs' | 'everything';
const targets: DeletionTarget[] = ['job', 'application', 'cover_letter', 'resume', 'profile', 'activity', 'jobs', 'everything'];
export const validDeletionTarget = (t: string): t is DeletionTarget => (targets as string[]).includes(t);
export const targetTakesId = (t: DeletionTarget): boolean => t === 'job' || t === 'application' || t === 'cover_letter' || t === 'resume';
