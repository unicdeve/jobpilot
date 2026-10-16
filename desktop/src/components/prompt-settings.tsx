import { cn } from "cn";
import { type ReactNode, useState } from "react";
import { Link } from "react-router-dom";
import {
  usePromptGuidance,
  useResetPromptGuidance,
  useSavePromptGuidance,
} from "../lib/queries/prompt-guidance.queries";
import { useToast } from "../lib/toast";
import type { PromptGuidance } from "../lib/types";
import { ConfirmDialog } from "./confirm";
import { Panel } from "./page";
import { SettingsHeading } from "./settings-heading";
import { ErrorLine, Loading } from "./status-line";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import { Textarea } from "./ui/textarea";

/// PromptSettings is where the user changes the guidance JobPilot judges a
/// fit and writes under (ADR-0031). Only the guidance is offered: the rules
/// that keep the model to what the candidate said, and keep a posting from
/// instructing it, are not shown as editable because nothing may change
/// them.
export function PromptSettings() {
  const { data, error } = usePromptGuidance();

  if (error) return <ErrorLine error={error} />;
  if (!data) return <Loading />;

  return (
    <>
      <SettingsHeading
        title="Prompts"
        description="How JobPilot judges a fit and writes for you. The rules that keep it to what you’ve told it, and stop a job posting from instructing it, aren’t part of this and can’t be changed."
      />
      <div className="space-y-4">
        {data.prompts.map((prompt) => (
          <PromptCard key={prompt.key} prompt={prompt} maxLength={data.max_length} />
        ))}
      </div>
      <p className="mt-4 text-xs text-muted-foreground">
        A change applies from the next job scored or the next draft written. Nothing already scored or written changes.
      </p>
    </>
  );
}

/// characters counts as core does, by code point, so an accented letter or
/// an emoji is one character here too.
function characters(text: string): number {
  return [...text].length;
}

function PromptCard({ prompt, maxLength }: { prompt: PromptGuidance; maxLength: number }) {
  const { notify, notifyError } = useToast();
  const current = prompt.custom ?? prompt.default;
  // The textarea is an editable draft over the saved guidance, dropped once
  // what it holds has been saved or reverted.
  const [draft, setDraft] = useState<string | null>(null);
  const [confirmReset, setConfirmReset] = useState(false);
  const [showDefault, setShowDefault] = useState(false);

  const save = useSavePromptGuidance({
    onSuccess: (saved) => {
      setDraft(null);
      notify(`Saved your guidance for ${saved.title.toLowerCase()}.`, "success");
    },
    onError: notifyError,
  });
  const reset = useResetPromptGuidance({
    onSuccess: (saved) => {
      setDraft(null);
      setConfirmReset(false);
      setShowDefault(false);
      notify(`Back to the default for ${saved.title.toLowerCase()}.`, "success");
    },
    onError: notifyError,
  });

  const text = draft ?? current;
  const trimmed = text.trim();
  const dirty = draft !== null && trimmed !== current.trim();
  const length = characters(trimmed);
  const tooLong = length > maxLength;
  const empty = trimmed === "";
  const isDefault = prompt.custom === undefined;
  // The count is shown only near the limit, where it is worth reading.
  const showCount = length >= maxLength * 0.9;
  const changed = draft !== null && draft !== current;

  let hint: ReactNode = null;
  if (empty && dirty) {
    hint = (
      <span className="text-destructive">
        Guidance can’t be empty. {isDefault ? "Revert to keep the default." : "To go back to the default, reset it."}
      </span>
    );
  } else if (prompt.key === "cover_letter") {
    hint = (
      <>
        How long a letter is, and its tone, are your writing preferences, under{" "}
        <Link to="/profile?tab=about" className="text-foreground underline">
          About you
        </Link>
        .
      </>
    );
  }
  const fieldId = `prompt-${prompt.key}`;

  return (
    <Panel>
      <div className="flex items-start gap-3 px-4 pt-3.5">
        <label htmlFor={fieldId} className="min-w-0 flex-1">
          <span className="flex items-center gap-2">
            <span className="text-[13px] font-medium">{prompt.title}</span>
            <Badge variant={isDefault ? "secondary" : "outline"}>{isDefault ? "Default" : "Your own"}</Badge>
          </span>
          <span className="mt-0.5 block text-xs text-muted-foreground">{prompt.used_for}</span>
        </label>
        {!isDefault && (
          <Button size="xs" variant="ghost" className="-mr-2" onClick={() => setShowDefault((shown) => !shown)}>
            {showDefault ? "Hide the default" : "Show the default"}
          </Button>
        )}
      </div>

      {prompt.default_changed && (
        <div className="mx-4 mt-3 flex items-center gap-3 rounded-lg border border-border bg-muted/50 px-3 py-2 text-xs text-subtle-foreground">
          <span className="min-w-0 flex-1">The default has changed since you edited this.</span>
          {!showDefault && (
            <Button size="xs" variant="ghost" onClick={() => setShowDefault(true)}>
              See the new default
            </Button>
          )}
          {/* Saving the guidance as it is records the default the user has
              now seen, which is what clears this (ADR-0031 §7). */}
          <Button
            size="xs"
            variant="outline"
            onClick={() => save.mutate({ key: prompt.key, text: current })}
            disabled={save.isPending}
          >
            Keep yours
          </Button>
        </div>
      )}

      {!isDefault && showDefault && (
        <div className="mx-4 mt-3">
          <p className="mb-1 text-xs font-medium text-muted-foreground">The default</p>
          <p className="rounded-lg bg-muted/50 px-2.5 py-2 text-[13px] whitespace-pre-wrap text-subtle-foreground select-text">
            {prompt.default}
          </p>
        </div>
      )}

      <div className="px-4 pt-3">
        {!isDefault && showDefault && (
          <label htmlFor={fieldId} className="mb-1 block text-xs font-medium text-muted-foreground">
            Yours
          </label>
        )}
        <Textarea
          id={fieldId}
          rows={3}
          value={text}
          aria-invalid={tooLong || (dirty && empty)}
          onChange={(e) => setDraft(e.target.value)}
        />
        {(hint || showCount) && (
          <div className="mt-1.5 flex items-start gap-3 text-xs text-muted-foreground">
            <span className="min-w-0 flex-1">{hint}</span>
            {showCount && (
              <span className={cn("shrink-0 tabular-nums", tooLong && "text-destructive")}>
                {length.toLocaleString()} / {maxLength.toLocaleString()}
              </span>
            )}
          </div>
        )}
      </div>

      {/* Actions appear once there is something to do: a change to save, or
          the user's own guidance to reset. */}
      {changed || !isDefault ? (
        <div className="mt-3 flex items-center justify-end gap-2 border-t border-border px-4 py-2.5">
          {!isDefault && (
            <Button size="sm" variant="ghost" className="-ml-2.5 mr-auto" onClick={() => setConfirmReset(true)}>
              Reset to default
            </Button>
          )}
          {changed && (
            <Button size="sm" variant="ghost" onClick={() => setDraft(null)}>
              Revert
            </Button>
          )}
          <Button
            size="sm"
            onClick={() => save.mutate({ key: prompt.key, text: trimmed })}
            disabled={!dirty || empty || tooLong || save.isPending}
          >
            {save.isPending ? "Saving…" : "Save"}
          </Button>
        </div>
      ) : (
        <div className="pb-3.5" />
      )}

      <ConfirmDialog
        open={confirmReset}
        onOpenChange={setConfirmReset}
        title={`Reset ${prompt.title.toLowerCase()} to the default?`}
        description="Your guidance is removed, and this prompt follows JobPilot’s default, including as later releases improve it."
        confirmLabel="Reset"
        onConfirm={() => reset.mutate(prompt.key)}
        pending={reset.isPending}
      />
    </Panel>
  );
}
