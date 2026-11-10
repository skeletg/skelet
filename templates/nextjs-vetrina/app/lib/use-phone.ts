import { useSyncExternalStore } from "react";

// The breakpoint the site's drawers turn into bottom sheets at, and the panels give way to them
export const PHONE_QUERY = "(max-width: 767px)";

export function isPhone(): boolean {
  return typeof window !== "undefined" && window.matchMedia(PHONE_QUERY).matches;
}

export function usePhone(): boolean {
  return useSyncExternalStore(
    (onChange) => {
      const query = window.matchMedia(PHONE_QUERY);
      query.addEventListener("change", onChange);
      return () => query.removeEventListener("change", onChange);
    },
    isPhone,
    () => false,
  );
}
