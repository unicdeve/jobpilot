import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { StatusBadge } from "../components/status-badge";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardTitle } from "../components/ui/card";
import { Checkbox } from "../components/ui/checkbox";
import { Input } from "../components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../components/ui/select";
import { Textarea } from "../components/ui/textarea";
import { runDuration } from "../lib/activity";
import {
  useAddQuestion,
  useAgentRuns,
  useApplication,
  useAllowedTransitions,
  useApplicationHistory,
  useCoverLetters,
  useEditCoverLetter,
  useJob,
  useGenerateCoverLetter,
  usePrepareInBrowser,
  useQuestions,
  useResolveQuestion,
  useSetQuestionAnswer,
  useTransitionApplication,
} from "../lib/queries";
import { useToast } from "../lib/toast";
import type {
  AgentRunWithEvents,
  Application,
  ApplicationQuestion,
  ApplicationState,
  CoverLetter,
  QuestionType,
} from "../lib/types";

// Base UI's Select shows the raw value in its trigger unless given the
// value -> label mapping.
const QUESTION_TYPE_LABELS: Record<QuestionType, string> = {
  factual: "Factual",
  narrative: "Narrative",
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
const OUTCOME_STATES: ApplicationState[] = [
  "SUBMITTED",
  "INTERVIEWING",
  "OFFERED",
  "NO_RESPONSE",
];

export function ApplicationDetail() {
  const { id } = useParams<{ id: string }>();

  // Each panel's mutations invalidate exactly the queries they affect, so
  // there is no whole-screen reload after every action.
  const { data: application, error } = useApplication(id);
  const { data: history } = useApplicationHistory(id);
  const { data: coverLetters } = useCoverLetters(id);
  const { data: questions } = useQuestions(id);
  const { data: agentRuns } = useAgentRuns(id);
  const { data: job } = useJob(application?.job_id);

  if (error) return <p className="text-sm text-red-600">{String(error.message)}</p>;
  if (!application || !id) return <p className="text-sm text-slate-500">Loading…</p>;

  return (
    <div className="max-w-3xl space-y-4">
      <div>
        <Link to="/applications" className="text-sm text-slate-500 hover:underline">
          ← Applications
        </Link>
        <div className="mt-1 flex items-center gap-3">
          <h1 className="text-xl font-semibold">{job ? job.title : "Application"}</h1>
          <StatusBadge value={application.state} />
        </div>
        {job && (
          <p className="text-sm text-slate-600">
            {job.company_name} ·{" "}
            <Link to={`/jobs/${application.job_id}`} className="hover:underline">
              View job
            </Link>
          </p>
        )}
        {application.submitted_at && (
          <p className="mt-1 text-sm text-slate-500">
            Submitted {new Date(application.submitted_at).toLocaleDateString(undefined, {
              year: "numeric",
              month: "long",
              day: "numeric",
            })}
          </p>
        )}
      </div>

      <NextStepPanel application={application} />

      {history && history.length > 0 && (
        <Card>
          <CardContent>
            <CardTitle>History</CardTitle>
            <ul className="mt-2 space-y-1 text-sm text-slate-600">
              {history.map((h) => (
                <li key={h.id}>
                  {h.from_state || "(none)"} → {h.to_state}
                  {h.reason && <span className="text-slate-400"> — {h.reason}</span>}
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      )}

      <BrowserAutomationPanel applicationId={id} runs={agentRuns ?? []} />

      <CoverLetterPanel
        applicationId={id}
        application={application}
        versions={coverLetters ?? []}
      />

      <QuestionsPanel applicationId={id} questions={questions ?? []} />
    </div>
  );
}

/// NextStepPanel offers the transitions the daemon says are possible, under
/// a heading that changes with where the application is.
///
/// It used to render all nine states as buttons labelled with the raw enum
/// and let the server reject the seven that were invalid, which taught the
/// user the state machine one error toast at a time. The allowed set is
/// fetched rather than restated here, so the buttons and the rule that
/// enforces them cannot drift apart.
function NextStepPanel({ application }: { application: Application }) {
  const { notify } = useToast();
  const [reason, setReason] = useState("");
  const { data: allowed, isPending } = useAllowedTransitions(application.id);

  const transitionApplication = useTransitionApplication({
    onSuccess: () => setReason(""),
    onError: (err) => notify(String(err)),
  });
  const pending = transitionApplication.isPending
    ? transitionApplication.variables?.toState
    : null;

  const transition = (toState: ApplicationState) =>
    transitionApplication.mutate({ id: application.id, toState, reason });

  // The whole point of the handoff: JobPilot filled the form and left the
  // browser open, and only the user knows whether they pressed send.
  const awaitingSubmission = application.state === "APPROVED";
  const afterSubmission = OUTCOME_STATES.includes(application.state);

  const button = (state: ApplicationState, variant: "default" | "secondary" = "secondary") => (
    <Button
      key={state}
      variant={variant}
      disabled={transitionApplication.isPending}
      onClick={() => transition(state)}
    >
      {pending === state ? "…" : STATE_ACTIONS[state]}
    </Button>
  );

  if (isPending) return null;

  if (!allowed || allowed.length === 0) {
    return (
      <Card>
        <CardContent>
          <CardTitle>Closed</CardTitle>
          <p className="mt-1 text-sm text-slate-500">
            This application is finished — there is nothing left to record.
          </p>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className={awaitingSubmission ? "ring-amber-300" : undefined}>
      <CardContent>
        <CardTitle>
          {awaitingSubmission ? "Did you send it?" : afterSubmission ? "What happened?" : "Next step"}
        </CardTitle>
        <p className="mt-1 text-sm text-slate-500">
          {awaitingSubmission
            ? "JobPilot never submits for you, so it cannot tell. Record it here and this stops waiting on you."
            : afterSubmission
              ? "Tell JobPilot what the employer said, so a sent application does not go quiet in your list."
              : "Where this application goes next."}
        </p>

        <div className="mt-3 flex flex-wrap gap-2">
          {awaitingSubmission && allowed.includes("SUBMITTED") && button("SUBMITTED", "default")}
          {allowed.filter((state) => !(awaitingSubmission && state === "SUBMITTED")).map((state) => button(state))}
        </div>

        <div className="mt-3">
          <Input
            placeholder="Reason (optional)"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
          />
        </div>
      </CardContent>
    </Card>
  );
}

function BrowserAutomationPanel({
  applicationId,
  runs,
}: {
  applicationId: string;
  runs: AgentRunWithEvents[];
}) {
  const { notify } = useToast();

  const prepareInBrowser = usePrepareInBrowser({
    onSuccess: () => notify("Browser window opened — review it there before submitting.", "success"),
    // A blocked/failed run is expected, structured output (a CAPTCHA, an
    // unreadable form, ...), not just a transport error — the reason is
    // also visible below once the run list refreshes, which it does on
    // failure too.
    onError: (err) => notify(String(err)),
  });

  const prepare = () => prepareInBrowser.mutate(applicationId);

  return (
    <Card>
      <CardContent>
        <CardTitle>Browser automation</CardTitle>
        <p className="mt-1 text-sm text-slate-500">
          Opens the job posting in a real Chrome window, fills in known answers, and leaves it open for you to
          review and submit yourself — it never submits on its own.
        </p>
        <div className="mt-2">
          <Button onClick={prepare} disabled={prepareInBrowser.isPending}>
            {prepareInBrowser.isPending ? "Preparing…" : "Prepare in browser"}
          </Button>
        </div>

        {runs.length > 0 && (
          <ul className="mt-3 divide-y divide-slate-100 rounded-md border border-slate-200">
            {runs.map(({ run, events }) => {
              const duration = runDuration(run);
              return (
                <li key={run.id} className="px-3 py-2.5">
                  <div className="flex items-center justify-between gap-3">
                    <span className="text-sm">
                      {new Date(run.started_at).toLocaleString()}
                      <span className="text-slate-500">
                        {duration && <> · took {duration}</>} · {events.length}{" "}
                        {events.length === 1 ? "step" : "steps"} ·{" "}
                        <Link to={`/activity/${run.id}`} className="underline">
                          What it did
                        </Link>
                      </span>
                    </span>
                    <StatusBadge value={run.status} />
                  </div>
                  {run.error && <p className="mt-1 text-sm text-red-700">{run.error}</p>}
                </li>
              );
            })}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

function CoverLetterPanel({
  applicationId,
  application,
  versions,
}: {
  applicationId: string;
  application: Application;
  versions: CoverLetter[];
}) {
  const { notify } = useToast();
  const [editingId, setEditingId] = useState<string | null>(null);
  const [draft, setDraft] = useState("");

  const generateCoverLetter = useGenerateCoverLetter({
    onError: (err) => notify(String(err)),
  });
  const editCoverLetter = useEditCoverLetter({
    onSuccess: () => setEditingId(null),
    onError: (err) => notify(String(err)),
  });

  const generate = () => generateCoverLetter.mutate(applicationId);

  const saveEdit = (coverLetterId: string) =>
    editCoverLetter.mutate({ coverLetterId, applicationId, content: draft });

  return (
    <Card>
      <CardContent>
        <CardTitle>Cover letter</CardTitle>
        <div className="mt-2">
          <Button onClick={generate} disabled={generateCoverLetter.isPending}>
            {generateCoverLetter.isPending
              ? "Generating…"
              : versions.length > 0
                ? "Regenerate"
                : "Generate"}
          </Button>
        </div>
        {versions.length === 0 ? (
          <p className="mt-2 text-sm text-slate-500">No cover letter generated yet.</p>
        ) : (
          <div className="mt-3 space-y-3">
            {versions.map((v) => (
              <div key={v.id} className="rounded-md border border-slate-200 p-3">
                <div className="mb-1 flex items-center justify-between text-xs text-slate-500">
                  <span>
                    {v.id === application.current_cover_letter_id ? "Current version" : "Earlier version"}
                    {v.user_edited && " · edited"}
                  </span>
                  {editingId !== v.id && (
                    <Button
                      variant="ghost"
                      onClick={() => {
                        setEditingId(v.id);
                        setDraft(v.content);
                      }}
                    >
                      Edit
                    </Button>
                  )}
                </div>
                {editingId === v.id ? (
                  <div className="space-y-2">
                    <Textarea rows={8} value={draft} onChange={(e) => setDraft(e.target.value)} />
                    <div className="flex gap-2">
                      <Button onClick={() => saveEdit(v.id)} disabled={editCoverLetter.isPending}>
                        {editCoverLetter.isPending ? "Saving…" : "Save"}
                      </Button>
                      <Button variant="secondary" onClick={() => setEditingId(null)}>
                        Cancel
                      </Button>
                    </div>
                  </div>
                ) : (
                  <p className="whitespace-pre-wrap text-sm text-slate-700">{v.content}</p>
                )}
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function QuestionsPanel({
  applicationId,
  questions,
}: {
  applicationId: string;
  questions: ApplicationQuestion[];
}) {
  const { notify } = useToast();
  const [label, setLabel] = useState("");
  const [type, setType] = useState<QuestionType>("factual");
  const [required, setRequired] = useState(false);
  const [answerDrafts, setAnswerDrafts] = useState<Record<string, string>>({});

  const onError = (err: Error) => notify(String(err));

  const addQuestionMutation = useAddQuestion({
    onSuccess: () => {
      setLabel("");
      setRequired(false);
    },
    onError,
  });
  const resolveQuestion = useResolveQuestion({ onError });
  const setQuestionAnswer = useSetQuestionAnswer({
    // Clear the draft only once the answer is stored, so a failed save
    // leaves what the user typed in the box.
    onSuccess: (_answer, { questionId }) =>
      setAnswerDrafts((drafts) => ({ ...drafts, [questionId]: "" })),
    onError,
  });

  const addQuestion = () => {
    if (!label.trim()) return;
    addQuestionMutation.mutate({ applicationId, label: label.trim(), type, required });
  };

  const resolve = (questionId: string) => resolveQuestion.mutate({ questionId, applicationId });

  const submitAnswer = (questionId: string) => {
    const answer = answerDrafts[questionId];
    if (!answer) return;
    setQuestionAnswer.mutate({ questionId, applicationId, answer });
  };

  return (
    <Card>
      <CardContent>
        <CardTitle>Questions</CardTitle>

        <div className="mt-2 flex flex-wrap items-end gap-2">
          <div className="min-w-48 flex-1">
            <Input placeholder="Question label…" value={label} onChange={(e) => setLabel(e.target.value)} />
          </div>
          <Select
            items={QUESTION_TYPE_LABELS}
            value={type}
            onValueChange={(value) => setType(value as QuestionType)}
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {Object.entries(QUESTION_TYPE_LABELS).map(([value, label]) => (
                <SelectItem key={value} value={value}>
                  {label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <label className="flex items-center gap-1.5 text-sm text-slate-600">
            <Checkbox checked={required} onCheckedChange={(checked) => setRequired(checked === true)} />
            Required
          </label>
          <Button onClick={addQuestion} disabled={addQuestionMutation.isPending}>
            {addQuestionMutation.isPending ? "Adding…" : "Add"}
          </Button>
        </div>

        {questions.length === 0 ? (
          <p className="mt-2 text-sm text-slate-500">No questions added yet.</p>
        ) : (
          <div className="mt-3 space-y-3">
            {questions.map((q) => (
              <div key={q.id} className="rounded-md border border-slate-200 p-3">
                <div className="flex items-center justify-between">
                  <p className="font-medium">{q.label}</p>
                  <div className="flex items-center gap-2">
                    {q.category && <StatusBadge value={q.category} />}
                    <StatusBadge value={q.status} />
                  </div>
                </div>
                {q.answer && <p className="mt-1 text-sm text-slate-700">{q.answer}</p>}
                <div className="mt-2 flex items-center gap-2">
                  <Button variant="secondary" onClick={() => resolve(q.id)}>
                    Resolve
                  </Button>
                  <Input
                    placeholder="Manual answer…"
                    className="max-w-64"
                    value={answerDrafts[q.id] ?? ""}
                    onChange={(e) => setAnswerDrafts((d) => ({ ...d, [q.id]: e.target.value }))}
                    onKeyDown={(e) => e.key === "Enter" && submitAnswer(q.id)}
                  />
                  <Button variant="secondary" onClick={() => submitAnswer(q.id)}>
                    Save answer
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
