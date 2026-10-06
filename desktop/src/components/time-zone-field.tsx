import { useMemo } from "react";
import { timeZoneLabel } from "../lib/format";
import type { Countries, TimeZone } from "../lib/types";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "./ui/select";

/// NO_ZONE is "not set", which a Select cannot hold as an empty string.
const NO_ZONE = "none";

/// countryCode reads a country as the profile writes it — "Nigeria", "NG" —
/// against the daemon's list, or undefined when it is not one.
function countryCode(countries: Countries | undefined, written: string): string | undefined {
  const text = written.trim().toLowerCase();
  if (!text || !countries) return undefined;
  return countries.countries.find((c) => c.code.toLowerCase() === text || c.name.toLowerCase() === text)?.code;
}

/// computerTimeZone is the zone this computer is set to, as an IANA name.
function computerTimeZone(): string | undefined {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone;
  } catch {
    return undefined;
  }
}

/// suggestedTimeZone is the zone a profile with none starts with: its
/// country's, when the country has only one, and this computer's otherwise
/// (ADR-0025 §2).
export function suggestedTimeZone(countries: Countries | undefined, country: string): string {
  if (!countries) return "";
  const code = countryCode(countries, country);
  const inCountry = code ? countries.time_zones.filter((zone) => zone.country === code) : [];
  if (inCountry.length === 1) return inCountry[0].id;
  const computer = computerTimeZone();
  const known = countries.time_zones.find((zone) => zone.id === computer);
  return known && (!code || known.country === code) ? known.id : "";
}

/// TimeZoneField chooses the candidate's time zone: the country's zones
/// first, west to east, then every other.
export function TimeZoneField({
  id,
  countries,
  country,
  value,
  onChange,
}: {
  id: string;
  countries: Countries | undefined;
  country: string;
  value: string;
  onChange: (zone: string) => void;
}) {
  const code = countryCode(countries, country);
  const { local, others, items } = useMemo(() => {
    const zones = countries?.time_zones ?? [];
    const byOffset = (a: TimeZone, b: TimeZone) => a.offset - b.offset || a.id.localeCompare(b.id);
    const local = zones.filter((zone) => zone.country === code).sort(byOffset);
    const others = zones.filter((zone) => zone.country !== code).sort(byOffset);
    const items: Record<string, string> = { [NO_ZONE]: "Not set" };
    for (const zone of zones) items[zone.id] = timeZoneLabel(zone);
    return { local, others, items };
  }, [countries, code]);
  const countryName = countries?.countries.find((c) => c.code === code)?.name;

  return (
    <Select
      items={items}
      value={value || NO_ZONE}
      onValueChange={(next) => onChange(next && next !== NO_ZONE ? next : "")}
    >
      <SelectTrigger id={id} className="w-full">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={NO_ZONE}>Not set</SelectItem>
        {local.length > 0 && (
          <SelectGroup>
            <SelectLabel>{countryName ?? "Your country"}</SelectLabel>
            {local.map((zone) => (
              <SelectItem key={zone.id} value={zone.id}>
                {timeZoneLabel(zone)}
              </SelectItem>
            ))}
          </SelectGroup>
        )}
        {local.length > 0 && <SelectSeparator />}
        <SelectGroup>
          {local.length > 0 && <SelectLabel>Everywhere else</SelectLabel>}
          {others.map((zone) => (
            <SelectItem key={zone.id} value={zone.id}>
              {timeZoneLabel(zone)}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  );
}
