"use client";

import { useEffect, useState, useSyncExternalStore } from "react";
import { useLinkStatus } from "next/link";

import {
  beginNavigation,
  endNavigation,
  navigationPending,
  navigationPendingServer,
  subscribeNavigation,
} from "./nav-progress";

// Under a prefetched link the pending phase never happens, so most clicks show nothing at all -
// the bar only earns its keep on the slow ones, which is exactly when it should appear
const SHOW_AFTER_MS = 140;
// Where the crawl stops - the last stretch belongs to the navigation actually finishing
const CREEP_CEILING = 92;
const CREEP_MS = 220;
const FINISH_MS = 300;

// Rendered inside every LocaleLink - draws no DOM, it exists because useLinkStatus only reports
// from a descendant of the Link it belongs to
export function NavPendingProbe() {
  const { pending } = useLinkStatus();
  useEffect(() => {
    if (!pending) return;
    beginNavigation();
    return endNavigation;
  }, [pending]);
  return null;
}

function useNavigationPending(): boolean {
  return useSyncExternalStore(
    subscribeNavigation,
    navigationPending,
    navigationPendingServer,
  );
}

export function NavProgress() {
  const pending = useNavigationPending();
  // null means nothing to draw - a number means the bar is up, at that percent
  const [progress, setProgress] = useState<number | null>(null);

  useEffect(() => {
    if (pending) {
      const start = setTimeout(() => setProgress(8), SHOW_AFTER_MS);
      // Decelerating: it can't know the real fraction, so it approaches the ceiling rather than
      // pretending to, and never parks on a number that reads as stuck
      const creep = setInterval(
        () => setProgress((p) => (p === null ? p : p + (CREEP_CEILING - p) * 0.18)),
        CREEP_MS,
      );
      return () => {
        clearTimeout(start);
        clearInterval(creep);
      };
    }
    // Next frame, not now: the width has to paint where the crawl left it before jumping to
    // full, or the transition has nothing to animate from. Updater form so a navigation that
    // finished before the bar ever appeared stays at null
    const finish = requestAnimationFrame(() =>
      setProgress((p) => (p === null ? null : 100)),
    );
    const done = setTimeout(() => setProgress(null), FINISH_MS);
    return () => {
      cancelAnimationFrame(finish);
      clearTimeout(done);
    };
  }, [pending]);

  const finishing = progress === 100;
  return (
    <div
      // Over the sticky header, and never in the way of a click
      className="pointer-events-none fixed inset-x-0 top-0 z-50 h-0.5"
      aria-hidden
    >
      <div
        className="h-full bg-signal transition-[width,opacity] ease-out"
        style={{
          width: `${progress ?? 0}%`,
          // Fades as it completes, so the last frame is the bar filling rather than vanishing
          opacity: progress === null || finishing ? 0 : 1,
          transitionDuration: finishing ? `160ms, ${FINISH_MS}ms` : `${CREEP_MS}ms, 120ms`,
        }}
      />
    </div>
  );
}
