"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";

// How long the leave animation runs before the bar unmounts - must match
// buy-bar-out's duration in globals.css
const LEAVE_MS = 180;

// Wraps a PDP buy row (qty stepper + add-to-cart). The in-flow row NEVER moves or
// hides; when it leaves the viewport a floating pill bar fades in with a second copy
// of the row (its own form and qty - both post the same action and hidden state).
// Grouped passes barChildren instead: its form is id-bound, so the bar submits the
// original via the `form` attribute rather than duplicating it. Phones show the bar
// whenever the row is off-screen (the buy box starts below the fold); desktop only
// once the row is scrolled ABOVE the viewport. data-stuck-buy scopes the bar-only
// styling (price beside the label, pill corners)
export function StickyBuyRow({
  children,
  barChildren,
  className = "",
}: {
  children: ReactNode;
  barChildren?: ReactNode;
  className?: string;
}) {
  const holderRef = useRef<HTMLDivElement>(null);
  // hidden -> shown (fade in) -> leaving (fade out) -> hidden
  const [phase, setPhase] = useState<"hidden" | "shown" | "leaving">("hidden");

  useEffect(() => {
    const holder = holderRef.current;
    if (!holder) return;
    const mobile = window.matchMedia("(max-width: 767px)");
    const io = new IntersectionObserver(
      ([entry]) => {
        // Desktop only shows the bar once the row is scrolled ABOVE the viewport - at
        // page top the buy box is ahead and a bar would be noise
        const above = entry.boundingClientRect.bottom <= 0;
        const show = !entry.isIntersecting && (mobile.matches || above);
        setPhase((p) => {
          if (show) return "shown";
          return p === "shown" ? "leaving" : p;
        });
      },
      // a sliver of slack so the bar doesn't flicker at the exact boundary
      { rootMargin: "-1px" },
    );
    io.observe(holder);
    return () => {
      io.disconnect();
    };
  }, []);

  // The leave phase exists purely to let the out animation finish before unmount
  useEffect(() => {
    if (phase !== "leaving") return;
    const t = window.setTimeout(() => {
      setPhase((p) => (p === "leaving" ? "hidden" : p));
    }, LEAVE_MS);
    return () => window.clearTimeout(t);
  }, [phase]);

  return (
    <>
      <div ref={holderRef} className={className}>
        {children}
      </div>
      {phase !== "hidden" && (
        <div
          data-stuck-buy=""
          className={`fixed bottom-3 left-1/2 z-30 w-[calc(100%-1.5rem)] max-w-xl -translate-x-1/2 rounded-full border border-border bg-background p-2 shadow-2xl shadow-black/25 ${
            phase === "leaving"
              ? "motion-safe:animate-[buy-bar-out_0.18s_ease-in_forwards]"
              : "motion-safe:animate-[buy-bar-in_0.25s_ease-out]"
          }`}
        >
          {barChildren ?? children}
        </div>
      )}
    </>
  );
}
