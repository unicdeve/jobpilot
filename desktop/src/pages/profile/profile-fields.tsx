import { FormField } from "../../components/form-field";
import { Section } from "../../components/page";
import { TimeZoneField } from "../../components/time-zone-field";
import { Input } from "../../components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../../components/ui/select";
import type { Candidate, Countries, RemotePreference } from "../../lib/types";

/// SetField changes one field of the profile being edited.
export type SetField = <K extends keyof Candidate>(key: K, value: Candidate[K]) => void;

interface FieldsProps {
  profile: Candidate;
  set: SetField;
}

export function ContactSection({ profile, set }: FieldsProps) {
  return (
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
  );
}

/// LocationSection is where the candidate lives, and the time zone they
/// work in. timeZone is the profile's, or the one suggested for it when it
/// has none yet.
export function LocationSection({
  profile,
  set,
  countries,
  timeZone,
}: FieldsProps & { countries: Countries | undefined; timeZone: string }) {
  return (
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
  );
}

/// NO_PREFERENCE stands for an unstated preference in the select, which
/// cannot hold an empty string as a choice of its own.
const NO_PREFERENCE = "none";

const REMOTE_LABELS: Record<string, string> = {
  [NO_PREFERENCE]: "No preference stated",
  remote: "Remote",
  hybrid: "Hybrid",
  onsite: "On-site",
};

export function AvailabilitySection({ profile, set }: FieldsProps) {
  return (
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
  );
}
