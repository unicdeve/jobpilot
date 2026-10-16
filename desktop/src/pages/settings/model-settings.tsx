import { useState } from "react";
import { LLMProviderFields, withProviderDefaults } from "../../components/llm-provider-fields";
import { Panel } from "../../components/page";
import { SettingsHeading } from "../../components/settings-heading";
import { ErrorLine, Loading } from "../../components/status-line";
import { Button } from "../../components/ui/button";
import { useLLMSettings, useSaveLLMSettings } from "../../lib/queries/llm.queries";
import { useToast } from "../../lib/toast";
import type { LLMSettings } from "../../lib/types";

export function ModelSettings() {
  const { notify, notifyError } = useToast();
  const { data: saved, error } = useLLMSettings();

  // The form is an editable draft over the saved settings: draft holds the
  // user's unsaved edits and falls back to what the daemon last returned.
  const [draft, setDraft] = useState<LLMSettings | null>(null);
  const settings = draft ?? (saved && withProviderDefaults(saved));

  const saveSettings = useSaveLLMSettings({
    onSuccess: (result) => {
      // The mutation writes result.settings into the cache, so dropping the
      // draft shows exactly what was persisted.
      setDraft(null);
      if (result.connectivity_error) {
        notify(`Settings saved, but could not connect: ${result.connectivity_error}`);
      } else {
        notify("Settings saved and verified.", "success");
      }
    },
    onError: notifyError,
  });

  return (
    <>
      <SettingsHeading
        title="AI model"
        description="The model JobPilot uses to read postings, score matches and write drafts, reached at the address below."
      />
      {error ? (
        <ErrorLine error={error} />
      ) : !settings ? (
        <Loading />
      ) : (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            saveSettings.mutate(settings);
          }}
        >
          <Panel className="flex flex-col gap-4 p-4">
            <LLMProviderFields settings={settings} onChange={setDraft} />
          </Panel>
          <div className="mt-4 flex items-center justify-end gap-2">
            {draft && (
              <Button type="button" variant="ghost" onClick={() => setDraft(null)}>
                Revert
              </Button>
            )}
            <Button type="submit" disabled={!draft || saveSettings.isPending}>
              {saveSettings.isPending ? "Checking…" : "Save and check"}
            </Button>
          </div>
        </form>
      )}
    </>
  );
}
