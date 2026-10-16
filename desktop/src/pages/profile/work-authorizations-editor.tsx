import { Section } from "../../components/page";
import { Button } from "../../components/ui/button";
import { Checkbox } from "../../components/ui/checkbox";
import { Input } from "../../components/ui/input";
import type { WorkAuthorization } from "../../lib/types";

/// WorkAuthorizationsEditor edits the right to work, per country.
///
/// It is per country because the question always is: "will you require
/// sponsorship" on a Berlin posting and the same words on a London posting
/// are different questions, and for many people they have different
/// answers. A country left unsaid means JobPilot answers nothing about it
/// rather than guessing — see ADR-0015.
export function WorkAuthorizationsEditor({
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
