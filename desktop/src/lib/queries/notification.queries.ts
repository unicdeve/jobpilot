import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../daemon";
import type { Notification, NotificationSettings } from "../types";
import { keys } from "./keys";
import type { MutationOpts } from "./mutation";

/// useUnseenNotifications loads what JobPilot told the user that they have
/// not seen in the app yet. A scheduled scout can add one at any time, so
/// it is read again every half minute: a local read of a small table.
export function useUnseenNotifications() {
  const api = useApi();
  return useQuery<Notification[]>({
    queryKey: keys.notifications,
    queryFn: () => api.listNotifications(true),
    refetchInterval: 30_000,
  });
}

/// useMarkNotificationsSeen marks one notification seen, or every one when
/// called without an id.
export function useMarkNotificationsSeen(opts?: MutationOpts<void, string | undefined>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id?: string) => api.markNotificationsSeen(id),
    ...opts,
    onSettled: (...args) => {
      void queryClient.invalidateQueries({ queryKey: keys.notifications });
      opts?.onSettled?.(...args);
    },
  });
}

export function useNotificationSettings() {
  const api = useApi();
  return useQuery<NotificationSettings>({
    queryKey: keys.notificationSettings,
    queryFn: () => api.getNotificationSettings(),
  });
}

export function useSaveNotificationSettings(opts?: MutationOpts<NotificationSettings, NotificationSettings>) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (settings: NotificationSettings) => api.saveNotificationSettings(settings),
    ...opts,
    onSuccess: (saved, ...rest) => {
      queryClient.setQueryData(keys.notificationSettings, saved);
      opts?.onSuccess?.(saved, ...rest);
    },
  });
}
