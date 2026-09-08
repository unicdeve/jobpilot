import { Badge } from "./ui/badge";

const stateColors: Record<string, string> = {
  DISCOVERED: "bg-slate-100 text-slate-700",
  MATCHED: "bg-sky-100 text-sky-700",
  PREPARING: "bg-amber-100 text-amber-700",
  READY_FOR_REVIEW: "bg-violet-100 text-violet-700",
  APPROVED: "bg-emerald-100 text-emerald-700",
  SUBMITTED: "bg-emerald-600 text-white",
  REJECTED: "bg-red-100 text-red-700",
  WITHDRAWN: "bg-slate-200 text-slate-600",
  FAILED: "bg-red-600 text-white",
  profile_fact: "bg-emerald-100 text-emerald-700",
  derived: "bg-sky-100 text-sky-700",
  generated: "bg-violet-100 text-violet-700",
  needs_user_input: "bg-amber-100 text-amber-700",
  pending: "bg-slate-100 text-slate-700",
  answered: "bg-emerald-100 text-emerald-700",
  running: "bg-sky-100 text-sky-700",
  completed: "bg-emerald-100 text-emerald-700",
  blocked: "bg-amber-100 text-amber-700",
  failed: "bg-red-600 text-white",
};

export function StatusBadge({ value }: { value: string }) {
  return (
    <Badge variant="outline" className={stateColors[value] ?? "bg-slate-100 text-slate-700"}>
      {value || "—"}
    </Badge>
  );
}
