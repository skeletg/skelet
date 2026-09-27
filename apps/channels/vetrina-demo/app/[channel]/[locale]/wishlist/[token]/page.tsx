import type { Metadata } from "next";

import { ProductCard } from "@/app/components/ProductCard";
import type { ProductSummary } from "@/app/lib/catalog/read";
import { gqlFetch } from "@/app/lib/graphql";
import { SharedWishlistDocument } from "@/app/lib/wishlist/operations.gql";

export const metadata: Metadata = {
  title: "Shared wishlist",
  robots: { index: false, follow: false },
};

export default async function SharedWishlistPage(
  props: PageProps<"/[channel]/[locale]/wishlist/[token]">,
) {
  const { token } = await props.params;
  let products: ProductSummary[] = [];
  try {
    const res = await gqlFetch(SharedWishlistDocument, {
      token: String(token),
    });
    products = res.data.sharedWishlist as unknown as ProductSummary[];
  } catch {
    products = [];
  }

  return (
    <main className="mx-auto w-full max-w-8xl px-6 md:px-10 pt-6 pb-14">
      <p className="text-xs uppercase tracking-wider text-muted-foreground">
        Shared wishlist
      </p>
      <h1 className="mt-1 text-3xl font-extrabold tracking-tight">
        Someone thinks you should see these
      </h1>
      {products.length === 0 ? (
        <p className="mt-6 text-sm text-muted-foreground">
          This wishlist is empty or the link is no longer valid.
        </p>
      ) : (
        <div className="mt-8 grid grid-cols-2 gap-6 sm:grid-cols-3 lg:grid-cols-4">
          {products.map((p) => (
            <ProductCard key={p.id} product={p} />
          ))}
        </div>
      )}
    </main>
  );
}
