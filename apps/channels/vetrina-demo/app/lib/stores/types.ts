import type { PublicStoresQuery, StoreProductQuery } from "./operations.gql";
export type StoresResult = PublicStoresQuery["publicStores"];
export type Store = StoresResult["items"][number];
export type StoreProduct = NonNullable<StoreProductQuery["product"]>;
export type ProductHit = { id: string; name: string | null; sku: string };
export const preferenceKey = (channelId: string) => `skelet:preferred-store:${channelId}`;
export function readPreference(channelId: string): string | null {
  try {
    const value = localStorage.getItem(preferenceKey(channelId));
    if (value && !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value)) {
      localStorage.removeItem(preferenceKey(channelId));
      return null;
    }
    return value;
  } catch { return null; }
}
export function savePreference(channelId: string, storeId: string | null) {
  try {
    if (storeId) localStorage.setItem(preferenceKey(channelId), storeId);
    else localStorage.removeItem(preferenceKey(channelId));
  } catch { /* Browser storage may be disabled. */ }
  window.dispatchEvent(new Event("skelet:preferred-store"));
}
export function directionsHref(store: Store): string | null {
  const address = [store.line1, store.line2, store.city, store.region, store.postalCode, store.countryCode].filter(Boolean).join(", ");
  const destination = store.latitude != null && store.longitude != null ? `${store.latitude},${store.longitude}` : address;
  return destination ? `https://www.google.com/maps/dir/?api=1&destination=${encodeURIComponent(destination)}` : null;
}
