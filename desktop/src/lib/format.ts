// How stored values read on screen. Sources report these fields in their own
// vocabulary, so anything not recognised is shown as-is rather than guessed.

import type { DiscoveryResult, Job, LinkKind, TimeZone, WorkingHours } from "./types";

/// LINK_LABELS names each kind of link the way the forms that ask for it do.
export const LINK_LABELS: Record<LinkKind, string> = {
  linkedin: "LinkedIn",
  github: "GitHub",
  portfolio: "Portfolio",
  website: "Website",
  twitter: "Twitter",
  other: "Other",
};

export const SOURCE_LABELS: Record<string, string> = {
  arbeitnow: "Arbeitnow",
  remotive: "Remotive",
  remoteok: "RemoteOK",
  himalayas: "Himalayas",
  hackernews: "Hacker News",
  weworkremotely: "We Work Remotely",
  jobicy: "Jobicy",
  "company-pages": "Company pages",
  "browser-import": "Added by URL",
};

function sourceLabel(sourceId: string): string {
  return SOURCE_LABELS[sourceId] ?? sourceId;
}

/// jobSource is where one job was found. A job from a company's own page
/// names the company's page rather than the source that read it; a list
/// row, which names the company a line above, just says "Hiring page".
export function jobSource(job: Pick<Job, "source_id" | "company_name">, short = false): string {
  if (job.source_id !== "company-pages") return sourceLabel(job.source_id);
  if (short || !job.company_name) return short ? "Hiring page" : "the company’s hiring page";
  return `${job.company_name}’s hiring page`;
}

export const REMOTE_LABELS = {
  remote: "Remote",
  hybrid: "Hybrid",
  onsite: "On-site",
} as const;

export function remoteLabel(remoteType: string): string | undefined {
  return (REMOTE_LABELS as Record<string, string>)[remoteType?.toLowerCase()];
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

/// MANY_PLACES is how many places a list names before it is counted rather
/// than read out: Himalayas lists a role open across EMEA as 149 countries.
const MANY_PLACES = 6;

/// jobLocation is where a job is, or undefined when the source did not say.
export function jobLocation(job: Pick<Job, "locations">): string | undefined {
  const locations = job.locations?.filter(Boolean) ?? [];
  if (locations.length === 0) return undefined;
  if (locations.length > MANY_PLACES) return `${locations.length} countries`;
  return locations.length > 2 ? `${locations.slice(0, 2).join(", ")} +${locations.length - 2}` : locations.join(", ");
}

/// workableLabel says where a job can be done from, as the daemon read it:
/// "Anywhere", the places it names, or nothing when it does not say.
export function workableLabel(job: Pick<Job, "workable">): string | undefined {
  const workable = job.workable;
  if (workable?.scope === "anywhere") return "Anywhere";
  if (workable?.scope !== "listed") return undefined;
  return placesLabel(workable.places);
}

/// placesLabel names up to three places, "+N" for a few more, and counts a
/// long list: "149 countries".
export function placesLabel(places: string[] | null | undefined): string | undefined {
  const named = places?.filter(Boolean) ?? [];
  if (named.length === 0) return undefined;
  if (named.length > MANY_PLACES) return `${named.length} countries`;
  return named.length > 3 ? `${named.slice(0, 3).join(", ")} +${named.length - 3}` : named.join(", ");
}

/// offerLabel names what a posting says about relocation or visa
/// sponsorship, for a label shown beside the sentence it came from.
export function offerLabel(topic: "relocation" | "visa", status: string): string | undefined {
  const subject = topic === "relocation" ? "Relocation" : "Visa sponsorship";
  switch (status) {
    case "offered":
      return `${subject} offered`;
    case "mentioned":
      return `${subject} mentioned`;
    case "declined":
      return topic === "relocation" ? "No relocation" : "No visa sponsorship";
  }
  return undefined;
}

/// formatOffset writes an offset as a person reads it: "UTC+1", "UTC−5",
/// "UTC+5:30", "UTC", as the daemon's domain.FormatOffset does.
function formatOffset(offset: number): string {
  let value = offset > 14 ? offset - 24 : offset;
  if (value === 0) return "UTC";
  const sign = value < 0 ? "−" : "+";
  value = Math.abs(value);
  const hours = Math.floor(value);
  const minutes = Math.round((value - hours) * 60);
  return `UTC${sign}${hours}${minutes > 0 ? `:${String(minutes).padStart(2, "0")}` : ""}`;
}

/// hoursWindow writes a job's working hours: "UTC−2 to UTC+4", or one zone.
export function hoursWindow(hours: Pick<WorkingHours, "from" | "to">): string {
  return hours.from === hours.to
    ? formatOffset(hours.from)
    : `${formatOffset(hours.from)} to ${formatOffset(hours.to)}`;
}

/// hoursShiftFor is how many hours a candidate at offset would move their
/// day to be inside a job's window: zero inside it, measured around the
/// clock, as domain.WorkingHours.ShiftFor.
export function hoursShiftFor(hours: Pick<WorkingHours, "from" | "to">, offset: number): number {
  const width = hours.to - hours.from;
  let into = (offset - hours.from) % 24;
  if (into < 0) into += 24;
  if (into <= width) return 0;
  return Math.min(into - width, 24 - into);
}

/// timeZoneLabel names a zone by its city and offset: "Lagos (UTC+1)",
/// "Indiana/Knox (UTC−6)".
export function timeZoneLabel(zone: TimeZone): string {
  const place = zone.id.split("/").slice(1).join("/").replace(/_/g, " ") || zone.id;
  return `${place} (${formatOffset(zone.offset)})`;
}

/// salaryLabel renders a salary range only when the source gave one, in its
/// own currency and period; nothing here converts or estimates.
export function salaryLabel(
  job: Pick<Job, "salary_min" | "salary_max" | "salary_currency" | "salary_period">,
): string | undefined {
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

/// discoverySummary is what a finished search says: how many jobs matched,
/// how many of them are new, and which sources were not heard from, since a
/// thin result from a source that did not answer is not a thin market. It
/// names the search, since several can finish one after another.
export function discoverySummary(result: DiscoveryResult, searchName: string): { message: string; complete: boolean } {
  const found = result.jobs.length;
  const sentences = [
    found === 0
      ? `No jobs matched “${searchName}”.`
      : `Found ${found} ${found === 1 ? "job" : "jobs"} for “${searchName}”, ${result.new === 0 ? "none" : result.new} new.`,
  ];

  const unreachable = result.sources.filter((s) => s.status === "unreachable").map((s) => sourceLabel(s.source_id));
  const failed = result.sources.filter((s) => s.status === "failed").map((s) => sourceLabel(s.source_id));
  if (unreachable.length > 0) sentences.push(`Couldn’t reach ${joinNames(unreachable)}.`);
  if (failed.length > 0) sentences.push(`${joinNames(failed)} answered with an error.`);
  const failing = failingCompanies([result]);
  if (failing.length > 0) sentences.push(`Couldn’t read the hiring page of ${joinNames(failing)}.`);

  return { message: sentences.join(" "), complete: unreachable.length + failed.length + failing.length === 0 };
}

/// failingCompanies names the watched companies whose pages did not
/// answer, each once (ADR-0024 §5).
function failingCompanies(results: DiscoveryResult[]): string[] {
  return [...new Set(results.flatMap((result) => result.sources.flatMap((source) => source.failing ?? [])))];
}

/// RefreshFailure is a search a launch refresh could not run. offline is a
/// run no source could be reached for.
export interface RefreshFailure {
  offline: boolean;
  message: string;
}

/// refreshSummary is the one thing a launch refresh says, however many
/// searches it ran: a toast for each, every time the app opens, would be
/// noise about work the user did not ask for.
export function refreshSummary(
  results: DiscoveryResult[],
  failures: RefreshFailure[],
): { message: string; complete: boolean } {
  if (results.length === 0) {
    const offline = failures.every((failure) => failure.offline);
    return {
      message: offline
        ? "Couldn’t reach the job sources, so there are no new jobs this time. The jobs already found are all here."
        : `Couldn’t look for new jobs: ${failures[0]?.message ?? "no search ran"}`,
      complete: false,
    };
  }

  const found = results.reduce((sum, result) => sum + result.new, 0);
  const searches = results.length;
  const sentences = [
    found === 0
      ? "No new jobs since the last search."
      : searches === 1
        ? `Found ${found} new ${found === 1 ? "job" : "jobs"}.`
        : `Found ${found} new ${found === 1 ? "job" : "jobs"} across ${searches} searches.`,
  ];

  // A source that did not answer any one search is named once.
  const outcomes = results.flatMap((result) => result.sources);
  const named = (status: string) => [
    ...new Set(outcomes.filter((s) => s.status === status).map((s) => sourceLabel(s.source_id))),
  ];
  const unreachable = named("unreachable");
  const failed = named("failed").filter((name) => !unreachable.includes(name));
  if (unreachable.length > 0) sentences.push(`Couldn’t reach ${joinNames(unreachable)}.`);
  if (failed.length > 0) sentences.push(`${joinNames(failed)} answered with an error.`);
  const failing = failingCompanies(results);
  if (failing.length > 0) sentences.push(`Couldn’t read the hiring page of ${joinNames(failing)}.`);
  if (failures.length > 0) {
    sentences.push(`${failures.length} ${failures.length === 1 ? "search" : "searches"} couldn’t run.`);
  }

  return {
    message: sentences.join(" "),
    complete: unreachable.length + failed.length + failing.length + failures.length === 0,
  };
}

/// joinNames lists names as a sentence does: "A", "A and B", "A, B and C".
function joinNames(names: string[]): string {
  if (names.length <= 1) return names.join("");
  return `${names.slice(0, -1).join(", ")} and ${names[names.length - 1]}`;
}
