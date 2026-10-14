"use client";
import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useTranslations } from "next-intl";
import { Link, useChannelId } from "@/app/components/LocaleLink";
import { useStores } from "./useStores";
import { StoreStatus } from "./StoreRows";
import { StoreFinder } from "./StoreFinder";

// Read the gate after hydration even on cached PDPs and navigation shells.
export function StoresLink() {
  const { data, previous, loading } = useStores(useChannelId(), {});
  const t = useTranslations("stores");
  return (data ?? (loading ? previous : null))?.enabled ? <Link className="text-sm underline" href="/stores">{t("title")}</Link> : null;
}

export function StoreAvailability({ productId, productName, needsVariant = false }: { productId?: string; productName?: string; needsVariant?: boolean }) {
  const t = useTranslations("stores");
  const { data, previous, error, retry } = useStores(useChannelId(), productId ? { productId } : {});
  const [open, setOpen] = useState(false);
  const dialog = useRef<HTMLDialogElement>(null);
  const gate = data ?? previous;
  const eligible = !!gate?.enabled && gate.stockCheckingAvailable && (needsVariant && !productId || gate.productEligible);
  useEffect(() => {
    if (open && eligible && dialog.current && !dialog.current.open) dialog.current.showModal();
  }, [open, eligible]);
  if (error) return <button className="mt-4 text-sm underline" onClick={retry}>{t("retryCheck")}</button>;
  if (!eligible) return null;
  return <div className="mt-4 space-y-2">
    {data?.preferredStore && productId && <p className="text-sm">{t("myStore")}: {data.preferredStore.name} · <StoreStatus status={data.preferredStore.stockStatus} /></p>}
    <button type="button" className="text-sm underline disabled:opacity-60" disabled={!productId} onClick={() => setOpen(true)}>{t("check")}</button>
    {!productId && needsVariant && <p className="text-xs text-muted-foreground">{t("selectVariant")}</p>}
    {open && createPortal(<dialog ref={dialog} aria-labelledby="store-availability-title" onClose={() => setOpen(false)} className="fixed inset-0 m-auto h-full max-h-dvh w-full max-w-none overflow-y-auto bg-background p-5 text-foreground backdrop:bg-black/50 sm:h-auto sm:max-h-[85dvh] sm:max-w-2xl sm:rounded-md sm:border sm:border-border sm:p-6">
      <div className="mb-5 flex items-center justify-between gap-4"><h2 id="store-availability-title" className="text-xl font-semibold">{t("check")}</h2><button autoFocus type="button" className="underline" onClick={() => dialog.current?.close()}>{t("close")}</button></div>
      <StoreFinder key={productId} productId={productId} productName={productName} />
    </dialog>, document.body)}
  </div>;
}
