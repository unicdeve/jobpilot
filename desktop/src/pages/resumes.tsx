import { FileText, Upload } from "lucide-react";
import { useState } from "react";
import { DeletionDialog } from "../components/deletion-dialog";
import { EmptyState } from "../components/empty-state";
import { Page, Panel } from "../components/page";
import { Loading } from "../components/status-line";
import { Button } from "../components/ui/button";
import { dateTime, relativeTime } from "../lib/format";
import { useCandidateId } from "../lib/queries/profile.queries";
import { useImportResume, useResumes } from "../lib/queries/resume.queries";
import { pickResumeFile } from "../lib/resume-file";
import { useToast } from "../lib/toast";
import type { Resume } from "../lib/types";

export function Resumes() {
  const { notify, notifyError } = useToast();
  const { candidateId } = useCandidateId();
  const { data: resumes, isPending } = useResumes(candidateId);
  const [deleting, setDeleting] = useState<Resume | null>(null);

  const importMutation = useImportResume({
    onSuccess: () => notify("Resume imported.", "success"),
    onError: notifyError,
  });

  const importResume = async () => {
    if (!candidateId) return;
    const file = await pickResumeFile();
    if (!file) return;
    importMutation.mutate({ candidateId, file });
  };

  if (!candidateId) {
    return (
      <EmptyState
        title="No candidate profile yet"
        description="A resume is imported against a candidate, so JobPilot needs a profile before it can hold one."
        actionLabel="Create your profile"
        actionTo="/profile"
      />
    );
  }

  const importButton = (
    <Button size="sm" onClick={importResume} disabled={importMutation.isPending}>
      <Upload data-icon="inline-start" />
      {importMutation.isPending ? "Importing…" : "Import resume"}
    </Button>
  );

  return (
    <Page title="Resumes" actions={importButton}>
      <p className="mb-5 text-sm text-muted-foreground">
        A resume is attached to the applications you start with it. PDF, Word and text files are read into plain text,
        which stays on this computer.
      </p>

      {isPending ? (
        <Loading />
      ) : !resumes || resumes.length === 0 ? (
        <Panel className="flex flex-col items-center gap-2 px-6 py-10 text-center">
          <FileText className="size-8 stroke-[1.25] text-muted-foreground/60" />
          <p className="text-sm font-medium text-subtle-foreground">No resumes yet</p>
          <p className="max-w-sm text-sm text-muted-foreground">Import one to attach it to your applications.</p>
        </Panel>
      ) : (
        <Panel className="divide-y divide-border">
          {resumes.map((r) => (
            <div key={r.id} className="flex items-center gap-3 px-4 py-3">
              <FileText className="size-5 shrink-0 text-muted-foreground" strokeWidth={1.5} />
              <div className="min-w-0 flex-1">
                <p className="truncate text-[13px] font-medium">{r.name}</p>
                <p className="truncate text-xs text-muted-foreground" title={r.original_file_path || undefined}>
                  <span title={dateTime(r.created_at)}>Imported {relativeTime(r.created_at)}</span>
                  {/* A resume uploaded from a browser has no path (ADR-0035 §4). */}
                  {r.original_file_path && <> · {r.original_file_path}</>}
                </p>
              </div>
              <Button size="sm" variant="ghost" onClick={() => setDeleting(r)}>
                Delete
              </Button>
            </div>
          ))}
        </Panel>
      )}

      <DeletionDialog
        target="resume"
        id={deleting?.id}
        open={deleting !== null}
        onOpenChange={(open) => !open && setDeleting(null)}
        title={`Delete “${deleting?.name}”?`}
        note="Applications that used it keep everything already written from it. The original file on your computer is not touched."
        confirmLabel="Delete resume"
        onDeleted={() => notify("Resume deleted.", "success")}
      />
    </Page>
  );
}
