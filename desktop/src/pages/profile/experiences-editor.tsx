import { FormField } from "../../components/form-field";
import { Section } from "../../components/page";
import { Button } from "../../components/ui/button";
import { Checkbox } from "../../components/ui/checkbox";
import { Input } from "../../components/ui/input";
import { Textarea } from "../../components/ui/textarea";
import type { CandidateExperience } from "../../lib/types";

/// ExperiencesEditor keeps the work history as dated entries.
///
/// The dates are what make "Current company" answerable and a total years
/// of experience countable — neither of which a paragraph could give.
export function ExperiencesEditor({
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
