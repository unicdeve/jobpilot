import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { FormField } from "../../components/form-field";
import { Button } from "../../components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "../../components/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "../../components/ui/select";
import { useCreateApplication } from "../../lib/queries/application.queries";
import { useResumes } from "../../lib/queries/resume.queries";
import { useToast } from "../../lib/toast";
/// StartApplicationButton asks which resume an application uses, in a
/// dialog, so the job's page is about the job rather than a form.
/// JobPilot never writes or fills the application (ADR-0034).
export function StartApplicationButton({
  candidateId,
  jobId,
  again,
}: {
  candidateId: string;
  jobId: string;
  again: boolean;
}) {
  const { notifyError } = useToast();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const { data: resumes } = useResumes(candidateId);
  // null rather than "" so the Select shows its placeholder: a resume is
  // optional on an application.
  const [resumeId, setResumeId] = useState<string | null>(null);

  // Base UI's Select renders the raw value in the trigger unless it is told
  // how values map to labels — which for a resume would mean showing its
  // UUID, the exact thing this screen exists to stop the user handling.
  const resumeLabels = Object.fromEntries((resumes ?? []).map((r) => [r.id, r.name]));

  const createApplication = useCreateApplication({
    // Going straight to the new application is the point of starting one
    // here: the next thing the user wants is the application page and their
    // details to copy into it.
    onSuccess: (application) => navigate(`/applications/${application.id}`),
    onError: notifyError,
  });

  const create = (event: React.FormEvent) => {
    event.preventDefault();
    createApplication.mutate({ jobId, candidateId, resumeId: resumeId ?? "" });
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <Button size="sm" onClick={() => setOpen(true)}>
        {again ? "Apply again" : "Start application"}
      </Button>
      <DialogContent>
        <form onSubmit={create}>
          <DialogTitle>{again ? "Start another application" : "Start an application"}</DialogTitle>
          <DialogDescription>
            You write and send the application yourself, on the employer’s site. JobPilot keeps your details ready to
            copy, your letter and answers in one place, and tracks what happens next.
          </DialogDescription>
          <div className="mt-4 flex flex-col gap-3">
            <FormField label="Resume" htmlFor="resume">
              <Select items={resumeLabels} value={resumeId} onValueChange={(value) => setResumeId(value)}>
                <SelectTrigger id="resume" className="w-full">
                  <SelectValue placeholder={resumes?.length ? "None" : "No resumes imported"} />
                </SelectTrigger>
                <SelectContent>
                  {resumes?.map((resume) => (
                    <SelectItem key={resume.id} value={resume.id}>
                      {resume.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FormField>
          </div>
          <DialogFooter>
            <DialogClose render={<Button type="button" variant="outline" />}>Cancel</DialogClose>
            <Button type="submit" disabled={createApplication.isPending}>
              {createApplication.isPending ? "Creating…" : "Create application"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
