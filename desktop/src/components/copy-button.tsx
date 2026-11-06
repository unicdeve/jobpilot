import { Check, Copy } from "lucide-react";
import { useEffect, useState } from "react";
import { useToast } from "../lib/toast";
import { Button } from "./ui/button";

/// CopyButton copies text to the clipboard, and says so for a moment in
/// place of its icon. JobPilot never types into a page, so copying is how
/// the candidate's own details reach an application (ADR-0034 §2).
export function CopyButton({ text, label }: { text: string; label: string }) {
  const { notifyError } = useToast();
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!copied) return;
    const timer = window.setTimeout(() => setCopied(false), 1500);
    return () => window.clearTimeout(timer);
  }, [copied]);

  const copy = () => {
    navigator.clipboard.writeText(text).then(
      () => setCopied(true),
      () => notifyError(new Error("Couldn’t copy to the clipboard.")),
    );
  };

  return (
    <Button
      size="icon-xs"
      variant="ghost"
      onClick={copy}
      aria-label={copied ? `${label} copied` : `Copy ${label}`}
      title={copied ? "Copied" : "Copy"}
      className="shrink-0 text-muted-foreground"
    >
      {copied ? <Check className="text-success" /> : <Copy />}
    </Button>
  );
}
