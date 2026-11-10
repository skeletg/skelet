"use client";

import { useSyncExternalStore } from "react";
import type { ReactNode } from "react";

const THRESHOLD_PX = 24;

function subscribe(onChange: () => void) {
  window.addEventListener("scroll", onChange, { passive: true });
  return () => window.removeEventListener("scroll", onChange);
}

export function HeaderShell({ children }: { children: ReactNode }) {
  const scrolled = useSyncExternalStore(
    subscribe,
    () => window.scrollY > THRESHOLD_PX,
    () => false,
  );
  return (
    <header
      data-scrolled={scrolled ? "" : undefined}
      // Named so the view transition treats the header as one persistent object instead of
      // snapshotting it per page - see the ::view-transition-group(site-header) rule in globals.css,
      // which then holds it still while the product image morphs underneath
      style={{ viewTransitionName: "site-header" }}
      className="site-header group/header sticky top-[var(--page-gutter,0px)] z-30 border-b border-border bg-background [transition:box-shadow_300ms,top_var(--assistant-speed)_var(--assistant-ease)] motion-reduce:transition-none data-[scrolled]:shadow-[0_8px_24px_-18px_oklch(0_0_0/0.5)]"
    >
      {children}
    </header>
  );
}
