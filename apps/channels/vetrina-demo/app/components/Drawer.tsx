"use client";

// Slide-in panel for the header menu, cart and auth drawers
// Modal underneath; the slide and backdrop fade are .drawer CSS in globals.css

import type { ReactNode } from "react";

import { Modal } from "@/app/components/Modal";

export type DrawerSide = "left" | "right" | "bottom";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  side: DrawerSide;
  // Always the panel's accessible name, rendered visibly unless hideTitle
  title: string;
  hideTitle?: boolean;
  className?: string;
  children: ReactNode;
};

const sideDefaults: Record<DrawerSide, string> = {
  left: "w-80 sm:w-96",
  right: "w-[calc(100vw-1.5rem)] sm:max-w-md",
  bottom: "h-[80vh] max-h-[40rem]",
};

export function Drawer({
  open,
  onOpenChange,
  side,
  title,
  hideTitle = false,
  className,
  children,
}: Props) {
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      label={title}
      className={[
        "drawer",
        `drawer-${side}`,
        "glass-strong flex flex-col gap-0 overflow-hidden rounded-xl p-0 text-sm shadow-2xl",
        sideDefaults[side],
        className ?? "",
      ].join(" ")}
    >
      {hideTitle ? null : (
        <h2 className="px-4 pt-4 font-heading text-base font-medium">{title}</h2>
      )}
      <button
        type="button"
        onClick={() => onOpenChange(false)}
        className="absolute right-3 top-3 z-10 flex size-8 items-center justify-center rounded-md text-muted-foreground transition hover:bg-muted hover:text-foreground"
      >
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth={2}
          strokeLinecap="round"
          className="size-4"
          aria-hidden="true"
        >
          <path d="M18 6 6 18M6 6l12 12" />
        </svg>
        <span className="sr-only">Close</span>
      </button>
      {children}
    </Modal>
  );
}
