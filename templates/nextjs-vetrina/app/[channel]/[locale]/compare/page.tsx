"use client";

import { Link } from "@/app/components/LocaleLink";
import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { PlusIcon } from "lucide-react";

import { BackButton } from "@/app/components/BackButton";
import { RatingStars } from "@/app/components/RatingStars";
import {
  COMPARE_MAX,
  clearCompare,
  removeFromCompare,
  useCompareIds,
} from "@skelet/storefront-react/compare/store";
import { productHref } from "@skelet/storefront-core/entity-href";
import { formatPrice } from "@skelet/storefront-core/format";

type CompareProduct = {
  id: string;
  url: string;
  sku: string;
  name: string;
  inStock: boolean;
  price: number;
  defaultPrice: number;
  compareAtPrice?: number | null;
  mainImage?: { url: string; altText?: string | null } | null;
  reviewAggregates?: { count: number; overallAverage?: number | null } | null;
  attributeValues: {
    attribute: { slug: string; name: string; unit: string; isPublic: boolean };
    selectedValues: { name: string }[];
    scalarValue?: unknown;
  }[];
};

export default function ComparePage() {
  const ids = useCompareIds();
  const t = useTranslations("compare.page");
  const tt = useTranslations("compare");
  const tc = useTranslations("common");
  const [products, setProducts] = useState<CompareProduct[] | null>(null);
  const [currency, setCurrency] = useState("USD");
  const [diffOnly, setDiffOnly] = useState(false);
  const key = ids.join(",");

  useEffect(() => {
    if (!key) return;
    let cancelled = false;
    fetch(`/api/compare-products?ids=${key}`)
      .then((r) => r.json())
      .then((res: { products?: CompareProduct[]; currency?: string }) => {
        if (cancelled) return;
        setProducts(res.products ?? []);
        if (res.currency) setCurrency(res.currency);
      })
      .catch(() => {
        if (!cancelled) setProducts([]);
      });
    return () => {
      cancelled = true;
    };
  }, [key]);

  if (ids.length > 0 && products === null) {
    return (
      <main className="mx-auto w-full max-w-8xl px-6 pt-6 pb-14 md:px-10">
        <h1 className="text-3xl font-extrabold tracking-tight">{tt("title")}</h1>
        <div className="glass mt-6 h-72 animate-pulse rounded-xl" />
      </main>
    );
  }

  if (ids.length === 0 || !products || products.length === 0) {
    return (
      <main className="mx-auto w-full max-w-8xl px-6 pt-6 pb-14 md:px-10">
        <h1 className="text-3xl font-extrabold tracking-tight">{tt("title")}</h1>
        <div className="glass mt-6 rounded-xl p-10 text-center">
          <p className="text-base font-semibold">{t("nothing")}</p>
          <p className="mx-auto mt-2 max-w-md text-sm leading-relaxed text-muted-foreground">
            {t("nothingBody", { max: COMPARE_MAX })}
          </p>
          <Link
            href="/products"
            className="mt-5 inline-flex h-10 items-center rounded-md bg-primary px-6 text-sm font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
          >
            {tc("browseProducts")}
          </Link>
        </div>
      </main>
    );
  }

  // Order follows the compare list, not whatever order the API returned
  const ordered = ids
    .map((id) => products.find((p) => p.id === id))
    .filter((p): p is CompareProduct => !!p);

  const attrText = (p: CompareProduct, slug: string): string => {
    const av = p.attributeValues.find((a) => a.attribute.slug === slug);
    if (!av) return "-";
    if (av.selectedValues.length > 0) {
      return av.selectedValues.map((v) => v.name).join(", ");
    }
    if (av.scalarValue != null && av.scalarValue !== "") {
      const unit = av.attribute.unit ? ` ${av.attribute.unit}` : "";
      return `${String(av.scalarValue)}${unit}`;
    }
    return "-";
  };

  const attrs: { slug: string; name: string }[] = [];
  const seen = new Set<string>();
  for (const p of ordered) {
    for (const av of p.attributeValues) {
      if (!av.attribute.isPublic || seen.has(av.attribute.slug)) continue;
      seen.add(av.attribute.slug);
      if (ordered.every((other) => attrText(other, av.attribute.slug) === "-")) continue;
      attrs.push({ slug: av.attribute.slug, name: av.attribute.name });
    }
  }

  type Row = {
    label: string;
    text: (p: CompareProduct) => string;
    cell?: (p: CompareProduct) => React.ReactNode;
  };
  const rows: Row[] = [
    {
      label: t("price"),
      text: (p) => formatPrice(p.price ?? p.defaultPrice, currency),
      cell: (p) => {
        const sell = p.price ?? p.defaultPrice;
        const was = p.compareAtPrice && p.compareAtPrice > sell ? p.compareAtPrice : null;
        return (
          <span className="flex flex-wrap items-baseline gap-x-2">
            <span className="text-lg font-bold tracking-tight tabular-nums">
              {formatPrice(sell, currency)}
            </span>
            {was && (
              <span className="text-sm text-muted-foreground line-through">
                {formatPrice(was, currency)}
              </span>
            )}
          </span>
        );
      },
    },
    {
      label: t("rating"),
      text: (p) =>
        p.reviewAggregates?.count && p.reviewAggregates.overallAverage != null
          ? `${p.reviewAggregates.overallAverage.toFixed(1)} (${p.reviewAggregates.count})`
          : t("noReviews"),
      cell: (p) =>
        p.reviewAggregates?.count && p.reviewAggregates.overallAverage != null ? (
          <RatingStars
            value={p.reviewAggregates.overallAverage}
            count={p.reviewAggregates.count}
          />
        ) : (
          <span className="text-muted-foreground">{t("noReviews")}</span>
        ),
    },
    {
      label: t("availability"),
      text: (p) => (p.inStock ? tc("inStock") : tc("outOfStock")),
      cell: (p) => (
        <span className="inline-flex items-center gap-1.5 text-sm font-medium">
          <span
            aria-hidden="true"
            className={`size-1.5 rounded-full ${p.inStock ? "bg-emerald-500" : "bg-border"}`}
          />
          <span className={p.inStock ? "text-emerald-700" : "text-muted-foreground"}>
            {p.inStock ? tc("inStock") : tc("outOfStock")}
          </span>
        </span>
      ),
    },
    { label: t("sku"), text: (p) => p.sku, cell: (p) => <span className="font-mono text-xs">{p.sku}</span> },
    ...attrs.map((a) => ({ label: a.name, text: (p: CompareProduct) => attrText(p, a.slug) })),
  ];

  const isSame = (row: Row) =>
    ordered.length > 1 && ordered.every((p) => row.text(p) === row.text(ordered[0]));
  const visibleRows = diffOnly ? rows.filter((r) => !isSame(r)) : rows;
  const sameCount = rows.filter(isSame).length;

  const prices = ordered.map((p) => p.price ?? p.defaultPrice);
  const cheapest = Math.min(...prices);
  const cheapestId =
    ordered.length > 1 && prices.filter((v) => v === cheapest).length === 1
      ? ordered[prices.indexOf(cheapest)].id
      : null;

  const showAddSlot = ordered.length < COMPARE_MAX;
  const cellPad = "px-4 py-3.5 align-top";

  return (
    <main className="mx-auto w-full max-w-8xl px-6 pt-6 pb-14 md:px-10">
      <BackButton className="mb-4" />
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-3xl font-extrabold tracking-tight">{tt("title")}</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {t("sideBySide", { count: ordered.length, max: COMPARE_MAX })}
          </p>
        </div>
        <div className="flex items-center gap-4">
          {sameCount > 0 && (
            <label className="flex cursor-pointer items-center gap-2 text-sm text-muted-foreground">
              <input
                type="checkbox"
                checked={diffOnly}
                onChange={(e) => setDiffOnly(e.target.checked)}
                className="size-4 rounded border-input accent-primary"
              />
              {t("diffOnly")}
            </label>
          )}
          <button
            type="button"
            onClick={clearCompare}
            className="text-sm font-medium text-muted-foreground transition-colors hover:text-destructive"
          >
            {tc("clearAll")}
          </button>
        </div>
      </div>

      <div className="glass mt-6 overflow-x-auto rounded-xl">
        {/* table-fixed - with auto layout one long spec cell resizes the column and the images */}
        <table className="w-full min-w-[720px] table-fixed border-collapse text-sm">
          <thead>
            <tr>
              <th className="sticky left-0 z-10 w-44 bg-card p-4 text-left align-bottom" />
              {ordered.map((p) => (
                <th key={p.id} className="min-w-[13rem] p-4 text-left align-bottom font-normal">
                  <Link href={productHref(p)} className="group block max-w-56">
                    <span className="block aspect-square w-full overflow-hidden rounded-lg bg-muted">
                      {p.mainImage?.url ? (
                        // eslint-disable-next-line @next/next/no-img-element
                        <img
                          src={p.mainImage.url}
                          alt={p.mainImage.altText || p.name}
                          className="size-full object-cover"
                        />
                      ) : (
                        <span className="flex size-full items-center justify-center text-xs text-muted-foreground">
                          {tc("noImage")}
                        </span>
                      )}
                    </span>
                    <span className="mt-3 block text-sm font-semibold leading-snug group-hover:underline">
                      {p.name}
                    </span>
                  </Link>
                  {cheapestId === p.id && (
                    <span className="mt-2 inline-flex items-center rounded-sm bg-emerald-500/10 px-2.5 py-0.5 text-[0.65rem] font-semibold uppercase tracking-wider text-emerald-700">
                      {t("lowest")}
                    </span>
                  )}
                  <div className="mt-3 flex items-center gap-3">
                    <Link
                      href={productHref(p)}
                      className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-xs font-semibold text-primary-foreground transition-colors hover:bg-primary/90"
                    >
                      {t("view")}
                    </Link>
                    <button
                      type="button"
                      onClick={() => removeFromCompare(p.id)}
                      className="text-xs font-medium text-muted-foreground transition-colors hover:text-destructive"
                    >
                      {tc("remove")}
                    </button>
                  </div>
                </th>
              ))}
              {showAddSlot && (
                <th className="w-56 p-4 align-bottom font-normal">
                  <Link
                    href="/products"
                    className="flex aspect-square w-full max-w-56 flex-col items-center justify-center gap-2 rounded-lg border border-dashed border-border text-muted-foreground transition-colors hover:border-foreground/40 hover:text-foreground"
                  >
                    <PlusIcon className="size-6" aria-hidden="true" />
                    <span className="text-xs font-medium">{t("addAnother")}</span>
                  </Link>
                </th>
              )}
            </tr>
          </thead>
          <tbody>
            {visibleRows.map((row, i) => (
              <tr
                key={row.label}
                className={`border-t border-border ${i % 2 === 1 ? "bg-muted/40" : "bg-card"}`}
              >
                <th
                  scope="row"
                  className={`sticky left-0 z-10 bg-inherit text-left text-xs font-medium uppercase tracking-wider text-muted-foreground ${cellPad}`}
                >
                  {row.label}
                </th>
                {ordered.map((p) => (
                  <td key={p.id} className={cellPad}>
                    {row.cell ? row.cell(p) : row.text(p)}
                  </td>
                ))}
                {showAddSlot && <td aria-hidden="true" />}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {diffOnly && visibleRows.length === 0 && (
        <p className="mt-4 text-sm text-muted-foreground">
          {t("allMatch")}
        </p>
      )}
    </main>
  );
}
