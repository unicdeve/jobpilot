import { useEffect, useState } from "react";
import { Panel } from "../../components/page";
import { SettingsHeading } from "../../components/settings-heading";
import { ErrorLine, Loading } from "../../components/status-line";
import { Checkbox } from "../../components/ui/checkbox";
import { Input } from "../../components/ui/input";
import { useNotificationSettings, useSaveNotificationSettings } from "../../lib/queries/notification.queries";
import { useToast } from "../../lib/toast";

/// NotificationSettingsSection is whether JobPilot tells the user what a
/// scout found while they were elsewhere, and what counts as a strong
/// match (ADR-0028 §6). They are the daemon's settings, so the CLI shares
/// them.
export function NotificationSettingsSection() {
  const { notifyError } = useToast();
  const { data: settings, error } = useNotificationSettings();
  const save = useSaveNotificationSettings({ onError: notifyError });
  // The score is edited here and saved when the field is left, so typing
  // "85" does not save an 8 on the way.
  const [score, setScore] = useState("");
  useEffect(() => {
    if (settings) setScore(String(settings.strong_match_score));
  }, [settings]);

  if (error) return <ErrorLine error={error} />;
  if (!settings) return <Loading />;

  const typed = Number(score);
  const scoreValid = Number.isInteger(typed) && typed >= 1 && typed <= 100;
  const saveScore = () => {
    if (scoreValid && typed !== settings.strong_match_score) save.mutate({ ...settings, strong_match_score: typed });
    if (!scoreValid) setScore(String(settings.strong_match_score));
  };

  return (
    <>
      <SettingsHeading
        title="Notifications"
        description="What a scout finds while you’re elsewhere. A notification says how many jobs and which scout, never the jobs themselves, since it can show on a locked screen."
      />
      <Panel className="divide-y divide-border">
        <label className="flex items-start gap-3 px-4 py-3">
          <Checkbox
            className="mt-0.5"
            checked={settings.enabled}
            disabled={save.isPending}
            onCheckedChange={(checked) => save.mutate({ ...settings, enabled: checked === true })}
          />
          <span>
            <span className="block text-[13px] font-medium">Notify me about scout runs</span>
            <span className="mt-0.5 block text-xs text-muted-foreground">
              When a scheduled scout finds jobs or can’t run, and when any run finds a strong match. Turned off, they
              still wait for you on the Dashboard.
            </span>
          </span>
        </label>
        <div className="flex items-center gap-4 px-4 py-3">
          <label htmlFor="strong-match-score" className="min-w-0 flex-1">
            <span className="block text-[13px] font-medium">A strong match scores at least</span>
            <span className="mt-0.5 block text-xs text-muted-foreground">Out of 100, as a job’s match is scored.</span>
          </label>
          <Input
            id="strong-match-score"
            type="number"
            min={1}
            max={100}
            className="w-20"
            value={score}
            aria-invalid={!scoreValid}
            onChange={(e) => setScore(e.target.value)}
            onBlur={saveScore}
            onKeyDown={(e) => e.key === "Enter" && e.currentTarget.blur()}
          />
        </div>
      </Panel>
    </>
  );
}
