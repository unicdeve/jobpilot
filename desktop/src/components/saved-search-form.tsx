import { useState } from "react";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { FormField } from "./form-field";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "./ui/select";
import type { SearchCriteria } from "../lib/types";

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
      criteria.excluded_companies?.length,
  );
}

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

  const set = <K extends keyof SearchCriteria>(key: K, value: SearchCriteria[K]) =>
    setCriteria((current) => ({ ...current, [key]: value }));

  const usable = name.trim() !== "" && hasCriterion(criteria);

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
        <FormField label="Location" htmlFor="search-location">
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
      </div>

      <p className="text-xs text-slate-500">
        Jobs must match every criterion you set. Keywords and title match each word in any order.
        Salary bounds keep jobs that do not advertise one, since most boards publish no salary.
      </p>

      <div className="flex items-center gap-2">
        <Button onClick={() => onSubmit(name.trim(), criteria)} disabled={!usable || pending}>
          {pending ? "Saving…" : submitLabel}
        </Button>
        {onCancel && (
          <Button variant="secondary" onClick={onCancel}>
            Cancel
          </Button>
        )}
        {!usable && (
          <span className="text-xs text-slate-500">
            A search needs a name and at least one criterion.
          </span>
        )}
      </div>
    </div>
  );
}
