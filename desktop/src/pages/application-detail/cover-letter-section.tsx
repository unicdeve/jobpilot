import { Sparkles } from "lucide-react";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { DeletionDialog } from "../../components/deletion-dialog";
import { DraftFlags, FlaggedText } from "../../components/draft-flags";
import { DetailSection } from "../../components/page";
import { Thinking } from "../../components/thinking";
import { ThinkingFrame } from "../../components/thinking-frame";
import { Button } from "../../components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "../../components/ui/dialog";
import { Textarea } from "../../components/ui/textarea";
import { relativeTime } from "../../lib/format";
import { useAboutYou } from "../../lib/queries/about-you.queries";
import { useEditCoverLetter, useGenerateCoverLetter } from "../../lib/queries/cover-letter.queries";
import {
  usePersonalQuestions,
  usePreparePersonalQuestions,
  useSavePersonalAnswers,
} from "../../lib/queries/question.queries";
import { useToast } from "../../lib/toast";
import type { Application, ApplicationAnswer, CoverLetter, PersonalAnswer } from "../../lib/types";

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
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [showEarlier, setShowEarlier] = useState(false);
  const [deletingVersion, setDeletingVersion] = useState<CoverLetter | null>(null);
  const [asking, setAsking] = useState(false);

  const generateCoverLetter = useGenerateCoverLetter({ onError: notifyError });
  const editCoverLetter = useEditCoverLetter({
    onSuccess: () => setEditing(false),
    onError: notifyError,
  });

  const current = versions.find((v) => v.id === application.current_cover_letter_id) ?? versions[0];
  const earlier = versions.filter((v) => v !== current);
  const writing = generateCoverLetter.isPending;

  return (
    <DetailSection
      title="Cover letter"
      actions={
        !editing && (
          <>
            {current && (
              <Button
                size="sm"
                variant="ghost"
                onClick={() => {
                  setDraft(current.content);
                  setEditing(true);
                }}
                // An edit to the letter being replaced would be saved to a
                // version that is no longer current once the new one lands.
                disabled={writing}
              >
                Edit
              </Button>
            )}
            <Button size="sm" variant="outline" onClick={() => setAsking(true)} disabled={writing}>
              <Sparkles data-icon="inline-start" />
              {writing ? "Writing…" : current ? "Rewrite" : "Write one"}
            </Button>
          </>
        )
      }
    >
      {!current ? (
        writing ? (
          <Thinking state="composing">Writing your cover letter…</Thinking>
        ) : (
          <p className="text-sm text-muted-foreground">
            No cover letter yet. JobPilot writes one from your profile, what you tell it about yourself, and this job.
          </p>
        )
      ) : editing ? (
        <div className="space-y-2">
          <Textarea rows={14} value={draft} onChange={(e) => setDraft(e.target.value)} />
          <div className="flex gap-2">
            <Button
              size="sm"
              onClick={() => editCoverLetter.mutate({ coverLetterId: current.id, applicationId, content: draft })}
              disabled={editCoverLetter.isPending}
            >
              {editCoverLetter.isPending ? "Saving…" : "Save"}
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setEditing(false)}>
              Cancel
            </Button>
          </div>
        </div>
      ) : (
        <>
          {/* The letter stays readable while its replacement is written, and
              is marked as the one being replaced. */}
          {writing && (
            <Thinking state="composing" className="mb-3">
              Writing a new version. This one stays until it is ready.
            </Thinking>
          )}
          <ThinkingFrame active={writing}>
            <div className="rounded-lg border border-border bg-card px-5 py-4">
              <p className="text-[13px] leading-relaxed whitespace-pre-wrap text-subtle-foreground select-text">
                <FlaggedText text={current.content} flags={current.flags} />
              </p>
            </div>
          </ThinkingFrame>
          <p className="mt-2 text-xs text-muted-foreground">
            {current.user_edited
              ? "Edited by you"
              : `Written${current.model_name ? ` with ${current.model_name}` : ""}`}{" "}
            {relativeTime(current.created_at)}
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
                    <p className="text-xs text-muted-foreground">
                      {v.user_edited ? "Edited" : "Written"} {relativeTime(v.created_at)}
                    </p>
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
      )}
      <LetterQuestionsDialog
        applicationId={applicationId}
        open={asking}
        onOpenChange={setAsking}
        onWrite={() => generateCoverLetter.mutate(applicationId)}
      />
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

/// LetterQuestionsDialog asks about this job before a letter is written:
/// what draws the candidate to it, and up to two questions the model
/// proposes from the posting (docs/02_features/16_about_you.md). Every one
/// can be skipped, and the letter is written either way. The fixed question
/// shows at once; the proposed ones follow while the posting is read.
function LetterQuestionsDialog({
  applicationId,
  open,
  onOpenChange,
  onWrite,
}: {
  applicationId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onWrite: () => void;
}) {
  const { notifyError } = useToast();
  const { data: about } = useAboutYou();
  const { data: questions } = usePersonalQuestions(applicationId, open);
  const prepare = usePreparePersonalQuestions();
  const saveAnswers = useSavePersonalAnswers();
  // Typed answers, by question: a proposed one by its id, the fixed one by
  // its key, since it has no row until it is answered.
  const [typed, setTyped] = useState<Record<string, string>>({});
  const { mutate: prepareQuestions } = prepare;

  useEffect(() => {
    if (!open) return;
    setTyped({});
    prepareQuestions(applicationId);
  }, [open, applicationId, prepareQuestions]);

  const idOf = (question: PersonalAnswer) => question.id || question.key;
  const aboutEmpty =
    about?.questions.every((question) => !question.answer) && !about.writing_sample && !about.writing_preferences;

  const write = async (withAnswers: boolean) => {
    const changed: ApplicationAnswer[] = withAnswers
      ? (questions ?? [])
          .filter((question) => typed[idOf(question)] !== undefined && typed[idOf(question)] !== question.answer)
          .map((question) => ({ id: question.id || undefined, key: question.key, answer: typed[idOf(question)] }))
      : [];
    try {
      if (changed.length > 0) await saveAnswers.mutateAsync({ applicationId, answers: changed });
      onOpenChange(false);
      onWrite();
    } catch (err) {
      notifyError(err);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogTitle>Before JobPilot writes</DialogTitle>
        <DialogDescription>
          A few words about this job make the letter yours. Skip any question; the letter is written either way.
        </DialogDescription>

        {aboutEmpty && (
          <p className="mt-3 rounded-md bg-muted px-3 py-2 text-[13px] text-subtle-foreground">
            Letters stay generic until you tell JobPilot about yourself — about five minutes.{" "}
            <Link
              to="/profile?tab=about"
              className="font-medium text-foreground underline"
              onClick={() => onOpenChange(false)}
            >
              Tell JobPilot about you
            </Link>
          </p>
        )}

        <div className="mt-4 space-y-4">
          {(questions ?? []).map((question) => (
            <label key={idOf(question)} className="block space-y-1.5">
              <span className="block text-[13px] font-medium">{question.question}</span>
              <Textarea
                rows={2}
                value={typed[idOf(question)] ?? question.answer}
                onChange={(e) => setTyped({ ...typed, [idOf(question)]: e.target.value })}
              />
            </label>
          ))}
          {prepare.isPending && <Thinking state="working">Reading the posting for a question or two…</Thinking>}
        </div>

        <DialogFooter>
          <DialogClose render={<Button variant="ghost" />}>Cancel</DialogClose>
          <Button variant="outline" onClick={() => void write(false)} disabled={saveAnswers.isPending}>
            Skip and write
          </Button>
          <Button onClick={() => void write(true)} disabled={saveAnswers.isPending}>
            {saveAnswers.isPending ? "Saving…" : "Write letter"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
