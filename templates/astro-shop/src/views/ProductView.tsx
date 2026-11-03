import type { ProductPageQuery } from "../graphql/product.gql";
import type { Currency } from "../lib/format";
import { paragraphs } from "../lib/format";
import { Choice } from "../components/Choice";
import { Price } from "../components/Price";
import { ProductImage } from "../components/ProductImage";
import { QuantityInput } from "../components/QuantityInput";
import { Button } from "../components/ui";

type Product = NonNullable<ProductPageQuery["product"]>;

export function ProductView({ product, currency }: { product: Product; currency: Currency }) {
  // A configurable product is bought as one of its variants, a simple one as itself
  // shortcut: bundles and products with custom options aren't sold here, add their forms if the catalog needs them
  const variants = product.kind === "CONFIGURABLE" ? product.children : [];
  const firstInStock = variants.find((v) => v.inStock);
  const sellable = product.kind === "SIMPLE" || variants.length > 0;
  const canBuy = product.inStock && (variants.length === 0 || !!firstInStock);
  const body = paragraphs(product.description);
  const name = product.name ?? "";

  return (
    <>
      <nav aria-label="Breadcrumb" className="mb-6 text-sm text-muted-foreground">
        <ol className="flex items-center gap-2">
          <li>
            <a href="/" className="hover:text-foreground">
              Shop
            </a>
          </li>
          <li aria-hidden="true">/</li>
          <li aria-current="page" className="truncate text-foreground">
            {name}
          </li>
        </ol>
      </nav>
      <article className="grid gap-10 md:grid-cols-[1.1fr_1fr] md:gap-14 lg:gap-20">
        <ProductImage name={name} image={product.mainImage} priority transitionName={`product-${product.id}`} sizes="(min-width: 768px) 55vw, 100vw" />
        <div className="md:sticky md:top-24 md:self-start">
          <h1 className="text-3xl font-semibold tracking-tight text-balance sm:text-4xl">{name}</h1>
          <p className="mt-3 text-2xl font-semibold">
            <Price amount={product.price} compareAt={product.compareAtPrice} onSale={product.onSale} currency={currency} />
          </p>
          <p className={`mt-2 inline-flex items-center gap-2 text-sm ${product.inStock ? "text-success" : "text-muted-foreground"}`}>
            <span className={`size-2 rounded-full ${product.inStock ? "bg-success" : "bg-muted-foreground"}`} />
            {product.inStock ? "In stock" : "Sold out"}
          </p>

          {sellable ? (
            // With scripts the add posts in the background and the shopper stays here; without, it lands on the cart
            <form method="post" action="/cart" className="mt-8 grid max-w-md gap-6" data-enhance="" data-done="Added to your cart.">
              <input type="hidden" name="intent" value="add" />
              {variants.length > 0 ? (
                <fieldset className="grid gap-2">
                  <legend className="mb-2 text-sm font-medium">Choose an option</legend>
                  {variants.map((v) => (
                    <Choice key={v.id} name="sku" value={v.sku} disabled={!v.inStock} checked={v.sku === firstInStock?.sku}>
                      <span className="flex-1">{(v.name ?? v.sku).replace(`${name} / `, "")}</span>
                      <span className="text-sm text-muted-foreground">{v.inStock ? <Price amount={v.price} currency={currency} /> : "Sold out"}</span>
                    </Choice>
                  ))}
                </fieldset>
              ) : (
                <input type="hidden" name="sku" value={product.sku} />
              )}
              <div className="flex items-end gap-3">
                <div className="grid gap-1 text-sm font-medium">
                  <span aria-hidden="true">Quantity</span>
                  <QuantityInput
                    id="quantity"
                    label="Quantity"
                    value={product.minOrderQty ?? 1}
                    min={product.minOrderQty ?? 1}
                    max={product.maxOrderQty}
                    step={product.quantityStep ?? 1}
                  />
                </div>
                <Button type="submit" disabled={!canBuy} className="flex-1">
                  {canBuy ? "Add to cart" : "Sold out"}
                </Button>
              </div>
              <p data-status="" role="status" className="-mt-3 min-h-5 text-sm text-muted-foreground empty:hidden"></p>
            </form>
          ) : (
            <p className="mt-8 text-muted-foreground">This product can't be ordered from this shop yet.</p>
          )}

          <ul className="mt-8 grid gap-2 border-t border-border pt-6 text-sm text-muted-foreground">
            <li>Free returns within 30 days</li>
            <li>Secure checkout, no account needed</li>
          </ul>

          {body.length > 0 && (
            <div className="mt-8 grid gap-4 border-t border-border pt-6 leading-relaxed text-muted-foreground">
              {body.map((p, i) => (
                <p key={i}>{p}</p>
              ))}
            </div>
          )}
        </div>
      </article>
    </>
  );
}
