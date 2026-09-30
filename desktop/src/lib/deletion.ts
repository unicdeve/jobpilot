// How a deletion's reach reads to the person confirming it, in the same
// words the CLI uses (cmd/jobpilot-cli/deletion.go).

import type { DeletionReach, RecordKind } from "./types";

const KIND_NAMES: Record<string, [string, string]> = {
  candidate: ["profile", "profiles"],
  candidate_detail: ["profile entry", "profile entries (experience, skills, links, work authorizations)"],
  resume: ["resume", "resumes"],
  job: ["job", "jobs"],
  match: ["match analysis", "match analyses"],
  application: ["application", "applications"],
  question: ["application question and answer", "application questions and answers"],
  cover_letter: ["cover letter version", "cover letter versions"],
  state_transition: ["status change", "status changes"],
  agent_run: ["agent run", "agent runs"],
  agent_step: ["agent step", "agent steps"],
  scout: ["scout", "scouts"],
  saved_search: ["saved search", "saved searches"],
  credential_name: ["API key name", "API key names"],
  personal_answer: ["thing you told JobPilot about yourself", "things you told JobPilot about yourself"],
  watched_company: ["watched company", "watched companies"],
  settings: ["setting", "settings"],
};

/// KIND_ORDER lists kinds the way a person reads what they are deleting:
/// the thing itself, then what hangs off it.
const KIND_ORDER = [
  "candidate",
  "candidate_detail",
  "personal_answer",
  "resume",
  "job",
  "match",
  "application",
  "cover_letter",
  "question",
  "state_transition",
  "agent_run",
  "agent_step",
  "saved_search",
  "scout",
  "watched_company",
  "credential_name",
  "settings",
];

export function countedKind(kind: RecordKind, n: number): string {
  const [one, many] = KIND_NAMES[kind] ?? [kind.replace(/_/g, " "), kind.replace(/_/g, " ")];
  return n === 1 ? `1 ${one}` : `${n} ${many}`;
}

/// countedKinds lists counts in reading order. A kind a newer daemon
/// reports is still shown, after the rest.
export function countedKinds(counts: Record<RecordKind, number>): string[] {
  const known = KIND_ORDER.filter((kind) => (counts[kind] ?? 0) > 0);
  const rest = Object.keys(counts)
    .filter((kind) => !KIND_ORDER.includes(kind) && counts[kind] > 0)
    .sort();
  return [...known, ...rest].map((kind) => countedKind(kind, counts[kind]));
}

export function totalDeleted(reach: DeletionReach): number {
  return Object.values(reach.deleted).reduce((sum, n) => sum + n, 0);
}

/// sentNote is the reminder a reach carries when applications that were
/// sent go with it: deleting them here withdraws nothing there.
export function sentNote(reach: DeletionReach): string | undefined {
  const sent = reach.sent_applications;
  if (sent === 1) return "1 of those applications was sent to an employer. Deleting it here withdraws nothing there.";
  if (sent > 1) return `${sent} of those applications were sent to employers. Deleting them here withdraws nothing there.`;
  return undefined;
}

export function keptNote(reach: DeletionReach): string | undefined {
  const kept = reach.kept_not_interested ?? 0;
  if (kept === 1) return "1 job marked not interested is kept, so it is not offered again.";
  if (kept > 1) return `${kept} jobs marked not interested are kept, so they are not offered again.`;
  return undefined;
}
