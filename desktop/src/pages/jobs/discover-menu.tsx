import { ChevronDown, Link2 } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useDiscover, useDiscoveryRuns } from "../../components/discovery-activity";
import { Button } from "../../components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogTitle,
} from "../../components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "../../components/ui/dropdown-menu";
import { Input } from "../../components/ui/input";
import { useImportJob } from "../../lib/queries/job.queries";
import { useSavedSearches } from "../../lib/queries/saved-search.queries";
import { useToast } from "../../lib/toast";

/// DiscoverMenu runs one of the user's saved searches against the job
/// sources. Discovery without criteria is refused by the daemon — the
/// boards return their whole listing regardless of what is asked — so with
/// no saved search the menu points at Settings instead.
///
/// A run already going or queued, from here or anywhere else, cannot be
/// asked for twice; another search can, and waits its turn. "Search all"
/// queues every search not already waiting, in the order they are listed.
export function DiscoverMenu() {
  const { data: searches } = useSavedSearches();
  const discover = useDiscover();
  const runs = useDiscoveryRuns();
  const pending = new Set(runs.map((run) => run.search.id));
  const notPending = (searches ?? []).filter((search) => !pending.has(search.id));
  const searchAll = () => {
    for (const search of notPending) discover.mutate({ id: search.id, name: search.name });
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button size="sm" />}>
        {runs.length > 0 ? "Searching…" : "Discover"}
        <ChevronDown data-icon="inline-end" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-60">
        <DropdownMenuGroup>
          <DropdownMenuLabel>Search your job sources with</DropdownMenuLabel>
          {(searches?.length ?? 0) > 1 && (
            <>
              <DropdownMenuItem disabled={notPending.length === 0} onClick={searchAll}>
                <span className="flex-1">All saved searches</span>
                <span className="text-xs text-muted-foreground tabular-nums">{searches?.length}</span>
              </DropdownMenuItem>
              <DropdownMenuSeparator />
            </>
          )}
          {searches?.map((search) => (
            <DropdownMenuItem
              key={search.id}
              disabled={pending.has(search.id)}
              onClick={() => discover.mutate({ id: search.id, name: search.name })}
            >
              <span className="flex-1 truncate">{search.name}</span>
              {search.is_default && <span className="text-xs text-muted-foreground">Default</span>}
            </DropdownMenuItem>
          ))}
          {searches?.length === 0 && (
            <DropdownMenuLabel className="font-normal">No saved searches yet.</DropdownMenuLabel>
          )}
        </DropdownMenuGroup>
        <DropdownMenuSeparator />
        <DropdownMenuItem render={<Link to="/settings?section=searches" />}>Manage searches…</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/// AddByUrlButton imports one posting from its URL. A URL no registered
/// source recognises is read in a real browser window, which the dialog
/// says, since a window appearing unannounced looks like something wrong.
export function AddByUrlButton() {
  const { notifyError } = useToast();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [url, setUrl] = useState("");
  const importJob = useImportJob({
    onSuccess: (job) => {
      setOpen(false);
      setUrl("");
      navigate(`/jobs/${job.id}`);
    },
    onError: notifyError,
  });

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    if (url.trim()) importJob.mutate(url.trim());
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <Button
        size="icon-sm"
        variant="ghost"
        title="Add a job by URL"
        aria-label="Add a job by URL"
        onClick={() => setOpen(true)}
      >
        <Link2 />
      </Button>
      <DialogContent>
        <form onSubmit={submit}>
          <DialogTitle>Add a job by URL</DialogTitle>
          <DialogDescription>
            Paste the link to a job posting. If it isn’t from a job board JobPilot knows, a browser window opens briefly
            to read the page.
          </DialogDescription>
          <Input
            autoFocus
            type="url"
            className="mt-4"
            placeholder="https://"
            value={url}
            onChange={(event) => setUrl(event.target.value)}
            disabled={importJob.isPending}
          />
          <DialogFooter>
            <DialogClose render={<Button type="button" variant="outline" />}>Cancel</DialogClose>
            <Button type="submit" disabled={!url.trim() || importJob.isPending}>
              {importJob.isPending ? "Adding…" : "Add job"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
