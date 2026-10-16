import { useState } from "react";
import { Link } from "react-router-dom";
import { FormField } from "../../components/form-field";
import { Button } from "../../components/ui/button";
import { Input } from "../../components/ui/input";
import { useInitializeProfile } from "../../lib/queries/profile.queries";
import { useToast } from "../../lib/toast";
import type { Candidate } from "../../lib/types";

/// ProfileStep creates the candidate. It asks for the same four fields the
/// CLI's onboard prompts for, and no more: the rest of the profile is
/// better filled from a resume or edited later on the Profile screen.
export function ProfileStep({ profile, exists, onNext }: { profile?: Candidate; exists: boolean; onNext: () => void }) {
  const { notifyError } = useToast();
  const [draft, setDraft] = useState({
    first_name: profile?.first_name ?? "",
    last_name: profile?.last_name ?? "",
    email: profile?.email ?? "",
    phone: profile?.phone ?? "",
    location_city: profile?.location_city ?? "",
    location_country: profile?.location_country ?? "",
  });

  const create = useInitializeProfile({
    onSuccess: () => onNext(),
    onError: notifyError,
  });

  if (exists) {
    return (
      <>
        <div>
          <h2 className="font-semibold">Profile created</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {[profile?.first_name, profile?.last_name].filter(Boolean).join(" ") || "Your candidate profile"} is saved.
            The{" "}
            <Link className="underline" to="/profile">
              Profile screen
            </Link>{" "}
            has the rest of the fields whenever you want them.
          </p>
        </div>
        <Button onClick={onNext}>Continue</Button>
      </>
    );
  }

  const set = (key: keyof typeof draft, value: string) => setDraft((current) => ({ ...current, [key]: value }));

  return (
    <>
      <div>
        <h2 className="font-semibold">Who are you applying as?</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          JobPilot matches jobs against this and fills applications from it. It never invents details you have not given
          it.
        </p>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <FormField label="First name" htmlFor="welcome-first">
          <Input id="welcome-first" value={draft.first_name} onChange={(e) => set("first_name", e.target.value)} />
        </FormField>
        <FormField label="Last name" htmlFor="welcome-last">
          <Input id="welcome-last" value={draft.last_name} onChange={(e) => set("last_name", e.target.value)} />
        </FormField>
        <FormField label="Email" htmlFor="welcome-email">
          <Input id="welcome-email" type="email" value={draft.email} onChange={(e) => set("email", e.target.value)} />
        </FormField>
        <FormField label="Phone" htmlFor="welcome-phone">
          <Input id="welcome-phone" value={draft.phone} onChange={(e) => set("phone", e.target.value)} />
        </FormField>
        <FormField label="City" htmlFor="welcome-city">
          <Input id="welcome-city" value={draft.location_city} onChange={(e) => set("location_city", e.target.value)} />
        </FormField>
        <FormField label="Country" htmlFor="welcome-country">
          <Input
            id="welcome-country"
            value={draft.location_country}
            onChange={(e) => set("location_country", e.target.value)}
          />
        </FormField>
      </div>

      <div className="flex items-center gap-2">
        <Button onClick={() => create.mutate(draft)} disabled={create.isPending || !draft.first_name.trim()}>
          {create.isPending ? "Creating…" : "Create profile"}
        </Button>
        {!draft.first_name.trim() && <span className="text-xs text-muted-foreground">A first name is required.</span>}
      </div>
    </>
  );
}
