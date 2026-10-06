import { Upload } from "lucide-react";
import { type ReactNode, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { FormField } from "../components/form-field";
import { Page, Panel, Section } from "../components/page";
import { Loading } from "../components/status-line";
import { suggestedTimeZone, TimeZoneField } from "../components/time-zone-field";
import { Button } from "../components/ui/button";
import { Checkbox } from "../components/ui/checkbox";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "../components/ui/dialog";
import { Input } from "../components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../components/ui/select";
import { Tabs, TabsList, TabsTrigger } from "../components/ui/tabs";
import { Textarea } from "../components/ui/textarea";
import {
  useCheckLLMHealth,
  useCountries,
  useImportResume,
  useInitializeProfile,
  useProfile,
  useProposeProfileDraft,
  useUpdateProfile,
} from "../lib/queries";
import { pickResumeFile } from "../lib/resume-file";
import { useToast } from "../lib/toast";
import type {
  Candidate,
  CandidateExperience,
  CandidateLink,
  CandidateSkill,
  LinkKind,
  LLMHealth,
  ProfileDraft,
  RemotePreference,
  WorkAuthorization,
} from "../lib/types";
import { LINK_KINDS } from "../lib/types";
import { AboutYouPage } from "./profile-about";

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

/// NO_PREFERENCE stands for an unstated preference in the select, which
/// cannot hold an empty string as a choice of its own.
const NO_PREFERENCE = "none";

const REMOTE_LABELS: Record<string, string> = {
  [NO_PREFERENCE]: "No preference stated",
  remote: "Remote",
  hybrid: "Hybrid",
  onsite: "On-site",
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

/// Profile is the candidate's facts, under Details, and what they say about
/// themselves, under About you (docs/02_features/16_about_you.md). The two
/// save separately: one is the profile a form reads, the other the
/// material a letter is written from.
export function Profile() {
  const [params, setParams] = useSearchParams();
  const tab = params.get("tab") === "about" ? "about" : "details";
  const tabs = (
    <Tabs
      className="mb-6"
      value={tab}
      onValueChange={(value) =>
        setParams(
          (current) => {
            const next = new URLSearchParams(current);
            if (value === "about") next.set("tab", "about");
            else next.delete("tab");
            return next;
          },
          { replace: true },
        )
      }
    >
      <TabsList>
        <TabsTrigger value="details">Details</TabsTrigger>
        <TabsTrigger value="about">About you</TabsTrigger>
      </TabsList>
    </Tabs>
  );
  return tab === "about" ? <AboutYouPage tabs={tabs} /> : <ProfileDetails tabs={tabs} />;
}

function ProfileDetails({ tabs }: { tabs: ReactNode }) {
  const { notify } = useToast();
  // No profile yet is null, which is first run: the form creates one.
  const { data: saved, isPending } = useProfile();
  const { data: countries } = useCountries();
  const exists = saved != null;

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

  if (isPending) return <Loading />;

  const profile = draft ?? saved ?? emptyProfile;
  // A profile with no time zone starts with its country's, or this
  // computer's (ADR-0025 §2), and saves it.
  const timeZone = profile.time_zone || suggestedTimeZone(countries, profile.location_country);

  const set = <K extends keyof Candidate>(key: K, value: Candidate[K]) => setDraft({ ...profile, [key]: value });

  const save = () => {
    if (exists) {
      updateProfile.mutate({ ...profile, time_zone: timeZone });
      return;
    }
    // created_at/updated_at are server-assigned; sending them as "" fails
    // Go's time.Time JSON parsing, so a brand-new profile omits them.
    const { created_at, updated_at, ...rest } = profile;
    initializeProfile.mutate({ ...rest, time_zone: timeZone });
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
    // A list the user has already built is theirs. Merging entry by entry
    // would need a notion of which proposed role is "the same" as one they
    // typed, and getting that wrong silently rewrites their history — so a
    // proposed list only fills an empty one.
    const pickList = <T,>(suggested: T[] | null | undefined, currentValue: T[] | null) =>
      currentValue?.length ? currentValue : (suggested ?? []);

    return {
      ...current,
      first_name: pick(suggestion.first_name, current.first_name),
      last_name: pick(suggestion.last_name, current.last_name),
      email: pick(suggestion.email, current.email),
      phone: pick(suggestion.phone, current.phone),
      location_city: pick(suggestion.location_city, current.location_city),
      location_region: pick(suggestion.location_region, current.location_region),
      location_country: pick(suggestion.location_country, current.location_country),
      summary: pick(suggestion.summary, current.summary),
      skills: pickList<CandidateSkill>(
        suggestion.skills?.map((skill) => ({
          ...toSkill(skill.name),
          years_experience: skill.years_experience ?? null,
        })),
        current.skills,
      ),
      experiences: pickList<CandidateExperience>(
        suggestion.experiences?.map((experience) => ({
          id: "",
          candidate_id: "",
          company: experience.company,
          title: experience.title,
          start_date: experience.start_date || null,
          end_date: experience.is_current ? null : experience.end_date || null,
          is_current: experience.is_current,
          description: experience.description,
        })),
        current.experiences,
      ),
      links: pickList<CandidateLink>(
        suggestion.links?.map((link) => ({
          id: "",
          candidate_id: "",
          kind: link.kind,
          url: link.url,
        })),
        current.links,
      ),
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

    const path = await pickResumeFile();
    if (!path) return;

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
      <Dialog open={!!llmProblem} onOpenChange={(open) => !open && setLlmProblem(null)}>
        <DialogContent>
          <DialogTitle>
            {llmProblem?.configured ? "Can't reach your AI provider" : "Set up your AI provider first"}
          </DialogTitle>
          <DialogDescription>
            {llmProblem?.configured
              ? "Prefilling reads your resume with the model you configured, and it isn't answering. Check that it's running, then try again."
              : "Prefilling reads your resume with the model you choose in Settings. Choose a provider and model there, then come back and import."}
          </DialogDescription>
          {llmProblem?.error && (
            <p className="mt-3 rounded-md bg-muted px-3 py-2 font-mono text-xs break-words text-muted-foreground">
              {llmProblem.error}
            </p>
          )}
          <DialogFooter>
            <DialogClose render={<Button variant="ghost" />}>Not now</DialogClose>
            <Button render={<Link to="/settings?section=model" />}>Open settings</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {tabs}

      <Panel className="flex flex-wrap items-center gap-x-4 gap-y-3 px-4 py-3">
        <div className="min-w-0 flex-1">
          <p className="text-[13px] font-medium">{exists ? "Fill in from a resume" : "Start from your resume"}</p>
          <p className="text-xs text-muted-foreground">
            JobPilot reads the resume and suggests what to put here. Nothing is saved until you review it and save.
          </p>
        </div>
        <label className="flex items-center gap-2 text-xs text-subtle-foreground">
          <Checkbox checked={overwriteExisting} onCheckedChange={(checked) => setOverwriteExisting(checked === true)} />
          Replace what’s already filled in
        </label>
        <Button size="sm" variant="outline" onClick={() => void importResumeToPrefill()} disabled={busy}>
          <Upload data-icon="inline-start" />
          {checkLLM.isPending ? "Checking…" : importing ? "Reading…" : "Import resume"}
        </Button>
      </Panel>

      <Section title="Contact" className="mt-8">
        <div className="grid gap-4 sm:grid-cols-2">
          <FormField label="First name" htmlFor="first_name">
            <Input id="first_name" value={profile.first_name} onChange={(e) => set("first_name", e.target.value)} />
          </FormField>
          <FormField label="Last name" htmlFor="last_name">
            <Input id="last_name" value={profile.last_name} onChange={(e) => set("last_name", e.target.value)} />
          </FormField>
          <FormField label="Email" htmlFor="email">
            <Input id="email" type="email" value={profile.email} onChange={(e) => set("email", e.target.value)} />
          </FormField>
          <FormField label="Phone" htmlFor="phone">
            <Input id="phone" value={profile.phone} onChange={(e) => set("phone", e.target.value)} />
          </FormField>
        </div>
      </Section>

      <Section title="Location">
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
        <div className="mt-4 grid gap-4 sm:grid-cols-3">
          <div className="sm:col-span-2">
            <FormField label="Time zone" htmlFor="time_zone">
              <TimeZoneField
                id="time_zone"
                countries={countries}
                country={profile.location_country}
                value={timeZone}
                onChange={(zone) => set("time_zone", zone)}
              />
            </FormField>
          </div>
          <p className="self-end pb-1.5 text-xs text-muted-foreground">
            {!profile.time_zone && timeZone
              ? "Suggested from your country or this computer. Saved with your profile."
              : "A search can leave out jobs whose working hours you can’t do from here."}
          </p>
        </div>
      </Section>

      <Section title="Summary" description="Used when matching you to jobs and when writing cover letters.">
        <Textarea
          id="summary"
          aria-label="Summary"
          rows={5}
          value={profile.summary}
          onChange={(e) => set("summary", e.target.value)}
        />
      </Section>

      <Section title="Availability">
        <div className="grid gap-4 sm:grid-cols-3">
          <FormField label="How you want to work" htmlFor="remote_preference">
            <Select
              items={REMOTE_LABELS}
              value={profile.remote_preference || NO_PREFERENCE}
              onValueChange={(value) =>
                set("remote_preference", (value === NO_PREFERENCE ? "" : value) as RemotePreference)
              }
            >
              <SelectTrigger id="remote_preference" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(REMOTE_LABELS).map(([value, label]) => (
                  <SelectItem key={value} value={value}>
                    {label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </FormField>
          <FormField label="Notice period (days)" htmlFor="notice_period_days">
            <Input
              id="notice_period_days"
              type="number"
              min={0}
              value={profile.notice_period_days ?? ""}
              onChange={(e) => set("notice_period_days", e.target.value === "" ? null : Number(e.target.value))}
            />
          </FormField>
          <FormField label="Earliest start date" htmlFor="earliest_start_date">
            <Input
              id="earliest_start_date"
              type="date"
              value={profile.earliest_start_date ?? ""}
              onChange={(e) => set("earliest_start_date", e.target.value || null)}
            />
          </FormField>
        </div>
      </Section>

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

/// LinksEditor edits one link per kind, which is the shape a form asks for:
/// a page with a LinkedIn field and a GitHub field wants a different URL in
/// each (ADR-0015). Every kind is shown, so an empty GitHub box is visibly
/// empty rather than silently missing.
function LinksEditor({ links, onChange }: { links: CandidateLink[]; onChange: (links: CandidateLink[]) => void }) {
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
    <Section title="Links" description={<>Application forms ask for these one at a time, so each has its own box.</>}>
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
    </Section>
  );
}

function emptyLink(kind: LinkKind): CandidateLink {
  return { id: "", candidate_id: "", kind, url: "" };
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
    <Section
      title="Work authorization"
      description={
        <>
          Per country, matched against where a job is. A country you leave out is one JobPilot will not answer for — it
          asks you instead.
        </>
      }
    >
      {authorizations.length === 0 ? (
        <p className="text-sm text-muted-foreground">No countries added yet.</p>
      ) : (
        <ul className="space-y-3">
          {authorizations.map((auth, index) => (
            <li
              key={auth.id || index}
              className="flex flex-wrap items-center gap-4 rounded-lg border border-border bg-card p-3"
            >
              <Input
                aria-label="Country"
                className="w-48"
                placeholder="Country"
                value={auth.country}
                onChange={(e) => update(index, { country: e.target.value })}
              />
              <label className="flex items-center gap-2 text-sm text-subtle-foreground">
                <Checkbox
                  checked={auth.is_authorized}
                  onCheckedChange={(checked) => update(index, { is_authorized: checked === true })}
                />
                Authorized to work here
              </label>
              <label className="flex items-center gap-2 text-sm text-subtle-foreground">
                <Checkbox
                  checked={auth.requires_sponsorship}
                  onCheckedChange={(checked) => update(index, { requires_sponsorship: checked === true })}
                />
                Needs sponsorship
              </label>
              <Button
                size="sm"
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
        size="sm"
        variant="outline"
        onClick={() =>
          onChange([
            ...authorizations,
            {
              id: "",
              candidate_id: "",
              country: "",
              is_authorized: false,
              requires_sponsorship: false,
            },
          ])
        }
      >
        Add a country
      </Button>
    </Section>
  );
}

function toSkill(name: string): CandidateSkill {
  return { id: "", candidate_id: "", name, years_experience: null };
}

/// SkillsEditor keeps skills as a list with an optional duration each.
///
/// The years box is optional on purpose: "I know Go" and "I have used Go
/// for four years" are different statements, and only the candidate can
/// make the second. A skill with no duration is simply not used to answer a
/// "how many years" question (ADR-0015).
function SkillsEditor({
  skills,
  onChange,
}: {
  skills: CandidateSkill[];
  onChange: (skills: CandidateSkill[]) => void;
}) {
  const update = (index: number, patch: Partial<CandidateSkill>) =>
    onChange(skills.map((skill, i) => (i === index ? { ...skill, ...patch } : skill)));

  return (
    <Section
      title="Skills"
      description={
        <>
          Years are optional. A skill without them is never used to answer a "how many years" question — you are asked
          instead.
        </>
      }
    >
      {skills.length === 0 ? (
        <p className="text-sm text-muted-foreground">No skills added yet.</p>
      ) : (
        <ul className="space-y-2">
          {skills.map((skill, index) => (
            <li key={skill.id || index} className="flex flex-wrap items-center gap-3">
              <Input
                aria-label="Skill"
                className="w-56"
                placeholder="e.g. Go"
                value={skill.name}
                onChange={(e) => update(index, { name: e.target.value })}
              />
              <Input
                aria-label="Years of experience"
                className="w-28"
                type="number"
                min={0}
                step="0.5"
                placeholder="Years"
                value={skill.years_experience ?? ""}
                onChange={(e) =>
                  update(index, {
                    years_experience: e.target.value === "" ? null : Number(e.target.value),
                  })
                }
              />
              <Button
                size="sm"
                variant="ghost"
                className="ml-auto"
                onClick={() => onChange(skills.filter((_, i) => i !== index))}
              >
                Remove
              </Button>
            </li>
          ))}
        </ul>
      )}

      <Button size="sm" variant="outline" onClick={() => onChange([...skills, toSkill("")])}>
        Add a skill
      </Button>
    </Section>
  );
}

/// ExperiencesEditor keeps the work history as dated entries.
///
/// The dates are what make "Current company" answerable and a total years
/// of experience countable — neither of which a paragraph could give.
function ExperiencesEditor({
  experiences,
  onChange,
}: {
  experiences: CandidateExperience[];
  onChange: (experiences: CandidateExperience[]) => void;
}) {
  const update = (index: number, patch: Partial<CandidateExperience>) =>
    onChange(experiences.map((item, i) => (i === index ? { ...item, ...patch } : item)));

  return (
    <Section
      title="Work history"
      description={
        <>
          Dates are what let JobPilot answer "Current company" and count your years of experience. An entry without them
          is still kept, it just is not counted.
        </>
      }
    >
      {experiences.length === 0 ? (
        <p className="text-sm text-muted-foreground">No roles added yet.</p>
      ) : (
        <ul className="space-y-4">
          {experiences.map((experience, index) => (
            <li key={experience.id || index} className="space-y-3 rounded-lg border border-border bg-card p-4">
              <div className="grid gap-3 sm:grid-cols-2">
                <FormField label="Title" htmlFor={`experience-title-${index}`}>
                  <Input
                    id={`experience-title-${index}`}
                    value={experience.title}
                    onChange={(e) => update(index, { title: e.target.value })}
                  />
                </FormField>
                <FormField label="Company" htmlFor={`experience-company-${index}`}>
                  <Input
                    id={`experience-company-${index}`}
                    value={experience.company}
                    onChange={(e) => update(index, { company: e.target.value })}
                  />
                </FormField>
              </div>

              <div className="grid gap-3 sm:grid-cols-2">
                <FormField label="Started" htmlFor={`experience-start-${index}`}>
                  <Input
                    id={`experience-start-${index}`}
                    type="date"
                    value={experience.start_date ?? ""}
                    onChange={(e) => update(index, { start_date: e.target.value || null })}
                  />
                </FormField>
                <FormField label="Ended" htmlFor={`experience-end-${index}`}>
                  <Input
                    id={`experience-end-${index}`}
                    type="date"
                    disabled={experience.is_current}
                    value={experience.is_current ? "" : (experience.end_date ?? "")}
                    onChange={(e) => update(index, { end_date: e.target.value || null })}
                  />
                </FormField>
              </div>

              <label className="flex items-center gap-2 text-sm text-subtle-foreground">
                <Checkbox
                  checked={experience.is_current}
                  onCheckedChange={(checked) =>
                    update(index, {
                      is_current: checked === true,
                      end_date: checked === true ? null : experience.end_date,
                    })
                  }
                />
                I work here now
              </label>

              <FormField label="What you did" htmlFor={`experience-description-${index}`}>
                <Textarea
                  id={`experience-description-${index}`}
                  rows={2}
                  value={experience.description}
                  onChange={(e) => update(index, { description: e.target.value })}
                />
              </FormField>

              <Button size="sm" variant="ghost" onClick={() => onChange(experiences.filter((_, i) => i !== index))}>
                Remove this role
              </Button>
            </li>
          ))}
        </ul>
      )}

      <Button
        size="sm"
        variant="outline"
        onClick={() =>
          onChange([
            ...experiences,
            {
              id: "",
              candidate_id: "",
              company: "",
              title: "",
              start_date: null,
              end_date: null,
              is_current: false,
              description: "",
            },
          ])
        }
      >
        Add a role
      </Button>
    </Section>
  );
}
