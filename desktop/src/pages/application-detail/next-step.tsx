import { useState } from "react";
import { Button } from "../../components/ui/button";
import { Input } from "../../components/ui/input";
import { useAllowedTransitions, useTransitionApplication } from "../../lib/queries/application.queries";
import { useToast } from "../../lib/toast";
import type { Application, ApplicationState } from "../../lib/types";

/// What each transition is called from the user's side. The states are the
/// daemon's vocabulary; these are the actions a person recognises taking.
const STATE_ACTIONS: Record<ApplicationState, string> = {
  DISCOVERED: "Move back to discovered",
  MATCHED: "Mark as matched",
  PREPARING: "Start preparing",
  READY_FOR_REVIEW: "Mark ready for review",
  APPROVED: "Approve",
  SUBMITTED: "I submitted this",
  INTERVIEWING: "I'm interviewing",
  OFFERED: "I got an offer",
  NO_RESPONSE: "Never heard back",
  REJECTED: "I was rejected",
  WITHDRAWN: "Withdraw",
  FAILED: "Mark as failed",
};

/// States that mean the employer's answer, rather than JobPilot's progress.
const OUTCOME_STATES: ApplicationState[] = ["SUBMITTED", "INTERVIEWING", "OFFERED", "NO_RESPONSE"];

/// NextStep offers the transitions the daemon says are possible, under a
/// heading that changes with where the application is.
///
/// The allowed set is fetched rather than restated here, so the buttons and
/// the rule that enforces them cannot drift apart.
export function NextStep({ application }: { application: Application }) {
  const { notifyError } = useToast();
  const [reason, setReason] = useState("");
  const { data: allowed, isPending } = useAllowedTransitions(application.id);

  const transitionApplication = useTransitionApplication({
    onSuccess: () => setReason(""),
    onError: notifyError,
  });
  const pending = transitionApplication.isPending ? transitionApplication.variables?.toState : null;

  const transition = (toState: ApplicationState) =>
    transitionApplication.mutate({ id: application.id, toState, reason });

  // The whole point of the handoff: JobPilot filled the form and left the
  // browser open, and only the user knows whether they pressed send.
  const awaitingSubmission = application.state === "APPROVED";
  const afterSubmission = OUTCOME_STATES.includes(application.state);

  if (isPending) return null;

  if (!allowed || allowed.length === 0) {
    return (
      <div className="mt-6 rounded-lg border border-border bg-muted/50 px-4 py-3">
        <p className="text-sm font-medium">Closed</p>
        <p className="mt-0.5 text-sm text-muted-foreground">
          This application is finished — there is nothing left to record.
        </p>
      </div>
    );
  }

  const primary: ApplicationState | undefined =
    awaitingSubmission && allowed.includes("SUBMITTED") ? "SUBMITTED" : undefined;

  return (
    <div
      className={`mt-6 rounded-lg border px-4 py-3.5 ${
        awaitingSubmission ? "border-warning/35 bg-warning/8" : "border-border bg-muted/50"
      }`}
    >
      <p className="text-sm font-medium">
        {awaitingSubmission ? "Did you send it?" : afterSubmission ? "What happened?" : "Next step"}
      </p>
      <p className="mt-0.5 text-sm text-muted-foreground">
        {awaitingSubmission
          ? "JobPilot never submits for you, so it cannot tell. Record it here and this stops waiting on you."
          : afterSubmission
            ? "Tell JobPilot what the employer said, so a sent application does not go quiet in your list."
            : "Where this application goes next."}
      </p>

      <div className="mt-3 flex flex-wrap gap-2">
        {[...(primary ? [primary] : []), ...allowed.filter((state) => state !== primary)].map((state) => (
          <Button
            key={state}
            size="sm"
            variant={state === primary ? "default" : "outline"}
            disabled={transitionApplication.isPending}
            onClick={() => transition(state)}
          >
            {pending === state ? "Saving…" : STATE_ACTIONS[state]}
          </Button>
        ))}
      </div>
      <Input
        className="mt-3 h-8 bg-background"
        placeholder="Add a note to the history (optional)"
        value={reason}
        onChange={(e) => setReason(e.target.value)}
      />
    </div>
  );
}
