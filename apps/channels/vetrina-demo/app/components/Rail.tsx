"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { ReactNode, RefObject } from "react";
import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react";

// How far the pointer travels before a press counts as a drag rather than a click
const DRAG_SLOP = 5;

// Mandatory snapping parks the rail on a card boundary, so the last few pixels of the track's own
// padding are never scrolled away - the ends are read off the cards, not off scrollLeft
const EDGE_SLOP = 8;

type RailState = {
  overflows: boolean;
  atStart: boolean;
  atEnd: boolean;
  pages: number;
  page: number;
};

type RailApi = RailState & {
  trackRef: RefObject<HTMLDivElement | null>;
  measure: () => void;
  goto: (page: number) => void;
  step: (dir: 1 | -1) => void;
};

const Ctx = createContext<RailApi | null>(null);

function useRail(): RailApi {
  const api = useContext(Ctx);
  if (!api) throw new Error("Rail parts must be used inside a RailProvider");
  return api;
}

// A carousel inside a section header adopts that section's provider so the arrows can sit in the
// header row; one with no section around it makes its own
export function useOptionalRail(): RailApi | null {
  return useContext(Ctx);
}

// Width of one card plus the gap between two - everything else is derived from it, so a rail
// always moves by whole cards and never leaves one sliced down the middle
function cardStep(el: HTMLDivElement): number {
  const first = el.firstElementChild;
  if (!first) return el.clientWidth;
  const gap = Number.parseFloat(getComputedStyle(el).columnGap) || 0;
  return first.getBoundingClientRect().width + gap;
}

// Holds the scroll state the arrows and dots read - they render outside the track (in the section
// header, under the cards), so the position can't just live in the scroller itself
export function RailProvider({ children }: { children: ReactNode }) {
  const trackRef = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<RailState>({
    overflows: false,
    atStart: true,
    atEnd: true,
    pages: 1,
    page: 0,
  });

  const measure = useCallback(() => {
    const el = trackRef.current;
    if (!el) return;
    const first = el.firstElementChild;
    const last = el.lastElementChild;
    if (!first || !last) return;
    const track = el.getBoundingClientRect();
    // Dots count screenfuls, not cards - a rail whose last page is half full still only has one
    // stop left, and a dot you can't scroll to reads as broken
    const max = el.scrollWidth - el.clientWidth;
    const pages = Math.max(1, Math.ceil(el.scrollWidth / el.clientWidth));
    setState({
      overflows: max > EDGE_SLOP,
      atStart: first.getBoundingClientRect().left - track.left >= -EDGE_SLOP,
      atEnd: track.right - last.getBoundingClientRect().right >= -EDGE_SLOP,
      pages,
      page: max <= 0 ? 0 : Math.round((el.scrollLeft / max) * (pages - 1)),
    });
  }, []);

  const scrollTo = useCallback((left: number) => {
    const el = trackRef.current;
    if (!el) return;
    const smooth = !window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    el.scrollTo({ left, behavior: smooth ? "smooth" : "auto" });
  }, []);

  const goto = useCallback(
    (page: number) => {
      const el = trackRef.current;
      if (!el) return;
      const max = el.scrollWidth - el.clientWidth;
      const pages = Math.max(1, Math.ceil(el.scrollWidth / el.clientWidth));
      scrollTo(pages < 2 ? 0 : (max * page) / (pages - 1));
    },
    [scrollTo],
  );

  const step = useCallback(
    (dir: 1 | -1) => {
      const el = trackRef.current;
      if (!el) return;
      const card = cardStep(el);
      const perPage = Math.max(1, Math.floor(el.clientWidth / card));
      const max = el.scrollWidth - el.clientWidth;
      let target = Math.min(max, Math.max(0, el.scrollLeft + dir * card * perPage));
      // Whole cards rarely divide into the track exactly - close the last half-card rather than
      // stopping a sliver short of the end
      if (max - target < card / 2) target = max;
      if (target < card / 2) target = 0;
      scrollTo(target);
    },
    [scrollTo],
  );

  const api = useMemo<RailApi>(
    () => ({ ...state, trackRef, measure, goto, step }),
    [state, measure, goto, step],
  );
  return <Ctx.Provider value={api}>{children}</Ctx.Provider>;
}

export function Rail({
  children,
  label,
}: {
  children: ReactNode;
  label?: string;
}) {
  const { trackRef, measure, overflows } = useRail();

  useEffect(() => {
    const el = trackRef.current;
    if (!el) return;
    // ResizeObserver fires once on observe, so the first measurement arrives through the callback
    // rather than a setState in this effect - it also catches cards that size themselves late
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    const first = el.firstElementChild;
    if (first) ro.observe(first);

    // A mouse wheel only sends deltaY, and the browser won't turn that into sideways scroll on
    // its own - without this the rail is unreachable to anyone without a trackpad
    const onWheel = (e: WheelEvent) => {
      if (Math.abs(e.deltaY) <= Math.abs(e.deltaX)) return; // a trackpad already scrolls sideways
      const max = el.scrollWidth - el.clientWidth;
      const stuck =
        (e.deltaY < 0 && el.scrollLeft <= 0) || (e.deltaY > 0 && el.scrollLeft >= max);
      // At either end the gesture belongs to the page again, or the rail traps the scroll
      if (stuck) return;
      e.preventDefault();
      el.scrollLeft = Math.min(max, Math.max(0, el.scrollLeft + e.deltaY));
    };
    // Non-passive because preventDefault is the whole point - React's own onWheel is passive
    el.addEventListener("wheel", onWheel, { passive: false });

    // Grab-and-drag, the thing a touch swipe does for free. Mouse only - touch and pen already
    // scroll natively, and taking that over would cost them the momentum
    let drag: { x: number; left: number; moved: boolean } | null = null;

    const onPointerDown = (e: PointerEvent) => {
      if (e.pointerType !== "mouse" || e.button !== 0) return;
      if (el.scrollWidth <= el.clientWidth) return;
      drag = { x: e.clientX, left: el.scrollLeft, moved: false };
    };

    const onPointerMove = (e: PointerEvent) => {
      if (!drag) return;
      const dx = e.clientX - drag.x;
      // Below the threshold this is still a click on whatever card is under the cursor
      if (!drag.moved && Math.abs(dx) < DRAG_SLOP) return;
      if (!drag.moved) {
        drag.moved = true;
        // Captured only once it's really a drag, so a plain click still reaches the card
        el.setPointerCapture(e.pointerId);
        el.classList.add("rail-dragging");
      }
      el.scrollLeft = drag.left - dx;
    };

    const endDrag = (e: PointerEvent) => {
      if (!drag) return;
      const dragged = drag.moved;
      drag = null;
      el.classList.remove("rail-dragging");
      if (!dragged) return;
      if (el.hasPointerCapture(e.pointerId)) el.releasePointerCapture(e.pointerId);
      // The release still fires a click on the card the drag ended over - swallow that one
      el.addEventListener("click", swallowClick, { capture: true, once: true });
      // Nothing to swallow if the pointer came to rest without a click behind it
      setTimeout(() => el.removeEventListener("click", swallowClick, true), 0);
    };

    const swallowClick = (e: Event) => {
      e.preventDefault();
      e.stopPropagation();
    };

    // Links and images start a native drag of their own otherwise, which cancels the scroll
    const onDragStart = (e: DragEvent) => {
      if (drag) e.preventDefault();
    };

    el.addEventListener("pointerdown", onPointerDown);
    el.addEventListener("pointermove", onPointerMove);
    el.addEventListener("pointerup", endDrag);
    el.addEventListener("pointercancel", endDrag);
    el.addEventListener("dragstart", onDragStart);

    return () => {
      ro.disconnect();
      el.removeEventListener("wheel", onWheel);
      el.removeEventListener("pointerdown", onPointerDown);
      el.removeEventListener("pointermove", onPointerMove);
      el.removeEventListener("pointerup", endDrag);
      el.removeEventListener("pointercancel", endDrag);
      el.removeEventListener("dragstart", onDragStart);
    };
  }, [trackRef, measure]);

  return (
    <div
      ref={trackRef}
      onScroll={measure}
      role={label ? "region" : undefined}
      aria-label={label}
      // Reachable by keyboard only while there's something to scroll - an unscrollable region in
      // the tab order is a dead stop
      tabIndex={overflows ? 0 : undefined}
      className={`scrollbar-none snap-touch contain-paint -mx-1 flex gap-4 overflow-x-auto px-1 pb-1 outline-none focus-visible:ring-2 focus-visible:ring-foreground/20 ${overflows ? "cursor-grab" : ""}`}
    >
      {children}
    </div>
  );
}

// Sits in the section header where the "view all" link used to - disabled rather than hidden at
// the ends, so the header doesn't reflow every time the rail reaches one
export function RailArrows({ className = "" }: { className?: string }) {
  const { overflows, atStart, atEnd, step } = useRail();
  if (!overflows) return null;
  return (
    <div className={`hidden items-center gap-2 sm:flex ${className}`}>
      <Arrow dir={-1} disabled={atStart} onClick={() => step(-1)} />
      <Arrow dir={1} disabled={atEnd} onClick={() => step(1)} />
    </div>
  );
}

function Arrow({
  dir,
  disabled,
  onClick,
}: {
  dir: 1 | -1;
  disabled: boolean;
  onClick: () => void;
}) {
  const Icon = dir === 1 ? ChevronRightIcon : ChevronLeftIcon;
  return (
    <button
      type="button"
      aria-label={dir === 1 ? "Scroll forward" : "Scroll back"}
      onClick={onClick}
      disabled={disabled}
      className="flex size-10 items-center justify-center rounded-md border border-border bg-background text-foreground transition hover:border-foreground/50 disabled:pointer-events-none disabled:opacity-30"
    >
      <Icon className="size-5" aria-hidden="true" />
    </button>
  );
}

// Position readout under the cards, in place of the scrollbar
export function RailDots({ className = "" }: { className?: string }) {
  const { overflows, pages, page, goto } = useRail();
  if (!overflows || pages < 2) return null;
  return (
    <div className={`mt-4 flex items-center justify-center gap-1.5 ${className}`}>
      {Array.from({ length: pages }, (_, i) => (
        <button
          key={i}
          type="button"
          aria-label={`Go to page ${i + 1} of ${pages}`}
          aria-current={i === page ? "true" : undefined}
          onClick={() => goto(i)}
          // Hit area stays finger-sized while the mark itself stays a hairline dot
          className="group flex h-6 w-4 items-center justify-center"
        >
          <span
            className={
              i === page
                ? "block h-1.5 w-5 rounded-full bg-foreground transition-all"
                : "block size-1.5 rounded-full bg-foreground/25 transition-all group-hover:bg-foreground/50"
            }
          />
        </button>
      ))}
    </div>
  );
}

// The carousel body, ready to drop into either kind of container. Layouts are called as plain
// functions from server components, so the "is there a provider around me" question has to be
// asked from inside a client component like this one rather than by the caller
export function RailFrame({
  children,
  label,
}: {
  children: ReactNode;
  label?: string;
}) {
  const framed = useOptionalRail();
  const body = (
    <>
      <Rail label={label}>{children}</Rail>
      <RailDots />
    </>
  );
  if (framed) return body;
  // No section header to hang them off, so the arrows sit above the cards on their own
  return (
    <RailProvider>
      <RailArrows className="mb-3 justify-end" />
      {body}
    </RailProvider>
  );
}
