import { open } from "@tauri-apps/plugin-dialog";
import { Button } from "../components/ui/button";
import { Card, CardContent } from "../components/ui/card";
import { useCandidateId, useDeleteResume, useImportResume, useResumes } from "../lib/queries";
import { useToast } from "../lib/toast";

export function Resumes() {
  const { notify } = useToast();
  const { candidateId } = useCandidateId();
  const { data: resumes } = useResumes(candidateId);

  const importMutation = useImportResume({
    onSuccess: () => notify("Resume imported.", "success"),
    onError: (err) => notify(String(err)),
  });
  const deleteMutation = useDeleteResume({
    onError: (err) => notify(String(err)),
  });

  const importResume = async () => {
    if (!candidateId) return;
    const path = await open({ multiple: false, title: "Select a resume file" });
    if (!path || Array.isArray(path)) return;
    importMutation.mutate({ candidateId, filePath: path });
  };

  const remove = (id: string) => {
    if (!candidateId) return;
    deleteMutation.mutate({ id, candidateId });
  };

  if (!candidateId) {
    return <p className="text-sm text-slate-500">No candidate profile found — create one on the Profile page first.</p>;
  }

  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <h1 className="text-xl font-semibold">Resumes</h1>
        <Button onClick={importResume} disabled={importMutation.isPending}>
          {importMutation.isPending ? "Importing…" : "Import resume"}
        </Button>
      </div>
      {!resumes || resumes.length === 0 ? (
        <p className="text-sm text-slate-500">No resumes imported yet.</p>
      ) : (
        <div className="space-y-2">
          {resumes.map((r) => (
            <Card key={r.id}>
              <CardContent className="flex items-center justify-between">
                <div>
                  <p className="font-medium">{r.name}</p>
                  <p className="text-xs text-slate-500">{r.original_file_path}</p>
                </div>
                <Button variant="destructive" onClick={() => remove(r.id)}>
                  Delete
                </Button>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
