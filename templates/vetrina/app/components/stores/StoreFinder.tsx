"use client";
import { useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { useChannelId } from "@/app/components/LocaleLink";
import { useActionForm } from "@skelet/storefront-next/forms/use-action-form";
import { findStoresAction } from "@skelet/storefront-next/stores/actions";
import { storeFiltersSchema } from "@skelet/storefront-next/stores/schemas";
import { STORE_PAGE_SIZE } from "@skelet/storefront-next/stores/types";
import { useForgetMissingPreference, usePreferredStore } from "./usePreferredStore";
import { StoreRows } from "./StoreRows";

type Coordinates = { latitude: number; longitude: number };
const inputClass = "mt-1 w-full rounded-md border border-border bg-background p-2";

export function StoreFinder({ productId, productName }: { productId?: string; productName?: string }) {
  const t = useTranslations("stores");
  const channelId = useChannelId();
  const preferredId = usePreferredStore(channelId);
  const { form, fields, formAction, onSubmit, pending, lastResult } = useActionForm(findStoresAction, storeFiltersSchema);
  const formRef = useRef<HTMLFormElement>(null);
  const [coordinates, setCoordinates] = useState<Coordinates | null>(null);
  const [geoState, setGeoState] = useState<"idle" | "loading" | "error">("idle");
  // The first search runs on open, and again once the hidden inputs carry a new product, position or preferred store;
  // the shopper's filters carry over, so switching variants keeps "in stock only"
  useEffect(() => formRef.current?.requestSubmit(), [productId, coordinates, preferredId]);
  const stores = lastResult?.stores;
  useForgetMissingPreference(channelId, lastResult);

  function nearMe() {
    if (!navigator.geolocation) return setGeoState("error");
    setGeoState("loading");
    navigator.geolocation.getCurrentPosition(position => {
      setCoordinates({ latitude: position.coords.latitude, longitude: position.coords.longitude });
      setGeoState("idle");
    }, () => setGeoState("error"), { timeout: 10000, maximumAge: 60000 });
  }

  if (stores && !stores.enabled) return <p>{t("disabled")}</p>;
  // The pager's submitter carries the offset; any other submit (a filter change) starts again from the first page
  const offset = lastResult?.query?.offset ?? 0;
  const canFilterStock = !!productId && !!stores?.stockCheckingAvailable && stores.productEligible;
  return <div className="space-y-5">
    {productName && <p className="font-medium">{productName}</p>}
    {/* The select and checkboxes apply at once; the text search waits for Enter, since a submit per keystroke queues
        one uncancellable server action per letter */}
    <form ref={formRef} id={form.id} action={formAction} onSubmit={onSubmit} onChange={e => { if (!(e.target instanceof HTMLInputElement && e.target.type === "text")) e.currentTarget.requestSubmit(); }} noValidate className="space-y-5">
      {productId && <input type="hidden" name="productId" value={productId} />}
      {preferredId && <input type="hidden" name="preferredStoreId" value={preferredId} />}
      {coordinates && <>
        <input type="hidden" name="latitude" value={coordinates.latitude} />
        <input type="hidden" name="longitude" value={coordinates.longitude} />
      </>}
      <div className="grid gap-3 sm:grid-cols-2">
        <label className="text-sm">{t("city")}
          <select key={fields.city.key} name={fields.city.name} defaultValue={String(fields.city.initialValue ?? "")} className={inputClass}>
            <option value="">{t("allCities")}</option>
            {(stores?.cities ?? []).map(city => <option key={city} value={city}>{city}</option>)}
          </select>
        </label>
        <label className="text-sm">{t("storeSearch")}
          <input key={fields.search.key} name={fields.search.name} defaultValue={String(fields.search.initialValue ?? "")} maxLength={200} className={inputClass} />
        </label>
      </div>
      <div className="flex flex-wrap items-center gap-4 text-sm">
        {(stores?.hasPickupStores || fields.pickupOnly.value === "on") && <label className="flex gap-2">
          <input type="checkbox" key={fields.pickupOnly.key} name={fields.pickupOnly.name} defaultChecked={fields.pickupOnly.initialValue === "on"} />{t("pickupOnly")}
        </label>}
        <label className="flex gap-2">
          <input type="checkbox" key={fields.inStockOnly.key} name={fields.inStockOnly.name} defaultChecked={fields.inStockOnly.initialValue === "on"} disabled={!canFilterStock} />{t("inStockOnly")}
        </label>
        <button type="button" className="underline" disabled={geoState === "loading"} onClick={nearMe}>{t(geoState === "loading" ? "locating" : "nearMe")}</button>
        {coordinates && <button type="button" className="underline" onClick={() => setCoordinates(null)}>{t("clearLocation")}</button>}
      </div>
      {geoState === "error" && <p role="status" className="text-sm">{t("geoDenied")}</p>}
      {stores && !stores.stockCheckingAvailable && <p>{t("directoryOnly")}</p>}
      {stores && productId && stores.stockCheckingAvailable && !stores.productEligible && <p>{t("unknownProduct")}</p>}
      {pending || !lastResult ? <p role="status">{t("loading")}</p> : !stores ? <div role="alert">
        <p>{t("error")}</p>
        <button type="submit" className="underline">{t("retry")}</button>
      </div> : <>
        <p role="status" className="text-sm">{t("count", { count: stores.totalCount })}</p>
        {stores.items.length ? <StoreRows stores={stores.items} channelId={channelId} preferredId={preferredId ?? undefined} /> : <p>{t("empty")}</p>}
        <div className="flex justify-between">
          <button type="submit" name="offset" value={Math.max(0, offset - STORE_PAGE_SIZE)} disabled={offset === 0}>{t("previous")}</button>
          <button type="submit" name="offset" value={offset + STORE_PAGE_SIZE} disabled={offset + STORE_PAGE_SIZE >= stores.totalCount}>{t("next")}</button>
        </div>
      </>}
    </form>
    {productId && <p className="text-xs text-muted-foreground">{t("notice")}</p>}
  </div>;
}
