import { useState } from "react";
import { useDiscover } from "../../components/discovery-activity";
import { SavedSearchForm } from "../../components/saved-search-form";
import { SearchTemplatePicker } from "../../components/search-template-picker";
import { Button } from "../../components/ui/button";
import { useCreateSavedSearch } from "../../lib/queries/saved-search.queries";
import { useToast } from "../../lib/toast";
import type { SavedSearch } from "../../lib/types";

/// SearchStep is the one step that cannot be skipped: discovery refuses to
/// run on empty criteria, so an installation with no saved search cannot
/// find a single job (docs/02_features/01_onboarding.md AC-005).
///
/// The searches it adds are run as it finishes, so setup ends with jobs
/// arriving rather than with empty lists and a Discover button to find
/// (AC-006). The runs are queued and outlive this screen; offline, each one
/// says it could not reach the sources, and setup is complete regardless.
export function SearchStep({
  hasSearch,
  onDone,
  onBack,
}: {
  hasSearch: boolean;
  onDone: (searching: boolean) => void;
  onBack: () => void;
}) {
  const { notify, notifyError } = useToast();
  const discover = useDiscover();
  // Templates first: this is where a first-time user is least able to tell a
  // search that will work from one that will quietly find nothing.
  const [writingOwn, setWritingOwn] = useState(false);
  const runAndFinish = (added: SavedSearch[]) => {
    for (const search of added) discover.mutate({ id: search.id, name: search.name });
    onDone(added.length > 0);
  };
  const create = useCreateSavedSearch({
    onSuccess: (search) => runAndFinish([search]),
    onError: notifyError,
  });

  if (hasSearch) {
    return (
      <>
        <div>
          <h2 className="font-semibold">You have a search</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            That is everything JobPilot needs. Add more of them any time under Settings.
          </p>
        </div>
        <Button onClick={() => onDone(false)}>Finish</Button>
      </>
    );
  }

  return (
    <>
      <div>
        <h2 className="font-semibold">What jobs are you looking for?</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          JobPilot only looks for what you ask it to. The job boards do no filtering of their own, so without a search
          it would import every posting on them — it refuses to run instead.
        </p>
      </div>

      {writingOwn ? (
        <>
          <SavedSearchForm
            submitLabel="Save and finish"
            pending={create.isPending}
            onSubmit={(name, criteria) => create.mutate({ name, criteria })}
            onCancel={onBack}
          />
          <button
            type="button"
            onClick={() => setWritingOwn(false)}
            className="self-start text-xs text-muted-foreground underline"
          >
            Back to the templates
          </button>
        </>
      ) : (
        <>
          <SearchTemplatePicker onCancel={onBack} onError={(message) => notify(message)} onImported={runAndFinish} />
          <button
            type="button"
            onClick={() => setWritingOwn(true)}
            className="self-start text-xs text-muted-foreground underline"
          >
            Write my own instead
          </button>
        </>
      )}
    </>
  );
}
