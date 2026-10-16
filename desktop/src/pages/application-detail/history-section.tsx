import { DetailSection } from "../../components/page";
import { statusLabel } from "../../components/status-badge";
import { relativeTime } from "../../lib/format";
import type { ApplicationStateTransition } from "../../lib/types";

export function HistorySection({ history }: { history: ApplicationStateTransition[] }) {
  return (
    <DetailSection title="History">
      <ol className="ml-1 space-y-3 border-l border-border pl-5">
        {[...history].reverse().map((h) => (
          <li key={h.id} className="relative">
            <span className="absolute top-1.5 -left-6 size-2 rounded-full bg-muted-foreground/50 ring-4 ring-background" />
            <p className="text-[13px]">
              {h.from_state ? `${statusLabel(h.from_state)} → ` : ""}
              <span className="font-medium">{statusLabel(h.to_state)}</span>
              <span className="text-muted-foreground"> · {relativeTime(h.created_at) ?? ""}</span>
            </p>
            {h.reason && <p className="mt-0.5 text-[13px] text-muted-foreground select-text">{h.reason}</p>}
          </li>
        ))}
      </ol>
    </DetailSection>
  );
}
