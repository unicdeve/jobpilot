import type { UseMutationOptions } from "@tanstack/react-query";

/// MutationOpts is the per-call slice of mutation options a screen supplies
/// (onSuccess/onError for toasts and navigation). Each mutation hook owns
/// its own cache invalidation, so callers never have to remember it.
export type MutationOpts<TData, TVars> = Omit<UseMutationOptions<TData, Error, TVars>, "mutationFn">;
