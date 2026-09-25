import { useEffect, useRef } from "react";
import { Button } from "./ui/button";

/// LoadMore fetches the next page when it scrolls into view, and is also a
/// real button: an observer that never fires (or a user on a keyboard) still
/// has a way to reach the rest of the list.
export function LoadMore({
  hasNextPage,
  isFetchingNextPage,
  fetchNextPage,
}: {
  hasNextPage: boolean;
  isFetchingNextPage: boolean;
  fetchNextPage: () => void;
}) {
  const sentinel = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = sentinel.current;
    if (!el || !hasNextPage) return;

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting && !isFetchingNextPage) fetchNextPage();
      },
      // Start fetching a little before the end is actually reached, so the
      // next page is usually there by the time the user scrolls to it.
      { rootMargin: "200px" },
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [hasNextPage, isFetchingNextPage, fetchNextPage]);

  if (!hasNextPage) return null;

  return (
    <div ref={sentinel} className="pt-2">
      <Button variant="outline" onClick={fetchNextPage} disabled={isFetchingNextPage}>
        {isFetchingNextPage ? "Loading…" : "Load more"}
      </Button>
    </div>
  );
}
