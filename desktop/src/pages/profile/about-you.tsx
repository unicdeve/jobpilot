import { type ReactNode, useState } from "react";
import { Link } from "react-router-dom";
import { Page, Panel, Section } from "../../components/page";
import { ErrorLine, Loading } from "../../components/status-line";
import { Button } from "../../components/ui/button";
import { Textarea } from "../../components/ui/textarea";
import { useAboutYou, useDeleteKeptAnswer, useSaveAboutYou } from "../../lib/queries/about-you.queries";
import { useToast } from "../../lib/toast";
import type { AboutYou, AboutYouUpdate, PersonalAnswerKey } from "../../lib/types";

/// Draft is the editable form over what is saved: each answer by key, and
/// the writing sample and preferences.
interface Draft {
  answers: Record<PersonalAnswerKey, string>;
  writing_sample: string;
  writing_preferences: string;
}

function draftFrom(about: AboutYou): Draft {
  return {
    answers: Object.fromEntries(about.questions.map((question) => [question.key, question.answer])),
    writing_sample: about.writing_sample,
    writing_preferences: about.writing_preferences,
  };
}

/// changes is only what differs from what is saved, so saving one answer
/// never rewrites the others.
function changes(about: AboutYou, draft: Draft): AboutYouUpdate {
  const update: AboutYouUpdate = {};
  const answers: Record<PersonalAnswerKey, string> = {};
  for (const question of about.questions) {
    if ((draft.answers[question.key] ?? "") !== question.answer)
      answers[question.key] = draft.answers[question.key] ?? "";
  }
  if (Object.keys(answers).length > 0) update.answers = answers;
  if (draft.writing_sample !== about.writing_sample) update.writing_sample = draft.writing_sample;
  if (draft.writing_preferences !== about.writing_preferences) update.writing_preferences = draft.writing_preferences;
  return update;
}

/// AboutYouPage is where the candidate says who they are, for letters and
/// written answers to be built from (docs/02_features/16_about_you.md).
/// Every question is optional, and the wording of each is the daemon's.
export function AboutYouPage({ tabs }: { tabs: ReactNode }) {
  const { notify, notifyError } = useToast();
  const { data: about, isPending, error } = useAboutYou();
  const [draft, setDraft] = useState<Draft | null>(null);
  const onError = (err: Error) => notifyError(err);

  const save = useSaveAboutYou({
    onSuccess: () => {
      setDraft(null);
      notify("Saved. Letters and answers from now on use it.", "success");
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
  const dirty = draft !== null && Object.keys(changes(about, draft)).length > 0;
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
        Cover letters and written answers are built from what you say here, in your words, and nothing is made up in its
        place. Every question is optional, and it all stays on this machine.
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
        title="How do you want to sound?"
        description="Tone and length for letters and written answers: warm or formal, short or thorough."
      >
        <Textarea
          aria-label="How do you want to sound?"
          rows={2}
          value={current.writing_preferences}
          onChange={(e) => setDraft({ ...current, writing_preferences: e.target.value })}
        />
      </Section>

      <Section
        title="A writing sample"
        description="Something you wrote and liked — an email, a post, a past letter. Letters match its voice; nothing in it is stated as a fact about you."
      >
        <Textarea
          aria-label="A writing sample"
          rows={6}
          value={current.writing_sample}
          onChange={(e) => setDraft({ ...current, writing_sample: e.target.value })}
        />
      </Section>

      <Section
        title="Kept for similar questions"
        description="Answers you wrote on applications and chose to keep. Written answers to similar questions draw on them."
      >
        {about.kept.length === 0 ? (
          <Panel className="px-4 py-3">
            <p className="text-sm text-muted-foreground">
              None yet. When you write an answer on an application yourself, you can keep it from there, under{" "}
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
