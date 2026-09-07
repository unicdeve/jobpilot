import { open } from "@tauri-apps/plugin-dialog";
import { useEffect, useState } from "react";
import { Button } from "../components/ui/button";
import { Card } from "../components/ui/card";
import { useApi } from "../lib/daemon";
import { useToast } from "../lib/toast";
import type { Resume } from "../lib/types";

export function Resumes() {
  const api = useApi();
  const { notify } = useToast();
  const [candidateId, setCandidateId] = useState<string | null>(null);
  const [resumes, setResumes] = useState<Resume[]>([]);
  const [importing, setImporting] = useState(false);

  const refresh = async (cid: string) => {
    setResumes(await api.listResumes(cid));
  };

  useEffect(() => {
    api
      .getProfile()
      .then(async (profile) => {
        setCandidateId(profile.id);
        await refresh(profile.id);
      })
      .catch((err) => notify(String(err)));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api]);

  const importResume = async () => {
    if (!candidateId) return;
    const path = await open({ multiple: false, title: "Select a resume file" });
    if (!path || Array.isArray(path)) return;

    setImporting(true);
    try {
      await api.importResume(candidateId, path);
      await refresh(candidateId);
      notify("Resume imported.", "success");
    } catch (err) {
      notify(String(err));
    } finally {
      setImporting(false);
    }
  };

  const remove = async (id: string) => {
    if (!candidateId) return;
    try {
      await api.deleteResume(id);
      await refresh(candidateId);
    } catch (err) {
      notify(String(err));
    }
  };

  if (!candidateId) {
    return <p className="text-sm text-slate-500">No candidate profile found — create one on the Profile page first.</p>;
  }

  return (
    <div>
      <div className="mb-4 flex items-center justify-between">
        <h1 className="text-xl font-semibold">Resumes</h1>
        <Button onClick={importResume} disabled={importing}>
          {importing ? "Importing…" : "Import resume"}
        </Button>
      </div>
      {resumes.length === 0 ? (
        <p className="text-sm text-slate-500">No resumes imported yet.</p>
      ) : (
        <div className="space-y-2">
          {resumes.map((r) => (
            <Card key={r.id} className="flex items-center justify-between">
              <div>
                <p className="font-medium">{r.name}</p>
                <p className="text-xs text-slate-500">{r.original_file_path}</p>
              </div>
              <Button variant="danger" onClick={() => remove(r.id)}>
                Delete
              </Button>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}
