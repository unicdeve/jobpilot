import { skipToken, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../daemon";
import type { Resume } from "../types";
import { keys } from "./keys";
import type { MutationOpts } from "./mutation";

export function useResumes(candidateId: string | null) {
  const api = useApi();
  return useQuery<Resume[]>({
    queryKey: keys.resumes.list(candidateId ?? ""),
    queryFn: candidateId !== null ? () => api.listResumes(candidateId) : skipToken,
  });
}

interface ImportResumeVars {
  candidateId: string;
  filePath: string;
}

export function useImportResume(opts?: MutationOpts<Resume, ImportResumeVars>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ candidateId, filePath }: ImportResumeVars) => api.importResume(candidateId, filePath),
    ...opts,
    onSuccess: (resume, variables, ...rest) => {
      void queryClient.invalidateQueries({ queryKey: keys.resumes.list(variables.candidateId) });
      opts?.onSuccess?.(resume, variables, ...rest);
    },
  });
}
