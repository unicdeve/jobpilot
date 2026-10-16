import { FormField } from "../../components/form-field";
import { Section } from "../../components/page";
import { Input } from "../../components/ui/input";
import type { CandidateLink, LinkKind } from "../../lib/types";
import { LINK_KINDS } from "../../lib/types";

// LINK_LABELS names each kind the way the forms that ask for it do.
const LINK_LABELS: Record<LinkKind, string> = {
  linkedin: "LinkedIn",
  github: "GitHub",
  portfolio: "Portfolio",
  website: "Website",
  twitter: "Twitter",
  other: "Other",
};

/// LinksEditor edits one link per kind, which is the shape a form asks for:
/// a page with a LinkedIn field and a GitHub field wants a different URL in
/// each (ADR-0015). Every kind is shown, so an empty GitHub box is visibly
/// empty rather than silently missing.
export function LinksEditor({
  links,
  onChange,
}: {
  links: CandidateLink[];
  onChange: (links: CandidateLink[]) => void;
}) {
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
