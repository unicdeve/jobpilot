import { openUrl } from "@tauri-apps/plugin-opener";
import { cn } from "cn";
import { Ellipsis, Plus } from "lucide-react";
import { useState } from "react";
import { Panel } from "./page";
import { SettingsHeading } from "./settings-heading";
import { ErrorLine, Loading } from "./status-line";
import { Button } from "./ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "./ui/dropdown-menu";
import { Input } from "./ui/input";
import { dateTime, relativeTime } from "../lib/format";
import {
  useCompanies,
  useCompanySuggestions,
  useRenameCompany,
  useUnwatchCompany,
  useWatchCompany,
} from "../lib/queries";
import { useToast } from "../lib/toast";
import type { BoardProvider, StarterCompany, WatchedCompany } from "../lib/types";

const PROVIDER_NAMES: Record<BoardProvider, string> = {
  greenhouse: "Greenhouse",
  lever: "Lever",
  ashby: "Ashby",
};

/// SUGGESTIONS_SHOWN is how many suggestions show before the rest are
/// asked for: the list is a place to start, not a second job board.
const SUGGESTIONS_SHOWN = 6;

/// CompanySettings manages the companies whose own hiring pages JobPilot
/// reads (docs/02_features/17_company_watchlists.md). A watched company is a
/// source, not a search: its openings reach the candidate through the saved
/// searches above, like any job board's.
export function CompanySettings() {
  const { notify } = useToast();
  const { data: companies, error, isPending } = useCompanies();
  const [adding, setAdding] = useState(false);
  const [address, setAddress] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const close = () => {
    setAdding(false);
    setAddress("");
    setFormError(null);
  };
  // A mistake in the address is said beside the field it is in.
  const watch = useWatchCompany({
    onSuccess: (company) => {
      close();
      notify(`Watching ${company.name}. Your searches now look at its hiring page too.`, "success");
    },
    onError: (err) => setFormError(err.message),
  });

  return (
    <section className="mt-10">
      <SettingsHeading
        title="Companies you watch"
        description="JobPilot reads each company’s own hiring page on Greenhouse, Lever or Ashby. Your searches find its openings as they do a job board’s; watching a company imports nothing by itself."
        actions={
          !adding && (
            <Button size="sm" variant="outline" onClick={() => setAdding(true)}>
              <Plus data-icon="inline-start" />
              Watch a company
            </Button>
          )
        }
      />

      {adding && (
        <form
          className="mb-4 rounded-lg border border-border bg-card p-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (address.trim() && !watch.isPending) watch.mutate({ address: address.trim() });
          }}
        >
          <label htmlFor="company-address" className="text-[13px] font-medium">
            Hiring page address
          </label>
          <p className="mt-0.5 text-xs text-muted-foreground">
            The company’s page, or any job on it: boards.greenhouse.io, job-boards.greenhouse.io, jobs.lever.co or
            jobs.ashbyhq.com.
          </p>
          <div className="mt-2.5 flex items-center gap-2">
            <Input
              id="company-address"
              autoFocus
              spellCheck={false}
              placeholder="https://job-boards.greenhouse.io/gitlab"
              value={address}
              aria-invalid={formError !== null}
              aria-describedby={formError ? "company-address-error" : undefined}
              onChange={(e) => {
                setAddress(e.target.value);
                setFormError(null);
              }}
            />
            <Button type="submit" disabled={!address.trim() || watch.isPending}>
              {watch.isPending ? "Checking…" : "Watch"}
            </Button>
            <Button type="button" variant="ghost" onClick={close}>
              Cancel
            </Button>
          </div>
          {formError && (
            <p id="company-address-error" role="alert" className="mt-2 text-xs text-destructive">
              {formError}
            </p>
          )}
        </form>
      )}

      {error ? (
        <ErrorLine error={error} />
      ) : isPending ? (
        <Loading />
      ) : companies.length === 0 ? (
        !adding && (
          <Panel className="px-4 py-6 text-center">
            <p className="text-sm text-muted-foreground">
              You watch no companies yet. Paste a hiring page’s address, press “Watch this company” on a job, or start
              from a suggestion below.
            </p>
          </Panel>
        )
      ) : (
        <Panel className="divide-y divide-border">
          {companies.map((company) => (
            <CompanyRow key={company.id} company={company} />
          ))}
        </Panel>
      )}

      <Suggestions />
    </section>
  );
}

function CompanyRow({ company }: { company: WatchedCompany }) {
  const { notify } = useToast();
  const [renaming, setRenaming] = useState(false);
  const [name, setName] = useState(company.name);
  const rename = useRenameCompany({
    onSuccess: () => setRenaming(false),
    onError: (err) => notify(err.message),
  });
  const unwatch = useUnwatchCompany({
    onSuccess: () => notify(`No longer watching ${company.name}. Jobs already found from it are kept.`, "success"),
    onError: (err) => notify(err.message),
  });

  if (renaming) {
    return (
      <form
        className="flex items-center gap-2 px-4 py-2.5"
        onSubmit={(e) => {
          e.preventDefault();
          if (name.trim()) rename.mutate({ id: company.id, name });
        }}
      >
        <Input aria-label="Company name" autoFocus value={name} onChange={(e) => setName(e.target.value)} />
        <Button type="submit" size="sm" disabled={!name.trim() || rename.isPending}>
          Save
        </Button>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          onClick={() => {
            setRenaming(false);
            setName(company.name);
          }}
        >
          Cancel
        </Button>
      </form>
    );
  }

  const check = describeCheck(company);
  return (
    <div className="flex items-center gap-3 px-4 py-3">
      <div className="min-w-0 flex-1">
        <p className="truncate text-[13px] font-medium">
          {company.name}
          <span className="ml-2 text-xs font-normal text-muted-foreground">{PROVIDER_NAMES[company.board.provider]}</span>
        </p>
        <p
          className={cn("mt-0.5 truncate text-xs", check.problem ? "text-warning" : "text-muted-foreground")}
          title={dateTime(company.checked_at)}
        >
          {check.text}
        </p>
      </div>
      <DropdownMenu>
        <DropdownMenuTrigger render={<Button size="icon-sm" variant="ghost" aria-label={`More for ${company.name}`} />}>
          <Ellipsis />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onClick={() => void openUrl(company.url)}>Open hiring page</DropdownMenuItem>
          <DropdownMenuItem onClick={() => setRenaming(true)}>Rename</DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant="destructive" disabled={unwatch.isPending} onClick={() => unwatch.mutate(company.id)}>
            Stop watching
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

/// describeCheck says how a company's page was last read. A page that has
/// gone is the one the candidate can act on: the company may have moved to
/// another provider, whose address they can watch instead.
function describeCheck(company: WatchedCompany): { text: string; problem: boolean } {
  const when = relativeTime(company.checked_at);
  if (!company.checked_at || !when) return { text: "Not checked yet", problem: false };
  switch (company.check) {
    case "ok":
      return { text: `${openRoles(company.open_roles)} · checked ${when}`, problem: false };
    case "gone":
      return { text: `Its hiring page was gone ${when}. The company may have moved to another provider.`, problem: true };
    case "unreachable":
      return { text: `Couldn’t be reached ${when}`, problem: true };
    default:
      return { text: `Answered with an error ${when}`, problem: true };
  }
}

function openRoles(count: number): string {
  return count === 1 ? "1 open role" : `${count} open roles`;
}

/// Suggestions is the starter list: companies a measurement found open to
/// work from anywhere or across continents. Each says what was measured
/// and when, never more (spec D2).
function Suggestions() {
  const { notify } = useToast();
  const { data: suggestions } = useCompanySuggestions();
  const [all, setAll] = useState(false);
  const watch = useWatchCompany({
    onSuccess: (company) => notify(`Watching ${company.name}. Your searches now look at its hiring page too.`, "success"),
    onError: (err) => notify(err.message),
  });

  if (!suggestions || suggestions.length === 0) return null;
  const shown = all ? suggestions : suggestions.slice(0, SUGGESTIONS_SHOWN);
  const pending = watch.isPending ? watch.variables : undefined;
  // The list is measured in one go, so its date is said once; a row
  // measured on another day says its own.
  const measuredOn = suggestions.reduce((latest, s) => (s.measured_on > latest ? s.measured_on : latest), "");

  return (
    <div className="mt-6">
      <h3 className="text-[13px] font-medium">Suggestions</h3>
      <p className="mt-0.5 mb-2.5 text-xs text-muted-foreground">
        Companies whose own pages had roles that could be done from anywhere, or from more than one continent, when
        JobPilot measured them on {formatDay(measuredOn)}. That is all each line says about them.
      </p>
      <Panel className="divide-y divide-border">
        {shown.map((suggestion) => (
          <div key={suggestion.url} className="flex items-center gap-3 px-4 py-2.5">
            <div className="min-w-0 flex-1">
              <p className="truncate text-[13px] font-medium">
                {suggestion.name}
                <span className="ml-2 text-xs font-normal text-muted-foreground">
                  {PROVIDER_NAMES[suggestion.board.provider]}
                </span>
              </p>
              <p className="mt-0.5 text-xs text-muted-foreground">{describeMeasurement(suggestion, measuredOn)}</p>
            </div>
            <Button
              size="sm"
              variant="ghost"
              disabled={watch.isPending}
              onClick={() => watch.mutate({ address: suggestion.url })}
            >
              {pending && "address" in pending && pending.address === suggestion.url ? "Checking…" : "Watch"}
            </Button>
          </div>
        ))}
      </Panel>
      {suggestions.length > SUGGESTIONS_SHOWN && (
        <Button size="sm" variant="link" className="mt-1.5 px-0" onClick={() => setAll(!all)}>
          {all ? "Show fewer" : `Show all ${suggestions.length}`}
        </Button>
      )}
    </div>
  );
}

/// describeMeasurement is what earned a suggestion its place, as measured.
function describeMeasurement(suggestion: StarterCompany, listMeasuredOn: string): string {
  const counted = `${suggestion.qualifying_roles} of ${openRoles(suggestion.open_roles)}`;
  return suggestion.measured_on === listMeasuredOn ? counted : `${counted}, measured ${formatDay(suggestion.measured_on)}`;
}

/// formatDay reads a YYYY-MM-DD day as the day it is wherever the reader
/// is, rather than as midnight UTC.
function formatDay(day: string): string {
  const [year, month, date] = day.split("-").map(Number);
  return new Date(year, month - 1, date).toLocaleDateString(undefined, { dateStyle: "medium" });
}
