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
import type { Candidate, LLMHealth, ProfileDraft } from "../lib/types";

const emptyProfile: Candidate = {
  id: "",
  first_name: "",
  last_name: "",
  contact: "",
  location: "",
  summary: "",
  work_preferences: "",
  salary_preferences: "",
  work_authorization: "",
  application_preferences: "",
  skills: "",
  experiences: "",
  education: "",
  certifications: "",
  links: "",
  created_at: "",
  updated_at: "",
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
      contact: pick(suggestion.contact, current.contact),
      location: pick(suggestion.location, current.location),
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
          <FormField label="Contact (email/phone)" htmlFor="contact">
            <Input id="contact" value={profile.contact} onChange={(e) => set("contact", e.target.value)} />
          </FormField>
          <FormField label="Location" htmlFor="location">
            <Input id="location" value={profile.location} onChange={(e) => set("location", e.target.value)} />
          </FormField>
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
          <FormField label="Salary preferences" htmlFor="salary_preferences">
            <Input
              id="salary_preferences"
              value={profile.salary_preferences}
              onChange={(e) => set("salary_preferences", e.target.value)}
            />
          </FormField>
          <FormField label="Work authorization" htmlFor="work_authorization">
            <Input
              id="work_authorization"
              value={profile.work_authorization}
              onChange={(e) => set("work_authorization", e.target.value)}
            />
          </FormField>
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
