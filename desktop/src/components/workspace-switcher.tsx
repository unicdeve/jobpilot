import { Check, ChevronUp, Plus, Settings, UserRound } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { errorMessage } from "../lib/api";
import { useDaemon } from "../lib/daemon";
import { Button } from "./ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogTitle } from "./ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "./ui/dropdown-menu";
import { Input } from "./ui/input";

export function WorkspaceSwitcher() {
  const { activeWorkspace, workspaces, switchWorkspace, createWorkspace, renameWorkspace } = useDaemon();
  const navigate = useNavigate();
  const [dialog, setDialog] = useState<"create" | "rename" | null>(null);
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);

  if (!activeWorkspace) return null;
  const initials = workspaceInitials(activeWorkspace.name);

  const openDialog = (kind: "create" | "rename") => {
    setDialog(kind);
    setName(kind === "rename" ? activeWorkspace.name : "");
    setError("");
  };

  const save = async () => {
    setSaving(true);
    setError("");
    try {
      if (dialog === "create") {
        await createWorkspace(name);
        navigate("/");
      } else if (dialog === "rename") {
        await renameWorkspace(activeWorkspace.id, name);
      }
      setDialog(null);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <button
              type="button"
              className="flex h-10 w-full items-center gap-2.5 rounded-lg px-2 text-left outline-none hover:bg-sidebar-accent focus-visible:ring-2 focus-visible:ring-ring"
            />
          }
        >
          <span className="flex size-7 shrink-0 items-center justify-center rounded-md bg-primary text-xs font-semibold text-primary-foreground">
            {initials}
          </span>
          <span className="min-w-0 flex-1">
            <span className="block truncate text-[13px] font-medium">{activeWorkspace.name}</span>
            <span className="block text-[11px] text-muted-foreground">Profile</span>
          </span>
          <ChevronUp className="size-3.5 text-muted-foreground" />
        </DropdownMenuTrigger>
        <DropdownMenuContent side="top" align="start" sideOffset={6} className="w-52">
          <DropdownMenuGroup>
            <DropdownMenuLabel>Profiles</DropdownMenuLabel>
            {workspaces.map((workspace) => (
              <DropdownMenuItem
                key={workspace.id}
                onClick={() => {
                  if (workspace.id === activeWorkspace.id) return;
                  void switchWorkspace(workspace.id).then(() => navigate("/"));
                }}
              >
                <span className="flex size-5 items-center justify-center rounded bg-muted text-[10px] font-semibold">
                  {workspaceInitials(workspace.name)}
                </span>
                <span className="min-w-0 flex-1 truncate">{workspace.name}</span>
                {workspace.id === activeWorkspace.id && <Check className="size-3.5 text-primary" />}
              </DropdownMenuItem>
            ))}
          </DropdownMenuGroup>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={() => openDialog("create")}>
            <Plus />
            Add profile
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => openDialog("rename")}>
            <UserRound />
            Rename profile…
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem render={<Link to="/settings" />}>
            <Settings />
            Settings
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <Dialog open={dialog !== null} onOpenChange={(open) => !open && setDialog(null)}>
        <DialogContent>
          <DialogTitle>{dialog === "create" ? "Add profile" : "Rename profile"}</DialogTitle>
          <DialogDescription>
            {dialog === "create"
              ? "This profile gets its own settings, searches, jobs, applications, credentials and browser sign-ins."
              : "The name identifies this profile only on this computer."}
          </DialogDescription>
          <form
            className="mt-4"
            onSubmit={(event) => {
              event.preventDefault();
              void save();
            }}
          >
            <Input
              autoFocus
              value={name}
              maxLength={80}
              placeholder="Profile name"
              aria-label="Profile name"
              onChange={(event) => setName(event.target.value)}
            />
            {error && <p className="mt-2 text-sm text-destructive">{error}</p>}
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => setDialog(null)}>
                Cancel
              </Button>
              <Button type="submit" disabled={saving || name.trim() === ""}>
                {saving ? "Saving…" : dialog === "create" ? "Add profile" : "Save"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
}

function workspaceInitials(name: string) {
  const words = name.trim().split(/\s+/).slice(0, 2);
  return words.map((word) => word[0]?.toUpperCase()).join("") || "JP";
}
