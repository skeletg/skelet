"use client";

import { useEffect } from "react";

// Removes a one-shot flag (?welcome=1, ?just_placed=1) from the address bar, so a refresh, bookmark or shared link
// can't replay the banner it shows. The purchase and sign-up events themselves are reported by the server
export function StripParam({ name }: { name: string }) {
  useEffect(() => {
    const url = new URL(window.location.href);
    if (!url.searchParams.has(name)) return;
    url.searchParams.delete(name);
    window.history.replaceState(null, "", url.pathname + url.search + url.hash);
  }, [name]);
  return null;
}
