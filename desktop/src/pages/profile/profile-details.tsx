import { type ReactNode, useState } from "react";
import { Page, Section } from "../../components/page";
import { Loading } from "../../components/status-line";
import { suggestedTimeZone } from "../../components/time-zone-field";
import { Button } from "../../components/ui/button";
import { Textarea } from "../../components/ui/textarea";
import { useInitializeProfile, useProfile, useUpdateProfile } from "../../lib/queries/profile.queries";
import { useCountries } from "../../lib/queries/saved-search.queries";
import { useToast } from "../../lib/toast";
import type { Candidate } from "../../lib/types";
import { ExperiencesEditor } from "./experiences-editor";
import { LinksEditor } from "./links-editor";
import { AvailabilitySection, ContactSection, LocationSection, type SetField } from "./profile-fields";
import { ResumePrefill } from "./resume-prefill";
import { SkillsEditor } from "./skills-editor";
import { WorkAuthorizationsEditor } from "./work-authorizations-editor";

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
  remote_preference: "",
  skills: [],
  experiences: [],
  links: [],
  work_authorizations: [],
};

/// newProfile is profile as a profile not created yet is sent.
/// created_at/updated_at are server-assigned; sending them as "" fails Go's
/// time.Time JSON parsing, so a brand-new profile omits them.
function newProfile(profile: Candidate): Partial<Candidate> {
  const { created_at, updated_at, ...rest } = profile;
  return rest;
}

export function ProfileDetails({ tabs }: { tabs: ReactNode }) {
  const { notify, notifyError } = useToast();
  // No profile yet is null, which is first run: the form creates one.
  const { data: saved, isPending } = useProfile();
  const { data: countries } = useCountries();
  const exists = saved != null;

  // The form is an editable draft over whatever is saved. Keeping edits
  // separate from the cached profile means a background refetch can never
  // overwrite something half-typed.
  const [draft, setDraft] = useState<Candidate | null>(null);

  const updateProfile = useUpdateProfile({
    onSuccess: () => {
      setDraft(null);
      notify("Profile saved.", "success");
    },
    onError: notifyError,
  });
  const initializeProfile = useInitializeProfile({
    onSuccess: () => {
      setDraft(null);
      notify("Profile created.", "success");
    },
    onError: notifyError,
  });
  const saving = updateProfile.isPending || initializeProfile.isPending;

  if (isPending) return <Loading />;

  const profile = draft ?? saved ?? emptyProfile;
  // A profile with no time zone starts with its country's, or this
  // computer's (ADR-0025 §2), and saves it.
  const timeZone = profile.time_zone || suggestedTimeZone(countries, profile.location_country);

  const set: SetField = (key, value) => setDraft({ ...profile, [key]: value });

  const save = () => {
    if (exists) updateProfile.mutate({ ...profile, time_zone: timeZone });
    else initializeProfile.mutate({ ...newProfile(profile), time_zone: timeZone });
  };

  const saveButton = (
    <Button size="sm" onClick={save} disabled={saving || (exists && !draft)}>
      {saving ? "Saving…" : exists ? "Save" : "Create profile"}
    </Button>
  );

  return (
    <Page
      title="Profile"
      actions={
        <>
          {draft && exists && (
            <Button size="sm" variant="ghost" onClick={() => setDraft(null)}>
              Revert
            </Button>
          )}
          {saveButton}
        </>
      }
    >
      {tabs}

      <ResumePrefill
        profile={profile}
        exists={exists}
        create={() => initializeProfile.mutateAsync(newProfile(profile))}
        onDraft={setDraft}
      />

      <ContactSection profile={profile} set={set} />
      <LocationSection profile={profile} set={set} countries={countries} timeZone={timeZone} />

      <Section title="Summary" description="Used when matching you to jobs and when writing cover letters.">
        <Textarea
          id="summary"
          aria-label="Summary"
          rows={5}
          value={profile.summary}
          onChange={(e) => set("summary", e.target.value)}
        />
      </Section>

      <AvailabilitySection profile={profile} set={set} />

      <SkillsEditor skills={profile.skills ?? []} onChange={(skills) => set("skills", skills)} />

      <ExperiencesEditor
        experiences={profile.experiences ?? []}
        onChange={(experiences) => set("experiences", experiences)}
      />

      <LinksEditor links={profile.links ?? []} onChange={(links) => set("links", links)} />

      <WorkAuthorizationsEditor
        authorizations={profile.work_authorizations ?? []}
        onChange={(authorizations) => set("work_authorizations", authorizations)}
      />

      <div className="mt-8 flex justify-end border-t border-border pt-5">{saveButton}</div>
    </Page>
  );
}
