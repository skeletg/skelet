"use client";

import { usePathname } from "next/navigation";
import { useEffect } from "react";

import { readClientConsent } from "@/app/lib/consent/client";
import { track } from "@/app/lib/track";

// A page_view for the server-side destinations on every page and client-side navigation - GA4 builds its sessions from
// them. It goes to the shop's own /t/e, so no analytics script loads; the backend sends it only where the visitor's
// choices allow, so any granted category is reason enough to send it
export function PageViewBeacon() {
  const pathname = usePathname();
  useEffect(() => {
    const c = readClientConsent();
    if (!c.analytics && !c.ads && !c.personalization) return;
    // A frame later, so the new page's title is in place before it is read
    const raf = requestAnimationFrame(() => track("page_view", crypto.randomUUID()));
    return () => cancelAnimationFrame(raf);
  }, [pathname]);
  return null;
}
