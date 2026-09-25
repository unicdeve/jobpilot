import { createContext, useContext } from "react";

/// PortalContainer is where dialogs, menus, popovers and selects put their
/// popups. The app shell gives each section its own, so that a popup opened
/// in a section is hidden with it when the user switches away — a portal
/// renders outside its section's DOM, beyond the reach of the <Activity>
/// that hides the section — and is there again when they come back.
/// Outside a section it is unset and popups go to <body>.
export const PortalContainer = createContext<HTMLElement | null>(null);

export function usePortalContainer(): HTMLElement | undefined {
  return useContext(PortalContainer) ?? undefined;
}
