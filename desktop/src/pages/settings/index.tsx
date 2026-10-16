import { cn } from "cn";
import { Bell, Database, Info, KeyRound, MessageSquareText, Palette, Search, Sparkles } from "lucide-react";
import { useSearchParams } from "react-router-dom";
import { CredentialSettings } from "../../components/credential-settings";
import { DataSettings } from "../../components/data-settings";
import { Page } from "../../components/page";
import { PromptSettings } from "../../components/prompt-settings";
import { AboutSection } from "./about-section";
import { AppearanceSettings } from "./appearance-settings";
import { JobSearchSettings } from "./job-search-settings";
import { ModelSettings } from "./model-settings";
import { NotificationSettingsSection } from "./notification-settings";

const SECTIONS = [
  { key: "appearance", label: "Appearance", icon: Palette },
  { key: "model", label: "AI model", icon: Sparkles },
  { key: "prompts", label: "Prompts", icon: MessageSquareText },
  { key: "searches", label: "Job searches", icon: Search },
  { key: "notifications", label: "Notifications", icon: Bell },
  { key: "keys", label: "API keys", icon: KeyRound },
  { key: "data", label: "Data", icon: Database },
  { key: "about", label: "About", icon: Info },
] as const;

type SectionKey = (typeof SECTIONS)[number]["key"];

/// Settings is one category at a time, chosen from a list beside it, as
/// System Settings is. The category is in the URL so another screen can
/// link straight to it — Jobs' "Manage searches…" does.
export function Settings() {
  const [params, setParams] = useSearchParams();
  const requested = params.get("section");
  const section: SectionKey = SECTIONS.some((s) => s.key === requested) ? (requested as SectionKey) : "appearance";

  return (
    <Page title="Settings" width="full" className="px-0 pt-0">
      <div className="flex min-h-[calc(100vh-3.25rem)]">
        <nav className="w-52 shrink-0 border-r border-border px-2.5 py-4">
          {SECTIONS.map(({ key, label, icon: Icon }) => (
            <button
              key={key}
              type="button"
              onClick={() => setParams({ section: key }, { replace: true })}
              className={cn(
                "flex h-7 w-full items-center gap-2.5 rounded-md px-2.5 text-left text-[13px] outline-none focus-visible:ring-2 focus-visible:ring-ring",
                section === key ? "bg-accent font-medium" : "text-subtle-foreground hover:bg-accent/60",
              )}
            >
              <Icon
                className={cn("size-4", section === key ? "text-primary" : "text-muted-foreground")}
                strokeWidth={1.75}
              />
              {label}
            </button>
          ))}
        </nav>
        <div className="min-w-0 flex-1 px-8 py-6">
          <div className="max-w-2xl">
            {section === "appearance" && <AppearanceSettings />}
            {section === "model" && <ModelSettings />}
            {section === "prompts" && <PromptSettings />}
            {section === "searches" && <JobSearchSettings />}
            {section === "notifications" && <NotificationSettingsSection />}
            {section === "keys" && <CredentialSettings />}
            {section === "data" && <DataSettings />}
            {section === "about" && <AboutSection />}
          </div>
        </div>
      </div>
    </Page>
  );
}
