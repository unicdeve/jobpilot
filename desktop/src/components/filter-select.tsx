import { cn } from "cn";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./ui/select";

/// FILTER_ALL stands for "no filter" in a FilterSelect, which cannot hold an
/// empty string as a distinct choice.
export const FILTER_ALL = "all";

/// FilterSelect is one filter's choices, the first of which is FILTER_ALL.
/// Choosing it reports an empty string, so a caller only ever sees a real
/// value or nothing.
export function FilterSelect({
  label,
  items,
  value,
  onChange,
  className,
  id,
}: {
  label: string;
  items: Record<string, string>;
  value: string;
  onChange: (value: string) => void;
  className?: string;
  id?: string;
}) {
  return (
    <Select
      items={items}
      value={value || FILTER_ALL}
      onValueChange={(next) => onChange(next && next !== FILTER_ALL ? next : "")}
    >
      <SelectTrigger id={id} size="sm" aria-label={label} className={cn("min-w-0", className)}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {Object.entries(items).map(([itemValue, itemLabel]) => (
          <SelectItem key={itemValue} value={itemValue}>
            {itemLabel}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
