// SIMPLE PDP shell - server-rendered, since a plain product has nothing to pick before the add

import { getTranslations } from "next-intl/server";

import { formatPrice } from "@/app/lib/cart/read";
import type { ProductDetail } from "@/app/lib/catalog/read";
import { AddToCartForm } from "@/app/components/AddToCartForm";
import { ApproxPrice } from "@/app/components/ApproxPrice";
import { BackInStockForm } from "@/app/components/BackInStockForm";
import { ExperimentGate } from "@/app/components/ExperimentGate";
import { AttributeList, Description, StockBadge } from "./parts";
import {
  PdpBuyBox,
  PdpMetaRow,
  PdpShell,
  PdpTaxonomy,
} from "./pdp-shell";

const PDP_LAYOUT_EXPERIMENT = "pdp_layout";

export async function SimplePDP({
  product,
  heading,
  signalsEnabled,
}: {
  product: ProductDetail;
  heading: string;
  signalsEnabled: boolean;
}) {
  const tp = await getTranslations("product");
  const price = product.price ?? product.defaultPrice;

  return (
    <PdpShell product={product} heading={heading} largeHeading>
      <PdpMetaRow product={product} />

      <PromoDisplays displays={product.promotionalDisplays} />

      <PdpBuyBox>
        <PriceLine product={product} price={price} saleLabel={tp("sale")} />
        {product.subscriptionTerms && product.subscriptionTerms.trialDays > 0 ? (
          <p className="mt-1 text-sm text-muted-foreground">
            {product.subscriptionTerms.trialDays}-day free trial, then recurring billing.
          </p>
        ) : null}

        {product.tierPrices && product.tierPrices.length > 0 ? (
          <div className="mt-3 w-fit rounded-lg border border-border/60 px-4 py-3">
            <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              {tp("volumePricing")}
            </p>
            <ul className="mt-1.5 grid gap-0.5 text-sm">
              {[...product.tierPrices]
                .sort((a, b) => a.minQty - b.minQty)
                .map((t) => (
                  <li key={t.minQty} className="tabular-nums">
                    {tp.rich("tierPrice", {
                      min: t.minQty,
                      price: () => (
                        <span className="font-semibold">
                          {formatPrice(t.price, product.defaultCurrencyCode)}
                        </span>
                      ),
                    })}
                  </li>
                ))}
            </ul>
          </div>
        ) : null}

        <StockBadge
          inStock={product.inStock}
          salableQuantity={product.salableQuantity}
        />

        {!product.inStock && product.backorderable ? (
          <p className="mt-2 text-sm text-amber-700 dark:text-amber-400">
            {tp("backorder")}
          </p>
        ) : null}

        {signalsEnabled && !product.inStock && !product.backorderable ? (
          <BackInStockForm
            productId={product.id}
            sku={product.sku}
            name={heading}
            image={product.mainImage?.url ?? ""}
          />
        ) : null}

        {/* Prism A/B "pdp_layout" - layout tokens are set as each variation's value in the dashboard */}
        <div className="mt-5">
          <AddToCartForm
            product={product}
            size="md"
            mobileSticky
            stickyPrice={formatPrice(price, product.defaultCurrencyCode)}
          />
          <ExperimentGate experiment={PDP_LAYOUT_EXPERIMENT} value="prominent_cta">
            <p className="mt-2 text-center text-xs text-muted-foreground">
              {tp("ctaNote")}
            </p>
          </ExperimentGate>
        </div>
      </PdpBuyBox>

      <Description
        label={tp("summary")}
        value={product.shortDescription}
        className="mt-6"
      />
      <Description
        label={tp("description")}
        value={product.description}
        className="mt-6"
      />

      <AttributeList values={product.attributeValues} className="mt-8" />

      <PdpTaxonomy product={product} />
    </PdpShell>
  );
}

// Price with its compare-at strike, sale badge and subscription interval
function PriceLine({
  product,
  price,
  saleLabel,
}: {
  product: ProductDetail;
  price: number;
  saleLabel: string;
}) {
  const terms = product.subscriptionTerms;
  return (
    <p className="font-mono text-3xl font-bold tracking-tight tabular-nums">
      {formatPrice(price, product.defaultCurrencyCode)}
      <ApproxPrice cents={price} className="text-base font-normal text-muted-foreground" />
      {product.compareAtPrice && product.compareAtPrice > price ? (
        <span className="ml-2 align-middle text-lg font-normal text-muted-foreground line-through">
          {formatPrice(product.compareAtPrice, product.defaultCurrencyCode)}
        </span>
      ) : null}
      {product.onSale ? (
        <span className="ml-2 inline-block -translate-y-1 rounded-sm bg-signal px-2.5 py-0.5 align-middle font-mono text-xs font-semibold uppercase tracking-wider text-white">
          {saleLabel}
        </span>
      ) : null}
      {terms ? (
        <span className="ml-1 align-middle text-base font-normal text-muted-foreground">
          {" / "}
          {terms.intervalCount === 1
            ? terms.intervalUnit.toLowerCase()
            : `${terms.intervalCount} ${terms.intervalUnit.toLowerCase()}s`}
        </span>
      ) : null}
    </p>
  );
}

// Rules whose product-level conditions match, priority asc - cart-level ones show in cart. A rule with a banner
// shows the image; one without shows its pill, so a text-only promo still reads near the price
function PromoDisplays({
  displays,
}: {
  displays: ProductDetail["promotionalDisplays"];
}) {
  if (!displays?.length) return null;
  const pills = displays.filter((d) => d.pillText && !d.bannerMedia);
  return (
    <>
      {displays
        .filter((d) => d.bannerMedia)
        .map((d) => (
          <div key={d.ruleId} className="mt-4 overflow-hidden rounded-lg">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img
              src={d.bannerMedia!.url}
              alt={d.bannerMedia!.altText || d.publicLabel}
              className="w-full object-cover"
            />
          </div>
        ))}
      {pills.length > 0 ? (
        <div className="mt-4 flex flex-wrap gap-2">
          {pills.map((d) => (
            <span
              key={d.ruleId}
              className="inline-flex items-center rounded-sm px-3 py-1 text-xs font-semibold uppercase tracking-wider"
              style={{
                backgroundColor: d.pillBgColor || "#dc2626",
                color: d.pillTextColor || "#ffffff",
              }}
            >
              {d.pillText}
            </span>
          ))}
        </div>
      ) : null}
    </>
  );
}
