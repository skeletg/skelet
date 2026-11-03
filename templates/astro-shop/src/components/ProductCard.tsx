import type { Currency } from "../lib/format";
import { Price } from "./Price";
import { ProductImage } from "./ProductImage";

export type CardProduct = {
  id: string;
  url: string;
  name: string | null;
  inStock: boolean;
  price: number;
  compareAtPrice?: number | null;
  onSale?: boolean | null;
  mainImage?: { url: string; altText?: string | null; width?: number | null; height?: number | null } | null;
};

type Props = { product: CardProduct; currency: Currency; priority?: boolean };

export function ProductCard({ product: p, currency, priority = false }: Props) {
  return (
    <a
      href={`/${p.url}`}
      className="group flex h-full flex-col overflow-hidden rounded-[1.125rem] border border-border bg-card transition-shadow duration-300 hover:shadow-soft focus-visible:outline-offset-4"
      data-reveal
    >
      <div className="relative">
        <ProductImage
          name={p.name}
          image={p.mainImage}
          priority={priority}
          transitionName={`product-${p.id}`}
          sizes="(min-width: 1024px) 33vw, 50vw"
          className="rounded-none"
        />
        {!p.inStock ? (
          <span className="absolute left-3 top-3 rounded-full bg-card/90 px-2.5 py-1 text-xs font-medium text-muted-foreground backdrop-blur">
            Sold out
          </span>
        ) : p.onSale ? (
          <span className="absolute left-3 top-3 rounded-full bg-destructive px-2.5 py-1 text-xs font-semibold text-destructive-foreground">Sale</span>
        ) : null}
      </div>
      <div className="flex flex-1 flex-col p-4 sm:p-5">
        <h2 className="mb-4 text-[0.95rem] font-semibold leading-snug sm:text-base">{p.name}</h2>
        <div className="mt-auto flex items-center justify-between gap-3 border-t border-border pt-4">
          <Price amount={p.price} compareAt={p.compareAtPrice} onSale={p.onSale} currency={currency} className="text-base font-semibold sm:text-lg" />
          <span aria-hidden="true" className="hidden rounded-full border border-border px-3 py-1.5 text-xs font-medium transition-colors group-hover:border-foreground group-hover:bg-primary group-hover:text-primary-foreground sm:inline-block">
            View →
          </span>
        </div>
      </div>
    </a>
  );
}
