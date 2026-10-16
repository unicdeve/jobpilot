import { Section } from "../../components/page";
import { Button } from "../../components/ui/button";
import { Input } from "../../components/ui/input";
import type { CandidateSkill } from "../../lib/types";

function toSkill(name: string): CandidateSkill {
  return { id: "", candidate_id: "", name, years_experience: null };
}

/// SkillsEditor keeps skills as a list with an optional duration each.
///
/// The years box is optional on purpose: "I know Go" and "I have used Go
/// for four years" are different statements, and only the candidate can
/// make the second. A skill with no duration is simply not used to answer a
/// "how many years" question (ADR-0015).
export function SkillsEditor({
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
