// How stored values read on screen. Sources report these fields in their own
// vocabulary, so anything not recognised is shown as-is rather than guessed.

import type { Job } from "./types";

const SOURCE_LABELS: Record<string, string> = {
  arbeitnow: "Arbeitnow",
  remotive: "Remotive",
  remoteok: "RemoteOK",
  himalayas: "Himalayas",
  hackernews: "Hacker News",
  "browser-import": "Added by URL",
};

export function sourceLabel(sourceId: string): string {
  return SOURCE_LABELS[sourceId] ?? sourceId;
}

const REMOTE_LABELS: Record<string, string> = {
  remote: "Remote",
  hybrid: "Hybrid",
  onsite: "On-site",
};

export function remoteLabel(remoteType: string): string | undefined {
  return REMOTE_LABELS[remoteType?.toLowerCase()];
}

/// employmentLabel returns a label only for the handful of employment types
/// every source means the same thing by. Arbeitnow, for one, puts seniority
/// ("berufserfahren") in this field, which is not an employment type at all.
export function employmentLabel(employmentType: string): string | undefined {
  const value = employmentType?.toLowerCase().replace(/[\s_]+/g, "-");
  switch (value) {
    case "full-time":
    case "fulltime":
      return "Full-time";
    case "part-time":
    case "parttime":
      return "Part-time";
    case "contract":
    case "contractor":
      return "Contract";
    case "internship":
      return "Internship";
    case "temporary":
      return "Temporary";
    default:
      return undefined;
  }
}

/// jobLocation is where a job is, or undefined when the source did not say.
export function jobLocation(job: Pick<Job, "locations">): string | undefined {
  const locations = job.locations?.filter(Boolean) ?? [];
  if (locations.length === 0) return undefined;
  return locations.length > 2 ? `${locations.slice(0, 2).join(", ")} +${locations.length - 2}` : locations.join(", ");
}

/// salaryLabel renders a salary range only when the source gave one, in its
/// own currency and period; nothing here converts or estimates.
export function salaryLabel(job: Pick<Job, "salary_min" | "salary_max" | "salary_currency" | "salary_period">): string | undefined {
  const { salary_min: min, salary_max: max, salary_currency: currency, salary_period: period } = job;
  if (!min && !max) return undefined;
  const format = (n: number) =>
    currency
      ? new Intl.NumberFormat(undefined, { style: "currency", currency, maximumFractionDigits: 0 }).format(n)
      : new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 }).format(n);
  let range: string;
  try {
    range = min && max && min !== max ? `${format(min)} – ${format(max)}` : format(min || max);
  } catch {
    // An unrecognised currency code: show the numbers and the code as given.
    range = [min, max].filter(Boolean).join(" – ") + (currency ? ` ${currency}` : "");
  }
  return period ? `${range} / ${period}` : range;
}

const relative = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });

/// relativeTime says how long ago something happened, at the precision a
/// person reads a list by: "3 hours ago", "yesterday", "2 weeks ago".
export function relativeTime(iso: string | undefined): string | undefined {
  if (!iso) return undefined;
  const then = new Date(iso).getTime();
  if (Number.isNaN(then) || then <= 0) return undefined;
  const seconds = (then - Date.now()) / 1000;
  const units: [Intl.RelativeTimeFormatUnit, number][] = [
    ["year", 31_536_000],
    ["month", 2_592_000],
    ["week", 604_800],
    ["day", 86_400],
    ["hour", 3_600],
    ["minute", 60],
  ];
  for (const [unit, size] of units) {
    if (Math.abs(seconds) >= size) return relative.format(Math.round(seconds / size), unit);
  }
  return "just now";
}

/// dateTime is an absolute timestamp for a tooltip or a detail line.
export function dateTime(iso: string | undefined): string | undefined {
  if (!iso) return undefined;
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return undefined;
  return date.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
}

/// scoreTone is the colour a match score reads in. The bands are for
/// scanning a list, not a verdict: the analysis is what says why.
export function scoreTone(score: number): string {
  if (score >= 75) return "bg-success/12 text-success";
  if (score >= 50) return "bg-warning/14 text-warning";
  return "bg-muted text-muted-foreground";
}
