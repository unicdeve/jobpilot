import { BorderBeam } from "border-beam";
import type { ReactNode } from "react";
import { useTheme } from "../lib/theme";

/// ThinkingFrame is apart from Thinking (thinking.tsx) because that is in
/// the shell, and this would load the beam into every window's first parse
/// for the one screen that uses it.
///
/// ThinkingFrame marks something JobPilot is replacing while it works, such
/// as a letter being rewritten: a glow breathes inside its border until the
/// new one arrives. It wraps one bordered element with a background, and
/// shows nothing while inactive. It is for the thing being replaced, not for
/// every wait; a Thinking line says what the wait is.
export function ThinkingFrame({ active, children }: { active: boolean; children: ReactNode }) {
  // The beam's own "auto" theme reads the OS, not the user's choice in
  // Settings → Appearance, so it is given the theme the window resolved to.
  const { resolved } = useTheme();
  // Breathing rather than travelling, and one held hue: the beam's rotating
  // types leave reduced motion to the app, and its hue shift drifts the
  // ocean palette to magenta, away from the app's blue.
  return (
    <BorderBeam size="pulse-inner" colorVariant="ocean" staticColors theme={resolved} active={active}>
      {children}
    </BorderBeam>
  );
}
