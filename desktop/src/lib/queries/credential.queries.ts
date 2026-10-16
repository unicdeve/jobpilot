import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../daemon";
import type { CredentialRef } from "../types";
import { keys } from "./keys";
import type { MutationOpts } from "./mutation";

/// useCredentials loads which credentials are configured. It returns refs
/// and never values — there is no query that reads a secret, because the
/// daemon serves no route that returns one.
export function useCredentials() {
  const api = useApi();
  return useQuery<CredentialRef[]>({
    queryKey: keys.credentials,
    queryFn: () => api.listCredentials(),
  });
}

interface SetCredentialVars {
  source: string;
  label: string;
  secret: string;
}

/// useSetCredential stores an API key in the OS keychain.
export function useSetCredential(opts?: MutationOpts<CredentialRef, SetCredentialVars>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ source, label, secret }: SetCredentialVars) => api.setCredential(source, label, secret),
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.credentials });
      opts?.onSuccess?.(...args);
    },
  });
}

/// useDeleteCredential removes a stored key.
export function useDeleteCredential(opts?: MutationOpts<void, CredentialRef>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (ref: CredentialRef) => api.deleteCredential(ref.source, ref.label),
    ...opts,
    onSuccess: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.credentials });
      opts?.onSuccess?.(...args);
    },
  });
}
