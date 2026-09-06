import { getCart, getFreeShippingThreshold } from "@/app/lib/cart/read";
import { getCartRecommendations } from "@/app/lib/catalog/read";
import { ProductCard } from "@/app/components/ProductCard";
import { CartView } from "./CartView";

export const metadata = {
  title: "Cart",
};

export default async function CartPage() {
  const [cart, freeShipThreshold] = await Promise.all([
    getCart(),
    getFreeShippingThreshold(),
  ]);
  const recommendations = cart ? await getCartRecommendations(cart.id) : [];

  const recommendationsNode =
    recommendations.length > 0 ? (
      <section className="mt-16">
        <h2 className="text-lg font-semibold tracking-tight">
          You might also need
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
