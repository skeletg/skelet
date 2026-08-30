"use client";

// CSS scroll-snap carousel - the browser owns drag, momentum and keyboard, this
// only tracks the centred slide. No infinite loop: prev/next wrap to the far end

import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";

export function useSnapCarousel(count: number, startAt = 0) {
  const ref = useRef<HTMLDivElement>(null);
  const [index, setIndex] = useState(startAt);

  const scrollTo = useCallback((i: number, smooth = true) => {
    const el = ref.current;
    if (!el) return;
    const clamped = Math.max(0, Math.min(i, el.children.length - 1));
    const child = el.children[clamped] as HTMLElement | undefined;
    if (!child) return;
    el.scrollTo({ left: child.offsetLeft - el.offsetLeft, behavior: smooth ? "smooth" : "auto" });
  }, []);

  // From scroll position, not from clicks - a drag has to report like a button
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    let frame = 0;
    const onScroll = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        const mid = el.scrollLeft + el.clientWidth / 2;
        let best = 0;
        let bestDist = Infinity;
        for (let i = 0; i < el.children.length; i++) {
          const c = el.children[i] as HTMLElement;
          const d = Math.abs(c.offsetLeft - el.offsetLeft + c.clientWidth / 2 - mid);
          if (d < bestDist) {
            bestDist = d;
            best = i;
          }
        }
        setIndex(best);
      });
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      cancelAnimationFrame(frame);
      el.removeEventListener("scroll", onScroll);
    };
  }, []);

  // Jump to the start slide without animating past the ones before it
  useEffect(() => {
    if (startAt > 0) scrollTo(startAt, false);
    // Only on mount - later moves come from the controls
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const prev = () => scrollTo(index === 0 ? count - 1 : index - 1);
  const next = () => scrollTo(index === count - 1 ? 0 : index + 1);

  return { ref, index, scrollTo, prev, next };
}

export function SnapTrack({
  trackRef,
  children,
  className = "",
}: {
  trackRef: React.RefObject<HTMLDivElement | null>;
  children: ReactNode;
  className?: string;
}) {
  return (
    <div ref={trackRef} className={`snap-track ${className}`}>
      {children}
    </div>
  );
}

export function SnapButton({
  dir,
  onClick,
  className = "",
}: {
  dir: "prev" | "next";
  onClick: () => void;
  className?: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={dir === "prev" ? "Previous image" : "Next image"}
      className={`absolute top-1/2 z-10 flex size-9 -translate-y-1/2 items-center justify-center rounded-md border border-border bg-background/90 text-foreground shadow transition hover:bg-background ${className}`}
    >
      <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth={2}
        strokeLinecap="round"
        strokeLinejoin="round"
        className="size-4"
        aria-hidden="true"
      >
        <path d={dir === "prev" ? "M15 18l-6-6 6-6" : "M9 18l6-6-6-6"} />
      </svg>
    </button>
  );
}
