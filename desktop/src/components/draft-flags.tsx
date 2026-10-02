import { TriangleAlert } from "lucide-react";
import type { ReactNode } from "react";
import type { DraftFlag } from "../lib/types";

/// FlaggedText shows a letter or written answer with the words its flags
/// are about marked, so the reviewer finds them without searching
/// (docs/02_features/19_draft_checks.md D3). A flag quotes the words as the
/// text has them, so every occurrence is marked.
export function FlaggedText({ text, flags }: { text: string; flags?: DraftFlag[] }) {
  const ranges = flaggedRanges(text, flags ?? []);
  if (ranges.length === 0) return <>{text}</>;

  const parts: ReactNode[] = [];
  let at = 0;
  for (const [start, end] of ranges) {
    if (start > at) parts.push(text.slice(at, start));
    parts.push(
      <mark key={start} className="rounded-sm bg-warning/20 text-inherit">
        {text.slice(start, end)}
      </mark>,
    );
    at = end;
  }
  if (at < text.length) parts.push(text.slice(at));
  return <>{parts}</>;
}

/// flaggedRanges is where each flag's words occur in text, in order and
/// without overlaps: where two overlap, the earlier wins.
function flaggedRanges(text: string, flags: DraftFlag[]): [number, number][] {
  const found: [number, number][] = [];
  for (const { excerpt } of flags) {
    if (!excerpt) continue;
    for (let i = text.indexOf(excerpt); i !== -1; i = text.indexOf(excerpt, i + excerpt.length)) {
      found.push([i, i + excerpt.length]);
    }
  }
  found.sort((a, b) => a[0] - b[0] || b[1] - a[1]);

  const ranges: [number, number][] = [];
  for (const range of found) {
    const last = ranges[ranges.length - 1];
    if (!last || range[0] >= last[1]) ranges.push(range);
  }
  return ranges;
}

/// DraftFlags lists what the reviewer should look at in a letter or written
/// answer, each with why. They are advice: nothing is blocked, and a flag
/// on something true can be left as it is.
export function DraftFlags({ flags }: { flags?: DraftFlag[] }) {
  if (!flags || flags.length === 0) return null;
  return (
    <div data-slot="draft-flags" className="mt-3 rounded-lg border border-warning/30 bg-warning/8 px-4 py-3">
      <p className="flex items-center gap-1.5 text-[13px] font-medium text-warning">
        <TriangleAlert className="size-3.5 shrink-0" />
        To review
      </p>
      <ul className="mt-1.5 space-y-1">
        {flags.map((flag, i) => (
          <li key={i} className="text-[13px] text-subtle-foreground">
            {flag.excerpt && <span className="font-medium text-foreground">“{flag.excerpt}”</span>}
            {flag.excerpt && " — "}
            {flag.reason}
          </li>
        ))}
      </ul>
    </div>
  );
}
