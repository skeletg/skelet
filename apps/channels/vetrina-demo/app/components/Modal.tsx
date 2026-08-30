"use client";

// Native <dialog> modal - showModal() brings focus trap, Escape, inert background
// and focus restore. Animation and scroll lock are the only bits it doesn't

import { useEffect, useRef, type ReactNode } from "react";

let openModals = 0;

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Accessible name - there's no visible heading */
  label: string;
  /** Classes for the <dialog> - position, size, surface */
  className?: string;
  /** Skip the page-recede - the lightbox dims instead */
  children: ReactNode;
};

export function Modal({
  open,
  onOpenChange,
  label,
  className = "",
  children,
}: Props) {
  const ref = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    // showModal on an open dialog throws
    if (open && !el.open) el.showModal();
    else if (!open && el.open) el.close();
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

  return (
    <dialog
      ref={ref}
      aria-label={label}
      // Catches Escape and form-method=dialog too, so closes report from one place
      onClose={() => onOpenChange(false)}
      onCancel={(e) => {
        e.preventDefault();
        onOpenChange(false);
      }}
      // The dialog box is the full-viewport target - a hit on it is a backdrop click
      onClick={(e) => {
        if (e.target === ref.current) onOpenChange(false);
      }}
      className={className}
    >
      {children}
    </dialog>
  );
}
