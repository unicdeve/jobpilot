import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
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
import { useApi } from "../lib/daemon";
import { useToast } from "../lib/toast";
import type {
  Application,
  ApplicationQuestion,
  ApplicationState,
  ApplicationStateTransition,
  CoverLetter,
  QuestionType,
} from "../lib/types";

const ALL_STATES: ApplicationState[] = [
  "DISCOVERED",
  "MATCHED",
  "PREPARING",
  "READY_FOR_REVIEW",
  "APPROVED",
  "SUBMITTED",
  "REJECTED",
  "WITHDRAWN",
  "FAILED",
];

export function ApplicationDetail() {
  const { id } = useParams<{ id: string }>();
  const api = useApi();
  const { notify } = useToast();

  const [application, setApplication] = useState<Application | null>(null);
  const [history, setHistory] = useState<ApplicationStateTransition[]>([]);
  const [coverLetters, setCoverLetters] = useState<CoverLetter[]>([]);
  const [questions, setQuestions] = useState<ApplicationQuestion[]>([]);

  const load = async () => {
    if (!id) return;
    const [app, hist, letters, qs] = await Promise.all([
      api.getApplication(id),
      api.listApplicationHistory(id),
      api.listCoverLetterVersions(id),
      api.listQuestions(id),
    ]);
    setApplication(app);
    setHistory(hist);
    setCoverLetters(letters);
    setQuestions(qs);
  };

  useEffect(() => {
    load().catch((err) => notify(String(err)));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, api]);

  if (!application || !id) return <p className="text-sm text-slate-500">Loading…</p>;

  return (
    <div className="max-w-3xl space-y-4">
      <div className="flex items-center gap-3">
        <h1 className="text-xl font-semibold">Application</h1>
        <StatusBadge value={application.state} />
      </div>

      <TransitionPanel
        application={application}
        onTransitioned={(updated) => {
          setApplication(updated);
          load().catch((err) => notify(String(err)));
        }}
      />

      {history.length > 0 && (
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

      <CoverLetterPanel
        applicationId={id}
        application={application}
        versions={coverLetters}
        onChanged={() => load().catch((err) => notify(String(err)))}
      />

      <QuestionsPanel
        applicationId={id}
        questions={questions}
        onChanged={() => load().catch((err) => notify(String(err)))}
      />
    </div>
  );
}

function TransitionPanel({
  application,
  onTransitioned,
}: {
  application: Application;
  onTransitioned: (updated: Application) => void;
}) {
  const api = useApi();
  const { notify } = useToast();
  const [reason, setReason] = useState("");
  const [pending, setPending] = useState<ApplicationState | null>(null);

  const transition = async (toState: ApplicationState) => {
    setPending(toState);
    try {
      const updated = await api.transitionApplication(application.id, toState, reason);
      setReason("");
      onTransitioned(updated);
    } catch (err) {
      // The server's allowedTransitions map is the single source of truth
      // for which transitions are valid; the UI never duplicates that rule
      // table, it just surfaces the rejection here.
      notify(String(err));
    } finally {
      setPending(null);
    }
  };

  return (
    <Card>
      <CardContent>
        <CardTitle>Transition state</CardTitle>
        <div className="mt-2 flex flex-wrap gap-2">
          {ALL_STATES.filter((s) => s !== application.state).map((s) => (
            <Button key={s} variant="secondary" disabled={pending !== null} onClick={() => transition(s)}>
              {pending === s ? "…" : s}
            </Button>
          ))}
        </div>
        <div className="mt-2">
          <Input placeholder="Reason (optional)" value={reason} onChange={(e) => setReason(e.target.value)} />
        </div>
      </CardContent>
    </Card>
  );
}

function CoverLetterPanel({
  applicationId,
  application,
  versions,
  onChanged,
}: {
  applicationId: string;
  application: Application;
  versions: CoverLetter[];
  onChanged: () => void;
}) {
  const api = useApi();
  const { notify } = useToast();
  const [generating, setGenerating] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [draft, setDraft] = useState("");

  const generate = async () => {
    setGenerating(true);
    try {
      await api.generateCoverLetter(applicationId);
      onChanged();
    } catch (err) {
      notify(String(err));
    } finally {
      setGenerating(false);
    }
  };

  const saveEdit = async (id: string) => {
    try {
      await api.editCoverLetter(id, draft);
      setEditingId(null);
      onChanged();
    } catch (err) {
      notify(String(err));
    }
  };

  return (
    <Card>
      <CardContent>
        <CardTitle>Cover letter</CardTitle>
        <div className="mt-2">
          <Button onClick={generate} disabled={generating}>
            {generating ? "Generating…" : versions.length > 0 ? "Regenerate" : "Generate"}
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
                      <Button onClick={() => saveEdit(v.id)}>Save</Button>
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
  onChanged,
}: {
  applicationId: string;
  questions: ApplicationQuestion[];
  onChanged: () => void;
}) {
  const api = useApi();
  const { notify } = useToast();
  const [label, setLabel] = useState("");
  const [type, setType] = useState<QuestionType>("factual");
  const [required, setRequired] = useState(false);
  const [answerDrafts, setAnswerDrafts] = useState<Record<string, string>>({});

  const addQuestion = async () => {
    if (!label.trim()) return;
    try {
      await api.addQuestion(applicationId, label.trim(), type, required);
      setLabel("");
      setRequired(false);
      onChanged();
    } catch (err) {
      notify(String(err));
    }
  };

  const resolve = async (questionId: string) => {
    try {
      await api.resolveQuestion(questionId);
      onChanged();
    } catch (err) {
      notify(String(err));
    }
  };

  const submitAnswer = async (questionId: string) => {
    const answer = answerDrafts[questionId];
    if (!answer) return;
    try {
      await api.setQuestionAnswer(questionId, answer);
      onChanged();
    } catch (err) {
      notify(String(err));
    }
  };

  return (
    <Card>
      <CardContent>
        <CardTitle>Questions</CardTitle>

        <div className="mt-2 flex flex-wrap items-end gap-2">
          <div className="min-w-48 flex-1">
            <Input placeholder="Question label…" value={label} onChange={(e) => setLabel(e.target.value)} />
          </div>
          <Select value={type} onValueChange={(value) => setType(value as QuestionType)}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="factual">Factual</SelectItem>
              <SelectItem value="narrative">Narrative</SelectItem>
            </SelectContent>
          </Select>
          <label className="flex items-center gap-1.5 text-sm text-slate-600">
            <Checkbox checked={required} onCheckedChange={(checked) => setRequired(checked === true)} />
            Required
          </label>
          <Button onClick={addQuestion}>Add</Button>
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
