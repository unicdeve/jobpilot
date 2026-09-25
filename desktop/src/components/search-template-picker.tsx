import { useMemo, useState } from "react";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import { Checkbox } from "./ui/checkbox";
import { useImportSearchTemplates, useSearchTemplates } from "../lib/queries";
import type { SavedSearch, SearchCriteria, SearchTemplate } from "../lib/types";

/// describeTemplateCriteria shows what a template actually searches on, so
/// the user adopts criteria they can see rather than a name they have to
/// trust. It is deliberately the raw fields: after importing, the edit form
/// shows the same words in the same boxes.
function describeTemplateCriteria(criteria: SearchCriteria): string {
  const parts: string[] = [];
  if (criteria.title?.trim()) parts.push(`title: ${criteria.title.trim()}`);
  if (criteria.keywords?.trim()) parts.push(`description: ${criteria.keywords.trim()}`);
  return parts.join(" · ");
}

/// SearchTemplatePicker offers the ready-made searches, several at a time.
///
/// Multi-select is the point rather than a convenience. Criteria cannot
/// express "or", so the same job titled Engineer at one board and Developer
/// at another needs two searches, and a picker that took one at a time would
/// quietly encourage the mistake the templates exist to fix.
export function SearchTemplatePicker({
  onImported,
  onCancel,
  onError,
}: {
  onImported: (created: SavedSearch[]) => void;
  onCancel: () => void;
  onError: (message: string) => void;
}) {
  const { data: groups, error, isPending } = useSearchTemplates();
  const [activeGroupId, setActiveGroupId] = useState<string | null>(null);
  const [selected, setSelected] = useState<Set<string>>(new Set());

  const importTemplates = useImportSearchTemplates({
    onSuccess: (created) => {
      setSelected(new Set());
      onImported(created);
    },
    onError: (err) => onError(String(err)),
  });

  const activeGroup = useMemo(() => {
    if (!groups?.length) return null;
    return groups.find((group) => group.id === activeGroupId) ?? groups[0];
  }, [groups, activeGroupId]);

  if (error) return <p className="text-sm text-destructive">{String(error.message)}</p>;
  if (isPending || !activeGroup) return <p className="text-sm text-muted-foreground">Loading…</p>;

  const toggle = (templateId: string) =>
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(templateId)) {
        next.delete(templateId);
      } else {
        next.add(templateId);
      }
      return next;
    });

  const selectGroup = (templates: SearchTemplate[], all: boolean) =>
    setSelected((current) => {
      const next = new Set(current);
      for (const template of templates) {
        if (all) {
          next.add(template.id);
        } else {
          next.delete(template.id);
        }
      }
      return next;
    });

  const groupFullySelected = activeGroup.templates.every((template) => selected.has(template.id));

  return (
    <div className="space-y-3 rounded-md border border-border p-3">
      <div>
        <h3 className="font-medium">Start from a template</h3>
        <p className="mt-1 text-xs text-muted-foreground">
          A search matches only jobs containing every word you set, so a narrow one finds almost
          nothing. These criteria were measured against live postings from every source. Adopt
          several — a copy becomes yours to edit, and nothing links it back here.
        </p>
      </div>

      <div className="flex flex-wrap gap-1">
        {groups.map((group) => (
          <Button
            key={group.id}
            size="sm"
            variant={group.id === activeGroup.id ? "secondary" : "ghost"}
            onClick={() => setActiveGroupId(group.id)}
          >
            {group.name}
          </Button>
        ))}
      </div>

      <div className="flex items-start justify-between gap-3">
        <p className="text-xs text-muted-foreground">{activeGroup.summary}</p>
        <Button
          size="xs"
          variant="ghost"
          className="shrink-0"
          onClick={() => selectGroup(activeGroup.templates, !groupFullySelected)}
        >
          {groupFullySelected ? "Clear group" : "Select all"}
        </Button>
      </div>

      <ul className="space-y-1">
        {activeGroup.templates.map((template) => {
          const checked = selected.has(template.id);
          return (
            <li key={template.id}>
              <label
                className="flex cursor-pointer items-start gap-2.5 rounded-md p-2 hover:bg-accent"
                htmlFor={`template-${template.id}`}
              >
                <Checkbox
                  id={`template-${template.id}`}
                  className="mt-0.5"
                  checked={checked}
                  onCheckedChange={() => toggle(template.id)}
                />
                <span className="min-w-0 flex-1">
                  <span className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-medium">{template.name}</span>
                    <Badge variant="outline" className="font-mono">
                      {describeTemplateCriteria(template.criteria)}
                    </Badge>
                  </span>
                  <span className="mt-0.5 block text-xs text-muted-foreground">
                    {template.summary}
                  </span>
                </span>
              </label>
            </li>
          );
        })}
      </ul>

      <div className="flex items-center gap-2">
        <Button
          disabled={selected.size === 0 || importTemplates.isPending}
          onClick={() => importTemplates.mutate([...selected])}
        >
          {importTemplates.isPending
            ? "Adding…"
            : selected.size === 0
              ? "Add searches"
              : `Add ${selected.size} ${selected.size === 1 ? "search" : "searches"}`}
        </Button>
        <Button variant="outline" onClick={onCancel}>
          Cancel
        </Button>
        {selected.size > 0 && (
          <span className="text-xs text-muted-foreground">
            Selected across all groups, not just this one.
          </span>
        )}
      </div>
    </div>
  );
}
