"use client";
import { startTransition, useActionState, useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useTranslations } from "next-intl";
import { useChannelId } from "@/app/components/LocaleLink";
import { findStoresAction } from "@skelet/storefront-next/stores/actions";
import { useForgetMissingPreference, usePreferredStore } from "./usePreferredStore";
import { StoreStatus } from "./StoreRows";
import { StoreFinder } from "./StoreFinder";

export function StoreAvailability({ productId, productName, needsVariant = false }: { productId?: string; productName?: string; needsVariant?: boolean }) {
  const t = useTranslations("stores");
  const channelId = useChannelId();
  const preferredId = usePreferredStore(channelId);
  const [result, findStores] = useActionState(findStoresAction, undefined);
  const [open, setOpen] = useState(false);
  const dialog = useRef<HTMLDialogElement>(null);
  // Checked after hydration, so a cached product page still follows the live switch and stock
  const check = useCallback(() => {
    const query = new FormData();
    if (productId) query.set("productId", productId);
    if (preferredId) query.set("preferredStoreId", preferredId);
    startTransition(() => findStores(query));
  }, [productId, preferredId, findStores]);
  useEffect(check, [check]);
  const stores = result?.stores;
  useForgetMissingPreference(channelId, result);
  const eligible = !!stores?.enabled && stores.stockCheckingAvailable && ((needsVariant && !productId) || stores.productEligible);
  useEffect(() => {
    if (open && eligible && dialog.current && !dialog.current.open) dialog.current.showModal();
  }, [open, eligible]);
  if (result && !stores) return <button className="mt-4 text-sm underline" onClick={check}>{t("retryCheck")}</button>;
  if (!eligible) return null;
  return <div className="mt-4 space-y-2">
    {stores.preferredStore && productId && <p className="text-sm">{t("myStore")}: {stores.preferredStore.name} · <StoreStatus status={stores.preferredStore.stockStatus} /></p>}
    <button type="button" className="text-sm underline disabled:opacity-60" disabled={!productId} onClick={() => setOpen(true)}>{t("check")}</button>
    {!productId && needsVariant && <p className="text-xs text-muted-foreground">{t("selectVariant")}</p>}
    {open && createPortal(<dialog ref={dialog} aria-labelledby="store-availability-title" onClose={() => setOpen(false)} className="fixed inset-0 m-auto h-full max-h-dvh w-full max-w-none overflow-y-auto bg-background p-5 text-foreground backdrop:bg-black/50 sm:h-auto sm:max-h-[85dvh] sm:max-w-2xl sm:rounded-md sm:border sm:border-border sm:p-6">
      <div className="mb-5 flex items-center justify-between gap-4"><h2 id="store-availability-title" className="text-xl font-semibold">{t("check")}</h2><button autoFocus type="button" className="underline" onClick={() => dialog.current?.close()}>{t("close")}</button></div>
      <StoreFinder key={productId} productId={productId} productName={productName} />
    </dialog>, document.body)}
  </div>;
}
