import { type ReactNode, useState } from "react";
import { Link } from "react-router-dom";
import { Page, Panel, Section } from "../../components/page";
import { ErrorLine, Loading } from "../../components/status-line";
import { Button } from "../../components/ui/button";
import { Textarea } from "../../components/ui/textarea";
import { useAboutYou, useDeleteKeptAnswer, useSaveAboutYou } from "../../lib/queries/about-you.queries";
import { useToast } from "../../lib/toast";
import type { AboutYou, AboutYouUpdate, PersonalAnswerKey } from "../../lib/types";

/// Draft is the editable form over what is saved: each answer by key.
interface Draft {
  answers: Record<PersonalAnswerKey, string>;
}

function draftFrom(about: AboutYou): Draft {
  return { answers: Object.fromEntries(about.questions.map((question) => [question.key, question.answer])) };
}

/// changes is only what differs from what is saved, so saving one answer
/// never rewrites the others.
function changes(about: AboutYou, draft: Draft): AboutYouUpdate {
  const answers: Record<PersonalAnswerKey, string> = {};
  for (const question of about.questions) {
    if ((draft.answers[question.key] ?? "") !== question.answer)
      answers[question.key] = draft.answers[question.key] ?? "";
  }
  return { answers };
}

/// AboutYouPage is where the candidate notes, in their own words, what they
/// will want to say when they apply (docs/02_features/16_about_you.md).
/// JobPilot writes nothing from it (ADR-0034 §5): it is their material, ready
/// to copy on an application. Every question is optional, and the wording
/// of each is the daemon's.
export function AboutYouPage({ tabs }: { tabs: ReactNode }) {
  const { notify, notifyError } = useToast();
  const { data: about, isPending, error } = useAboutYou();
  const [draft, setDraft] = useState<Draft | null>(null);
  const onError = (err: Error) => notifyError(err);

  const save = useSaveAboutYou({
    onSuccess: () => {
      setDraft(null);
      notify("Saved.", "success");
    },
    onError,
  });
  const forget = useDeleteKeptAnswer({ onError });

  if (isPending) return <Loading />;
  if (error) return <ErrorLine error={error} />;

  if (!about) {
    return (
      <Page title="Profile">
        {tabs}
        <Panel className="px-4 py-6 text-center">
          <p className="text-sm text-muted-foreground">
            Create your profile under Details first. What you say here belongs to it.
          </p>
        </Panel>
      </Page>
    );
  }

  const current = draft ?? draftFrom(about);
  const dirty = draft !== null && Object.keys(changes(about, draft).answers).length > 0;
  const setAnswer = (key: PersonalAnswerKey, answer: string) =>
    setDraft({ ...current, answers: { ...current.answers, [key]: answer } });

  const saveButton = (
    <Button size="sm" onClick={() => draft && save.mutate(changes(about, draft))} disabled={!dirty || save.isPending}>
      {save.isPending ? "Saving…" : "Save"}
    </Button>
  );

  return (
    <Page
      title="Profile"
      actions={
        <>
          {dirty && (
            <Button size="sm" variant="ghost" onClick={() => setDraft(null)}>
              Revert
            </Button>
          )}
          {saveButton}
        </>
      }
    >
      {tabs}

      <p className="text-sm text-muted-foreground">
        Your own answers to the questions applications tend to ask, ready to copy when you write a letter or answer a
        form. JobPilot never writes them for you. Every question is optional, and it all stays on this machine.
      </p>

      {about.questions.map((question) => (
        <Section key={question.key} title={question.question} description={question.hint}>
          <Textarea
            aria-label={question.question}
            rows={3}
            value={current.answers[question.key] ?? ""}
            onChange={(e) => setAnswer(question.key, e.target.value)}
          />
        </Section>
      ))}

      <Section
        title="Kept for similar questions"
        description="Answers you wrote on applications and chose to keep. A similar question on another application suggests them."
      >
        {about.kept.length === 0 ? (
          <Panel className="px-4 py-3">
            <p className="text-sm text-muted-foreground">
              None yet. When you answer a question on an application, you can keep the answer from there, under{" "}
              <Link to="/applications" className="text-foreground underline">
                Applications
              </Link>
              .
            </p>
          </Panel>
        ) : (
          <Panel className="divide-y divide-border">
            {about.kept.map((kept) => (
              <div key={kept.id} className="flex items-start gap-3 px-4 py-3">
                <div className="min-w-0 flex-1">
                  <p className="text-[13px] font-medium">{kept.question}</p>
                  <p className="mt-1 line-clamp-3 text-[13px] whitespace-pre-wrap text-subtle-foreground select-text">
                    {kept.answer}
                  </p>
                </div>
                <Button
                  size="xs"
                  variant="ghost"
                  onClick={() => forget.mutate(kept.id)}
                  disabled={forget.isPending && forget.variables === kept.id}
                >
                  Forget
                </Button>
              </div>
            ))}
          </Panel>
        )}
      </Section>

      <div className="mt-8 flex justify-end border-t border-border pt-5">{saveButton}</div>
    </Page>
  );
}
