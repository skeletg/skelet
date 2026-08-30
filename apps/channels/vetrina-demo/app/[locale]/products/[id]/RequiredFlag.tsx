"use client";

import { useState, type ReactNode } from "react";

import { isProgrammaticFocus } from "./focus-intent";

// Tracks whether a skipped selection control should still be flagged, and hands that flag to the control so the highlight lands on the real
// thing you interact with - the choice card, the input - instead of a box drawn around the whole block including its label and help text
export function RequiredFlag({
  active,
  attemptSeq,
  children,
  className = "",
  anchorId,
}: {
  /** The control is still missing something after an add-to-cart attempt. */
  active: boolean;
  /** Increments on every failed attempt; re-arms a dismissed flag. */
  attemptSeq: number;
  /** Receives the current flag so it can style its own inner controls. */
  children: (invalid: boolean) => ReactNode;
  className?: string;
  /** Scroll target for the hint's "jump to this option" link. */
  anchorId?: string;
}) {
  const [dismissedSeq, setDismissedSeq] = useState<number | null>(null);
  const invalid = active && dismissedSeq !== attemptSeq;
  const dismiss = () => {
    if (active && !isProgrammaticFocus()) setDismissedSeq(attemptSeq);
  };

  return (
    <div
      id={anchorId}
      // -1 so the hint's jump link can park focus here when the group has no focusable
      // control of its own; never in the natural tab order
      tabIndex={anchorId ? -1 : undefined}
      onPointerDownCapture={dismiss}
      onFocusCapture={dismiss}
      className={className || undefined}
    >
      {children(invalid)}
    </div>
  );
}
