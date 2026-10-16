import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../daemon";
import type { Candidate, ProfileDraft } from "../types";
import { keys } from "./keys";
import type { MutationOpts } from "./mutation";

/// useProfile loads the single candidate profile, which is null until one
/// is created. "No profile" is an ordinary first-run state, so it is data
/// rather than an error: an errored query with no data is refetched by
/// every component that mounts it and reads as pending meanwhile, and
/// RequireSetup, which unmounts the screens while the profile is pending,
/// turned that into a loop of requests that never settled.
export function useProfile() {
  const api = useApi();
  return useQuery<Candidate | null>({
    queryKey: keys.profile,
    queryFn: () => api.getProfile(),
  });
}

/// useCandidateId is the common case of useProfile: a screen that only needs
/// the candidate's ID to load its own data. It reads the shared cached
/// profile rather than fetching it again per screen.
export function useCandidateId() {
  const { data, isPending, isError } = useProfile();
  return { candidateId: data?.id ?? null, isPending, isError, noProfile: data === null };
}

/// The profile's country is what a search works from by default, so saving
/// the profile refreshes the countries too.
export function useUpdateProfile(opts?: MutationOpts<Candidate, Candidate>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (profile: Candidate) => api.updateProfile(profile),
    ...opts,
    onSuccess: (updated, ...rest) => {
      queryClient.setQueryData(keys.profile, updated);
      void queryClient.invalidateQueries({ queryKey: keys.countries });
      opts?.onSuccess?.(updated, ...rest);
    },
  });
}

export function useInitializeProfile(opts?: MutationOpts<Candidate, Partial<Candidate>>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (profile: Partial<Candidate>) => api.initializeProfile(profile),
    ...opts,
    onSuccess: (created, ...rest) => {
      queryClient.setQueryData(keys.profile, created);
      void queryClient.invalidateQueries({ queryKey: keys.countries });
      opts?.onSuccess?.(created, ...rest);
    },
  });
}

/// useProposeProfileDraft reads a resume's markdown and returns a
/// suggestion. Nothing is persisted, so there is nothing to invalidate.
export function useProposeProfileDraft(opts?: MutationOpts<ProfileDraft, string>) {
  const api = useApi();
  return useMutation({
    mutationFn: (resumeMarkdown: string) => api.proposeProfileDraft(resumeMarkdown),
    ...opts,
  });
}
