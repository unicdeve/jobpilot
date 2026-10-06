import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { timeZoneLabel } from "../lib/format";
import { useCountries, useProfile } from "../lib/queries";
import type { SearchCriteria } from "../lib/types";
import { FormField } from "./form-field";
import { Button } from "./ui/button";
import { Checkbox } from "./ui/checkbox";
import { Input } from "./ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./ui/select";

const REMOTE_LABELS: Record<string, string> = {
  "": "Any",
  remote: "Remote",
  hybrid: "Hybrid",
  onsite: "On-site",
};

const EMPLOYMENT_LABELS: Record<string, string> = {
  "": "Any",
  full_time: "Full time",
  part_time: "Part time",
  contract: "Contract",
  internship: "Internship",
};

/// hasCriterion mirrors the daemon's rule that a search must narrow
/// something. The Save button stays disabled until it does, so the rejection
/// is visible before the round trip rather than as an error afterwards.
export function hasCriterion(criteria: SearchCriteria): boolean {
  return Boolean(
    criteria.keywords?.trim() ||
      criteria.title?.trim() ||
      criteria.location?.trim() ||
      criteria.remote_preference?.trim() ||
      criteria.employment_type?.trim() ||
      criteria.salary_min ||
      criteria.salary_max ||
      criteria.excluded_companies?.length ||
      criteria.work_from?.trim(),
  );
}

/// SHIFT_LABELS are how far a candidate would move their working day for a
/// job's hours, up to twelve, which reaches every zone.
const SHIFT_LABELS: Record<string, string> = Object.fromEntries(
  Array.from({ length: 13 }, (_, hours) => [
    String(hours),
    hours === 0 ? "0 hours" : hours === 1 ? "1 hour" : `${hours} hours`,
  ]),
);

/// NO_COUNTRY is the "I can work from" choice for no preference, which a
/// Select cannot hold as an empty string.
const NO_COUNTRY = "none";

export const emptyCriteria: SearchCriteria = {
  keywords: "",
  title: "",
  location: "",
  remote_preference: "",
  employment_type: "",
  salary_min: 0,
  salary_max: 0,
  excluded_companies: [],
};

/// SavedSearchForm edits one named search. It is the only place in the
/// desktop app where search criteria are entered, so that what the user can
/// search on is described once.
///
/// A new search starts with the profile's country as the one the candidate
/// works from (ADR-0023 §4); criteria that leave work_from undefined take
/// it, and a search that says "" has chosen none.
export function SavedSearchForm({
  initialName = "",
  initialCriteria = emptyCriteria,
  submitLabel,
  pending,
  onSubmit,
  onCancel,
}: {
  initialName?: string;
  initialCriteria?: SearchCriteria;
  submitLabel: string;
  pending: boolean;
  onSubmit: (name: string, criteria: SearchCriteria) => void;
  onCancel?: () => void;
}) {
  const [name, setName] = useState(initialName);
  const [criteria, setCriteria] = useState<SearchCriteria>(initialCriteria);

  const { data: countries } = useCountries();
  const { data: profile } = useProfile();
  const zone = countries?.time_zones.find((z) => z.id === profile?.time_zone);

  const set = <K extends keyof SearchCriteria>(key: K, value: SearchCriteria[K]) =>
    setCriteria((current) => ({ ...current, [key]: value }));

  const effective: SearchCriteria = {
    ...criteria,
    work_from: criteria.work_from ?? countries?.profile_country ?? "",
  };
  if (!effective.work_from) effective.include_relocation = false;
  if (!effective.fit_hours) effective.hours_shift = 0;
  const usable = name.trim() !== "" && hasCriterion(effective);

  const countryItems = useMemo(
    () => ({
      [NO_COUNTRY]: "No preference",
      ...Object.fromEntries((countries?.countries ?? []).map((country) => [country.code, country.name])),
    }),
    [countries],
  );

  return (
    <div className="space-y-3">
      <FormField label="Name" htmlFor="search-name">
        <Input
          id="search-name"
          placeholder="e.g. Backend Go roles"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </FormField>

      <div className="grid grid-cols-2 gap-3">
        <FormField label="Keywords" htmlFor="search-keywords">
          <Input
            id="search-keywords"
            placeholder="golang backend"
            value={criteria.keywords ?? ""}
            onChange={(e) => set("keywords", e.target.value)}
          />
        </FormField>
        <FormField label="Title contains" htmlFor="search-title">
          <Input
            id="search-title"
            placeholder="engineer"
            value={criteria.title ?? ""}
            onChange={(e) => set("title", e.target.value)}
          />
        </FormField>
        <FormField label="I can work from" htmlFor="search-work-from">
          <Select
            items={countryItems}
            value={effective.work_from || NO_COUNTRY}
            onValueChange={(value) => set("work_from", value && value !== NO_COUNTRY ? value : "")}
          >
            <SelectTrigger id="search-work-from" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {Object.entries(countryItems).map(([value, label]) => (
                <SelectItem key={value} value={value}>
                  {label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </FormField>
        <FormField label="Job location contains" htmlFor="search-location">
          <Input
            id="search-location"
            placeholder="lagos"
            value={criteria.location ?? ""}
            onChange={(e) => set("location", e.target.value)}
          />
        </FormField>
        <FormField label="Remote type" htmlFor="search-remote">
          <Select
            items={REMOTE_LABELS}
            value={criteria.remote_preference ?? ""}
            onValueChange={(value) => set("remote_preference", value ?? "")}
          >
            <SelectTrigger id="search-remote" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {Object.entries(REMOTE_LABELS).map(([value, label]) => (
                <SelectItem key={value || "any"} value={value}>
                  {label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </FormField>
        <FormField label="Employment type" htmlFor="search-employment">
          <Select
            items={EMPLOYMENT_LABELS}
            value={criteria.employment_type ?? ""}
            onValueChange={(value) => set("employment_type", value ?? "")}
          >
            <SelectTrigger id="search-employment" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {Object.entries(EMPLOYMENT_LABELS).map(([value, label]) => (
                <SelectItem key={value || "any"} value={value}>
                  {label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </FormField>
        <FormField label="Minimum salary" htmlFor="search-salary-min">
          <Input
            id="search-salary-min"
            type="number"
            min={0}
            value={criteria.salary_min || ""}
            onChange={(e) => set("salary_min", Number(e.target.value) || 0)}
          />
        </FormField>
        <FormField label="Maximum salary" htmlFor="search-salary-max">
          <Input
            id="search-salary-max"
            type="number"
            min={0}
            value={criteria.salary_max || ""}
            onChange={(e) => set("salary_max", Number(e.target.value) || 0)}
          />
        </FormField>
        <div className="col-span-2">
          <FormField label="Exclude companies (comma separated)" htmlFor="search-excluded">
            <Input
              id="search-excluded"
              placeholder="Acme, Globex"
              value={(criteria.excluded_companies ?? []).join(", ")}
              onChange={(e) =>
                set(
                  "excluded_companies",
                  e.target.value
                    .split(",")
                    .map((company) => company.trim())
                    .filter(Boolean),
                )
              }
            />
          </FormField>
        </div>
      </div>

      {effective.work_from && (
        <label className="flex items-start gap-3">
          <Checkbox
            className="mt-0.5"
            checked={effective.include_relocation ?? false}
            onCheckedChange={(checked) => set("include_relocation", checked === true)}
          />
          <span>
            <span className="block text-[13px] font-medium">
              Also include jobs elsewhere that offer relocation or visa sponsorship
            </span>
            <span className="mt-0.5 block text-xs text-muted-foreground">
              Jobs that only mention it are included too, with the sentence that says so. A job that refuses sponsorship
              is left out unless your profile says you can already work there.
            </span>
          </span>
        </label>
      )}

      {zone ? (
        <div className="flex items-start gap-3">
          <Checkbox
            id="search-fit-hours"
            className="mt-0.5"
            checked={effective.fit_hours ?? false}
            onCheckedChange={(checked) => set("fit_hours", checked === true)}
          />
          <div className="min-w-0">
            <label htmlFor="search-fit-hours" className="block text-[13px] font-medium">
              Leave out jobs whose working hours I can’t do
            </label>
            <span className="mt-0.5 block text-xs text-muted-foreground">
              From {timeZoneLabel(zone)}, as your profile says. Only a job that requires hours is left out; one that
              prefers them, or says nothing, is kept.
            </span>
            {effective.fit_hours && (
              <div className="mt-2 flex items-center gap-2 text-[13px]">
                <span id="search-hours-shift-label">I can move my working day by up to</span>
                <Select
                  items={SHIFT_LABELS}
                  value={String(effective.hours_shift ?? 0)}
                  onValueChange={(value) => set("hours_shift", Number(value) || 0)}
                >
                  <SelectTrigger aria-labelledby="search-hours-shift-label" className="w-28">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {Object.entries(SHIFT_LABELS).map(([value, label]) => (
                      <SelectItem key={value} value={value}>
                        {label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
          </div>
        </div>
      ) : (
        <p className="text-xs text-muted-foreground">
          <Link to="/profile" className="underline underline-offset-2 hover:text-foreground">
            Set your time zone
          </Link>{" "}
          to leave out jobs whose working hours you can’t do.
        </p>
      )}

      <p className="text-xs text-muted-foreground">
        Jobs must match every criterion you set. Keywords and title match each word in any order. Salary bounds keep
        jobs that do not advertise one, since most boards publish no salary, and “I can work from” keeps jobs that do
        not say where they can be done from.
      </p>

      <div className="flex items-center gap-2">
        <Button onClick={() => onSubmit(name.trim(), effective)} disabled={!usable || pending}>
          {pending ? "Saving…" : submitLabel}
        </Button>
        {onCancel && (
          <Button variant="outline" onClick={onCancel}>
            Cancel
          </Button>
        )}
        {!usable && (
          <span className="text-xs text-muted-foreground">A search needs a name and at least one criterion.</span>
        )}
      </div>
    </div>
  );
}
