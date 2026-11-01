"use client";

import { Link, usePathname } from "@/app/components/LocaleLink";

import { useEffect, useState } from "react";
import { XIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import {
  COMPARE_MAX,
  clearCompare,
  removeFromCompare,
  useCompareIds,
} from "@skelet/storefront-react/compare/store";
import { mediaThumb } from "@skelet/storefront-core/media-thumb";

type TrayProduct = {
  id: string;
  name: string;
  sku: string;
  mainImage?: { url: string; altText?: string | null } | null;
};

export function CompareTray() {
  const ids = useCompareIds();
  const pathname = usePathname();
  const t = useTranslations("compare");
  const tc = useTranslations("common");
  const [products, setProducts] = useState<TrayProduct[]>([]);
  const key = ids.join(",");

  useEffect(() => {
    if (!key) return;
    let cancelled = false;
    fetch(`/api/compare-products?ids=${key}`)
      .then((r) => r.json())
      .then((d: { products?: TrayProduct[] }) => {
        if (!cancelled) setProducts(d.products ?? []);
      })
      .catch(() => {
        if (!cancelled) setProducts([]);
      });
    return () => {
      cancelled = true;
    };
  }, [key]);

  if (ids.length === 0 || pathname === "/compare") return null;

  // Order by the stored ids so the tray matches the order things were added
  const picked = ids
    .map((id) => products.find((p) => p.id === id))
    .filter((p): p is TrayProduct => !!p);
  const emptySlots = Math.max(0, COMPARE_MAX - ids.length);
  const ready = ids.length >= 2;

  return (
    <div className="pointer-events-none fixed inset-x-0 bottom-4 z-40 flex justify-center px-4">
      <div
        role="region"
        aria-label={t("list")}
        className="glass-strong pointer-events-auto flex max-w-full items-center gap-3 overflow-x-auto rounded-xl px-3 py-2.5"
      >
        <span className="hidden shrink-0 pl-1 pr-1 text-xs font-medium uppercase tracking-wider text-muted-foreground sm:block">
          {t("title")}
        </span>

        <ul className="flex shrink-0 items-center gap-2">
          {picked.map((p) => (
            <li key={p.id} className="relative">
              <span className="block size-12 overflow-hidden rounded-lg bg-muted">
                {p.mainImage?.url ? (
                  // eslint-disable-next-line @next/next/no-img-element
                  <img
                    src={mediaThumb(p.mainImage.url, 160)}
                    alt={p.mainImage.altText || p.name}
                    title={p.name}
                    className="size-full object-cover"
                  loading="lazy" decoding="async" />
                ) : (
                  <span className="flex size-full items-center justify-center px-1 text-center text-[9px] leading-tight text-muted-foreground">
                    {p.sku}
                  </span>
                )}
              </span>
              <button
                type="button"
                onClick={() => removeFromCompare(p.id)}
                aria-label={t("remove", { name: p.name })}
                className="absolute -right-1.5 -top-1.5 flex size-5 items-center justify-center rounded-full bg-foreground text-background shadow-sm transition-colors hover:bg-foreground/85"
              >
                <XIcon className="size-3" aria-hidden="true" />
              </button>
            </li>
          ))}
          {Array.from({ length: emptySlots }, (_, i) => (
            <li
              key={`slot-${i}`}
              aria-hidden="true"
              className="hidden size-12 rounded-lg border border-dashed border-border sm:block"
            />
          ))}
        </ul>

        <span aria-hidden="true" className="h-8 w-px shrink-0 bg-border" />

        <div className="flex shrink-0 items-center gap-1">
          {ready ? (
            <Link
              href="/compare"
              className="inline-flex h-10 items-center rounded-md bg-primary px-5 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
            >
              {t("compareN", { count: ids.length })}
            </Link>
          ) : (
            <span className="inline-flex h-10 items-center rounded-md bg-muted px-5 text-sm font-semibold text-muted-foreground">
              {t("addOneMore")}
            </span>
          )}
          <button
            type="button"
            onClick={clearCompare}
            className="inline-flex h-10 items-center rounded-md px-3 text-sm font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            {tc("clear")}
          </button>
        </div>
      </div>
    </div>
  );
}
