import { getCart, getFreeShippingThreshold } from "@/app/lib/cart/read";
import { getMyCompany } from "@/app/lib/b2b/actions";
import { getB2BEnabled } from "@/app/lib/config/read";
import { getCartRecommendations } from "@/app/lib/catalog/read";
import { ProductCard } from "@/app/components/ProductCard";
import { CartView } from "./CartView";

export const metadata = {
  title: "Cart",
};

export default async function CartPage() {
  const [cart, freeShipThreshold, company, b2bEnabled] = await Promise.all([
    getCart(),
    getFreeShippingThreshold(),
    getMyCompany(),
    getB2BEnabled(),
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
      company={company}
      b2bEnabled={b2bEnabled}
      freeShipThreshold={freeShipThreshold}
      recommendations={recommendationsNode}
    />
  );
}
