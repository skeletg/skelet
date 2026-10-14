"use client";
import { useEffect, useRef, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { useChannelId } from "@/app/components/LocaleLink";
import type { ProductHit, StoreProduct } from "@/app/lib/stores/types";
import { useStores } from "./useStores";
import { StoreRows } from "./StoreRows";

export function StoreFinder({ productId, productName, allowProductSearch = false }: { productId?: string; productName?: string; allowProductSearch?: boolean }) {
  const t = useTranslations("stores");
  const locale = useLocale();
  const channelId = useChannelId();
  const [city, setCity] = useState("");
  const [search, setSearch] = useState("");
  const [pickupOnly, setPickupOnly] = useState(false);
  const [inStockOnly, setInStockOnly] = useState(false);
  const [offset, setOffset] = useState(0);
  const [coordinates, setCoordinates] = useState<{ latitude: number; longitude: number } | null>(null);
  const [geoState, setGeoState] = useState<"idle" | "loading" | "error">("idle");
  const [selected, setSelected] = useState<StoreProduct | null>(null);
  const [selectedVariant, setSelectedVariant] = useState("");
  const [productSearch, setProductSearch] = useState("");
  const [hits, setHits] = useState<ProductHit[]>([]);
  const [searchedProducts, setSearchedProducts] = useState(false);
  const [productPending, setProductPending] = useState(false);
  const [productError, setProductError] = useState(false);
  const productRequest = useRef<AbortController | null>(null);
  useEffect(() => () => productRequest.current?.abort(), []);
  const activeProduct = productId ?? (selected?.kind === "SIMPLE" ? selected.id : selectedVariant || undefined);
  const filters: Record<string, string> = { city, search, offset: String(offset), pickupOnly: String(pickupOnly) };
  if (activeProduct) { filters.productId = activeProduct; filters.inStockOnly = String(inStockOnly); }
  if (coordinates) { filters.latitude = String(coordinates.latitude); filters.longitude = String(coordinates.longitude); }
  const { data, error, loading, retry, cities, hasPickupStores } = useStores(channelId, filters);
  async function productRequestFor(action: "search" | "product", value: string) {
    productRequest.current?.abort();
    const controller = new AbortController(); productRequest.current = controller;
    setProductPending(true); setProductError(false); setHits([]);
    setSearchedProducts(action === "search");
    if (action === "product") { setSelected(null); setSelectedVariant(""); setInStockOnly(false); setOffset(0); }
    try {
      const params = new URLSearchParams({ action, locale, [action === "search" ? "search" : "productId"]: value });
      const res = await fetch(`/api/stores?${params}`, { signal: controller.signal, cache: "no-store" });
      if (!res.ok) throw new Error("Unavailable");
      const result = await res.json();
      if (controller.signal.aborted) return;
      if (action === "search") setHits(result.items);
      else setSelected(result.product);
    } catch { if (!controller.signal.aborted) setProductError(true); }
    finally { if (!controller.signal.aborted) setProductPending(false); }
  }
  function nearMe() {
    if (!navigator.geolocation) { setGeoState("error"); return; }
    setGeoState("loading");
    navigator.geolocation.getCurrentPosition(position => {
      setCoordinates({ latitude: position.coords.latitude, longitude: position.coords.longitude });
      setOffset(0); setGeoState("idle");
    }, () => setGeoState("error"), { timeout: 10000, maximumAge: 60000 });
  }
  const reset = () => setOffset(0);
  if (data && !data.enabled) return <p>{t("disabled")}</p>;
  return <div className="space-y-5">
    {(productName || selected) && <p className="font-medium">{productName ?? selected?.name ?? selected?.sku}</p>}
    {allowProductSearch && data?.stockCheckingAvailable !== false && <div className="space-y-3">
      <form className="flex gap-2" onSubmit={e => { e.preventDefault(); void productRequestFor("search", productSearch); }}>
        <label className="flex-1 text-sm">{t("productSearch")}<input className="mt-1 w-full rounded-md border border-border bg-background p-2" value={productSearch} maxLength={200} onChange={e => setProductSearch(e.target.value)} /></label>
        <button className="self-end rounded-md border border-border p-2" disabled={productPending || !productSearch.trim()}>{t("searchButton")}</button>
      </form>
      {productPending && <p role="status">{t("loading")}</p>}
      {productError && <p role="alert">{t("productError")}</p>}
      <ul className="divide-y divide-border">{hits.map(hit => <li key={hit.id}><button className="w-full py-2 text-left underline" onClick={() => void productRequestFor("product", hit.id)}>{hit.name || hit.sku} · {hit.sku}</button></li>)}</ul>
      {searchedProducts && !productPending && !productError && hits.length === 0 && <p>{t("noProducts")}</p>}
      {selected?.kind === "CONFIGURABLE" && <label className="block text-sm">{t("variant")}<select className="ml-2 rounded-md border border-border bg-background p-2" value={selectedVariant} onChange={e => { setSelectedVariant(e.target.value); reset(); }}><option value="">{t("selectVariant")}</option>{selected.children.filter(child => child.kind === "SIMPLE").map(child => <option key={child.id} value={child.id}>{child.name || child.sku} · {child.sku}</option>)}</select></label>}
      {selected && !["SIMPLE", "CONFIGURABLE"].includes(selected.kind) && <p>{t("unsupported")}</p>}
      {selected && <button className="text-sm underline" onClick={() => { setSelected(null); setSelectedVariant(""); setInStockOnly(false); reset(); }}>{t("clearProduct")}</button>}
    </div>}
    <div className="grid gap-3 sm:grid-cols-2">
      <label className="text-sm">{t("city")}<select className="mt-1 w-full rounded-md border border-border bg-background p-2" value={city} onChange={e => { setCity(e.target.value); reset(); }}><option value="">{t("allCities")}</option>{cities.map(value => <option key={value} value={value}>{value}</option>)}</select></label>
      <label className="text-sm">{t("storeSearch")}<input className="mt-1 w-full rounded-md border border-border bg-background p-2" value={search} maxLength={200} onChange={e => { setSearch(e.target.value); reset(); }} /></label>
    </div>
    <div className="flex flex-wrap items-center gap-4 text-sm">
      {(hasPickupStores || pickupOnly) && <label className="flex gap-2"><input type="checkbox" checked={pickupOnly} onChange={e => { setPickupOnly(e.target.checked); reset(); }} />{t("pickupOnly")}</label>}
      <label className="flex gap-2"><input type="checkbox" checked={inStockOnly} disabled={!activeProduct || !data?.stockCheckingAvailable || !data.productEligible} onChange={e => { setInStockOnly(e.target.checked); reset(); }} />{t("inStockOnly")}</label>
      <button className="underline" disabled={geoState === "loading"} onClick={nearMe}>{t(geoState === "loading" ? "locating" : "nearMe")}</button>
      {coordinates && <button className="underline" onClick={() => { setCoordinates(null); reset(); }}>{t("clearLocation")}</button>}
    </div>
    {geoState === "error" && <p role="status" className="text-sm">{t("geoDenied")}</p>}
    {data && !data.stockCheckingAvailable && <p>{t("directoryOnly")}</p>}
    {data && activeProduct && data.stockCheckingAvailable && !data.productEligible && <p>{t("unknownProduct")}</p>}
    {error ? <div role="alert"><p>{t("error")}</p><button className="underline" onClick={retry}>{t("retry")}</button></div> : loading ? <p role="status">{t("loading")}</p> : data && <>
      <p role="status" className="text-sm">{t("count", { count: data.totalCount })}</p>
      {data.items.length ? <StoreRows stores={data.items} channelId={channelId} preferredId={data.preferredStore?.id} /> : <p>{t("empty")}</p>}
      <div className="flex justify-between"><button disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - 20))}>{t("previous")}</button><button disabled={offset + 20 >= data.totalCount} onClick={() => setOffset(offset + 20)}>{t("next")}</button></div>
    </>}
    {activeProduct && <p className="text-xs text-muted-foreground">{t("notice")}</p>}
  </div>;
}
