"use client";
import { useTranslations } from "next-intl";
import { directionsHref, savePreference, type Store } from "@skelet/storefront-core/stores/types";

const days = ["monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"] as const;
export function StoreStatus({ status }: { status: Store["stockStatus"] }) {
  const t = useTranslations("stores");
  return status ? <span className="text-sm font-medium" data-stock-status={status}>{t(`status.${status}`)}</span> : null;
}
export function StoreRows({ stores, channelId, preferredId }: { stores: Store[]; channelId: string; preferredId?: string }) {
  const t = useTranslations("stores");
  return <ul className="divide-y divide-border" aria-label={t("results")}>
    {stores.map(store => {
      const directions = directionsHref(store);
      const hours = store.hours && typeof store.hours === "object" ? store.hours as Record<string, { closed?: boolean; open?: string; close?: string }> : {};
      const scheduled = days.filter(day => hours[day] && typeof hours[day] === "object" && (hours[day].closed === true || (typeof hours[day].open === "string" && typeof hours[day].close === "string")));
      return <li key={store.id} className="space-y-2 py-5">
        <div className="flex flex-wrap items-center justify-between gap-2"><h3 className="font-semibold">{store.name}</h3><StoreStatus status={store.stockStatus} /></div>
        {store.distanceKm != null && <p className="text-sm">{t("distance", { distance: store.distanceKm.toFixed(1) })}</p>}
        <p className="text-sm text-muted-foreground">{[store.line1, store.line2, store.city, store.region, store.postalCode, store.countryCode].filter(Boolean).join(", ")}</p>
        <p className="text-sm">{t(store.pickupEnabled ? "pickupAvailable" : "walkInShopping")}</p>
        {store.phone && <a className="block text-sm underline" href={`tel:${store.phone}`}>{store.phone}</a>}
        {scheduled.length > 0 && <details className="text-sm"><summary>{t("hours")}</summary><dl className="mt-2 space-y-1">{scheduled.map(day => <div className="flex justify-between gap-4" key={day}><dt>{t(`days.${day}`)}</dt><dd>{hours[day].closed ? t("closed") : `${hours[day].open}–${hours[day].close}`}</dd></div>)}</dl>{store.timezone && <p className="mt-1 text-xs text-muted-foreground">{store.timezone}</p>}</details>}
        <div className="flex gap-4 text-sm">
          {directions && <a className="underline" href={directions} target="_blank" rel="noopener noreferrer">{t("directions")}</a>}
          <button type="button" className="underline" disabled={preferredId === store.id} onClick={() => savePreference(channelId, store.id)}>{t(preferredId === store.id ? "myStore" : "setMyStore")}</button>
        </div>
      </li>;
    })}
  </ul>;
}
