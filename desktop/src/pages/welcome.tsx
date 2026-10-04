import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useDiscover } from "../components/discovery-activity";
import { FormField } from "../components/form-field";
import { LLMProviderFields, withProviderDefaults } from "../components/llm-provider-fields";
import { SavedSearchForm } from "../components/saved-search-form";
import { SearchTemplatePicker } from "../components/search-template-picker";
import { Button } from "../components/ui/button";
import { Card, CardContent } from "../components/ui/card";
import { Input } from "../components/ui/input";
import {
  useCreateSavedSearch,
  useImportResume,
  useInitializeProfile,
  useLLMSettings,
  useProfile,
  useProposeProfileDraft,
  useResumes,
  useSavedSearches,
  useSaveLLMSettings,
  useUpdateProfile,
} from "../lib/queries";
import { useToast } from "../lib/toast";
import type { Candidate, LLMSettings, SavedSearch } from "../lib/types";
import { pickResumeFile } from "../lib/resume-file";
import { rememberSkip } from "../lib/setup-skip";

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
    rememberSkip();
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
        Four things, none of them an account. Everything you enter stays on this machine, unless
        you choose a hosted model.
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
          {current === 1 && (
            <ProfileStep profile={profile} exists={done.profile} onNext={() => setStep(2)} />
          )}
          {current === 2 && (
            <ResumeStep
              profile={profile}
              llmConfigured={done.llm}
              imported={done.resume}
              onNext={() => setStep(3)}
            />
          )}
          {current === 3 && (
            <SearchStep hasSearch={done.search} onDone={finish} onBack={() => setStep(2)} />
          )}
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

/// ProviderStep configures the local LLM. Saving reports connectivity as a
/// warning rather than a failure, matching the daemon: an install with no
/// internet, or with Ollama not started yet, must still be able to finish
/// setting itself up (docs/02_features/01_onboarding.md AC-003).
function ProviderStep({ settings, onNext }: { settings?: LLMSettings; onNext: () => void }) {
  const [draft, setDraft] = useState<LLMSettings>(() =>
    withProviderDefaults(settings ?? { provider: "", base_url: "", model: "" }),
  );
  const [result, setResult] = useState<{ ok: boolean; message: string } | null>(null);

  const save = useSaveLLMSettings({
    onSuccess: (saved) =>
      setResult(
        saved.connectivity_error
          ? { ok: false, message: saved.connectivity_error }
          : { ok: true, message: "Connected — your model answered." },
      ),
    onError: (err) => setResult({ ok: false, message: String(err) }),
  });

  return (
    <>
      <div>
        <h2 className="font-semibold">Choose the AI model</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Matching, cover letters and reading a resume all run on the model you choose. A model on
          this computer keeps everything here; a hosted one is sent what it reads.
        </p>
      </div>

      <LLMProviderFields settings={draft} onChange={setDraft} />

      {result && (
        <p
          data-slot="connectivity"
          className={`text-sm ${result.ok ? "text-success" : "text-warning"}`}
        >
          {result.ok ? result.message : `Saved, but could not connect: ${result.message}`}
        </p>
      )}

      <div className="flex items-center gap-2">
        <Button
          variant="outline"
          onClick={() => save.mutate(draft)}
          disabled={save.isPending || !draft.model.trim()}
        >
          {save.isPending ? "Saving…" : "Save and test"}
        </Button>
        <Button onClick={onNext} disabled={!result}>
          Continue
        </Button>
        {!result && !draft.model.trim() && (
          <span className="text-xs text-muted-foreground">A model name is required.</span>
        )}
        {result && !result.ok && (
          <span className="text-xs text-muted-foreground">
            You can continue and start it later — only the AI steps need it.
          </span>
        )}
      </div>
    </>
  );
}

/// ProfileStep creates the candidate. It asks for the same four fields the
/// CLI's onboard prompts for, and no more: the rest of the profile is
/// better filled from a resume or edited later on the Profile screen.
function ProfileStep({
  profile,
  exists,
  onNext,
}: {
  profile?: Candidate;
  exists: boolean;
  onNext: () => void;
}) {
  const { notify } = useToast();
  const [draft, setDraft] = useState({
    first_name: profile?.first_name ?? "",
    last_name: profile?.last_name ?? "",
    email: profile?.email ?? "",
    phone: profile?.phone ?? "",
    location_city: profile?.location_city ?? "",
    location_country: profile?.location_country ?? "",
  });

  const create = useInitializeProfile({
    onSuccess: () => onNext(),
    onError: (err) => notify(String(err)),
  });

  if (exists) {
    return (
      <>
        <div>
          <h2 className="font-semibold">Profile created</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {[profile?.first_name, profile?.last_name].filter(Boolean).join(" ") ||
              "Your candidate profile"}{" "}
            is saved. The <Link className="underline" to="/profile">Profile screen</Link> has the
            rest of the fields whenever you want them.
          </p>
        </div>
        <Button onClick={onNext}>Continue</Button>
      </>
    );
  }

  const set = (key: keyof typeof draft, value: string) =>
    setDraft((current) => ({ ...current, [key]: value }));

  return (
    <>
      <div>
        <h2 className="font-semibold">Who are you applying as?</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          JobPilot matches jobs against this and fills applications from it. It never invents
          details you have not given it.
        </p>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <FormField label="First name" htmlFor="welcome-first">
          <Input
            id="welcome-first"
            value={draft.first_name}
            onChange={(e) => set("first_name", e.target.value)}
          />
        </FormField>
        <FormField label="Last name" htmlFor="welcome-last">
          <Input
            id="welcome-last"
            value={draft.last_name}
            onChange={(e) => set("last_name", e.target.value)}
          />
        </FormField>
        <FormField label="Email" htmlFor="welcome-email">
          <Input
            id="welcome-email"
            type="email"
            value={draft.email}
            onChange={(e) => set("email", e.target.value)}
          />
        </FormField>
        <FormField label="Phone" htmlFor="welcome-phone">
          <Input
            id="welcome-phone"
            value={draft.phone}
            onChange={(e) => set("phone", e.target.value)}
          />
        </FormField>
        <FormField label="City" htmlFor="welcome-city">
          <Input
            id="welcome-city"
            value={draft.location_city}
            onChange={(e) => set("location_city", e.target.value)}
          />
        </FormField>
        <FormField label="Country" htmlFor="welcome-country">
          <Input
            id="welcome-country"
            value={draft.location_country}
            onChange={(e) => set("location_country", e.target.value)}
          />
        </FormField>
      </div>

      <div className="flex items-center gap-2">
        <Button
          onClick={() => create.mutate(draft)}
          disabled={create.isPending || !draft.first_name.trim()}
        >
          {create.isPending ? "Creating…" : "Create profile"}
        </Button>
        {!draft.first_name.trim() && (
          <span className="text-xs text-muted-foreground">A first name is required.</span>
        )}
      </div>
    </>
  );
}

/// ResumeStep is optional, as it is in the CLI. Importing one also prefills
/// the profile, which is the only step here that needs the LLM — hence its
/// place after the provider step, and the explanation when that was skipped.
function ResumeStep({
  profile,
  llmConfigured,
  imported,
  onNext,
}: {
  profile?: Candidate;
  llmConfigured: boolean;
  imported: boolean;
  onNext: () => void;
}) {
  const { notify } = useToast();
  const importResume = useImportResume();
  const proposeDraft = useProposeProfileDraft();
  const updateProfile = useUpdateProfile();
  const busy = importResume.isPending || proposeDraft.isPending || updateProfile.isPending;

  const pick = async () => {
    const path = await pickResumeFile();
    if (!path || !profile) return;

    try {
      const resume = await importResume.mutateAsync({
        candidateId: profile.id,
        filePath: path,
      });
      notify("Resume imported.", "success");

      // A prefill failure must never undo the import, which succeeded and is
      // useful on its own — the same rule the CLI's onboard follows.
      try {
        const suggestion = await proposeDraft.mutateAsync(resume.markdown_content);
        // A field the LLM did not find comes back empty, and a field the
        // user already typed in the previous step is theirs: a suggestion
        // only fills a blank, so nothing they entered is overwritten.
        const fill = (suggested: string, current: string) => current || suggested;
        await updateProfile.mutateAsync({
          ...profile,
          first_name: fill(suggestion.first_name, profile.first_name),
          last_name: fill(suggestion.last_name, profile.last_name),
          email: fill(suggestion.email, profile.email),
          phone: fill(suggestion.phone, profile.phone),
          location_city: fill(suggestion.location_city, profile.location_city),
          location_country: fill(suggestion.location_country, profile.location_country),
          summary: fill(suggestion.summary, profile.summary),
          // Lists the user has already built are theirs; a suggestion only
          // fills an empty one.
          skills: profile.skills?.length
            ? profile.skills
            : (suggestion.skills ?? []).map((skill) => ({
                id: "",
                candidate_id: "",
                name: skill.name,
                years_experience: skill.years_experience ?? null,
              })),
          experiences: profile.experiences?.length
            ? profile.experiences
            : (suggestion.experiences ?? []).map((experience) => ({
                id: "",
                candidate_id: "",
                company: experience.company,
                title: experience.title,
                start_date: experience.start_date || null,
                end_date: experience.is_current ? null : experience.end_date || null,
                is_current: experience.is_current,
                description: experience.description,
              })),
          links: profile.links?.length
            ? profile.links
            : (suggestion.links ?? []).map((link) => ({
                id: "",
                candidate_id: "",
                kind: link.kind,
                url: link.url,
              })),
        });
        notify("Profile prefilled from your resume — review it on the Profile screen.", "success");
      } catch (err) {
        notify(`Resume imported, but couldn't read details from it: ${err}`);
      }
      onNext();
    } catch (err) {
      notify(String(err));
    }
  };

  return (
    <>
      <div>
        <h2 className="font-semibold">Import a resume</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Optional. JobPilot converts it to text, keeps it for applications, and reads your details
          out of it so you do not have to type them again.
        </p>
      </div>

      {imported && <p className="text-sm text-success">A resume is already imported.</p>}

      {!llmConfigured && (
        <p className="text-sm text-warning">
          Reading details out of a resume needs the AI provider, which is not set up yet. You can
          still import the file — it just will not prefill anything.
        </p>
      )}

      <div className="flex items-center gap-2">
        <Button variant="outline" onClick={() => void pick()} disabled={busy || !profile}>
          {busy ? "Importing…" : "Choose a file"}
        </Button>
        <Button onClick={onNext}>{imported ? "Continue" : "Skip this step"}</Button>
      </div>
    </>
  );
}

/// SearchStep is the one step that cannot be skipped: discovery refuses to
/// run on empty criteria, so an installation with no saved search cannot
/// find a single job (docs/02_features/01_onboarding.md AC-005).
///
/// The searches it adds are run as it finishes, so setup ends with jobs
/// arriving rather than with empty lists and a Discover button to find
/// (AC-006). The runs are queued and outlive this screen; offline, each one
/// says it could not reach the sources, and setup is complete regardless.
function SearchStep({
  hasSearch,
  onDone,
  onBack,
}: {
  hasSearch: boolean;
  onDone: (searching: boolean) => void;
  onBack: () => void;
}) {
  const { notify } = useToast();
  const discover = useDiscover();
  // Templates first: this is where a first-time user is least able to tell a
  // search that will work from one that will quietly find nothing.
  const [writingOwn, setWritingOwn] = useState(false);
  const runAndFinish = (added: SavedSearch[]) => {
    for (const search of added) discover.mutate({ id: search.id, name: search.name });
    onDone(added.length > 0);
  };
  const create = useCreateSavedSearch({
    onSuccess: (search) => runAndFinish([search]),
    onError: (err) => notify(String(err)),
  });

  if (hasSearch) {
    return (
      <>
        <div>
          <h2 className="font-semibold">You have a search</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            That is everything JobPilot needs. Add more of them any time under Settings.
          </p>
        </div>
        <Button onClick={() => onDone(false)}>Finish</Button>
      </>
    );
  }

  return (
    <>
      <div>
        <h2 className="font-semibold">What jobs are you looking for?</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          JobPilot only looks for what you ask it to. The job boards do no filtering of their own,
          so without a search it would import every posting on them — it refuses to run instead.
        </p>
      </div>

      {writingOwn ? (
        <>
          <SavedSearchForm
            submitLabel="Save and finish"
            pending={create.isPending}
            onSubmit={(name, criteria) => create.mutate({ name, criteria })}
            onCancel={onBack}
          />
          <button
            type="button"
            onClick={() => setWritingOwn(false)}
            className="self-start text-xs text-muted-foreground underline"
          >
            Back to the templates
          </button>
        </>
      ) : (
        <>
          <SearchTemplatePicker
            onCancel={onBack}
            onError={(message) => notify(message)}
            onImported={runAndFinish}
          />
          <button
            type="button"
            onClick={() => setWritingOwn(true)}
            className="self-start text-xs text-muted-foreground underline"
          >
            Write my own instead
          </button>
        </>
      )}
    </>
  );
}
