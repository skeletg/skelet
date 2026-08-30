"use client";

import { useEffect, useState } from "react";

import type { ProductSummary } from "@/app/lib/catalog/read";
import { ProductCard } from "./ProductCard";

// Recently-viewed products, tracked client-side in localStorage (guests and signed-in alike) and hydrated through
// /api/products-by-ids so the rail shows live channel pricing. RecordView runs on every PDP

const KEY = "storefront_recently_viewed";
const MAX = 12;

function readIds(): string[] {
  try {
    const v = JSON.parse(localStorage.getItem(KEY) ?? "[]") as unknown;
    return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : [];
  } catch {
    return [];
  }
}

export function RecordView({ productId }: { productId: string }) {
  useEffect(() => {
    const ids = [productId, ...readIds().filter((i) => i !== productId)].slice(0, MAX);
    try {
      localStorage.setItem(KEY, JSON.stringify(ids));
    } catch {
    }
  }, [productId]);
  return null;
}

export function RecentlyViewedRail({ excludeId }: { excludeId?: string }) {
  const [products, setProducts] = useState<ProductSummary[]>([]);

  useEffect(() => {
    const ids = readIds().filter((i) => i !== excludeId).slice(0, 8);
    if (ids.length === 0) return;
    let cancelled = false;
    fetch(`/api/products-by-ids?ids=${ids.join(",")}`)
      .then((r) => r.json())
      .then((d: { products: ProductSummary[] }) => {
        if (!cancelled) setProducts(d.products ?? []);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [excludeId]);

  if (products.length === 0) return null;
  return (
    <section className="mt-16">
      <h2 className="text-lg font-semibold tracking-tight">Recently viewed</h2>
      <div className="mt-4 grid grid-cols-2 gap-6 sm:grid-cols-3 lg:grid-cols-4">
        {products.map((p) => (
          <ProductCard key={p.id} product={p} variant="compact" />
        ))}
      </div>
    </section>
  );
}
