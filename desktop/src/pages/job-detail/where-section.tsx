import type { ReactNode } from "react";
import { DetailSection } from "../../components/page";
import { hoursShiftFor, hoursWindow, jobSource, offerLabel, placesLabel } from "../../lib/format";
import { useProfile } from "../../lib/queries/profile.queries";
import { useCountries } from "../../lib/queries/saved-search.queries";
import type { Job, WorkingHours } from "../../lib/types";

/// WhereSection says where the job can be done from and whether it helps a
/// candidate move, as the daemon read the posting (ADR-0023), with the
/// posting's own words beside each: a label read from text is only as good
/// as the sentence it came from, so the sentence is shown.
export function WhereSection({ job }: { job: Job }) {
  const { data: countries } = useCountries();
  const home = countries?.countries.find((c) => c.code === countries.profile_country);
  const workable = job.workable;
  const places = workable?.places?.filter(Boolean) ?? [];

  let summary: ReactNode;
  let quote: string | undefined;
  switch (workable?.scope) {
    case "anywhere":
      summary = "Open to candidates anywhere.";
      break;
    case "listed":
      summary = `${job.remote_type === "remote" ? "Open in" : "Based in"} ${placesLabel(places)}.`;
      break;
    default:
      quote = workable?.note;
      summary = !quote
        ? "The posting doesn’t say where it can be done from."
        : quote === job.title
          ? `The listing on ${jobSource(job)} and the title disagree about where this can be done from:`
          : "It isn’t clear where this can be done from. The posting says:";
  }
  const openToYou =
    home && workable && (workable.scope === "anywhere" || (workable.countries ?? []).includes(home.code));
  const listed = job.locations?.filter(Boolean) ?? [];

  const offers = [
    { topic: "relocation" as const, offer: job.relocation },
    { topic: "visa" as const, offer: job.visa_sponsorship },
  ].filter(({ offer }) => offer?.status);

  return (
    <DetailSection title="Where you can work">
      <div className="space-y-3 text-[13px] leading-snug">
        <div>
          <p className="text-subtle-foreground select-text">
            {summary}
            {home && workable?.scope !== "unknown" && (
              <span className={openToYou ? "text-success" : "text-warning"}>
                {" "}
                {openToYou
                  ? `You can do it from ${home.name}.`
                  : job.remote_type === "remote"
                    ? `Not open in ${home.name}.`
                    : `You’d have to move from ${home.name}.`}
              </span>
            )}
          </p>
          {quote && <Evidence>{quote}</Evidence>}
          {workable?.scope !== "anywhere" && listed.length > 0 && (
            <SourceLocations source={capitalize(jobSource(job))} locations={listed} />
          )}
        </div>
        {job.hours?.status && <HoursLine hours={job.hours} />}
        {offers.map(({ topic, offer }) => (
          <div key={topic}>
            <p
              className={
                offer.status === "offered"
                  ? "font-medium text-success"
                  : offer.status === "declined"
                    ? "font-medium text-subtle-foreground"
                    : "font-medium text-foreground"
              }
            >
              {offerLabel(topic, offer.status)}
            </p>
            {offer.evidence && <Evidence>{offer.evidence}</Evidence>}
          </div>
        ))}
      </div>
    </DetailSection>
  );
}

/// HoursLine is the working hours a job asks for, against the candidate's
/// own time zone when the profile has one (ADR-0025). Where and when stay
/// separate lines: an anywhere job asking for CET hours is both.
function HoursLine({ hours }: { hours: WorkingHours }) {
  const { data: countries } = useCountries();
  const { data: profile } = useProfile();
  const zone = countries?.time_zones.find((z) => z.id === profile?.time_zone);
  const shift = zone ? hoursShiftFor(hours, zone.offset) : undefined;
  const required = hours.status === "required";

  return (
    <div>
      <p className="font-medium text-foreground">
        {required ? "Hours required" : "Hours preferred"}: {hoursWindow(hours)}
        {shift !== undefined && (
          <span className={shift === 0 ? "font-normal text-success" : "font-normal text-warning"}>
            {" "}
            {shift === 0
              ? "· inside your time zone."
              : `· ${shift === 1 ? "an hour" : `${formatHours(shift)} hours`} from yours${required ? "" : ", but only preferred"}.`}
          </span>
        )}
      </p>
      {hours.evidence && <Evidence>{hours.evidence}</Evidence>}
    </div>
  );
}

function formatHours(hours: number): string {
  return Number.isInteger(hours) ? String(hours) : hours.toFixed(1);
}

/// SourceLocations is what the source itself listed, read out when it is
/// short and behind a disclosure when it is a long list of countries.
function SourceLocations({ source, locations }: { source: string; locations: string[] }) {
  if (locations.length <= 6) {
    return (
      <p className="mt-1 text-xs text-muted-foreground select-text">
        {source} lists: {locations.join(" · ")}
      </p>
    );
  }
  return (
    <details className="mt-1 text-xs text-muted-foreground">
      <summary className="cursor-default select-none hover:text-subtle-foreground">
        {source} lists {locations.length} countries
      </summary>
      <p className="mt-1 leading-relaxed select-text">{locations.join(" · ")}</p>
    </details>
  );
}

/// Evidence is the posting's own sentence behind a label.
function Evidence({ children }: { children: string }) {
  return (
    <blockquote className="mt-1 border-l-2 border-border pl-2.5 text-xs leading-relaxed text-muted-foreground select-text">
      “{children}”
    </blockquote>
  );
}

function capitalize(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1);
}
