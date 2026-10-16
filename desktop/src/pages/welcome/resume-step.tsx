import { Thinking } from "../../components/thinking";
import { Button } from "../../components/ui/button";
import { errorMessage } from "../../lib/api";
import { applyProfileDraft } from "../../lib/profile-draft";
import { useProposeProfileDraft, useUpdateProfile } from "../../lib/queries/profile.queries";
import { useImportResume } from "../../lib/queries/resume.queries";
import { pickResumeFile } from "../../lib/resume-file";
import { useToast } from "../../lib/toast";
import type { Candidate } from "../../lib/types";

/// ResumeStep is optional, as it is in the CLI. Importing one also prefills
/// the profile, which is the only step here that needs the LLM — hence its
/// place after the provider step, and the explanation when that was skipped.
export function ResumeStep({
  profile,
  llmConfigured,
  imported,
  onNext,
}: {
  profile?: Candidate;
  llmConfigured: boolean;
  imported: boolean;
  onNext: () => void;
}) {
  const { notify, notifyError } = useToast();
  const importResume = useImportResume();
  const proposeDraft = useProposeProfileDraft();
  const updateProfile = useUpdateProfile();
  const busy = importResume.isPending || proposeDraft.isPending || updateProfile.isPending;

  const pick = async () => {
    const path = await pickResumeFile();
    if (!path || !profile) return;

    try {
      const resume = await importResume.mutateAsync({
        candidateId: profile.id,
        filePath: path,
      });
      notify("Resume imported.", "success");

      // A prefill failure must never undo the import, which succeeded and is
      // useful on its own — the same rule the CLI's onboard follows.
      try {
        const suggestion = await proposeDraft.mutateAsync(resume.markdown_content);
        // A field the user already typed in the previous step is theirs: a
        // suggestion only fills a blank, so nothing they entered is
        // overwritten.
        await updateProfile.mutateAsync(applyProfileDraft(profile, suggestion));
        notify("Profile prefilled from your resume — review it on the Profile screen.", "success");
      } catch (err) {
        notify(`Resume imported, but couldn't read details from it: ${errorMessage(err)}`);
      }
      onNext();
    } catch (err) {
      notifyError(err);
    }
  };

  return (
    <>
      <div>
        <h2 className="font-semibold">Import a resume</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Optional. JobPilot converts it to text, keeps it for applications, and reads your details out of it so you do
          not have to type them again.
        </p>
      </div>

      {imported && <p className="text-sm text-success">A resume is already imported.</p>}

      {!llmConfigured && (
        <p className="text-sm text-warning">
          Reading details out of a resume needs the AI provider, which is not set up yet. You can still import the file
          — it just will not prefill anything.
        </p>
      )}

      <div className="flex items-center gap-2">
        <Button variant="outline" onClick={() => void pick()} disabled={busy || !profile}>
          {busy ? "Importing…" : "Choose a file"}
        </Button>
        <Button onClick={onNext}>{imported ? "Continue" : "Skip this step"}</Button>
      </div>

      {proposeDraft.isPending && <Thinking state="working">Reading your details from the resume…</Thinking>}
    </>
  );
}
