import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";
import { redirect } from "next/navigation";

import { withLocalePrefix } from "@skelet/storefront-next/locale/path";
import { HeartIcon } from "lucide-react";

import {
  AccountPageHeader,
  EmptyState,
} from "@/app/components/account/AccountUI";
import { ProductCard } from "@/app/components/ProductCard";
import { getCurrentCustomer } from "@skelet/storefront-next/customer/read";
import { gqlFetch } from "@skelet/storefront-next/graphql";
import {
  MyWishlistDocument,
  MyWishlistShareTokenDocument,
} from "@skelet/storefront-next/wishlist/operations.gql";
import { ShareLinkButton } from "./ShareLinkButton";
import { MoveToCartButton } from "./MoveToCartButton";
import type { ProductSummary } from "@skelet/storefront-next/catalog/read";
import { WishlistMerge } from "./WishlistMerge";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/account/wishlist">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.wishlist" });
  return { title: t("title") };
}

export default async function WishlistPage({ params }: PageProps<"/[channel]/[locale]/account/wishlist">) {
  // Locale straight off the segment - a page taking no props has no request scope for
  // getLocale() to read, so it silently answered with the default language
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "account.wishlist" });
  const tc = await getTranslations({ locale, namespace: "common" });

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
        title={t("title")}
        description={
          products.length > 0 ? t("count", { count: products.length }) : undefined
        }
        action={shareToken ? <ShareLinkButton token={shareToken} /> : undefined}
      />
      <WishlistMerge serverIds={products.map((p) => p.id)} />
      {products.length === 0 ? (
        <EmptyState
          icon={<HeartIcon className="size-5" aria-hidden="true" />}
          title={t("emptyTitle")}
          description={t("emptyBody")}
          actionHref="/products"
          actionLabel={tc("browseProducts")}
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
