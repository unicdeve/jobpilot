import { useLayoutEffect, useRef, type RefObject } from "react";

/// useScrollRestoration remembers where each screen was scrolled to and puts
/// it back when that screen is shown again.
///
/// React Router's own ScrollRestoration is not usable here: it requires a
/// data router (this app uses <HashRouter> with <Routes>), and it restores
/// the *window's* scroll position, while this app scrolls an inner element —
/// the <main> region beside the sidebar — and the window never scrolls at
/// all.
///
/// scrollRef is the scrolling element; contentRef wraps what is inside it,
/// so that its resizing tells us when the content is finally tall enough for
/// a saved position to be reachable. key identifies the screen, normally the
/// route's pathname.
export function useScrollRestoration(
  scrollRef: RefObject<HTMLElement | null>,
  contentRef: RefObject<HTMLElement | null>,
  key: string,
) {
  // Positions live for as long as the component holding this hook is
  // mounted, which for the app shell means the lifetime of the window.
  const positions = useRef(new Map<string, number>());

  useLayoutEffect(() => {
    const scroller = scrollRef.current;
    const content = contentRef.current;
    if (!scroller) return;

    const target = positions.current.get(key) ?? 0;
    let restoring = target > 0;

    // A screen's content usually arrives after this effect runs: a cached
    // query renders immediately, but fonts, images and any still-loading
    // query change the height afterwards. So rather than setting scrollTop
    // once and hoping, keep trying until the content is tall enough for the
    // saved position to be reachable.
    const restore = () => {
      if (!restoring) return;
      if (scroller.scrollHeight - scroller.clientHeight < target) return;
      scroller.scrollTop = target;
      restoring = false;
    };

    const onScroll = () => {
      // While restoring, the only scrolling is our own, and recording it
      // would overwrite the saved position with a partial value.
      if (restoring) return;
      positions.current.set(key, scroller.scrollTop);
    };

    // The user moving the page themselves outranks a restore that has not
    // managed to complete yet; without this, late-arriving content could
    // yank them away from where they chose to be.
    const onUserScroll = () => {
      restoring = false;
    };

    restore();

    const observer = content ? new ResizeObserver(restore) : null;
    observer?.observe(content!);

    scroller.addEventListener("scroll", onScroll, { passive: true });
    for (const event of ["wheel", "touchstart", "keydown"] as const) {
      scroller.addEventListener(event, onUserScroll, { passive: true });
    }

    return () => {
      observer?.disconnect();
      scroller.removeEventListener("scroll", onScroll);
      for (const event of ["wheel", "touchstart", "keydown"] as const) {
        scroller.removeEventListener(event, onUserScroll);
      }
    };
  }, [scrollRef, contentRef, key]);
}
