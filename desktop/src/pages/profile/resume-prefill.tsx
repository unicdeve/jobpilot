import { Upload } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import { Panel } from "../../components/page";
import { Thinking } from "../../components/thinking";
import { Button } from "../../components/ui/button";
import { Checkbox } from "../../components/ui/checkbox";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "../../components/ui/dialog";
import { errorMessage } from "../../lib/api";
import { applyProfileDraft } from "../../lib/profile-draft";
import { useCheckLLMHealth } from "../../lib/queries/llm.queries";
import { useProposeProfileDraft } from "../../lib/queries/profile.queries";
import { useImportResume } from "../../lib/queries/resume.queries";
import { pickResumeFile } from "../../lib/resume-file";
import { useToast } from "../../lib/toast";
import type { Candidate, LLMHealth } from "../../lib/types";

/// ResumePrefill imports a resume and fills the profile form from what the
/// model reads in it. Nothing is saved: onDraft receives the filled-in
/// profile for the user to review and save.
///
/// A resume belongs to a profile, so one not created yet is created first,
/// by create, with whatever is typed so far.
export function ResumePrefill({
  profile,
  exists,
  create,
  onDraft,
}: {
  profile: Candidate;
  exists: boolean;
  create: () => Promise<Candidate>;
  onDraft: (profile: Candidate) => void;
}) {
  const { notify, notifyError } = useToast();
  const [overwriteExisting, setOverwriteExisting] = useState(false);
  // Prefilling reads the resume with the local LLM, so it is checked before
  // the file picker opens rather than after: the old order created the
  // profile, imported the resume, and only then failed on the part the user
  // actually asked for.
  const [llmProblem, setLlmProblem] = useState<LLMHealth | null>(null);
  const checkLLM = useCheckLLMHealth();
  const importResume = useImportResume();
  const proposeDraft = useProposeProfileDraft();

  const importing = importResume.isPending || proposeDraft.isPending;

  const prefill = async () => {
    let health: LLMHealth;
    try {
      health = await checkLLM.mutateAsync();
    } catch (err) {
      notify(`Could not check your AI provider: ${errorMessage(err)}`);
      return;
    }
    if (!health.ok) {
      setLlmProblem(health);
      return;
    }

    const file = await pickResumeFile();
    if (!file) return;

    try {
      let current = profile;
      if (!exists) {
        current = await create();
        onDraft(current);
      }

      const resume = await importResume.mutateAsync({ candidateId: current.id, file });
      notify("Resume imported.", "success");

      try {
        const suggestion = await proposeDraft.mutateAsync(resume.markdown_content);
        onDraft(applyProfileDraft(current, suggestion, overwriteExisting));
        notify("Profile prefilled from resume — review before saving.", "success");
      } catch (err) {
        notify(`Resume imported, but couldn't generate suggestions: ${errorMessage(err)}`);
      }
    } catch (err) {
      notifyError(err);
    }
  };

  return (
    <>
      <Dialog open={!!llmProblem} onOpenChange={(open) => !open && setLlmProblem(null)}>
        <DialogContent>
          <DialogTitle>
            {llmProblem?.configured ? "Can't reach your AI provider" : "Set up your AI provider first"}
          </DialogTitle>
          <DialogDescription>
            {llmProblem?.configured
              ? "Prefilling reads your resume with the model you configured, and it isn't answering. Check that it's running, then try again."
              : "Prefilling reads your resume with the model you choose in Settings. Choose a provider and model there, then come back and import."}
          </DialogDescription>
          {llmProblem?.error && (
            <p className="mt-3 rounded-md bg-muted px-3 py-2 font-mono text-xs break-words text-muted-foreground">
              {llmProblem.error}
            </p>
          )}
          <DialogFooter>
            <DialogClose render={<Button variant="ghost" />}>Not now</DialogClose>
            <Button render={<Link to="/settings?section=model" />}>Open settings</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Panel className="flex flex-wrap items-center gap-x-4 gap-y-3 px-4 py-3">
        <div className="min-w-0 flex-1">
          <p className="text-[13px] font-medium">{exists ? "Fill in from a resume" : "Start from your resume"}</p>
          {proposeDraft.isPending ? (
            <Thinking state="working" className="-my-0.5 text-xs">
              Reading your details from the resume…
            </Thinking>
          ) : (
            <p className="text-xs text-muted-foreground">
              JobPilot reads the resume and suggests what to put here. Nothing is saved until you review it and save.
            </p>
          )}
        </div>
        <label className="flex items-center gap-2 text-xs text-subtle-foreground">
          <Checkbox checked={overwriteExisting} onCheckedChange={(checked) => setOverwriteExisting(checked === true)} />
          Replace what’s already filled in
        </label>
        <Button size="sm" variant="outline" onClick={() => void prefill()} disabled={importing || checkLLM.isPending}>
          <Upload data-icon="inline-start" />
          {checkLLM.isPending ? "Checking…" : importing ? "Reading…" : "Import resume"}
        </Button>
      </Panel>
    </>
  );
}
