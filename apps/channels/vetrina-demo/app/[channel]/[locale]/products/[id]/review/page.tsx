// Review submission page

import { redirect } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import { withLocalePrefix } from "@/app/lib/locale/path";

import { getProduct } from "@/app/lib/catalog/read";
import { getCustomerOrNull } from "@/app/lib/customer/read";
import { productHref } from "@/app/lib/entity-href";
import { loadMyReviewState } from "@/app/lib/reviews/actions-client";
import { getProductWithReviews } from "@/app/lib/reviews/read";
import { ReviewForm } from "./ReviewForm";

type Params = { id: string };

export default async function ReviewSubmissionPage({
  params,
}: {
  params: Promise<Params>;
}) {
  const { id } = await params;
  const customer = await getCustomerOrNull();

  const { product } = await getProductWithReviews(id);
  if (!product) redirect(await productPage(id));
  // ProductWithReviews is a public op - sent without the session so the PDP can cache it - so its customerCanReview
  // always reads NOT_AUTHENTICATED, which bounced every signed-in customer between here and the login page
  const eligibility =
    (await loadMyReviewState(id)).eligibility ?? product!.customerCanReview;
  if (!eligibility.canReview) {
    if (eligibility.reason === "NOT_AUTHENTICATED") {
      const returnTo = encodeURIComponent(`/products/${id}/review`);
      redirect(await withLocalePrefix(`/account/login?returnTo=${returnTo}`, await getLocale()));
    }
    if (eligibility.reason !== "ALREADY_REVIEWED") redirect(await productPage(id));
  }

  const t = await getTranslations("product.reviewForm");
  return (
    <main className="mx-auto w-full max-w-2xl px-6 pt-6 pb-10">
      <h1 className="text-2xl font-extrabold tracking-tight">{t("title")}</h1>
      <p className="mt-1 text-sm text-muted-foreground">
        {t.rich("reviewing", {
          name: () => (
            <span className="font-medium text-foreground">
              {product!.name ?? product!.sku}
            </span>
          ),
        })}
      </p>
      <ReviewForm
        productId={id}
        questions={product!.reviewQuestions}
        isGuest={!customer}
      />
    </main>
  );
}

// The PDP lives only at its pretty URL - /products/<id> is a 404 since the catch-all took over
async function productPage(id: string): Promise<string> {
  const { product } = await getProduct(id);
  return withLocalePrefix(product ? productHref(product) : "/", await getLocale());
}
