import { redirect } from "next/navigation";

import { withLocalePrefix } from "@/app/lib/locale/path";
import { HeartIcon } from "lucide-react";

import {
  AccountPageHeader,
  EmptyState,
} from "@/app/components/account/AccountUI";
import { ProductCard } from "@/app/components/ProductCard";
import { getCurrentCustomer } from "@/app/lib/customer/read";
import { gqlFetch } from "@/app/lib/graphql";
import {
  MyWishlistDocument,
  MyWishlistShareTokenDocument,
} from "@/app/lib/wishlist/operations.gql";
import { ShareLinkButton } from "./ShareLinkButton";
import { MoveToCartButton } from "./MoveToCartButton";
import type { ProductSummary } from "@/app/lib/catalog/read";
import { WishlistMerge } from "./WishlistMerge";

export const metadata = { title: "Wishlist" };

export default async function WishlistPage({ params }: PageProps<"/[locale]/account/wishlist">) {
  // Locale straight off the segment - a page taking no props has no request scope for
  // getLocale() to read, so it silently answered with the default language
  const { locale } = await params;

  const customer = await getCurrentCustomer();
  if (!customer) redirect(await withLocalePrefix("/account/login?returnTo=/account/wishlist", locale));

  let products: ProductSummary[] = [];
  try {
    const res = await gqlFetch(MyWishlistDocument, {});
    products = res.data.myWishlist as unknown as ProductSummary[];
  } catch {
    products = [];
  }
  let shareToken = "";
  try {
    const tok = await gqlFetch(MyWishlistShareTokenDocument, {});
    shareToken = tok.data.myWishlistShareToken;
  } catch {
    // No share link is a missing feature, not a missing wishlist - the products stay on
    // the page
    shareToken = "";
  }

  return (
    <>
      <AccountPageHeader
        title="Wishlist"
        description={
          products.length > 0
            ? `${products.length} item${products.length === 1 ? "" : "s"} saved for later.`
            : undefined
        }
        action={shareToken ? <ShareLinkButton token={shareToken} /> : undefined}
      />
      <WishlistMerge serverIds={products.map((p) => p.id)} />
      {products.length === 0 ? (
        <EmptyState
          icon={<HeartIcon className="size-5" aria-hidden="true" />}
          title="Nothing saved yet"
          description="Tap the heart on any product and it waits for you here."
          actionHref="/products"
          actionLabel="Browse products"
        />
      ) : (
        <div className="grid grid-cols-2 gap-5 lg:grid-cols-3">
          {products.map((p) => (
            <div key={p.id}>
              <ProductCard product={p} variant="compact" />
              <MoveToCartButton productId={p.id} />
            </div>
          ))}
        </div>
      )}
    </>
  );
}
