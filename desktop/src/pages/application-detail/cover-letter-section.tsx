import { PenLine } from "lucide-react";
import { useState } from "react";
import { CopyButton } from "../../components/copy-button";
import { DeletionDialog } from "../../components/deletion-dialog";
import { DraftFlags, FlaggedText } from "../../components/draft-flags";
import { DetailSection } from "../../components/page";
import { Button } from "../../components/ui/button";
import { Textarea } from "../../components/ui/textarea";
import { relativeTime } from "../../lib/format";
import { useCreateCoverLetter, useEditCoverLetter } from "../../lib/queries/cover-letter.queries";
import { useToast } from "../../lib/toast";
import type { Application, CoverLetter } from "../../lib/types";

/// Editing is what the editor is open for: changing the current version in
/// place, or writing a new one, which the earlier versions are kept beside.
type Editing = { kind: "edit"; letter: CoverLetter } | { kind: "new" } | null;

/// CoverLetterSection is where the candidate writes their letter (ADR-0034
/// §1). JobPilot never writes one; the draft checks only point out what a
/// reader might take for filler.
export function CoverLetterSection({
  applicationId,
  application,
  versions,
}: {
  applicationId: string;
  application: Application;
  versions: CoverLetter[];
}) {
  const { notify, notifyError } = useToast();
  const [editing, setEditing] = useState<Editing>(null);
  const [draft, setDraft] = useState("");
  const [showEarlier, setShowEarlier] = useState(false);
  const [deletingVersion, setDeletingVersion] = useState<CoverLetter | null>(null);

  const close = () => setEditing(null);
  const createCoverLetter = useCreateCoverLetter({ onSuccess: close, onError: notifyError });
  const editCoverLetter = useEditCoverLetter({ onSuccess: close, onError: notifyError });
  const saving = createCoverLetter.isPending || editCoverLetter.isPending;

  const current = versions.find((v) => v.id === application.current_cover_letter_id) ?? versions[0];
  const earlier = versions.filter((v) => v !== current);

  const startEditing = (next: Editing, text: string) => {
    setDraft(text);
    setEditing(next);
  };
  const save = () => {
    if (!draft.trim() || !editing) return;
    if (editing.kind === "edit") {
      editCoverLetter.mutate({ coverLetterId: editing.letter.id, applicationId, content: draft });
    } else {
      createCoverLetter.mutate({ applicationId, content: draft });
    }
  };

  return (
    <DetailSection
      title="Cover letter"
      description={
        !current && !editing ? "Write it yourself, in your own voice. It stays here, in versions." : undefined
      }
      actions={
        !editing &&
        (current ? (
          <>
            <CopyButton text={current.content} label="cover letter" />
            <Button
              size="sm"
              variant="ghost"
              onClick={() => startEditing({ kind: "edit", letter: current }, current.content)}
            >
              Edit
            </Button>
            <Button size="sm" variant="outline" onClick={() => startEditing({ kind: "new" }, current.content)}>
              New version
            </Button>
          </>
        ) : (
          <Button size="sm" variant="outline" onClick={() => startEditing({ kind: "new" }, "")}>
            <PenLine data-icon="inline-start" />
            Write a letter
          </Button>
        ))
      }
    >
      {editing ? (
        <div className="space-y-2">
          <Textarea
            autoFocus
            rows={14}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            aria-label="Cover letter"
            placeholder="Dear …"
          />
          <div className="flex items-center gap-2">
            <Button size="sm" onClick={save} disabled={!draft.trim() || saving}>
              {saving ? "Saving…" : editing.kind === "new" && current ? "Save as new version" : "Save"}
            </Button>
            <Button size="sm" variant="ghost" onClick={close}>
              Cancel
            </Button>
            <span className="ml-auto text-xs text-muted-foreground tabular-nums">
              {wordCount(draft)} {wordCount(draft) === 1 ? "word" : "words"}
            </span>
          </div>
        </div>
      ) : current ? (
        <>
          <div className="rounded-lg border border-border bg-card px-5 py-4">
            <p className="text-[13px] leading-relaxed whitespace-pre-wrap text-subtle-foreground select-text">
              <FlaggedText text={current.content} flags={current.flags} />
            </p>
          </div>
          <p className="mt-2 text-xs text-muted-foreground">
            Saved {relativeTime(current.created_at)}
            {earlier.length > 0 && (
              <>
                {" · "}
                <button type="button" className="underline" onClick={() => setShowEarlier(!showEarlier)}>
                  {showEarlier ? "Hide" : "Show"} {earlier.length} earlier{" "}
                  {earlier.length === 1 ? "version" : "versions"}
                </button>
              </>
            )}
          </p>
          <DraftFlags flags={current.flags} />
          {showEarlier && (
            <div className="mt-3 space-y-3">
              {earlier.map((v) => (
                <div key={v.id} className="rounded-lg border border-dashed border-border px-5 py-4">
                  <div className="mb-2 flex items-center justify-between gap-3">
                    <p className="text-xs text-muted-foreground">Saved {relativeTime(v.created_at)}</p>
                    <Button size="xs" variant="ghost" onClick={() => setDeletingVersion(v)}>
                      Delete
                    </Button>
                  </div>
                  <p className="text-[13px] leading-relaxed whitespace-pre-wrap text-muted-foreground select-text">
                    {v.content}
                  </p>
                </div>
              ))}
            </div>
          )}
        </>
      ) : (
        <p className="text-sm text-muted-foreground">No cover letter yet.</p>
      )}
      <DeletionDialog
        target="cover_letter"
        id={deletingVersion?.id}
        open={deletingVersion !== null}
        onOpenChange={(open) => !open && setDeletingVersion(null)}
        title="Delete this earlier version?"
        note="The version this application uses is kept."
        confirmLabel="Delete version"
        onDeleted={() => notify("Version deleted.", "success")}
      />
    </DetailSection>
  );
}

function wordCount(text: string): number {
  const trimmed = text.trim();
  return trimmed ? trimmed.split(/\s+/).length : 0;
}
