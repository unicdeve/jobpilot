import { ArrowLeft, ChevronRight, Ellipsis, Send } from "lucide-react";
import { useState } from "react";
import { Link, useLocation, useNavigate, useParams } from "react-router-dom";
import { DeletionDialog } from "../../components/deletion-dialog";
import { RecordGone } from "../../components/list-pane";
import { Toolbar } from "../../components/page";
import { splitBackClass } from "../../components/split-view";
import { StatusBadge } from "../../components/status-badge";
import { ErrorLine, Loading } from "../../components/status-line";
import { Button } from "../../components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../../components/ui/dropdown-menu";
import { isNotFound } from "../../lib/api";
import { dateTime, relativeTime } from "../../lib/format";
import { useApplication, useApplicationHistory } from "../../lib/queries/application.queries";
import { useCoverLetters } from "../../lib/queries/cover-letter.queries";
import { useJob } from "../../lib/queries/job.queries";
import { useQuestions } from "../../lib/queries/question.queries";
import { useToast } from "../../lib/toast";
import { ApplySection } from "./apply-section";
import { CoverLetterSection } from "./cover-letter-section";
import { HistorySection } from "./history-section";
import { NextStep } from "./next-step";
import { QuestionsSection } from "./questions-section";

export function ApplicationDetail() {
  const { id } = useParams<{ id: string }>();
  const { search } = useLocation();

  // Each section's mutations invalidate exactly the queries they affect, so
  // there is no whole-screen reload after every action.
  const { data: application, error } = useApplication(id);
  const { data: history } = useApplicationHistory(id);
  const { data: coverLetters } = useCoverLetters(id);
  const { data: questions } = useQuestions(id);
  const { data: job } = useJob(application?.job_id);

  const back = (
    <Button
      size="icon-sm"
      variant="ghost"
      className={splitBackClass}
      aria-label="Back to applications"
      render={<Link to={{ pathname: "/applications", search }} />}
    >
      <ArrowLeft />
    </Button>
  );

  if (isNotFound(error)) {
    return (
      <RecordGone
        back={back}
        icon={<Send />}
        title="This application no longer exists"
        description="It may have been deleted. Choose another application from the list."
      />
    );
  }
  if (error) return <ErrorLine error={error} />;
  if (!application || !id) return <Loading />;

  return (
    <div className="min-h-full">
      <Toolbar
        className="px-5"
        leading={back}
        actions={
          <>
            <Button size="sm" variant="ghost" render={<Link to={`/jobs/${application.job_id}`} />}>
              View job
              <ChevronRight data-icon="inline-end" />
            </Button>
            <ApplicationMenu applicationId={id} title={job?.title} />
          </>
        }
      />

      <article className="max-w-3xl px-8 pt-6 pb-12">
        <header>
          <div className="flex items-start gap-3">
            <h2 className="min-w-0 flex-1 text-xl leading-tight font-semibold select-text">
              {job ? job.title : "Application"}
            </h2>
            <StatusBadge value={application.state} className="mt-1 shrink-0" />
          </div>
          <p className="mt-1 text-sm text-subtle-foreground">{job?.company_name}</p>
          <p className="mt-3 text-xs text-muted-foreground">
            Started <span title={dateTime(application.created_at)}>{relativeTime(application.created_at)}</span>
            {application.submitted_at && (
              <>
                {" · "}submitted{" "}
                <span title={dateTime(application.submitted_at)}>{relativeTime(application.submitted_at)}</span>
              </>
            )}
          </p>
        </header>

        <NextStep application={application} />

        {/* The candidate writes and sends the application themselves
            (ADR-0034): the page to apply on and their details come first,
            then their letter and answers. */}
        <ApplySection application={application} job={job} />

        <CoverLetterSection applicationId={id} application={application} versions={coverLetters ?? []} />

        <QuestionsSection applicationId={id} questions={questions ?? []} />

        {history && history.length > 0 && <HistorySection history={history} />}
      </article>
    </div>
  );
}

/// ApplicationMenu holds what is done to the application rather than with it.
function ApplicationMenu({ applicationId, title }: { applicationId: string; title?: string }) {
  const { notify } = useToast();
  const navigate = useNavigate();
  const { search } = useLocation();
  const [deleting, setDeleting] = useState(false);
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={<Button size="icon-sm" variant="ghost" aria-label="More actions" title="More actions" />}
        >
          <Ellipsis />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem variant="destructive" onClick={() => setDeleting(true)}>
            Delete application…
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <DeletionDialog
        target="application"
        id={applicationId}
        open={deleting}
        onOpenChange={setDeleting}
        title={title ? `Delete the application for “${title}”?` : "Delete this application?"}
        note="The job stays."
        confirmLabel="Delete application"
        onDeleted={() => {
          notify("Application deleted.", "success");
          navigate({ pathname: "/applications", search });
        }}
      />
    </>
  );
}
