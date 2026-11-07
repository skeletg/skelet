import { CheckIcon, ChevronDownIcon, InfoIcon } from "lucide-react";
import { useTranslations } from "next-intl";

import type { AssistantDetail } from "@skelet/storefront-core/assistant/types";
import { displayName } from "@skelet/storefront-core/display";
import { productHref } from "@skelet/storefront-core/entity-href";
import { formatPrice } from "@skelet/storefront-core/format";
import { mediaThumb } from "@skelet/storefront-core/media-thumb";
import { AddToCartForm } from "@/app/components/AddToCartForm";
import { Link, usePathname } from "@/app/components/LocaleLink";
import { RatingStars } from "@/app/components/RatingStars";
import { priceView } from "@/app/lib/product-price";

// The product the assistant is explaining: what it is and costs, whether it's in stock, the assistant's take on it laid
// out to scan - summary, highlights, who it suits, what to weigh - and its specs, folded away once there's a take to read
export function ProductSpotlight({ detail }: { detail: AssistantDetail }) {
  const { product, specs, notes } = detail;
  const explained = notes.summary !== "" || notes.highlights.length > 0 || notes.goodFor.length > 0 || notes.keepInMind.length > 0;
  const t = useTranslations("assistant.detail");
  const tc = useTranslations("product.card");
  const common = useTranslations("common");
  const name = displayName(product.name, product.sku);
  const href = productHref(product);
  const { pricedBySelection, sellPrice, compareAt, savePct } = priceView(product);
  const reviews = product.reviewAggregates;
  // On the product's own page a link to it goes nowhere, so the panel keeps only what the page can't do from here
  const here = usePathname() === href;
  const simple = product.kind === "SIMPLE";
  return (
    <section aria-label={name} className="overflow-hidden rounded-xl border border-border bg-background">
      <div className="flex gap-3 p-3">
        <Link href={href} tabIndex={-1} aria-hidden="true" className="size-20 shrink-0 overflow-hidden rounded-lg bg-muted">
          {product.mainImage ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={mediaThumb(product.mainImage.url, 160, product.mainImage.width)}
              alt=""
              width={80}
              height={80}
              loading="lazy"
              decoding="async"
              className="h-full w-full object-cover"
            />
          ) : (
            <span className="flex h-full items-center justify-center text-[10px] text-muted-foreground">{common("noImage")}</span>
          )}
        </Link>
        <div className="min-w-0 flex-1">
          <Link href={href} className="line-clamp-2 font-semibold leading-snug hover:underline">
            {name}
          </Link>
          <p className="mt-1 flex flex-wrap items-baseline gap-x-2 font-mono">
            {pricedBySelection ? (
              <span className="text-sm font-semibold text-muted-foreground">{tc("seeOptions")}</span>
            ) : (
              <span className="text-base font-semibold tabular-nums">{formatPrice(sellPrice, product.defaultCurrencyCode)}</span>
            )}
            {compareAt ? <span className="text-xs text-muted-foreground line-through">{formatPrice(compareAt, product.defaultCurrencyCode)}</span> : null}
            {savePct >= 5 ? (
              <span className="rounded-sm bg-signal-fill px-1.5 py-0.5 text-[0.6rem] font-bold uppercase tracking-wider text-white">{tc("save", { percent: savePct })}</span>
            ) : null}
          </p>
          <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
            {product.inStock ? (
              <span className="inline-flex items-center gap-1">
                <CheckIcon className="size-3.5" aria-hidden="true" />
                {t("inStock")}
              </span>
            ) : (
              <span className="rounded-sm border border-border px-1.5 py-0.5 font-mono text-[0.6rem] uppercase tracking-wider">{tc("soldOut")}</span>
            )}
            {reviews && reviews.count > 0 && reviews.overallAverage != null ? <RatingStars value={reviews.overallAverage} count={reviews.count} /> : null}
          </div>
        </div>
      </div>
      {notes.summary ? <p className="px-3 pb-3 text-sm leading-relaxed">{notes.summary}</p> : null}
      {notes.highlights.length > 0 ? (
        <ul className="space-y-2.5 border-t border-border px-3 py-3">
          {notes.highlights.map((h, i) => (
            <li key={i} className="flex gap-2.5">
              <span className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-signal/10 text-signal">
                <CheckIcon className="size-3" strokeWidth={2.5} aria-hidden="true" />
              </span>
              <span className="min-w-0">
                <span className="block text-sm font-semibold leading-snug">{h.title}</span>
                {h.detail ? <span className="block text-xs leading-snug text-muted-foreground">{h.detail}</span> : null}
              </span>
            </li>
          ))}
        </ul>
      ) : null}
      {notes.goodFor.length > 0 ? (
        <div className="border-t border-border px-3 py-3">
          <p className="mb-1.5 text-xs font-medium text-muted-foreground">{t("goodFor")}</p>
          <ul className="flex flex-wrap gap-1.5">
            {notes.goodFor.map((tag, i) => (
              <li key={i} className="rounded-full border border-border px-2.5 py-0.5 text-xs">
                {tag}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      {notes.keepInMind.length > 0 ? (
        <div className="border-t border-border px-3 py-3">
          <p className="mb-1.5 text-xs font-medium text-muted-foreground">{t("keepInMind")}</p>
          <ul className="space-y-1.5">
            {notes.keepInMind.map((note, i) => (
              <li key={i} className="flex gap-2 text-xs leading-snug">
                <InfoIcon className="mt-px size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
                {note}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      {specs.length > 0 && explained ? (
        <details className="group border-t border-border">
          <summary className="flex cursor-pointer list-none items-center justify-between px-3 py-2.5 text-xs font-medium text-muted-foreground transition hover:text-foreground [&::-webkit-details-marker]:hidden">
            {t("specs", { count: specs.length })}
            <ChevronDownIcon className="size-4 transition-transform group-open:rotate-180" aria-hidden="true" />
          </summary>
          <SpecTable specs={specs} />
        </details>
      ) : specs.length > 0 ? (
        <SpecTable specs={specs} />
      ) : null}
      {here && !simple ? null : (
        <div className={`grid gap-2 border-t border-border p-3 ${here ? "grid-cols-1" : "grid-cols-2"}`}>
          {simple ? (
            <AddToCartForm product={product} size="sm" hideQty />
          ) : (
            <Link
              href={href}
              className="inline-flex h-10 items-center justify-center rounded-md bg-primary px-3 text-xs font-semibold text-primary-foreground transition-colors hover:bg-primary/85"
            >
              {tc("chooseOptions")}
            </Link>
          )}
          {here ? null : (
            <Link href={href} className="inline-flex h-10 items-center justify-center rounded-md border border-border px-3 text-xs font-semibold transition-colors hover:bg-muted">
              {t("view")}
            </Link>
          )}
        </div>
      )}
    </section>
  );
}

function SpecTable({ specs }: { specs: AssistantDetail["specs"] }) {
  const t = useTranslations("assistant.detail");
  return (
    <dl className="grid grid-cols-[minmax(0,2fr)_minmax(0,3fr)] text-xs">
      {specs.map((spec, i) => (
        <div key={i} className="contents">
          <dt className="border-t border-border/60 px-3 py-1.5 text-muted-foreground">{spec.label}</dt>
          <dd className="border-t border-border/60 px-3 py-1.5 font-medium">{typeof spec.value === "boolean" ? t(spec.value ? "yes" : "no") : spec.value}</dd>
        </div>
      ))}
    </dl>
  );
}
