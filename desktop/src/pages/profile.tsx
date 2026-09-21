import { open } from "@tauri-apps/plugin-dialog";
import { useState } from "react";
import { Link } from "react-router-dom";
import { Button } from "../components/ui/button";
import { Checkbox } from "../components/ui/checkbox";
import { FormField } from "../components/form-field";
import { Card, CardContent } from "../components/ui/card";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "../components/ui/dialog";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import { Textarea } from "../components/ui/textarea";
import {
  useCheckLLMHealth,
  useImportResume,
  useInitializeProfile,
  useProfile,
  useProposeProfileDraft,
  useUpdateProfile,
} from "../lib/queries";
import { useToast } from "../lib/toast";
import { LINK_KINDS } from "../lib/types";
import type { Candidate, CandidateLink, LinkKind, LLMHealth, ProfileDraft, WorkAuthorization } from "../lib/types";

const emptyProfile: Candidate = {
  id: "",
  first_name: "",
  last_name: "",
  email: "",
  phone: "",
  location_city: "",
  location_region: "",
  location_country: "",
  notice_period_days: null,
  earliest_start_date: null,
  summary: "",
  work_preferences: "",
  application_preferences: "",
  skills: "",
  experiences: "",
  education: "",
  certifications: "",
  links: [],
  work_authorizations: [],
  created_at: "",
  updated_at: "",
};

// LINK_LABELS names each kind the way the forms that ask for it do.
const LINK_LABELS: Record<LinkKind, string> = {
  linkedin: "LinkedIn",
  github: "GitHub",
  portfolio: "Portfolio",
  website: "Website",
  twitter: "Twitter",
  other: "Other",
};

export function Profile() {
  const { notify } = useToast();
  // A daemon with no candidate yet answers getProfile with an error, so an
  // errored query means "first run", not a failure to show the user.
  const { data: saved, isPending, isError: noProfileYet } = useProfile();
  const exists = !noProfileYet;

  // The form is an editable draft over whatever is saved. Keeping edits
  // separate from the cached profile means a background refetch can never
  // overwrite something half-typed.
  const [draft, setDraft] = useState<Candidate | null>(null);
  const [overwriteExisting, setOverwriteExisting] = useState(false);
  // Prefilling reads the resume with the local LLM, so it is checked before
  // the file picker opens rather than after: the old order created the
  // profile, imported the resume, and only then failed on the part the user
  // actually asked for.
  const [llmProblem, setLlmProblem] = useState<LLMHealth | null>(null);
  const checkLLM = useCheckLLMHealth();

  const updateProfile = useUpdateProfile({
    onSuccess: () => {
      setDraft(null);
      notify("Profile saved.", "success");
    },
    onError: (err) => notify(String(err)),
  });
  const initializeProfile = useInitializeProfile({
    onSuccess: () => {
      setDraft(null);
      notify("Profile created.", "success");
    },
    onError: (err) => notify(String(err)),
  });
  const importResume = useImportResume();
  const proposeDraft = useProposeProfileDraft();

  const saving = updateProfile.isPending || initializeProfile.isPending;
  const importing = importResume.isPending || proposeDraft.isPending;
  const busy = importing || checkLLM.isPending;

  if (isPending) return <p className="text-sm text-slate-500">Loading…</p>;

  const profile = draft ?? saved ?? emptyProfile;

  const set = <K extends keyof Candidate>(key: K, value: Candidate[K]) =>
    setDraft({ ...profile, [key]: value });

  const save = () => {
    if (exists) {
      updateProfile.mutate(profile);
      return;
    }
    // created_at/updated_at are server-assigned; sending them as "" fails
    // Go's time.Time JSON parsing, so a brand-new profile omits them.
    const { created_at, updated_at, ...rest } = profile;
    initializeProfile.mutate(rest);
  };

  // A blank suggestion field (the LLM found nothing) never changes the
  // current field. When overwriteExisting is off, a suggested field is only
  // applied if the current field is empty, so it never clobbers a value the
  // user already typed or a previously saved profile value.
  const applySuggestion = (current: Candidate, suggestion: ProfileDraft): Candidate => {
    const pick = (suggested: string, currentValue: string) => {
      if (!suggested) return currentValue;
      if (overwriteExisting) return suggested;
      return currentValue || suggested;
    };
    return {
      ...current,
      first_name: pick(suggestion.first_name, current.first_name),
      last_name: pick(suggestion.last_name, current.last_name),
      email: pick(suggestion.contact, current.email),
      location_city: pick(suggestion.location, current.location_city),
      summary: pick(suggestion.summary, current.summary),
      skills: pick(suggestion.skills.join(", "), current.skills),
    };
  };

  const importResumeToPrefill = async () => {
    let health: LLMHealth;
    try {
      health = await checkLLM.mutateAsync();
    } catch (err) {
      notify(`Could not check your AI provider: ${err}`);
      return;
    }
    if (!health.ok) {
      setLlmProblem(health);
      return;
    }

    const path = await open({ multiple: false, title: "Select a resume file" });
    if (!path || Array.isArray(path)) return;

    try {
      // A resume can only be associated with a candidate that already has an
      // ID, so a brand-new profile is created (with whatever's typed so far)
      // before the resume is imported against it.
      let current = profile;
      if (!exists) {
        const { created_at, updated_at, ...rest } = current;
        current = await initializeProfile.mutateAsync(rest);
        setDraft(current);
      }

      const resume = await importResume.mutateAsync({
        candidateId: current.id,
        filePath: path,
      });
      notify("Resume imported.", "success");

      try {
        const suggestion = await proposeDraft.mutateAsync(resume.markdown_content);
        setDraft(applySuggestion(current, suggestion));
        notify("Profile prefilled from resume — review before saving.", "success");
      } catch (err) {
        notify(`Resume imported, but couldn't generate suggestions: ${err}`);
      }
    } catch (err) {
      notify(String(err));
    }
  };

  return (
    <div className="max-w-2xl">
      <Dialog open={!!llmProblem} onOpenChange={(open) => !open && setLlmProblem(null)}>
        <DialogContent>
          <DialogTitle>
            {llmProblem?.configured ? "Can't reach your AI provider" : "Set up your AI provider first"}
          </DialogTitle>
          <DialogDescription>
            {llmProblem?.configured
              ? "Prefilling reads your resume with the LLM you configured, and it isn't answering. Check that it's running, then try again."
              : "Prefilling reads your resume with a local LLM — JobPilot never sends it anywhere else. Choose a provider and model in Settings, then come back and import."}
          </DialogDescription>
          {llmProblem?.error && (
            <p className="mt-3 rounded-md bg-muted px-3 py-2 font-mono text-xs break-words text-muted-foreground">
              {llmProblem.error}
            </p>
          )}
          <DialogFooter>
            <Button variant="ghost" render={<DialogClose>Not now</DialogClose>} />
            <Button render={<Link to="/settings">Open settings</Link>} />
          </DialogFooter>
        </DialogContent>
      </Dialog>
        <div className="flex items-center justify-between mb-4 gap-3">
          <Label htmlFor="overwrite_existing">
            <Checkbox
              id="overwrite_existing"
              checked={overwriteExisting}
              onCheckedChange={(checked) => setOverwriteExisting(checked === true)}
            />
            Overwrite existing fields with resume data
          </Label>
          <Button variant="outline" onClick={() => void importResumeToPrefill()} disabled={busy}>
            {checkLLM.isPending ? "Checking…" : importing ? "Importing…" : "Import resume to prefill"}
          </Button>
      </div>
      {!exists && (
        <p className="mb-4 text-sm text-slate-500">
          No candidate profile yet — fill this in to create one, or import a resume to prefill it.
        </p>
      )}
      <Card>
        <CardContent className="space-y-4">
          <FormField label="First Name" htmlFor="first_name">
            <Input
              id="first_name"
              value={profile.first_name}
              onChange={(e) => set("first_name", e.target.value)}
            />
          </FormField>
          <FormField label="Last Name" htmlFor="last_name">
            <Input
              id="last_name"
              value={profile.last_name}
              onChange={(e) => set("last_name", e.target.value)}
            />
          </FormField>
          <FormField label="Email" htmlFor="email">
            <Input id="email" type="email" value={profile.email} onChange={(e) => set("email", e.target.value)} />
          </FormField>
          <FormField label="Phone" htmlFor="phone">
            <Input id="phone" value={profile.phone} onChange={(e) => set("phone", e.target.value)} />
          </FormField>
          <div className="grid gap-4 sm:grid-cols-3">
            <FormField label="City" htmlFor="location_city">
              <Input
                id="location_city"
                value={profile.location_city}
                onChange={(e) => set("location_city", e.target.value)}
              />
            </FormField>
            <FormField label="State / region" htmlFor="location_region">
              <Input
                id="location_region"
                value={profile.location_region}
                onChange={(e) => set("location_region", e.target.value)}
              />
            </FormField>
            <FormField label="Country" htmlFor="location_country">
              <Input
                id="location_country"
                value={profile.location_country}
                onChange={(e) => set("location_country", e.target.value)}
              />
            </FormField>
          </div>
          <FormField label="Summary" htmlFor="summary">
            <Textarea
              id="summary"
              rows={3}
              value={profile.summary}
              onChange={(e) => set("summary", e.target.value)}
            />
          </FormField>
          <FormField label="Skills" htmlFor="skills">
            <Input id="skills" value={profile.skills} onChange={(e) => set("skills", e.target.value)} />
          </FormField>
          <div className="grid gap-4 sm:grid-cols-2">
            <FormField label="Notice period (days)" htmlFor="notice_period_days">
              <Input
                id="notice_period_days"
                type="number"
                min={0}
                value={profile.notice_period_days ?? ""}
                onChange={(e) =>
                  set("notice_period_days", e.target.value === "" ? null : Number(e.target.value))
                }
              />
            </FormField>
            <FormField label="Earliest start date" htmlFor="earliest_start_date">
              <Input
                id="earliest_start_date"
                type="date"
                value={profile.earliest_start_date?.slice(0, 10) ?? ""}
                onChange={(e) => set("earliest_start_date", e.target.value || null)}
              />
            </FormField>
          </div>

          <LinksEditor
            links={profile.links ?? []}
            onChange={(links) => set("links", links)}
          />

          <WorkAuthorizationsEditor
            authorizations={profile.work_authorizations ?? []}
            onChange={(authorizations) => set("work_authorizations", authorizations)}
          />
          <FormField label="Application preferences (tone/length for generated content)" htmlFor="application_preferences">
            <Textarea
              id="application_preferences"
              rows={2}
              value={profile.application_preferences}
              onChange={(e) => set("application_preferences", e.target.value)}
            />
          </FormField>
          <Button onClick={save} disabled={saving}>
            {exists ? "Save" : "Create profile"}
          </Button>
        </CardContent>
      </Card>
    </div>
  );
}

/// LinksEditor edits one link per kind, which is the shape a form asks for:
/// a page with a LinkedIn field and a GitHub field wants a different URL in
/// each (ADR-0015). Every kind is shown, so an empty GitHub box is visibly
/// empty rather than silently missing.
function LinksEditor({
  links,
  onChange,
}: {
  links: CandidateLink[];
  onChange: (links: CandidateLink[]) => void;
}) {
  const urlFor = (kind: LinkKind) => links.find((link) => link.kind === kind)?.url ?? "";

  const setURL = (kind: LinkKind, url: string) => {
    const rest = links.filter((link) => link.kind !== kind);
    if (!url.trim()) {
      onChange(rest);
      return;
    }
    const existing = links.find((link) => link.kind === kind);
    onChange([...rest, { ...(existing ?? emptyLink(kind)), kind, url }]);
  };

  return (
    <section className="space-y-3">
      <div>
        <h3 className="text-sm font-medium text-slate-900">Links</h3>
        <p className="text-xs text-slate-500">
          Application forms ask for these one at a time, so each has its own box.
        </p>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        {LINK_KINDS.map((kind) => (
          <FormField key={kind} label={LINK_LABELS[kind]} htmlFor={`link-${kind}`}>
            <Input
              id={`link-${kind}`}
              value={urlFor(kind)}
              placeholder="https://"
              onChange={(e) => setURL(kind, e.target.value)}
            />
          </FormField>
        ))}
      </div>
    </section>
  );
}

function emptyLink(kind: LinkKind): CandidateLink {
  return { id: "", candidate_id: "", kind, url: "", created_at: "", updated_at: "" };
}

/// WorkAuthorizationsEditor edits the right to work, per country.
///
/// It is per country because the question always is: "will you require
/// sponsorship" on a Berlin posting and the same words on a London posting
/// are different questions, and for many people they have different
/// answers. A country left unsaid means JobPilot answers nothing about it
/// rather than guessing — see ADR-0015.
function WorkAuthorizationsEditor({
  authorizations,
  onChange,
}: {
  authorizations: WorkAuthorization[];
  onChange: (authorizations: WorkAuthorization[]) => void;
}) {
  const update = (index: number, patch: Partial<WorkAuthorization>) =>
    onChange(authorizations.map((auth, i) => (i === index ? { ...auth, ...patch } : auth)));

  return (
    <section className="space-y-3">
      <div>
        <h3 className="text-sm font-medium text-slate-900">Work authorization</h3>
        <p className="text-xs text-slate-500">
          Per country, matched against where a job is. A country you leave out is one JobPilot
          will not answer for — it asks you instead.
        </p>
      </div>

      {authorizations.length === 0 ? (
        <p className="text-sm text-slate-500">No countries added yet.</p>
      ) : (
        <ul className="space-y-3">
          {authorizations.map((auth, index) => (
            <li key={auth.id || index} className="flex flex-wrap items-center gap-4 rounded-md border border-slate-200 p-3">
              <Input
                aria-label="Country"
                className="w-48"
                placeholder="Country"
                value={auth.country}
                onChange={(e) => update(index, { country: e.target.value })}
              />
              <label className="flex items-center gap-2 text-sm text-slate-700">
                <input
                  type="checkbox"
                  checked={auth.is_authorized}
                  onChange={(e) => update(index, { is_authorized: e.target.checked })}
                />
                Authorized to work here
              </label>
              <label className="flex items-center gap-2 text-sm text-slate-700">
                <input
                  type="checkbox"
                  checked={auth.requires_sponsorship}
                  onChange={(e) => update(index, { requires_sponsorship: e.target.checked })}
                />
                Needs sponsorship
              </label>
              <Button
                variant="ghost"
                className="ml-auto"
                onClick={() => onChange(authorizations.filter((_, i) => i !== index))}
              >
                Remove
              </Button>
            </li>
          ))}
        </ul>
      )}

      <Button
        variant="secondary"
        onClick={() =>
          onChange([
            ...authorizations,
            {
              id: "",
              candidate_id: "",
              country: "",
              is_authorized: false,
              requires_sponsorship: false,
              created_at: "",
              updated_at: "",
            },
          ])
        }
      >
        Add a country
      </Button>
    </section>
  );
}
