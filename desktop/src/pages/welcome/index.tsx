import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Card, CardContent } from "../../components/ui/card";
import { useDaemon } from "../../lib/daemon";
import { useLLMSettings } from "../../lib/queries/llm.queries";
import { useProfile } from "../../lib/queries/profile.queries";
import { useResumes } from "../../lib/queries/resume.queries";
import { useSavedSearches } from "../../lib/queries/saved-search.queries";
import { rememberSkip } from "../../lib/setup-skip";
import { useToast } from "../../lib/toast";
import { ProfileStep } from "./profile-step";
import { ProviderStep } from "./provider-step";
import { ResumeStep } from "./resume-step";
import { SearchStep } from "./search-step";

const steps = ["AI provider", "Your profile", "Resume", "Job search"] as const;

/// Welcome sequences the four things a usable installation needs. The same
/// four are reachable from Settings, Profile and Resumes on their own, and
/// each of those screens says what is missing — but nothing put them in an
/// order, so a new user met three separate dead ends and had to work out
/// that they were one task. This is the desktop equivalent of the CLI's
/// `onboard` command, in the same order, for the same reason: prefilling a
/// profile from a resume needs the LLM, so the LLM comes first.
///
/// Progress is derived from what actually exists rather than from a stored
/// "completed" flag, so leaving halfway through and coming back resumes at
/// the first thing still missing, and a flag can never disagree with the
/// database.
export function Welcome() {
  const { activeWorkspace } = useDaemon();
  const navigate = useNavigate();
  const { notify } = useToast();

  const { data: llm, isPending: llmPending } = useLLMSettings();
  const { data, isPending: profilePending } = useProfile();
  const profile = data ?? undefined;
  const { data: resumes } = useResumes(profile?.id ?? null);
  const { data: searches, isPending: searchesPending } = useSavedSearches();

  const done = {
    llm: !!llm?.provider,
    profile: !!profile,
    resume: (resumes?.length ?? 0) > 0,
    search: (searches?.length ?? 0) > 0,
  };

  const loading = llmPending || profilePending || searchesPending;

  // Which step to open is derived once, when the queries first settle: a new
  // install starts at the top, a half-finished one at the first gap. It is
  // the user's to move after that. Deriving it on every render instead looks
  // right and is not: finishing a step flips its "done" flag, which skips
  // the screen forward before the user has read what just happened — the
  // provider step's connectivity result vanished that way.
  const [step, setStep] = useState<number | null>(null);
  useEffect(() => {
    if (loading || step !== null) return;
    setStep(!done.llm ? 0 : !done.profile ? 1 : 3);
  }, [loading, step, done.llm, done.profile]);

  const skip = () => {
    if (activeWorkspace) rememberSkip(activeWorkspace.id);
    navigate("/");
  };

  // searching is whether finishing started the first search, which then
  // reports itself as it runs and when it is done.
  const finish = (searching: boolean) => {
    notify(
      searching ? "Setup complete. JobPilot is looking for your first jobs." : "Setup complete — JobPilot is ready.",
      "success",
    );
    navigate("/");
  };

  if (loading || step === null) return <p className="p-6 text-sm text-muted-foreground">Loading…</p>;
  const current = step;

  return (
    <div className="mx-auto flex min-h-screen max-w-2xl flex-col justify-center px-6 py-10">
      <h1 className="text-xl font-semibold">Set up JobPilot</h1>
      <p className="mt-1 text-sm text-muted-foreground">
        Four things, none of them an account. Everything you enter stays on this machine, unless you choose a hosted
        model.
      </p>

      <ol className="mt-5 flex flex-wrap gap-2" aria-label="Setup steps">
        {steps.map((label, index) => {
          const complete = [done.llm, done.profile, done.resume, done.search][index];
          const active = index === current;
          return (
            <li key={label}>
              <button
                type="button"
                onClick={() => setStep(index)}
                data-slot="step"
                data-state={active ? "active" : complete ? "complete" : "todo"}
                className={`rounded-full px-3 py-1 text-xs font-medium ${
                  active
                    ? "bg-sidebar-accent text-foreground"
                    : complete
                      ? "bg-success/12 text-success"
                      : "bg-muted text-muted-foreground"
                }`}
              >
                {complete && !active ? "✓ " : `${index + 1}. `}
                {label}
              </button>
            </li>
          );
        })}
      </ol>

      <Card className="mt-4">
        <CardContent className="space-y-4">
          {current === 0 && <ProviderStep settings={llm} onNext={() => setStep(1)} />}
          {current === 1 && <ProfileStep profile={profile} exists={done.profile} onNext={() => setStep(2)} />}
          {current === 2 && (
            <ResumeStep profile={profile} llmConfigured={done.llm} imported={done.resume} onNext={() => setStep(3)} />
          )}
          {current === 3 && <SearchStep hasSearch={done.search} onDone={finish} onBack={() => setStep(2)} />}
        </CardContent>
      </Card>

      <div className="mt-4 text-center">
        <button type="button" onClick={skip} className="text-xs text-muted-foreground underline">
          Skip setup — I'll find my way around
        </button>
      </div>
    </div>
  );
}
