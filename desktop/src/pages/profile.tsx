import { open } from "@tauri-apps/plugin-dialog";
import { useEffect, useState } from "react";
import { Button } from "../components/ui/button";
import { Checkbox } from "../components/ui/checkbox";
import { FormField } from "../components/form-field";
import { Card, CardContent } from "../components/ui/card";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import { Textarea } from "../components/ui/textarea";
import { useApi } from "../lib/daemon";
import { useToast } from "../lib/toast";
import type { Candidate, ProfileDraft } from "../lib/types";

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
  const api = useApi();
  const { notify } = useToast();
  const [profile, setProfile] = useState<Candidate | null>(null);
  const [exists, setExists] = useState(true);
  const [saving, setSaving] = useState(false);
  const [importing, setImporting] = useState(false);
  const [overwriteExisting, setOverwriteExisting] = useState(false);

  useEffect(() => {
    api
      .getProfile()
      .then(setProfile)
      .catch(() => {
        setExists(false);
        setProfile(emptyProfile);
      });
  }, [api]);

  if (!profile) return <p className="text-sm text-slate-500">Loading…</p>;

  const set = <K extends keyof Candidate>(key: K, value: Candidate[K]) =>
    setProfile((p) => (p ? { ...p, [key]: value } : p));

  const save = async () => {
    if (!profile) return;
    setSaving(true);
    try {
      if (exists) {
        const updated = await api.updateProfile(profile);
        setProfile(updated);
        notify("Profile saved.", "success");
      } else {
        // created_at/updated_at are server-assigned; sending them as "" fails
        // Go's time.Time JSON parsing, so a brand-new profile omits them.
        const { created_at, updated_at, ...rest } = profile;
        const created = await api.initializeProfile(rest);
        setProfile(created);
        setExists(true);
        notify("Profile created.", "success");
      }
    } catch (err) {
      notify(String(err));
    } finally {
      setSaving(false);
    }
  };

  // A blank draft field (the LLM found nothing) never changes the current
  // field. When overwriteExisting is off, a draft field is only applied if
  // the current field is empty, so it never clobbers a value the user
  // already typed or a previously saved profile value.
  const applyDraft = (draft: ProfileDraft) => {
    const pick = (draftValue: string, currentValue: string) => {
      if (!draftValue) return currentValue;
      if (overwriteExisting) return draftValue;
      return currentValue || draftValue;
    };
    setProfile((p) =>
      p
        ? {
            ...p,
            first_name: pick(draft.first_name, p.first_name),
            last_name: pick(draft.last_name, p.last_name),
            contact: pick(draft.contact, p.contact),
            location: pick(draft.location, p.location),
            summary: pick(draft.summary, p.summary),
            skills: pick(draft.skills.join(", "), p.skills),
          }
        : p,
    );
  };

  const importResumeToPrefill = async () => {
    if (!profile) return;
    const path = await open({ multiple: false, title: "Select a resume file" });
    if (!path || Array.isArray(path)) return;

    setImporting(true);
    try {
      // A resume can only be associated with a candidate that already has an
      // ID, so a brand-new profile is created (with whatever's typed so far)
      // before the resume is imported against it.
      let candidateId = profile.id;
      if (!exists) {
        const { created_at, updated_at, ...rest } = profile;
        const created = await api.initializeProfile(rest);
        candidateId = created.id;
        setProfile(created);
        setExists(true);
      }

      const resume = await api.importResume(candidateId, path);
      notify("Resume imported.", "success");

      try {
        const draft = await api.proposeProfileDraft(resume.markdown_content);
        applyDraft(draft);
        notify("Profile prefilled from resume — review before saving.", "success");
      } catch (err) {
        notify(`Resume imported, but couldn't generate suggestions: ${err}`);
      }
    } catch (err) {
      notify(String(err));
    } finally {
      setImporting(false);
    }
  };

  return (
    <div className="max-w-2xl">
      <div className="mb-4 flex items-center justify-between">
        <h1 className="text-xl font-semibold">Profile</h1>
        <div className="flex items-center gap-3">
          <Label htmlFor="overwrite_existing">
            <Checkbox
              id="overwrite_existing"
              checked={overwriteExisting}
              onCheckedChange={(checked) => setOverwriteExisting(checked === true)}
            />
            Overwrite existing fields with resume data
          </Label>
          <Button variant="outline" onClick={importResumeToPrefill} disabled={importing}>
            {importing ? "Importing…" : "Import resume to prefill"}
          </Button>
        </div>
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
