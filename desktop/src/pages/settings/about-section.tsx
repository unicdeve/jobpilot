import { Panel } from "../../components/page";
import { SettingsHeading } from "../../components/settings-heading";
import { ErrorLine, Loading } from "../../components/status-line";
import { useVersions } from "../../lib/queries/version.queries";

/// AboutSection shows which JobPilot this is: the app's version and the
/// daemon's, since an app uses a newer daemon it finds rather than
/// replacing it (ADR-0029 §2, §4).
export function AboutSection() {
  const { data: versions, error } = useVersions();

  if (error) return <ErrorLine error={error} />;
  if (!versions) return <Loading />;

  return (
    <>
      <SettingsHeading
        title="About"
        description="To update JobPilot, install a newer release over this one. Your data stays where it is, and JobPilot never checks for updates by itself."
      />
      <Panel className="divide-y divide-border">
        <VersionRow label="JobPilot" detail="This app." version={versions.app} />
        <VersionRow
          label="Background service"
          detail="Runs scouts and keeps your data, and carries on after the app is closed."
          version={versions.daemon}
        />
      </Panel>
    </>
  );
}

function VersionRow({ label, detail, version }: { label: string; detail: string; version: string }) {
  return (
    <div className="flex items-center gap-4 px-4 py-3">
      <span className="min-w-0 flex-1">
        <span className="block text-[13px] font-medium">{label}</span>
        <span className="mt-0.5 block text-xs text-muted-foreground">{detail}</span>
      </span>
      <span className="text-[13px] tabular-nums text-subtle-foreground select-text">
        {version === "dev" ? "Development build" : version}
      </span>
    </div>
  );
}
