import { openUrl } from "@tauri-apps/plugin-opener";
import { ChevronDown, ExternalLink } from "lucide-react";
import { type FormEvent, useState } from "react";
import { Link } from "react-router-dom";
import { CopyButton } from "../../components/copy-button";
import { DetailSection } from "../../components/page";
import { Button } from "../../components/ui/button";
import { Input } from "../../components/ui/input";
import { LINK_LABELS } from "../../lib/format";
import { useAboutYou } from "../../lib/queries/about-you.queries";
import { useSetApplyUrl } from "../../lib/queries/application.queries";
import { useProfile } from "../../lib/queries/profile.queries";
import { useToast } from "../../lib/toast";
import type { AboutYou, Application, Candidate, Job } from "../../lib/types";

/// ApplySection is where the candidate applies, themselves (ADR-0034 §2):
/// it opens the application page in their own browser, and keeps their
/// details beside it, ready to copy. JobPilot never types into the page and
/// never submits it.
export function ApplySection({ application, job }: { application: Application; job: Job | undefined }) {
  const { notifyError } = useToast();
  const link = application.apply_url || job?.canonical_url;

  const open = () => {
    if (link) openUrl(link).catch(notifyError);
  };

  return (
    <DetailSection
      title="Apply"
      description="Apply on the employer’s site, in your own words. Your details are below, ready to copy."
      actions={
        <Button size="sm" onClick={open} disabled={!link}>
          <ExternalLink data-icon="inline-start" />
          Open application page
        </Button>
      }
    >
      <ApplyLink application={application} jobLink={job?.canonical_url} />
      <YourDetails />
    </DetailSection>
  );
}

/// ApplyLink says which page "Open application page" opens, and lets the
/// candidate point it at the form itself when the job's link is only the
/// posting.
function ApplyLink({ application, jobLink }: { application: Application; jobLink: string | undefined }) {
  const { notifyError } = useToast();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const setApplyUrl = useSetApplyUrl({
    onSuccess: () => {
      setEditing(false);
      setDraft("");
    },
    onError: notifyError,
  });
  const ownLink = Boolean(application.apply_url);
  const shown = application.apply_url || jobLink;

  const submit = (event: FormEvent) => {
    event.preventDefault();
    setApplyUrl.mutate({ applicationId: application.id, url: draft.trim() });
  };

  if (editing) {
    return (
      <form className="mb-4 flex gap-2" onSubmit={submit}>
        <Input
          autoFocus
          type="url"
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          placeholder="https://jobs.example.com/…/application"
          aria-label="Application page address"
        />
        <Button size="sm" type="submit" className="shrink-0" disabled={!draft.trim() || setApplyUrl.isPending}>
          {setApplyUrl.isPending ? "Saving…" : "Save"}
        </Button>
        <Button size="sm" type="button" variant="ghost" className="shrink-0" onClick={() => setEditing(false)}>
          Cancel
        </Button>
      </form>
    );
  }

  return (
    <p className="mb-4 text-[13px] text-muted-foreground">
      {ownLink ? "Opens your link: " : "Opens the job’s link: "}
      {shown ? <span className="break-all text-foreground select-text">{shown}</span> : <span>none yet</span>}
      {" · "}
      <Button
        size="xs"
        variant="link"
        className="h-auto px-0 text-[13px]"
        onClick={() => {
          setDraft(application.apply_url ?? "");
          setEditing(true);
        }}
      >
        {ownLink ? "Change" : "Use a different link"}
      </Button>
      {ownLink && (
        <>
          {" · "}
          <Button
            size="xs"
            variant="link"
            className="h-auto px-0 text-[13px]"
            onClick={() => setApplyUrl.mutate({ applicationId: application.id, url: "" })}
            disabled={setApplyUrl.isPending}
          >
            Use the job’s own link
          </Button>
        </>
      )}
    </p>
  );
}

interface Detail {
  label: string;
  value: string;
}

/// profileDetails is what an application form asks for that the profile
/// holds, in the order forms ask for it, leaving out what is empty.
function profileDetails(profile: Candidate): Detail[] {
  const details: Detail[] = [];
  const add = (label: string, value: string | null | undefined) => {
    if (value?.trim()) details.push({ label, value: value.trim() });
  };
  add("First name", profile.first_name);
  add("Last name", profile.last_name);
  add("Email", profile.email);
  add("Phone", profile.phone);
  add(
    "Location",
    [profile.location_city, profile.location_region, profile.location_country]
      .filter((part) => part?.trim())
      .join(", "),
  );
  for (const link of profile.links ?? []) add(LINK_LABELS[link.kind] ?? "Link", link.url);
  const current = profile.experiences?.find((experience) => experience.is_current);
  add("Current title", current?.title);
  add("Current company", current?.company);
  if (profile.earliest_start_date) add("Available from", profile.earliest_start_date);
  if (profile.notice_period_days != null) {
    const days = profile.notice_period_days;
    add("Notice period", days <= 0 ? "Immediately" : days === 1 ? "1 day" : `${days} days`);
  }
  for (const authorization of profile.work_authorizations ?? []) {
    if (!authorization.country.trim()) continue;
    add(
      `Work in ${authorization.country}`,
      `${authorization.is_authorized ? "Authorized" : "Not authorized"}, ${
        authorization.requires_sponsorship ? "needs sponsorship" : "no sponsorship needed"
      }`,
    );
  }
  return details;
}

/// ownAnswers is what the candidate wrote under About you: their standing
/// answers and the answers they kept.
function ownAnswers(about: AboutYou | null | undefined): Detail[] {
  if (!about) return [];
  return [
    ...about.questions.filter((q) => q.answer.trim()).map((q) => ({ label: q.question, value: q.answer })),
    ...about.kept.map((kept) => ({ label: kept.question, value: kept.answer })),
  ];
}

/// YourDetails is the candidate's own material, a click from the clipboard.
/// Their written answers are folded away until asked for: they are long,
/// and a form needs them less often than an email address.
function YourDetails() {
  const { data: profile } = useProfile();
  const { data: about } = useAboutYou();
  const [showAnswers, setShowAnswers] = useState(false);

  if (!profile) return null;
  const details = profileDetails(profile);
  const answers = ownAnswers(about);

  return (
    <div className="overflow-hidden rounded-lg border border-border bg-card">
      {details.length === 0 ? (
        <p className="px-4 py-3 text-[13px] text-muted-foreground">
          Your profile has no details to copy yet. Add them under{" "}
          <Link to="/profile" className="text-foreground underline">
            Profile
          </Link>
          .
        </p>
      ) : (
        <dl className="divide-y divide-border">
          {details.map((detail) => (
            <DetailRow key={`${detail.label}:${detail.value}`} detail={detail} />
          ))}
        </dl>
      )}

      <div className="border-t border-border">
        <button
          type="button"
          className="flex w-full items-center justify-between gap-3 px-4 py-2.5 text-left text-[13px] font-medium hover:bg-accent/50"
          onClick={() => setShowAnswers(!showAnswers)}
          aria-expanded={showAnswers}
        >
          Your answers from About you
          <span className="flex items-center gap-1.5 text-xs font-normal text-muted-foreground">
            {answers.length}
            <ChevronDown className={`size-4 transition-transform ${showAnswers ? "rotate-180" : ""}`} />
          </span>
        </button>
        {showAnswers &&
          (answers.length === 0 ? (
            <p className="border-t border-border px-4 py-3 text-[13px] text-muted-foreground">
              Nothing yet. Write your own answers under{" "}
              <Link to="/profile?tab=about" className="text-foreground underline">
                About you
              </Link>
              , and keep the ones you write here.
            </p>
          ) : (
            <dl className="divide-y divide-border border-t border-border">
              {answers.map((answer) => (
                <DetailRow key={`${answer.label}:${answer.value}`} detail={answer} long />
              ))}
            </dl>
          ))}
      </div>
    </div>
  );
}

/// DetailRow is one thing to copy. A short value sits beside its label; a
/// written answer, under its question.
function DetailRow({ detail, long }: { detail: Detail; long?: boolean }) {
  if (long) {
    return (
      <div className="flex items-start gap-3 px-4 py-2.5">
        <div className="min-w-0 flex-1">
          <dt className="text-xs font-medium text-muted-foreground">{detail.label}</dt>
          <dd className="mt-0.5 line-clamp-4 text-[13px] whitespace-pre-wrap text-subtle-foreground select-text">
            {detail.value}
          </dd>
        </div>
        <CopyButton text={detail.value} label={detail.label} />
      </div>
    );
  }
  return (
    <div className="flex items-center gap-3 px-4 py-2">
      <dt className="w-32 shrink-0 text-xs text-muted-foreground">{detail.label}</dt>
      <dd className="min-w-0 flex-1 truncate text-[13px] select-text" title={detail.value}>
        {detail.value}
      </dd>
      <CopyButton text={detail.value} label={detail.label} />
    </div>
  );
}
