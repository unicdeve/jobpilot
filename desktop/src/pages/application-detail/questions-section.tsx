import { BookmarkPlus, Plus, Sparkles } from "lucide-react";
import { useState } from "react";
import { DraftFlags, FlaggedText } from "../../components/draft-flags";
import { DetailSection } from "../../components/page";
import { StatusBadge } from "../../components/status-badge";
import { Button } from "../../components/ui/button";
import { Checkbox } from "../../components/ui/checkbox";
import { Input } from "../../components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../../components/ui/select";
import { Textarea } from "../../components/ui/textarea";
import { useKeepAnswer } from "../../lib/queries/about-you.queries";
import { useAddQuestion, useResolveQuestion, useSetQuestionAnswer } from "../../lib/queries/question.queries";
import { useToast } from "../../lib/toast";
import type { ApplicationQuestion, QuestionType } from "../../lib/types";

// Base UI's Select shows the raw value in its trigger unless given the
// value -> label mapping.
const QUESTION_TYPE_LABELS: Record<QuestionType, string> = {
  factual: "Short answer",
  narrative: "Written answer",
};

/// QuestionsSection lists what the employer asks and where each answer came
/// from. Answering by hand is opened per question, so a list of answered
/// questions is not also a wall of empty text boxes.
export function QuestionsSection({
  applicationId,
  questions,
}: {
  applicationId: string;
  questions: ApplicationQuestion[];
}) {
  const [adding, setAdding] = useState(false);
  const needsYou = questions.filter((q) => q.status !== "answered").length;

  return (
    <DetailSection
      title="Questions"
      description={
        questions.length === 0
          ? undefined
          : needsYou > 0
            ? `${needsYou} of ${questions.length} still need an answer.`
            : `All ${questions.length} answered.`
      }
      actions={
        !adding && (
          <Button size="sm" variant="outline" onClick={() => setAdding(true)}>
            <Plus data-icon="inline-start" />
            Add question
          </Button>
        )
      }
    >
      {adding && <AddQuestionForm applicationId={applicationId} onDone={() => setAdding(false)} />}
      {questions.length === 0 && !adding && (
        <p className="text-sm text-muted-foreground">
          No questions yet. Preparing in the browser reads them from the application form, or add one by hand.
        </p>
      )}
      {questions.length > 0 && (
        <ul className="divide-y divide-border overflow-hidden rounded-lg border border-border bg-card">
          {questions.map((q) => (
            <QuestionRow key={q.id} applicationId={applicationId} question={q} />
          ))}
        </ul>
      )}
    </DetailSection>
  );
}

function QuestionRow({ applicationId, question }: { applicationId: string; question: ApplicationQuestion }) {
  const { notify, notifyError } = useToast();
  const [answering, setAnswering] = useState(false);
  const [draft, setDraft] = useState("");
  const onError = (err: Error) => notifyError(err);

  const resolveQuestion = useResolveQuestion({ onError });
  const keep = useKeepAnswer({
    onSuccess: () => notify("Kept. Written answers to similar questions will draw on it.", "success"),
    onError,
  });
  const setQuestionAnswer = useSetQuestionAnswer({
    // Close the editor only once the answer is stored, so a failed save
    // leaves what the user typed in the box.
    onSuccess: () => {
      setAnswering(false);
      setDraft("");
    },
    onError,
  });

  const save = () => {
    if (draft.trim()) setQuestionAnswer.mutate({ questionId: question.id, applicationId, answer: draft.trim() });
  };

  return (
    <li className="px-4 py-3">
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <p className="text-[13px] font-medium">
            {question.label}
            {question.required && (
              <span className="ml-1 text-destructive" title="Required">
                *
              </span>
            )}
          </p>
          {question.answer ? (
            <p className="mt-1 text-[13px] whitespace-pre-wrap text-subtle-foreground select-text">
              <FlaggedText text={question.answer} flags={question.flags} />
            </p>
          ) : (
            <p className="mt-1 text-[13px] text-muted-foreground">No answer yet.</p>
          )}
        </div>
        <StatusBadge value={question.category || question.status} className="shrink-0" />
      </div>
      {!answering && <DraftFlags flags={question.flags} />}

      {answering ? (
        <div className="mt-2.5 space-y-2">
          {question.type === "narrative" ? (
            <Textarea autoFocus rows={4} value={draft} onChange={(e) => setDraft(e.target.value)} />
          ) : (
            <Input
              autoFocus
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && save()}
            />
          )}
          <div className="flex gap-2">
            <Button size="sm" onClick={save} disabled={!draft.trim() || setQuestionAnswer.isPending}>
              {setQuestionAnswer.isPending ? "Saving…" : "Save answer"}
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setAnswering(false)}>
              Cancel
            </Button>
          </div>
        </div>
      ) : (
        <div className="mt-2 -ml-2 flex gap-1">
          <Button
            size="xs"
            variant="ghost"
            onClick={() => resolveQuestion.mutate({ questionId: question.id, applicationId })}
            disabled={resolveQuestion.isPending}
          >
            <Sparkles data-icon="inline-start" />
            {resolveQuestion.isPending ? "Working it out…" : question.answer ? "Work it out again" : "Work it out"}
          </Button>
          <Button
            size="xs"
            variant="ghost"
            onClick={() => {
              setDraft(question.answer ?? "");
              setAnswering(true);
            }}
          >
            {question.answer ? "Edit answer" : "Answer myself"}
          </Button>
          {/* Only an answer the candidate wrote can be kept: it is their words
              that later answers draw on (docs/02_features/16_about_you.md D6). */}
          {question.type === "narrative" && question.user_edited && question.answer.trim() && (
            <Button size="xs" variant="ghost" onClick={() => keep.mutate(question.id)} disabled={keep.isPending}>
              <BookmarkPlus data-icon="inline-start" />
              {keep.isPending ? "Keeping…" : "Keep for similar questions"}
            </Button>
          )}
        </div>
      )}
    </li>
  );
}

function AddQuestionForm({ applicationId, onDone }: { applicationId: string; onDone: () => void }) {
  const { notifyError } = useToast();
  const [label, setLabel] = useState("");
  const [type, setType] = useState<QuestionType>("factual");
  const [required, setRequired] = useState(false);
  const addQuestion = useAddQuestion({ onSuccess: onDone, onError: notifyError });

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    if (label.trim()) addQuestion.mutate({ applicationId, label: label.trim(), type, required });
  };

  return (
    <form onSubmit={submit} className="mb-3 rounded-lg border border-border bg-card p-3">
      <Input
        autoFocus
        placeholder="The question, as the employer asks it"
        value={label}
        onChange={(e) => setLabel(e.target.value)}
      />
      <div className="mt-2 flex flex-wrap items-center gap-3">
        <Select items={QUESTION_TYPE_LABELS} value={type} onValueChange={(value) => setType(value as QuestionType)}>
          <SelectTrigger size="sm" aria-label="Kind of answer">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {Object.entries(QUESTION_TYPE_LABELS).map(([value, text]) => (
              <SelectItem key={value} value={value}>
                {text}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <label className="flex items-center gap-1.5 text-sm text-subtle-foreground">
          <Checkbox checked={required} onCheckedChange={(checked) => setRequired(checked === true)} />
          Required
        </label>
        <div className="ml-auto flex gap-2">
          <Button type="button" size="sm" variant="ghost" onClick={onDone}>
            Cancel
          </Button>
          <Button type="submit" size="sm" disabled={!label.trim() || addQuestion.isPending}>
            {addQuestion.isPending ? "Adding…" : "Add"}
          </Button>
        </div>
      </div>
    </form>
  );
}
