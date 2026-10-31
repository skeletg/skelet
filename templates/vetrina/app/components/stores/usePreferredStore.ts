"use client";
import { useEffect, useSyncExternalStore } from "react";
import type { StoresFormResult } from "@skelet/storefront-next/stores/actions";
import { readPreference, savePreference } from "@skelet/storefront-next/stores/types";

// savePreference announces changes in this tab; "storage" carries them from other tabs
function subscribe(onChange: () => void) {
  window.addEventListener("skelet:preferred-store", onChange);
  window.addEventListener("storage", onChange);
  return () => {
    window.removeEventListener("skelet:preferred-store", onChange);
    window.removeEventListener("storage", onChange);
  };
}

export function usePreferredStore(channelId: string): string | null {
  return useSyncExternalStore(subscribe, () => readPreference(channelId), () => null);
}

// The backend answered for the stored preference and no longer lists that store, so forget it. A disabled feature is
// temporary and doesn't count, nor does an answer to an older preference than the one now stored
export function useForgetMissingPreference(channelId: string, result: StoresFormResult | undefined) {
  useEffect(() => {
    const askedFor = result?.query?.preferredStoreId;
    if (result?.stores?.enabled && !result.stores.preferredStore && askedFor && askedFor === readPreference(channelId)) {
      savePreference(channelId, null);
    }
  }, [channelId, result]);
}
