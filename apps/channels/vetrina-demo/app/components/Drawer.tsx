"use client";

// Slide-in panel for the header menu, cart and auth drawers
// Modal underneath; the slide and backdrop fade are .drawer CSS in globals.css

import type { ReactNode } from "react";
import { useTranslations } from "next-intl";

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
  const t = useTranslations("common");
  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      label={title}
      dragDismiss
      className={[
        "drawer",
        `drawer-${side}`,
        "glass-strong flex flex-col gap-0 overflow-hidden rounded-xl p-0 text-sm shadow-2xl",
        sideDefaults[side],
        className ?? "",
      ].join(" ")}
    >
      {/* Phones only. touch-none stops the browser claiming the swipe as a scroll */}
      <div
        data-drawer-handle
        aria-hidden="true"
        className="flex shrink-0 cursor-grab touch-none justify-center pb-2 pt-3.5 active:cursor-grabbing md:hidden"
      >
        <span className="h-1 w-10 rounded-full bg-foreground/30" />
      </div>
      {hideTitle ? null : (
        <h2 className="px-4 pt-4 font-heading text-base font-medium md:pt-4">{title}</h2>
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
        <span className="sr-only">{t("close")}</span>
      </button>
      {children}
    </Modal>
  );
}
