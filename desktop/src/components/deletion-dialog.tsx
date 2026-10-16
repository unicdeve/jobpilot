import { LoaderCircle, TriangleAlert } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import { ApiError } from "../lib/api";
import { countedKinds, keptNote, sentNote, totalDeleted } from "../lib/deletion";
import { useDeleteRecords, useDeletionPreview } from "../lib/queries/deletion.queries";
import type { DeletionReach, DeletionResult, DeletionTarget } from "../lib/types";
import { Button } from "./ui/button";
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogTitle } from "./ui/dialog";
import { Input } from "./ui/input";

/// The word "delete all data" asks to be typed, so it is never confirmed by
/// reflex.
const CONFIRM_WORD = "delete";

/// DeletionDialog shows what a deletion takes with it and carries it out
/// once confirmed (ADR-0021). The preview is fetched as the dialog opens,
/// so what it lists is what is stored now, and the deletion is sent with the
/// preview's token: if anything changed in between, the daemon refuses it,
/// and the dialog shows what it reaches now and asks again.
export function DeletionDialog({
  target,
  id,
  open,
  onOpenChange,
  title,
  confirmLabel,
  note,
  onDeleted,
}: {
  target: DeletionTarget;
  id?: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  confirmLabel: string;
  /// note is anything the reach cannot say: that an original file on disk
  /// is untouched, say.
  note?: ReactNode;
  onDeleted?: (result: DeletionResult) => void;
}) {
  const preview = useDeletionPreview(open ? target : null, id);
  // Set when a deletion was refused because its reach changed: the reach as
  // it is now, which replaces the preview until the dialog is reopened.
  const [changed, setChanged] = useState<DeletionReach | null>(null);
  const [typed, setTyped] = useState("");
  const [failure, setFailure] = useState<string | null>(null);

  useEffect(() => {
    if (!open) {
      setChanged(null);
      setTyped("");
      setFailure(null);
    }
  }, [open]);

  const remove = useDeleteRecords({
    onSuccess: (result) => {
      onOpenChange(false);
      onDeleted?.(result);
    },
    onError: (err) => {
      if (err instanceof ApiError && err.reach) {
        setChanged(err.reach);
        setTyped("");
        return;
      }
      setFailure(err.message);
    },
  });

  const reach = changed ?? preview.data;
  const everything = target === "everything";
  const nothing = reach !== undefined && totalDeleted(reach) === 0 && (reach.outside?.length ?? 0) === 0;
  const confirmed = !everything || typed.trim() === CONFIRM_WORD;
  const confirm = () => reach && remove.mutate({ target, id, token: reach.token });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogTitle>{title}</DialogTitle>
        {preview.isPending && !changed ? (
          <DialogDescription className="flex items-center gap-2">
            <LoaderCircle className="size-4 animate-spin" />
            Checking what this deletes…
          </DialogDescription>
        ) : preview.error && !changed ? (
          // A refusal — the cover letter in use — is the whole answer.
          <DialogDescription className="text-destructive">{preview.error.message}</DialogDescription>
        ) : reach ? (
          <div className="mt-3 space-y-3 text-sm">
            {changed && (
              <p className="flex items-start gap-2 rounded-md bg-warning/10 px-3 py-2 text-[13px] text-warning">
                <TriangleAlert className="mt-px size-4 shrink-0" />
                This changed since it was shown. This is what it deletes now.
              </p>
            )}
            <ReachSummary reach={reach} />
            {note && <p className="text-muted-foreground">{note}</p>}
            {!nothing && <p className="text-muted-foreground">This cannot be undone.</p>}
            {everything && !nothing && (
              <label className="block space-y-1.5">
                <span className="text-[13px]">
                  Type <span className="font-mono font-medium">{CONFIRM_WORD}</span> to confirm
                </span>
                <Input
                  value={typed}
                  onChange={(event) => setTyped(event.target.value)}
                  autoComplete="off"
                  spellCheck={false}
                  aria-label={`Type ${CONFIRM_WORD} to confirm`}
                />
              </label>
            )}
            {failure && <p className="text-destructive">{failure}</p>}
          </div>
        ) : null}
        <DialogFooter>
          <DialogClose render={<Button variant="outline" />}>{reach && !nothing ? "Cancel" : "Close"}</DialogClose>
          {reach && !nothing && (
            <Button variant="destructive" onClick={confirm} disabled={!confirmed || remove.isPending}>
              {remove.isPending ? "Deleting…" : confirmLabel}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/// ReachSummary lists what a deletion takes, keeps and leaves outside.
function ReachSummary({ reach }: { reach: DeletionReach }) {
  const deleted = countedKinds(reach.deleted);
  const detached = countedKinds(reach.detached);
  const sent = sentNote(reach);
  const kept = keptNote(reach);
  const outside = reach.outside ?? [];

  if (deleted.length === 0 && outside.length === 0) {
    return (
      <>
        <p className="text-muted-foreground">There is nothing to delete.</p>
        {kept && <p className="text-muted-foreground">{kept}</p>}
      </>
    );
  }
  return (
    <>
      {deleted.length > 0 && <Section heading="This deletes" lines={deleted} />}
      {sent && <p className="text-[13px] text-warning">{sent}</p>}
      {detached.length > 0 && <Section heading="These stay, but lose their link to what is deleted" lines={detached} />}
      {kept && <p className="text-muted-foreground">{kept}</p>}
      {outside.length > 0 && <Section heading="Outside the database, it also removes" lines={outside} />}
      {(reach.environment_credentials ?? []).map((name) => (
        <p key={name} className="text-[13px] text-warning">
          The environment variable <span className="font-mono">{name}</span> supplies an API key. JobPilot cannot unset
          it; remove it where it is set.
        </p>
      ))}
    </>
  );
}

function Section({ heading, lines }: { heading: string; lines: string[] }) {
  return (
    <div>
      <p className="text-[13px] font-medium">{heading}</p>
      <ul className="mt-1 list-disc space-y-0.5 pl-5 text-subtle-foreground">
        {lines.map((line) => (
          <li key={line}>{line}</li>
        ))}
      </ul>
    </div>
  );
}
