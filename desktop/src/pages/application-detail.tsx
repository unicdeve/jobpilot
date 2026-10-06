import {
  ArrowLeft,
  BookmarkPlus,
  ChevronRight,
  Ellipsis,
  Globe,
  LoaderCircle,
  Plus,
  Send,
  Sparkles,
} from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { DeletionDialog } from "../components/deletion-dialog";
import { DraftFlags, FlaggedText } from "../components/draft-flags";
import { RecordGone } from "../components/list-pane";
import { DetailSection, Toolbar } from "../components/page";
import { splitBackClass } from "../components/split-view";
import { StatusBadge, statusLabel } from "../components/status-badge";
import { ErrorLine, Loading } from "../components/status-line";
import { Button } from "../components/ui/button";
import { Checkbox } from "../components/ui/checkbox";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "../components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../components/ui/dropdown-menu";
import { Input } from "../components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../components/ui/select";
import { Textarea } from "../components/ui/textarea";
import { runDuration } from "../lib/activity";
import { isNotFound } from "../lib/api";
import { dateTime, relativeTime } from "../lib/format";
import {
  useAboutYou,
  useAddQuestion,
  useAgentRuns,
  useAllowedTransitions,
  useApplication,
  useApplicationHistory,
  useCoverLetters,
  useEditCoverLetter,
  useGenerateCoverLetter,
  useJob,
  useKeepAnswer,
  usePersonalQuestions,
  usePrepareInBrowser,
  usePreparePersonalQuestions,
  useQuestions,
  useResolveQuestion,
  useSavePersonalAnswers,
  useSetQuestionAnswer,
  useTransitionApplication,
} from "../lib/queries";
import { useToast } from "../lib/toast";
import type {
  AgentRunWithEvents,
  Application,
  ApplicationAnswer,
  ApplicationQuestion,
  ApplicationState,
  ApplicationStateTransition,
  CoverLetter,
  PersonalAnswer,
  QuestionType,
} from "../lib/types";

// Base UI's Select shows the raw value in its trigger unless given the
// value -> label mapping.
const QUESTION_TYPE_LABELS: Record<QuestionType, string> = {
  factual: "Short answer",
  narrative: "Written answer",
};

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

export function ApplicationDetail() {
  const { id } = useParams<{ id: string }>();
  const { search } = useLocation();

  // Each section's mutations invalidate exactly the queries they affect, so
  // there is no whole-screen reload after every action.
  const { data: application, error } = useApplication(id);
  const { data: history } = useApplicationHistory(id);
  const { data: coverLetters } = useCoverLetters(id);
  const { data: questions } = useQuestions(id);
  const { data: agentRuns } = useAgentRuns(id);
  const { data: job } = useJob(application?.job_id);

  const back = (
    <Button
      size="icon-sm"
      variant="ghost"
      className={splitBackClass}
      aria-label="Back to applications"
      render={<Link to={{ pathname: "/applications", search }} />}
    >
      <ArrowLeft />
    </Button>
  );

  if (isNotFound(error)) {
    return (
      <RecordGone
        back={back}
        icon={<Send />}
        title="This application no longer exists"
        description="It may have been deleted. Choose another application from the list."
      />
    );
  }
  if (error) return <ErrorLine error={error} />;
  if (!application || !id) return <Loading />;

  return (
    <div className="min-h-full">
      <Toolbar
        className="px-5"
        leading={back}
        actions={
          <>
            <Button size="sm" variant="ghost" render={<Link to={`/jobs/${application.job_id}`} />}>
              View job
              <ChevronRight data-icon="inline-end" />
            </Button>
            <ApplicationMenu applicationId={id} title={job?.title} />
          </>
        }
      />

      <article className="max-w-3xl px-8 pt-6 pb-12">
        <header>
          <div className="flex items-start gap-3">
            <h2 className="min-w-0 flex-1 text-xl leading-tight font-semibold select-text">
              {job ? job.title : "Application"}
            </h2>
            <StatusBadge value={application.state} className="mt-1 shrink-0" />
          </div>
          <p className="mt-1 text-sm text-subtle-foreground">{job?.company_name}</p>
          <p className="mt-3 text-xs text-muted-foreground">
            {application.mode === "assist" ? "Assist mode — JobPilot writes, you fill in the form" : "Review mode"}
            {" · "}started <span title={dateTime(application.created_at)}>{relativeTime(application.created_at)}</span>
            {application.submitted_at && (
              <>
                {" · "}submitted{" "}
                <span title={dateTime(application.submitted_at)}>{relativeTime(application.submitted_at)}</span>
              </>
            )}
          </p>
        </header>

        <NextStep application={application} />

        <QuestionsSection applicationId={id} questions={questions ?? []} />

        <CoverLetterSection applicationId={id} application={application} versions={coverLetters ?? []} />

        {/* Assist never touches an employer's form
            (docs/00_product/04_application_modes.md), so it has no browser
            preparation to offer. */}
        {application.mode === "review" && <BrowserSection applicationId={id} runs={agentRuns ?? []} />}

        {history && history.length > 0 && <HistorySection history={history} />}
      </article>
    </div>
  );
}

/// ApplicationMenu holds what is done to the application rather than with it.
function ApplicationMenu({ applicationId, title }: { applicationId: string; title?: string }) {
  const { notify } = useToast();
  const navigate = useNavigate();
  const { search } = useLocation();
  const [deleting, setDeleting] = useState(false);
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={<Button size="icon-sm" variant="ghost" aria-label="More actions" title="More actions" />}
        >
          <Ellipsis />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem variant="destructive" onClick={() => setDeleting(true)}>
            Delete application…
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <DeletionDialog
        target="application"
        id={applicationId}
        open={deleting}
        onOpenChange={setDeleting}
        title={title ? `Delete the application for “${title}”?` : "Delete this application?"}
        note="The job stays."
        confirmLabel="Delete application"
        onDeleted={() => {
          notify("Application deleted.", "success");
          navigate({ pathname: "/applications", search });
        }}
      />
    </>
  );
}

/// NextStep offers the transitions the daemon says are possible, under a
/// heading that changes with where the application is.
///
/// The allowed set is fetched rather than restated here, so the buttons and
/// the rule that enforces them cannot drift apart.
function NextStep({ application }: { application: Application }) {
  const { notify } = useToast();
  const [reason, setReason] = useState("");
  const { data: allowed, isPending } = useAllowedTransitions(application.id);

  const transitionApplication = useTransitionApplication({
    onSuccess: () => setReason(""),
    onError: (err) => notify(String(err)),
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

/// QuestionsSection lists what the employer asks and where each answer came
/// from. Answering by hand is opened per question, so a list of answered
/// questions is not also a wall of empty text boxes.
function QuestionsSection({ applicationId, questions }: { applicationId: string; questions: ApplicationQuestion[] }) {
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
  const { notify } = useToast();
  const [answering, setAnswering] = useState(false);
  const [draft, setDraft] = useState("");
  const onError = (err: Error) => notify(String(err));

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
  const { notify } = useToast();
  const [label, setLabel] = useState("");
  const [type, setType] = useState<QuestionType>("factual");
  const [required, setRequired] = useState(false);
  const addQuestion = useAddQuestion({ onSuccess: onDone, onError: (err) => notify(String(err)) });

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

function CoverLetterSection({
  applicationId,
  application,
  versions,
}: {
  applicationId: string;
  application: Application;
  versions: CoverLetter[];
}) {
  const { notify } = useToast();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [showEarlier, setShowEarlier] = useState(false);
  const [deletingVersion, setDeletingVersion] = useState<CoverLetter | null>(null);
  const [asking, setAsking] = useState(false);

  const generateCoverLetter = useGenerateCoverLetter({ onError: (err) => notify(String(err)) });
  const editCoverLetter = useEditCoverLetter({
    onSuccess: () => setEditing(false),
    onError: (err) => notify(String(err)),
  });

  const current = versions.find((v) => v.id === application.current_cover_letter_id) ?? versions[0];
  const earlier = versions.filter((v) => v !== current);

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
              >
                Edit
              </Button>
            )}
            <Button
              size="sm"
              variant="outline"
              onClick={() => setAsking(true)}
              disabled={generateCoverLetter.isPending}
            >
              <Sparkles data-icon="inline-start" />
              {generateCoverLetter.isPending ? "Writing…" : current ? "Rewrite" : "Write one"}
            </Button>
          </>
        )
      }
    >
      {!current ? (
        <p className="text-sm text-muted-foreground">
          No cover letter yet. JobPilot writes one from your profile, what you tell it about yourself, and this job.
        </p>
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
          <div className="rounded-lg border border-border bg-card px-5 py-4">
            <p className="text-[13px] leading-relaxed whitespace-pre-wrap text-subtle-foreground select-text">
              <FlaggedText text={current.content} flags={current.flags} />
            </p>
          </div>
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
  const { notify } = useToast();
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
      notify(String(err));
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
          {prepare.isPending && (
            <p className="flex items-center gap-2 text-[13px] text-muted-foreground">
              <LoaderCircle className="size-3.5 animate-spin" />
              Reading the posting for a question or two…
            </p>
          )}
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

function BrowserSection({ applicationId, runs }: { applicationId: string; runs: AgentRunWithEvents[] }) {
  const { notify } = useToast();
  const prepareInBrowser = usePrepareInBrowser({
    onSuccess: () => notify("Browser window opened — review it there before submitting.", "success"),
    // A blocked or failed run is expected, structured output (a CAPTCHA, an
    // unreadable form), not just a transport error; its reason is also
    // listed below once the runs refresh, which they do on failure too.
    onError: (err) => notify(String(err)),
  });

  return (
    <DetailSection
      title="Prepare in browser"
      description="Opens the application form in Chrome and fills in what JobPilot knows. The window stays open for you to check and submit — JobPilot never submits."
      actions={
        <Button
          size="sm"
          variant="outline"
          onClick={() => prepareInBrowser.mutate(applicationId)}
          disabled={prepareInBrowser.isPending}
        >
          <Globe data-icon="inline-start" />
          {prepareInBrowser.isPending ? "Preparing…" : runs.length > 0 ? "Prepare again" : "Prepare"}
        </Button>
      }
    >
      {runs.length > 0 && (
        <ul className="divide-y divide-border overflow-hidden rounded-lg border border-border bg-card">
          {runs.map(({ run, events }) => {
            const duration = runDuration(run);
            return (
              <li key={run.id}>
                <Link to={`/activity/${run.id}`} className="flex items-start gap-3 px-4 py-2.5 hover:bg-accent/60">
                  <div className="min-w-0 flex-1">
                    <p className="text-[13px]">
                      <span title={dateTime(run.started_at)}>{relativeTime(run.started_at)}</span>
                      <span className="text-muted-foreground">
                        {duration && <> · took {duration}</>} · {events.length} {events.length === 1 ? "step" : "steps"}
                      </span>
                    </p>
                    {run.error && <p className="mt-0.5 text-[13px] text-destructive">{run.error}</p>}
                  </div>
                  <StatusBadge value={run.status} className="shrink-0" />
                  <ChevronRight className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                </Link>
              </li>
            );
          })}
        </ul>
      )}
    </DetailSection>
  );
}

function HistorySection({ history }: { history: ApplicationStateTransition[] }) {
  return (
    <DetailSection title="History">
      <ol className="ml-1 space-y-3 border-l border-border pl-5">
        {[...history].reverse().map((h) => (
          <li key={h.id} className="relative">
            <span className="absolute top-1.5 -left-6 size-2 rounded-full bg-muted-foreground/50 ring-4 ring-background" />
            <p className="text-[13px]">
              {h.from_state ? `${statusLabel(h.from_state)} → ` : ""}
              <span className="font-medium">{statusLabel(h.to_state)}</span>
              <span className="text-muted-foreground"> · {relativeTime(h.created_at) ?? ""}</span>
            </p>
            {h.reason && <p className="mt-0.5 text-[13px] text-muted-foreground select-text">{h.reason}</p>}
          </li>
        ))}
      </ol>
    </DetailSection>
  );
}
