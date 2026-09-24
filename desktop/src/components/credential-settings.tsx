import { useState } from "react";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Card, CardContent } from "./ui/card";
import { FormField } from "./form-field";
import { useCredentials, useDeleteCredential, useSetCredential } from "../lib/queries";
import { useToast } from "../lib/toast";

/// KNOWN_SOURCES are the job sources that can use a key, so the common case
/// is a choice rather than a spelling test. The field stays free text
/// because a connector added by a later release should not need this list
/// changed to be configurable.
const KNOWN_SOURCES = ["adzuna", "usajobs", "jooble"];

/// CredentialSettings manages API keys for job sources.
///
/// A key is stored in the operating system's keychain and never in
/// JobPilot's database. There is no way to read one back: the daemon has no
/// authentication, so a route that returned a secret would make it readable
/// by any process on the machine (ADR-0019 §4). The form says so where the
/// key is typed, because a field that silently turns into dots reads as a
/// product that has lost the value.
export function CredentialSettings() {
  const { notify } = useToast();
  const { data: credentials, error, isPending } = useCredentials();
  const [adding, setAdding] = useState(false);
  const [source, setSource] = useState("");
  const [label, setLabel] = useState("");
  const [secret, setSecret] = useState("");

  const reset = () => {
    setAdding(false);
    setSource("");
    setLabel("");
    setSecret("");
  };

  const onError = (err: unknown) => notify(String(err));

  const store = useSetCredential({
    onSuccess: (ref) => {
      reset();
      notify(`Stored the key for ${ref.source}. It cannot be shown again.`, "success");
    },
    onError,
  });
  const remove = useDeleteCredential({
    onSuccess: () => notify("Key removed.", "success"),
    onError,
  });

  const usable = source.trim() !== "" && secret.trim() !== "";

  return (
    <Card>
      <CardContent className="space-y-3">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h2 className="font-semibold">Job source keys</h2>
            <p className="mt-1 text-sm text-slate-500">
              Some job sources need an API key. Keys are kept in your operating system's keychain,
              never in JobPilot's database, and JobPilot cannot show one again once it is stored.
            </p>
          </div>
          {!adding && <Button onClick={() => setAdding(true)}>Add key</Button>}
        </div>

        {adding && (
          // A form rather than a div: it gives the key field Enter-to-submit,
          // and it is what the browser asks for around a password input.
          <form
            className="space-y-3 rounded-md border border-slate-200 p-3"
            onSubmit={(e) => {
              e.preventDefault();
              if (usable && !store.isPending) {
                store.mutate({ source: source.trim(), label: label.trim(), secret });
              }
            }}
          >
            <div className="grid grid-cols-2 gap-3">
              <FormField label="Source" htmlFor="credential-source">
                <Input
                  id="credential-source"
                  list="credential-sources"
                  placeholder="adzuna"
                  value={source}
                  onChange={(e) => setSource(e.target.value)}
                />
                <datalist id="credential-sources">
                  {KNOWN_SOURCES.map((known) => (
                    <option key={known} value={known} />
                  ))}
                </datalist>
              </FormField>
              <FormField label="Label (optional)" htmlFor="credential-label">
                <Input
                  id="credential-label"
                  placeholder="default"
                  value={label}
                  onChange={(e) => setLabel(e.target.value)}
                />
              </FormField>
            </div>

            <FormField label="API key" htmlFor="credential-secret">
              <Input
                id="credential-secret"
                type="password"
                autoComplete="off"
                spellCheck={false}
                value={secret}
                onChange={(e) => setSecret(e.target.value)}
              />
            </FormField>

            <p className="text-xs text-slate-500">
              You will not be able to view this key again. Keep your own copy if you need one —
              to change it later, enter it here again.
            </p>

            <div className="flex items-center gap-2">
              <Button type="submit" disabled={!usable || store.isPending}>
                {store.isPending ? "Storing…" : "Store in keychain"}
              </Button>
              <Button type="button" variant="secondary" onClick={reset}>
                Cancel
              </Button>
              <span className="text-xs text-slate-500">
                Lower-case letters, digits and hyphens for the source and label.
              </span>
            </div>
          </form>
        )}

        {error ? (
          <p className="text-sm text-red-600">{String(error.message)}</p>
        ) : isPending ? (
          <p className="text-sm text-slate-500">Loading…</p>
        ) : credentials.length === 0 ? (
          !adding && (
            <p className="text-sm text-slate-500">
              No keys stored. Every job source JobPilot uses today works without one.
            </p>
          )
        ) : (
          <div className="space-y-2">
            {credentials.map((ref) => (
              <div
                key={`${ref.source}/${ref.label}`}
                className="flex items-center justify-between gap-3 rounded-md border border-slate-200 p-3"
              >
                <div>
                  <p className="font-medium">{ref.source}</p>
                  <p className="mt-1 text-xs text-slate-500">
                    {ref.label} · stored in your keychain
                  </p>
                </div>
                <Button
                  variant="ghost"
                  disabled={remove.isPending}
                  onClick={() => remove.mutate(ref)}
                >
                  Remove
                </Button>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
