import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { ProductCard } from "@/app/components/ProductCard";
import type { ProductSummary } from "@skelet/storefront-next/catalog/read";
import { gqlFetch } from "@skelet/storefront-next/graphql";
import { SharedWishlistDocument } from "@skelet/storefront-next/wishlist/operations.gql";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/wishlist/[token]">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "wishlist" });
  return { title: t("shared"), robots: { index: false, follow: false } };
}

export default async function SharedWishlistPage(
  props: PageProps<"/[channel]/[locale]/wishlist/[token]">,
) {
  const { token, locale } = await props.params;
  const t = await getTranslations({ locale, namespace: "wishlist" });
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
        {t("shared")}
      </p>
      <h1 className="mt-1 text-3xl font-extrabold tracking-tight">
        {t("headline")}
      </h1>
      {products.length === 0 ? (
        <p className="mt-6 text-sm text-muted-foreground">
          {t("empty")}
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
