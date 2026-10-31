import type { Metadata } from "next";
import { getTranslations } from "next-intl/server";

import { getCart, getFreeShippingThreshold } from "@skelet/storefront-next/cart/read";
import { getCartRecommendations } from "@skelet/storefront-next/catalog/read";
import { ProductCard } from "@/app/components/ProductCard";
import { CartView } from "./CartView";

export async function generateMetadata({
  params,
}: PageProps<"/[channel]/[locale]/cart">): Promise<Metadata> {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "cart" });
  return { title: t("title") };
}

export default async function CartPage({
  params,
}: PageProps<"/[channel]/[locale]/cart">) {
  const { locale } = await params;
  const t = await getTranslations({ locale, namespace: "cart" });
  const [cart, freeShipThreshold] = await Promise.all([
    getCart(),
    getFreeShippingThreshold(),
  ]);
  const recommendations = cart ? await getCartRecommendations(cart.id) : [];

  const recommendationsNode =
    recommendations.length > 0 ? (
      <section className="mt-16">
        <h2 className="text-lg font-semibold tracking-tight">
          {t("alsoNeed")}
        </h2>
        <div className="mt-4 grid grid-cols-2 gap-6 sm:grid-cols-3 lg:grid-cols-4">
          {recommendations.map((p) => (
            <ProductCard key={p.id} product={p} variant="compact" />
          ))}
        </div>
      </section>
    ) : null;

  return (
    <CartView
      freeShipThreshold={freeShipThreshold}
      recommendations={recommendationsNode}
    />
  );
}
