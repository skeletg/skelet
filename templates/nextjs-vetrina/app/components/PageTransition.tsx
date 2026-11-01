"use client";

import { usePathname } from "next/navigation";
import { ViewTransition, type ReactNode } from "react";

import { RECORDING_BLOCK_CLASS, isPrivateRecordingPath } from "@skelet/storefront-core/analytics/recording-policy";

// Load-more walks /pg/2, /pg/3 onto the same listing and appends to it, so that suffix is not a
// route change - fading there would flicker rows the shopper is already reading
const PAGE_SEGMENT = /\/pg\/\d+$/;

// React only starts a view transition when a boundary takes part, so without this wrapper a plain
// page-to-page navigation hard-cuts
//
// The key is the gate - update= fires on any commit that moves the slot, hydration settling or a
// drawer locking the scrollbar included, which is what made every landing flash and the cart
// glitch on first open. Re-keying means only a route change is an enter and nothing else animates
export function PageTransition({ children }: { children: ReactNode }) {
  const route = usePathname().replace(PAGE_SEGMENT, "");
  // Blocked in the same commit that renders a private page, so recorders never see its DOM mid-navigation
  const block = isPrivateRecordingPath(route) ? ` ${RECORDING_BLOCK_CLASS}` : "";
  return (
    <ViewTransition key={route} enter="page-fade" default="none">
      <div className={`flex-1${block}`}>{children}</div>
    </ViewTransition>
  );
}
