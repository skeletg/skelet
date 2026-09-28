"use client";

import { useMemo, useState, useActionState } from "react";
import { useTranslations } from "next-intl";
import { Link } from "@/app/components/LocaleLink";

import { addGroupedToCart } from "@/app/lib/cart/actions";
import { displayName } from "@/app/lib/display";
import { productHref } from "@/app/lib/entity-href";
import { formatPrice } from "@/app/lib/format";
import type { GroupedMember, ProductDetail } from "@/app/lib/catalog/read";
import { CompareButton } from "@/app/components/CompareButton";
import { RatingStars } from "@/app/components/RatingStars";
import { WishlistButton } from "@/app/components/WishlistButton";
import { QtySelector } from "@/app/components/QtySelector";
import { deriveQtyBounds, type QtyBounds } from "@/app/components/qty";
import { RequiredFlag } from "./RequiredFlag";
import { MissingOptionLinks } from "./MissingOptionLinks";
import { jumpToOption } from "./focus-intent";

const GROUPED_ITEMS_ANCHOR = "grouped-items";

const memberAnchorId = (memberId: string) => `grouped-member-${memberId}`;
import { AddToCartButton } from "@/app/components/product/AddToCartButton";
import { StickyBuyRow } from "@/app/components/product/StickyBuyRow";
import { AddToCartFeedback } from "@/app/components/product/AddToCartFeedback";
import { OptionControl, useConfiguredOptions } from "./ProductOptions";
import {
  AttributeList,
  Description,
  ProductGallery,
  ProductReassurance,
  optionAnchorId,
  SelectionHint,
  StockBadge,
} from "./parts";

const FORM_ID = "grouped-add-form";

export function GroupedPDP({ product }: { product: ProductDetail }) {
  const [addState, formAction] = useActionState(addGroupedToCart, null);
  const t = useTranslations("product.pdp");
  const tp = useTranslations("product");
  const members = useMemo(
    () =>
      [...product.groupedMembers].sort((a, b) => a.sortOrder - b.sortOrder),
    [product.groupedMembers],
  );

  const bounds = useMemo(() => {
    const out: Record<string, QtyBounds> = {};
    for (const m of members) out[m.member.id] = deriveQtyBounds(m.member);
    return out;
  }, [members]);

  const [qty, setQty] = useState<Record<string, number>>(() => {
    const out: Record<string, number> = {};
    for (const m of members) {
      out[m.member.id] = m.member.inStock
        ? snapQty(m.defaultQuantity, deriveQtyBounds(m.member))
        : 0;
    }
    return out;
  });

  const setMemberQty = (memberId: string, value: number) => {
    const b = bounds[memberId];
    setQty((prev) => {
      const current = prev[memberId] ?? 0;
      let next = Math.max(0, value);
      const floor = b ? floorQty(b) : 1;
      // 0 means "not in the selection" and the gap below the member's minimum is
      // unsellable - so stepping down drops the row, stepping up jumps to it
      if (next > 0 && next < floor) next = value < current ? 0 : floor;
      if (b?.max != null) next = Math.min(next, b.max);
      return { ...prev, [memberId]: next };
    });
  };

  const totalQty = Object.values(qty).reduce((n, v) => n + v, 0);
  const totalMembers = Object.values(qty).filter((v) => v > 0).length;
  const [attemptSeq, setAttemptSeq] = useState(0);
  const attempted = attemptSeq > 0;

  // A grouped wrapper carries no price of its own (defaultPrice is 0) - the members are
  // what's for sale, so the headline shows their range instead of a bogus $0.00
  const memberPrices = members
    .map((m) => m.member.price ?? m.member.defaultPrice)
    .filter((n) => n > 0);
  const minPrice = memberPrices.length > 0 ? Math.min(...memberPrices) : 0;
  const maxPrice = memberPrices.length > 0 ? Math.max(...memberPrices) : 0;

  const heading = displayName(product.name, product.sku);

  // Soft-block the add when a picked member still needs a required option
  const guardSubmit = (e: React.FormEvent<HTMLFormElement>) => {
    let blockedMember: string | null = null;
    try {
      for (const raw of new FormData(e.currentTarget).getAll("line")) {
        const line = JSON.parse(String(raw)) as {
          memberId: string;
          quantity: number;
          missingRequired: string[];
          uploading: boolean;
          forcedConflict: boolean;
        };
        if (
          line.quantity > 0 &&
          (line.missingRequired.length > 0 || line.uploading || line.forcedConflict)
        ) {
          blockedMember = line.memberId;
          break;
        }
      }
    } catch {
      return;
    }
    if (!blockedMember) return;
    e.preventDefault();
    // Re-arm the highlights and jump to the offending row - it's easily below the fold,
    // and a button that just does nothing reads as broken
    setAttemptSeq((n) => n + 1);
    document
      .getElementById(memberAnchorId(blockedMember))
      ?.scrollIntoView({ behavior: "smooth", block: "center" });
  };

  return (
    <div className="mt-6 grid grid-cols-1 gap-10 md:grid-cols-2 md:items-start">
      {/* Source order gives phones the name first; md+ placement restores the columns */}
      <div className="md:col-start-2 md:row-start-1">
        {product.primaryCategory && (
          <p className="font-mono text-xs font-medium uppercase tracking-[0.18em] text-signal">
            {displayName(
              product.primaryCategory.name,
              product.primaryCategory.slug,
            )}
          </p>
        )}
        <h1 className="mt-1 text-3xl font-extrabold tracking-tight">
          {heading}
        </h1>
      </div>
      <div className="md:col-start-1 md:row-start-1 md:row-span-2 md:sticky md:top-28 md:self-start">
        <ProductGallery
          media={product.media}
          fallback={product.mainImage}
          alt={heading}
          morphFromProductId={product.id}
        />
      </div>

      <div className="md:col-start-2 md:row-start-2">
        <div className="mt-2 flex flex-wrap items-center gap-3">
          {product.reviewAggregates &&
            product.reviewAggregates.count > 0 &&
            product.reviewAggregates.overallAverage != null && (
              <a
                href="#reviews"
                className="inline-flex items-center gap-1.5 hover:underline"
              >
                <RatingStars value={product.reviewAggregates.overallAverage} />
                <span className="text-sm text-muted-foreground">
                  {product.reviewAggregates.overallAverage.toFixed(1)} (
                  {product.reviewAggregates.count})
                </span>
              </a>
            )}
          <p className="text-sm text-muted-foreground">SKU {product.sku}</p>
          <WishlistButton productId={product.id} size="sm" />
          <CompareButton productId={product.id} />
        </div>

        <div className="glass mt-5 rounded-xl p-6">
        {memberPrices.length > 0 ? (
          <>
            <p className="text-2xl font-bold tracking-tight">
              {formatPrice(minPrice, product.defaultCurrencyCode)}
              {maxPrice !== minPrice && (
                <>
                  {" - "}
                  {formatPrice(maxPrice, product.defaultCurrencyCode)}
                </>
              )}
            </p>
            <p className="mt-1 text-sm text-muted-foreground">
              {t("pricedSeparately")}
            </p>
          </>
        ) : (
          <p className="text-2xl font-bold tracking-tight">
            {formatPrice(product.defaultPrice, product.defaultCurrencyCode)}
          </p>
        )}

        <StockBadge
          inStock={product.inStock}
          salableQuantity={product.salableQuantity}
        />

        <section className="mt-8">
          <h2 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
            {t("included")}
          </h2>
          <RequiredFlag
            active={attempted && totalQty === 0}
            attemptSeq={attemptSeq}
            className="mt-3"
            anchorId={optionAnchorId(GROUPED_ITEMS_ANCHOR)}
          >
            {(invalid) => (
              <ul className="space-y-2">
                {members.map((m) => (
                  <GroupedRow
                    key={m.id}
                    member={m}
                    qty={qty[m.member.id] ?? 0}
                    onChange={(v) => setMemberQty(m.member.id, v)}
                    bounds={bounds[m.member.id]}
                    attemptSeq={attemptSeq}
                    invalid={invalid}
                  />
                ))}
              </ul>
            )}
          </RequiredFlag>
        </section>

        <section className="mt-6 rounded-xl border border-border/60 bg-muted/30 p-4 text-sm">
          {totalQty === 0 ? (
            <p className="text-muted-foreground">{t("noneSelected")}</p>
          ) : (
            <p className="text-foreground">
              {t.rich("selectedSummary", {
                items: totalQty,
                products: totalMembers,
                b: (chunks) => <span className="font-medium">{chunks}</span>,
              })}
            </p>
          )}
        </section>

        {/* No single addGroupedToCart mutation - each member is its own cart line, submitted as
            its own form-bound "line" JSON. Qty is per row since there's no one quantity */}
        <StickyBuyRow
          className="mt-6"
          barChildren={
            <AddToCartButton
              form={FORM_ID}
              blocked={totalQty === 0}
              onBlockedClick={() => {
                setAttemptSeq((n) => n + 1);
                jumpToOption(GROUPED_ITEMS_ANCHOR);
              }}
              className="w-full"
            >
              {totalQty === 0 ? t("pickAtLeastOne") : t("addSelected")}
            </AddToCartButton>
          }
        >
        <form
          id={FORM_ID}
          action={formAction}
          onSubmit={guardSubmit}
          className="flex items-center gap-3"
        >
          <AddToCartButton
            blocked={totalQty === 0}
            onBlockedClick={() => {
              setAttemptSeq((n) => n + 1);
              jumpToOption(GROUPED_ITEMS_ANCHOR);
            }}
            className="flex-1"
          >
            {totalQty === 0 ? t("pickAtLeastOne") : t("addSelected")}
          </AddToCartButton>
        </form>
        </StickyBuyRow>
        <AddToCartFeedback state={addState} />
        {attempted && totalQty === 0 ? (
          <SelectionHint key={attemptSeq}>
            {t.rich("chooseAtLeastOne", {
              links: () => (
                <MissingOptionLinks
                  items={[{ id: GROUPED_ITEMS_ANCHOR, label: t("whatsIncluded") }]}
                />
              ),
            })}
          </SelectionHint>
        ) : null}

        <ProductReassurance className="mt-5 border-t border-border pt-4" />
        </div>

        <Description
          label={tp("summary")}
          value={product.shortDescription}
          className="mt-6"
        />

        <Description
          label={tp("description")}
          value={product.description}
          className="mt-8"
        />

        <AttributeList values={product.attributeValues} className="mt-8" />
      </div>
    </div>
  );
}

function roundTo(n: number, decimals: number): number {
  if (decimals <= 0) return Math.round(n);
  const f = Math.pow(10, decimals);
  return Math.round(n * f) / f;
}

function floorQty(b: QtyBounds): number {
  const step = b.step > 0 ? b.step : 1;
  return roundTo(Math.ceil((b.min ?? step) / step) * step, b.decimals);
}

function snapQty(n: number, b: QtyBounds): number {
  if (n <= 0) return 0;
  const step = b.step > 0 ? b.step : 1;
  const snapped = roundTo(Math.round(n / step) * step, b.decimals);
  return Math.min(Math.max(snapped, floorQty(b)), b.max ?? Infinity);
}

function GroupedRow({
  member,
  qty,
  onChange,
  bounds,
  attemptSeq,
  invalid = false,
}: {
  member: GroupedMember;
  qty: number;
  onChange: (next: number) => void;
  bounds: QtyBounds;
  attemptSeq: number;
  invalid?: boolean;
}) {
  const p = member.member;
  const hasOptions = p.options.length > 0;
  const t = useTranslations("product.pdp");
  const tc = useTranslations("common");
  const opts = useConfiguredOptions(p.options, {
    defaultPrice: p.defaultPrice,
    effectivePrice: p.price ?? p.defaultPrice,
    productSku: p.sku,
  });
  const qtyLocked = opts.forcedQty != null;
  const effectiveQty = qtyLocked && qty > 0 ? (opts.forcedQty as number) : qty;
  const showOptions = hasOptions && qty > 0;
  const attempted = attemptSeq > 0;

  return (
    <li
      id={memberAnchorId(p.id)}
      className={`rounded-xl border p-3 transition ${
        qty > 0
          ? "border-foreground/40 bg-muted/40"
          : "border-border/60 hover:border-foreground/30"
      }`}
    >
      {/* This row's slice of the submit - qty, picks, and the flags the action pre-checks */}
      <input
        type="hidden"
        name="line"
        form={FORM_ID}
        value={JSON.stringify({
          memberId: p.id,
          productSku: p.sku,
          productName: displayName(p.name, p.sku),
          unitPrice: p.defaultPrice,
          quantity: effectiveQty,
          hasOptions,
          options: opts.payload,
          missingRequired: opts.missingRequired,
          uploading: opts.uploading,
          forcedConflict: opts.forcedQtyConflict,
        })}
      />
      <div className="flex items-center gap-3">
        <div className="size-14 shrink-0 overflow-hidden rounded-lg border border-border/60 bg-muted">
          {p.mainImage ? (
            // eslint-disable-next-line @next/next/no-img-element
            <img
              src={p.mainImage.url}
              alt={p.mainImage.altText || p.sku}
              className="h-full w-full object-cover"
            />
          ) : null}
        </div>
        <div className="flex-1 min-w-0">
          <Link
            href={productHref(p)}
            className="text-sm font-medium hover:underline"
          >
            {displayName(p.name, p.sku)}
          </Link>
          <p className="text-xs text-muted-foreground">
            {formatPrice(opts.configuredTotal, p.defaultCurrencyCode)}
            {opts.optionsDelta !== 0 ? t("configured") : ""} ·{" "}
            {p.inStock ? tc("inStock") : tc("outOfStock")}
          </p>
        </div>
        {/* min=0 so a row can be dialed out - the real floor lives in
            setMemberQty, which knows a step down from a step up */}
        <QtySelector
          value={effectiveQty}
          onValueChange={onChange}
          min={0}
          max={bounds?.max}
          step={bounds?.step}
          decimals={bounds?.decimals}
          disabled={qtyLocked || !p.inStock}
          invalid={invalid}
          size="sm"
        />
      </div>

      {/* Member options unfold once the row is selected (qty > 0) */}
      {showOptions ? (
        <div className="mt-3 space-y-4 border-t border-border pt-3">
          {/* Same per-option flag the other PDPs use - marks the choice cards
              and inputs themselves, not the row */}
          {opts.visibleOptions.map((opt) => (
            <RequiredFlag
              key={opt.id}
              active={attempted && opts.missingRequiredIds.includes(opt.id)}
              attemptSeq={attemptSeq}
              anchorId={optionAnchorId(opt.id)}
            >
              {(optionInvalid) => (
                <OptionControl
                  option={opt}
                  currency={p.defaultCurrencyCode}
                  picked={opts.sel[opt.id] ?? new Set()}
                  textValue={opts.text[opt.id] ?? ""}
                  file={opts.files[opt.id]}
                  onToggle={(vid) => {
                    // Snap the row qty when a forced-qty value gets selected - the
                    // server rejects mismatches (PRODUCT_OPTION_QTY_FORCED)
                    const wasPicked = opts.sel[opt.id]?.has(vid) ?? false;
                    opts.toggleValue(opt, vid);
                    const v = opt.values.find((x) => x.id === vid);
                    if (!wasPicked && v?.forcedLineQty != null) {
                      onChange(v.forcedLineQty);
                    }
                  }}
                  onText={(v) => opts.setOptionText(opt.id, v)}
                  onFile={(e) => opts.onFileChange(opt, e)}
                  invalid={optionInvalid}
                />
              )}
            </RequiredFlag>
          ))}
          {qtyLocked ? (
            <p className="text-xs text-muted-foreground">
              {t("qtyLocked", { qty: opts.forcedQty ?? 0 })}
            </p>
          ) : null}
          {opts.forcedQtyConflict ? (
            <p role="alert" className="text-xs font-medium text-red-600 dark:text-red-400">
              {t("qtyConflict")}
            </p>
          ) : null}
          {/* Quiet note until an add is blocked on it, then the same
              jump-to-the-option hint the other PDPs show. Both read live off
              option state so they clear the instant the shopper picks */}
          {opts.missingRequired.length === 0 ? null : attempted ? (
            <SelectionHint key={attemptSeq}>
              {t.rich("pickForItem", {
                links: () => <MissingOptionLinks items={opts.missingRequiredItems} />,
              })}
            </SelectionHint>
          ) : (
            <p className="text-xs text-muted-foreground">
              {t.rich("requiredList", {
                count: opts.missingRequired.length,
                names: () => (
                  <span className="text-foreground">
                    {opts.missingRequired.join(", ")}
                  </span>
                ),
              })}
            </p>
          )}
        </div>
      ) : null}
    </li>
  );
}
