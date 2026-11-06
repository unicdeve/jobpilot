import { BookmarkPlus, Plus } from "lucide-react";
import { useState } from "react";
import { CopyButton } from "../../components/copy-button";
import { DraftFlags, FlaggedText } from "../../components/draft-flags";
import { DetailSection } from "../../components/page";
import { Button } from "../../components/ui/button";
import { Input } from "../../components/ui/input";
import { Textarea } from "../../components/ui/textarea";
import { useKeepAnswer } from "../../lib/queries/about-you.queries";
import { useAddQuestion, useDeleteQuestion, useSetQuestionAnswer } from "../../lib/queries/question.queries";
import { useToast } from "../../lib/toast";
import type { AnswerSuggestion, ApplicationQuestion } from "../../lib/types";

/// QuestionsSection is the candidate's notes on what the form asks, and
/// their own answers (ADR-0034 §4). JobPilot suggests an answer only from
/// the profile or an answer they kept, and never fills one in.
export function QuestionsSection({
  applicationId,
  questions,
}: {
  applicationId: string;
  questions: ApplicationQuestion[];
}) {
  const [adding, setAdding] = useState(false);
  const unanswered = questions.filter((q) => !q.answer.trim()).length;

  return (
    <DetailSection
      title="Questions"
      description={
        questions.length === 0
          ? undefined
          : unanswered > 0
            ? `${unanswered} of ${questions.length} still need your answer.`
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
          Note the questions the application form asks, and write your answers here, beside your letter.
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

/// suggestionSource says where a suggested answer came from.
function suggestionSource(suggestion: AnswerSuggestion): string {
  switch (suggestion.source) {
    case "profile":
      return "From your profile";
    case "derived":
      return "Worked out from your profile";
    case "kept_answer":
      return `Your answer to “${suggestion.field}”`;
  }
}

function QuestionRow({ applicationId, question }: { applicationId: string; question: ApplicationQuestion }) {
  const { notify, notifyError } = useToast();
  const [answering, setAnswering] = useState(false);
  const [draft, setDraft] = useState("");
  const onError = (err: Error) => notifyError(err);

  const keep = useKeepAnswer({
    onSuccess: () => notify("Kept. A similar question on another application will suggest it.", "success"),
    onError,
  });
  const removeQuestion = useDeleteQuestion({ onError });
  const setQuestionAnswer = useSetQuestionAnswer({
    // Close the editor only once the answer is stored, so a failed save
    // leaves what the user typed in the box.
    onSuccess: () => {
      setAnswering(false);
      setDraft("");
    },
    onError,
  });

  const answer = (text: string) => setQuestionAnswer.mutate({ questionId: question.id, applicationId, answer: text });
  const startAnswering = (text: string) => {
    setDraft(text);
    setAnswering(true);
  };
  // A suggestion the answer already says is not worth offering again.
  const suggestion =
    question.suggestion && question.suggestion.answer !== question.answer ? question.suggestion : undefined;

  return (
    <li className="px-4 py-3">
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
          <p className="text-[13px] font-medium select-text">{question.label}</p>
          {question.answer && !answering && (
            <p className="mt-1 text-[13px] whitespace-pre-wrap text-subtle-foreground select-text">
              <FlaggedText text={question.answer} flags={question.flags} />
            </p>
          )}
          {!question.answer && !answering && !suggestion && (
            <p className="mt-1 text-[13px] text-muted-foreground">Not answered yet.</p>
          )}
        </div>
        {question.answer && !answering && <CopyButton text={question.answer} label="answer" />}
      </div>
      {!answering && <DraftFlags flags={question.flags} />}

      {suggestion && !answering && (
        <div className="mt-2.5 rounded-md border border-border bg-muted/50 px-3 py-2">
          <p className="text-xs text-muted-foreground">{suggestionSource(suggestion)}</p>
          <p className="mt-0.5 line-clamp-3 text-[13px] whitespace-pre-wrap select-text">{suggestion.answer}</p>
          <div className="mt-1.5 -ml-2 flex gap-1">
            <Button
              size="xs"
              variant="ghost"
              onClick={() => answer(suggestion.answer)}
              disabled={setQuestionAnswer.isPending}
            >
              Use this
            </Button>
            <Button size="xs" variant="ghost" onClick={() => startAnswering(suggestion.answer)}>
              Edit first
            </Button>
          </div>
        </div>
      )}

      {answering ? (
        <div className="mt-2.5 space-y-2">
          <Textarea
            autoFocus
            rows={4}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            aria-label={`Your answer to ${question.label}`}
          />
          <div className="flex gap-2">
            <Button size="sm" onClick={() => answer(draft)} disabled={!draft.trim() || setQuestionAnswer.isPending}>
              {setQuestionAnswer.isPending ? "Saving…" : "Save answer"}
            </Button>
            <Button size="sm" variant="ghost" onClick={() => setAnswering(false)}>
              Cancel
            </Button>
          </div>
        </div>
      ) : (
        <div className="mt-2 -ml-2 flex flex-wrap gap-1">
          <Button size="xs" variant="ghost" onClick={() => startAnswering(question.answer)}>
            {question.answer ? "Edit answer" : "Write an answer"}
          </Button>
          {question.answer.trim() && (
            <Button size="xs" variant="ghost" onClick={() => keep.mutate(question.id)} disabled={keep.isPending}>
              <BookmarkPlus data-icon="inline-start" />
              {keep.isPending ? "Keeping…" : "Keep for similar questions"}
            </Button>
          )}
          <Button
            size="xs"
            variant="ghost"
            className="text-muted-foreground"
            onClick={() => removeQuestion.mutate({ questionId: question.id, applicationId })}
            disabled={removeQuestion.isPending}
          >
            Remove
          </Button>
        </div>
      )}
    </li>
  );
}

function AddQuestionForm({ applicationId, onDone }: { applicationId: string; onDone: () => void }) {
  const { notifyError } = useToast();
  const [label, setLabel] = useState("");
  const addQuestion = useAddQuestion({ onSuccess: onDone, onError: notifyError });

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    if (label.trim()) addQuestion.mutate({ applicationId, label: label.trim() });
  };

  return (
    <form onSubmit={submit} className="mb-3 flex gap-2 rounded-lg border border-border bg-card p-3">
      <Input
        autoFocus
        placeholder="The question, as the form asks it"
        value={label}
        onChange={(e) => setLabel(e.target.value)}
        aria-label="Question"
      />
      <Button type="button" size="sm" variant="ghost" className="shrink-0" onClick={onDone}>
        Cancel
      </Button>
      <Button type="submit" size="sm" className="shrink-0" disabled={!label.trim() || addQuestion.isPending}>
        {addQuestion.isPending ? "Adding…" : "Add"}
      </Button>
    </form>
  );
}
