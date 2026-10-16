import { cn } from "cn";
import { Monitor, Moon, Sun } from "lucide-react";
import { SettingsHeading } from "../../components/settings-heading";
import { type ThemePreference, useTheme } from "../../lib/theme";

const THEMES: { value: ThemePreference; label: string; icon: typeof Sun }[] = [
  { value: "system", label: "System", icon: Monitor },
  { value: "light", label: "Light", icon: Sun },
  { value: "dark", label: "Dark", icon: Moon },
];

/// AppearanceSettings is the one setting kept in the desktop rather than the
/// daemon: how this window looks is a preference of this device, and the CLI
/// has no use for it.
export function AppearanceSettings() {
  const { preference, setPreference } = useTheme();
  return (
    <>
      <SettingsHeading
        title="Appearance"
        description="System follows your computer’s light or dark setting as it changes."
      />
      {/* Native radios, hidden inside cards that show the choice: arrow keys
          move between them as they do in any radio group. */}
      <fieldset className="grid min-w-0 grid-cols-3 gap-3">
        <legend className="sr-only">Theme</legend>
        {THEMES.map(({ value, label, icon: Icon }) => {
          const selected = preference === value;
          return (
            <label
              key={value}
              className={cn(
                "flex flex-col items-center gap-2 rounded-lg border bg-card p-3 text-[13px] has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring",
                selected ? "border-primary ring-1 ring-primary" : "border-border hover:bg-accent/40",
              )}
            >
              <input
                type="radio"
                name="theme"
                value={value}
                checked={selected}
                onChange={() => setPreference(value)}
                className="sr-only"
              />
              <ThemePreview theme={value} />
              <span className="flex items-center gap-1.5">
                <Icon className="size-3.5 text-muted-foreground" />
                {label}
              </span>
            </label>
          );
        })}
      </fieldset>
    </>
  );
}

/// ThemePreview is a miniature window in the theme it names. It is drawn
/// with fixed colours on purpose: it shows the other theme, not this one.
function ThemePreview({ theme }: { theme: ThemePreference }) {
  const half = (dark: boolean) => (
    <div className={cn("flex h-full flex-1", dark ? "bg-[#1f2024]" : "bg-[#fcfcfd]")}>
      <div className={cn("w-1/3 border-r", dark ? "border-white/10 bg-[#26272b]" : "border-black/8 bg-[#f1f2f4]")} />
      <div className="flex flex-1 flex-col gap-1 p-1.5">
        <div className={cn("h-1.5 w-3/4 rounded-sm", dark ? "bg-white/25" : "bg-black/20")} />
        <div className={cn("h-1.5 w-1/2 rounded-sm", dark ? "bg-white/12" : "bg-black/10")} />
        <div className="mt-auto h-2 w-1/3 rounded-sm bg-[#3b82f6]" />
      </div>
    </div>
  );
  return (
    <div className="flex h-16 w-full overflow-hidden rounded-md border border-border">
      {theme === "system" ? (
        <>
          {half(false)}
          {half(true)}
        </>
      ) : (
        half(theme === "dark")
      )}
    </div>
  );
}
