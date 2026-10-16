import { useState } from "react";
import { LLMProviderFields, withProviderDefaults } from "../../components/llm-provider-fields";
import { Button } from "../../components/ui/button";
import { errorMessage } from "../../lib/api";
import { useSaveLLMSettings } from "../../lib/queries/llm.queries";
import type { LLMSettings } from "../../lib/types";

/// ProviderStep configures the local LLM. Saving reports connectivity as a
/// warning rather than a failure, matching the daemon: an install with no
/// internet, or with Ollama not started yet, must still be able to finish
/// setting itself up (docs/02_features/01_onboarding.md AC-003).
export function ProviderStep({ settings, onNext }: { settings?: LLMSettings; onNext: () => void }) {
  const [draft, setDraft] = useState<LLMSettings>(() =>
    withProviderDefaults(settings ?? { provider: "", base_url: "", model: "" }),
  );
  const [result, setResult] = useState<{ ok: boolean; message: string } | null>(null);

  const save = useSaveLLMSettings({
    onSuccess: (saved) =>
      setResult(
        saved.connectivity_error
          ? { ok: false, message: saved.connectivity_error }
          : { ok: true, message: "Connected — your model answered." },
      ),
    onError: (err) => setResult({ ok: false, message: errorMessage(err) }),
  });

  return (
    <>
      <div>
        <h2 className="font-semibold">Choose the AI model</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Matching, cover letters and reading a resume all run on the model you choose. A model on this computer keeps
          everything here; a hosted one is sent what it reads.
        </p>
      </div>

      <LLMProviderFields settings={draft} onChange={setDraft} />

      {result && (
        <p data-slot="connectivity" className={`text-sm ${result.ok ? "text-success" : "text-warning"}`}>
          {result.ok ? result.message : `Saved, but could not connect: ${result.message}`}
        </p>
      )}

      <div className="flex items-center gap-2">
        <Button variant="outline" onClick={() => save.mutate(draft)} disabled={save.isPending || !draft.model.trim()}>
          {save.isPending ? "Saving…" : "Save and test"}
        </Button>
        <Button onClick={onNext} disabled={!result}>
          Continue
        </Button>
        {!result && !draft.model.trim() && (
          <span className="text-xs text-muted-foreground">A model name is required.</span>
        )}
        {result && !result.ok && (
          <span className="text-xs text-muted-foreground">
            You can continue and start it later — only the AI steps need it.
          </span>
        )}
      </div>
    </>
  );
}
