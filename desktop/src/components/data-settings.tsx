import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useToast } from "../lib/toast";
import type { DeletionResult, DeletionTarget } from "../lib/types";
import { DeletionDialog } from "./deletion-dialog";
import { Panel } from "./page";
import { SettingsHeading } from "./settings-heading";
import { Button } from "./ui/button";

interface DataAction {
  target: DeletionTarget;
  title: string;
  description: string;
  button: string;
  dialogTitle: string;
  confirmLabel: string;
  note?: string;
  done: string;
}

/// DATA_ACTIONS are the deletions that are not about one record
/// (docs/02_features/15_data_deletion.md), mildest first.
const DATA_ACTIONS: DataAction[] = [
  {
    target: "activity",
    title: "Clear activity",
    description: "Every agent run and its steps. Applications and jobs are untouched.",
    button: "Clear activity…",
    dialogTitle: "Clear the activity history?",
    confirmLabel: "Clear activity",
    done: "Activity cleared.",
  },
  {
    target: "jobs",
    title: "Clear jobs",
    description:
      "Every stored job no application uses. Jobs you marked not interested are kept, so they are not offered again.",
    button: "Clear jobs…",
    dialogTitle: "Clear unused jobs?",
    confirmLabel: "Clear jobs",
    note: "A later search may find the same jobs again.",
    done: "Unused jobs cleared.",
  },
  {
    target: "profile",
    title: "Delete your profile",
    description: "Your profile, resumes, match analyses and applications. Jobs, searches, scouts and settings stay.",
    button: "Delete profile…",
    dialogTitle: "Delete your profile?",
    confirmLabel: "Delete profile",
    done: "Profile deleted.",
  },
  {
    target: "everything",
    title: "Reset this profile",
    description:
      "Everything in this profile, including its API keys and browser sign-ins. Your other profiles and device preferences are untouched.",
    button: "Reset profile…",
    dialogTitle: "Reset this profile?",
    confirmLabel: "Reset profile",
    note: "The profile remains available and starts again at setup.",
    done: "Profile reset.",
  },
];

/// DataSettings is where data leaves JobPilot in bulk. Each action shows
/// what it takes before it takes it.
export function DataSettings() {
  const { notify } = useToast();
  const navigate = useNavigate();
  const [open, setOpen] = useState<DeletionTarget | null>(null);

  const deleted = (action: DataAction, result: DeletionResult) => {
    notify(action.done, "success");
    for (const notice of result.notices ?? []) notify(notice);
    // Without a profile there is nothing to use until setup is done again.
    if (action.target === "profile" || action.target === "everything") navigate("/welcome");
  };

  return (
    <>
      <SettingsHeading
        title="Data"
        description="Everything JobPilot knows is stored on this computer. Delete a single job or application from its own page; clear things in bulk here."
      />
      <Panel className="divide-y divide-border">
        {DATA_ACTIONS.map((action) => (
          <div key={action.target} className="flex items-center justify-between gap-6 px-4 py-3">
            <div>
              <p className="text-[13px] font-medium">{action.title}</p>
              <p className="mt-1 text-xs text-muted-foreground">{action.description}</p>
            </div>
            <Button
              size="sm"
              variant={action.target === "everything" ? "destructive" : "outline"}
              className="shrink-0"
              onClick={() => setOpen(action.target)}
            >
              {action.button}
            </Button>
          </div>
        ))}
      </Panel>
      {DATA_ACTIONS.map((action) => (
        <DeletionDialog
          key={action.target}
          target={action.target}
          open={open === action.target}
          onOpenChange={(isOpen) => setOpen(isOpen ? action.target : null)}
          title={action.dialogTitle}
          note={action.note}
          confirmLabel={action.confirmLabel}
          onDeleted={(result) => deleted(action, result)}
        />
      ))}
    </>
  );
}
