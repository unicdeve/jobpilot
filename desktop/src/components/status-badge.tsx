import { cn } from "cn";
import { Badge } from "./ui/badge";

/// A tone is a colour role, not a colour: each one is a theme token, so a
/// badge reads correctly in light and dark alike.
type Tone = "neutral" | "info" | "warning" | "success" | "highlight" | "danger" | "solidSuccess" | "solidDanger";

const toneClasses: Record<Tone, string> = {
  neutral: "bg-muted text-subtle-foreground",
  info: "bg-info/12 text-info",
  warning: "bg-warning/14 text-warning",
  success: "bg-success/12 text-success",
  highlight: "bg-highlight/12 text-highlight",
  danger: "bg-destructive/12 text-destructive",
  solidSuccess: "bg-success text-white dark:text-background",
  solidDanger: "bg-destructive text-white dark:text-background",
};

const tones: Record<string, Tone> = {
  // application states
  DISCOVERED: "neutral",
  MATCHED: "info",
  PREPARING: "warning",
  READY_FOR_REVIEW: "highlight",
  APPROVED: "success",
  SUBMITTED: "solidSuccess",
  INTERVIEWING: "info",
  OFFERED: "success",
  NO_RESPONSE: "neutral",
  REJECTED: "danger",
  WITHDRAWN: "neutral",
  FAILED: "solidDanger",
  // where an answer came from
  profile_fact: "success",
  derived: "info",
  generated: "highlight",
  needs_user_input: "warning",
  // question status
  pending: "neutral",
  answered: "success",
  // agent runs and steps
  running: "info",
  completed: "success",
  blocked: "warning",
  failed: "solidDanger",
  error: "danger",
  allowed: "success",
  denied: "warning",
};

/// labels override the default wording, which is the value with its case
/// and underscores tidied: READY_FOR_REVIEW reads "Ready for review".
const labels: Record<string, string> = {
  profile_fact: "From profile",
  derived: "Derived",
  generated: "Generated",
  needs_user_input: "Needs you",
  NO_RESPONSE: "No response",
};

export function statusLabel(value: string): string {
  if (!value) return "—";
  if (labels[value]) return labels[value];
  const words = value.replace(/_/g, " ").toLowerCase();
  return words.charAt(0).toUpperCase() + words.slice(1);
}

export function StatusBadge({ value, className }: { value: string; className?: string }) {
  return (
    <Badge variant="outline" className={cn("border-transparent", toneClasses[tones[value] ?? "neutral"], className)}>
      {statusLabel(value)}
    </Badge>
  );
}
