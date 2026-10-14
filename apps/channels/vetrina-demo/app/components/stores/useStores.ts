"use client";
import { useLocale } from "next-intl";
import { useEffect, useState } from "react";
import { readPreference, savePreference, type StoresResult } from "@/app/lib/stores/types";

export function useStores(channelId: string, filters: Record<string, string>) {
  const locale = useLocale();
  const [refresh, setRefresh] = useState(0);
  const [snapshot, setSnapshot] = useState<{ key: string; data: StoresResult | null; error: boolean } | null>(null);
  const [success, setSuccess] = useState<{ filterKey: string; channelId: string; data: StoresResult } | null>(null);
  const filterKey = JSON.stringify({ ...filters, locale });
  const requestKey = JSON.stringify([channelId, filterKey, refresh]);
  useEffect(() => {
    const reload = () => setRefresh(n => n + 1);
    window.addEventListener("skelet:preferred-store", reload);
    window.addEventListener("storage", reload);
    window.addEventListener("focus", reload);
    return () => {
      window.removeEventListener("skelet:preferred-store", reload);
      window.removeEventListener("storage", reload);
      window.removeEventListener("focus", reload);
    };
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    const params = new URLSearchParams(JSON.parse(filterKey) as Record<string, string>);
    const preferred = readPreference(channelId);
    if (preferred) params.set("preferredStoreId", preferred);
    fetch(`/api/stores?${params}`, { signal: controller.signal, cache: "no-store" })
      .then(async response => {
        if (!response.ok) throw new Error("Unable to load stores");
        return response.json() as Promise<StoresResult>;
      })
      .then(result => {
        if (!active) return;
        // Disablement is temporary; don't erase the preference in that case.
        if (result.enabled && preferred && !result.preferredStore) savePreference(channelId, null);
        setSuccess({ filterKey, channelId, data: result });
        setSnapshot({ key: requestKey, data: result, error: false });
      })
      .catch(() => { if (active) setSnapshot({ key: requestKey, data: null, error: true }); });
    return () => { active = false; controller.abort(); };
  }, [channelId, filterKey, requestKey]);
  // Clear rows synchronously when the request changes. Keep only the capability
  // gate for same-product refreshes, and channel-wide city choices, while loading.
  const loading = snapshot?.key !== requestKey;
  return {
    data: loading ? null : snapshot?.data ?? null,
    previous: success?.channelId === channelId && success.filterKey === filterKey ? success.data : null,
    hasPickupStores: success?.channelId === channelId && success.data.hasPickupStores,
    cities: success?.channelId === channelId ? success.data.cities : [],
    error: !loading && !!snapshot?.error,
    loading,
    retry: () => setRefresh(n => n + 1),
  };
}
