import { Link } from "@/app/components/LocaleLink";
import { ArrowRightIcon, SparklesIcon, TagIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import type { ProductSummary } from "@skelet/storefront-next/catalog/read";
import type { HeaderCategory, HeaderCollection } from "@skelet/storefront-next/header/read";
import { displayName } from "@skelet/storefront-next/display";
import { categoryHref, collectionHref } from "@skelet/storefront-next/entity-href";

export function PromoMosaic({
  deals,
  arrivals,
  category,
  collection,
}: {
  deals: ProductSummary[];
  arrivals: ProductSummary[];
  category: HeaderCategory | null;
  collection: HeaderCollection | null;
}) {
  const t = useTranslations("home.promo");
  const tc = useTranslations("common");
  const lead = deals[0] ?? arrivals[0] ?? null;
  const maxPct = deals.reduce((max, d) => {
    const sell = d.price ?? d.defaultPrice;
    const pct = d.compareAtPrice
      ? Math.round((1 - sell / d.compareAtPrice) * 100)
      : 0;
    return Math.max(max, pct);
  }, 0);
  const arrivalImage =
    arrivals.find((p) => p.id !== lead?.id && p.mainImage)?.mainImage ?? null;

  return (
    <section
      aria-label={t("label")}
      className="mx-auto w-full max-w-8xl px-6 md:px-10 pt-6"
    >
      <div className="grid auto-rows-[9.5rem] grid-cols-2 gap-3 lg:auto-rows-[10.5rem] lg:grid-cols-4">
        <div className="col-span-2 row-span-2">
          <Link
            href="/products"
            className="relative flex h-full flex-col justify-end overflow-hidden rounded-lg bg-midnight-deep p-7 text-white"
          >
            {lead?.mainImage && (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={lead.mainImage.url}
                alt=""
                aria-hidden="true"
                className="absolute inset-0 h-full w-full object-cover opacity-60"
              />
            )}
            <div
              aria-hidden="true"
              className="absolute inset-0 bg-gradient-to-t from-midnight-deep via-midnight-deep/40 to-transparent"
            />
            <div className="relative">
              <p className="font-mono text-xs font-medium uppercase tracking-[0.2em] text-signal">
                {deals.length > 0 ? t("dealDays") : t("featured")}
              </p>
              <p className="mt-2 text-4xl font-extrabold leading-none tracking-tight">
                {deals.length > 0 ? t("upTo", { percent: maxPct }) : t("freshPicks")}
              </p>
              <p className="mt-2 text-sm text-white/70">
                {deals.length > 0 ? t("dealsBody") : t("freshBody")}
              </p>
              <span className="mt-4 inline-flex h-10 items-center rounded-md bg-white px-5 text-sm font-semibold text-midnight-deep">
                {tc("shopNow")}
              </span>
            </div>
          </Link>
        </div>

        {category && (
          <div className="h-full">
            <Link
              href={categoryHref(category)}
              className="glass glass-hover flex h-full flex-col justify-between rounded-lg p-5"
            >
              <TagIcon className="h-6 w-6 text-signal" aria-hidden="true" />
              <span>
                <span className="block text-base font-bold leading-tight">
                  {displayName(category.name, category.slug)}
                </span>
                <span className="mt-1 flex items-center gap-1 font-mono text-xs font-medium uppercase tracking-[0.08em] text-signal">
                  {tc("shopNow")}
                  <ArrowRightIcon className="h-3.5 w-3.5" aria-hidden="true" />
                </span>
              </span>
            </Link>
          </div>
        )}

        <div className="h-full">
          <Link
            href="/products"
            className="relative flex h-full flex-col justify-end overflow-hidden rounded-lg bg-muted p-5"
          >
            {arrivalImage && (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={arrivalImage.url}
                alt=""
                aria-hidden="true"
                className="absolute inset-0 h-full w-full object-cover"
              />
            )}
            <div
              aria-hidden="true"
              className="absolute inset-0 bg-gradient-to-t from-black/70 via-black/25 to-transparent"
            />
            <span className="relative block text-base font-bold leading-tight text-white">
              {t("newArrivals")}
            </span>
            <span className="relative mt-1 flex items-center gap-1 font-mono text-xs font-medium uppercase tracking-[0.08em] text-white/85">
              {t("justLanded")}
              <ArrowRightIcon className="h-3.5 w-3.5" aria-hidden="true" />
            </span>
          </Link>
        </div>

        {collection && (
          <div className="col-span-2 h-full">
            <Link
              href={collectionHref(collection)}
              className="glass-hover flex h-full items-center justify-between gap-4 rounded-lg border border-border bg-muted p-6"
            >
              <span className="flex min-w-0 items-center gap-4">
                <SparklesIcon
                  className="h-6 w-6 shrink-0 text-signal"
                  aria-hidden="true"
                />
                <span className="min-w-0">
                  <span className="block truncate text-lg font-bold">
                    {displayName(collection.name, collection.slug)}
                  </span>
                  <span className="block text-sm text-muted-foreground">
                    {t("curated")}
                  </span>
                </span>
              </span>
              <ArrowRightIcon
                className="h-5 w-5 shrink-0 text-muted-foreground"
                aria-hidden="true"
              />
            </Link>
          </div>
        )}
      </div>
    </section>
  );
}
