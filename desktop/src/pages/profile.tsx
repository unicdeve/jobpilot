import { useEffect, useState } from "react";
import { Button } from "../components/ui/button";
import { Card } from "../components/ui/card";
import { FormField, Input, Textarea } from "../components/ui/field";
import { useApi } from "../lib/daemon";
import { useToast } from "../lib/toast";
import type { Candidate } from "../lib/types";

const emptyProfile: Candidate = {
  id: "",
  name: "",
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
        const created = await api.initializeProfile(profile);
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

  return (
    <div className="max-w-2xl">
      <h1 className="mb-4 text-xl font-semibold">Profile</h1>
      {!exists && (
        <p className="mb-4 text-sm text-slate-500">
          No candidate profile yet — fill this in to create one.
        </p>
      )}
      <Card className="space-y-4">
        <FormField label="Name" htmlFor="name">
          <Input id="name" value={profile.name} onChange={(e) => set("name", e.target.value)} />
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
      </Card>
    </div>
  );
}
