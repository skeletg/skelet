"use client";

// Native <dialog> modal - showModal() brings focus trap, Escape, inert background
// and focus restore. Animation and scroll lock are the only bits it doesn't

import { useCallback, useEffect, useRef, type ReactNode } from "react";

// Past this it closes on release, under it it springs back
const DISMISS_PX = 110;
// A flick closes even from a short drag
const FLICK_PX_PER_MS = 0.5;

let openModals = 0;

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Accessible name - there's no visible heading */
  label: string;
  /** Classes for the <dialog> - position, size, surface */
  className?: string;
  /** Phone-only drag-to-close; needs a [data-drawer-handle] child */
  dragDismiss?: boolean;
  /** Skip the page-recede - the lightbox dims instead */
  children: ReactNode;
};

export function Modal({
  open,
  onOpenChange,
  label,
  className = "",
  dragDismiss = false,
  children,
}: Props) {
  const ref = useRef<HTMLDialogElement>(null);
  // Not state - it changes every pointermove and must not render
  const drag = useRef({ id: -1, startY: 0, startT: 0, dy: 0, moved: false });

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    // showModal on an open dialog throws
    if (open && !el.open) {
      // Cleared on the way in; doing it on close would cut the exit short
      el.style.transition = "";
      el.style.translate = "";
      el.style.setProperty("--drawer-drag", "0");
      el.showModal();
    } else if (!open && el.open) el.close();
  }, [open]);

  // Refcounted - cart, menu and auth are all mounted at once, last one out clears
  useEffect(() => {
    if (!open) return;
    openModals++;
    document.body.style.overflow = "hidden";
    return () => {
      openModals--;
      if (openModals <= 0) {
        document.body.style.overflow = "";
      }
    };
  }, [open]);

  // Clearing the inline styles hands the sheet back to the CSS transition
  const release = useCallback((el: HTMLDialogElement) => {
    el.style.transition = "";
    el.style.translate = "";
    el.style.setProperty("--drawer-drag", "0");
  }, []);

  const onPointerDown = (e: React.PointerEvent<HTMLDialogElement>) => {
    const el = ref.current;
    if (!dragDismiss || !el || e.pointerType === "mouse") return;
    if (!window.matchMedia("(max-width: 767px)").matches) return;
    const target = e.target as HTMLElement;
    // Declared, not inferred: touch-action must be set before the gesture starts
    if (!target.closest("[data-drawer-handle]")) return;
    drag.current = { id: e.pointerId, startY: e.clientY, startT: e.timeStamp, dy: 0, moved: false };
    el.style.transition = "none";
  };

  const onPointerMove = (e: React.PointerEvent<HTMLDialogElement>) => {
    const el = ref.current;
    const d = drag.current;
    if (!el || d.id !== e.pointerId) return;
    const dy = e.clientY - d.startY;
    if (dy <= 0) { d.dy = 0; el.style.translate = "0 0"; return; }
    if (!d.moved && dy > 4) {
      d.moved = true;
      // Claimed only once it's clearly a drag, so taps still work
      el.setPointerCapture(e.pointerId);
    }
    d.dy = dy;
    el.style.translate = `0 ${dy}px`;
    // Backdrop follows the sheet
    el.style.setProperty("--drawer-drag", String(Math.min(dy / DISMISS_PX, 1)));
  };

  const onPointerUp = (e: React.PointerEvent<HTMLDialogElement>) => {
    const el = ref.current;
    const d = drag.current;
    if (!el || d.id !== e.pointerId) return;
    drag.current = { ...d, id: -1 };
    if (el.hasPointerCapture(e.pointerId)) el.releasePointerCapture(e.pointerId);
    const velocity = d.dy / Math.max(e.timeStamp - d.startT, 1);
    if (d.moved && (d.dy > DISMISS_PX || velocity > FLICK_PX_PER_MS)) {
      // Continue downward - resetting first would bounce the sheet up, then fade
      el.style.transition = "";
      el.style.translate = "0 100%";
      onOpenChange(false);
      return;
    }
    release(el);
  };

  return (
    <dialog
      ref={ref}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerUp}
      aria-label={label}
      // Catches Escape and form-method=dialog too, so closes report from one place
      onClose={() => onOpenChange(false)}
      onCancel={(e) => {
        e.preventDefault();
        onOpenChange(false);
      }}
      // The dialog box is the full-viewport target - a hit on it is a backdrop click
      onClick={(e) => {
        // A drag that ends on the dialog box is not a backdrop tap
        if (drag.current.moved) { drag.current.moved = false; return; }
        if (e.target === ref.current) onOpenChange(false);
      }}
      className={className}
    >
      {children}
    </dialog>
  );
}
