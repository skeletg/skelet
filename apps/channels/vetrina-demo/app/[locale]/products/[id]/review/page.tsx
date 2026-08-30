// Review submission page

import { redirect } from "next/navigation";
import { getLocale } from "next-intl/server";
import { withLocalePrefix } from "@/app/lib/locale/path";

import { getCustomerOrNull } from "@/app/lib/customer/read";
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
  if (!product) {
    redirect(await withLocalePrefix(`/products/${id}`, await getLocale()));
  }
  const eligibility = product!.customerCanReview;
  if (!eligibility.canReview) {
    if (eligibility.reason === "NOT_AUTHENTICATED") {
      const returnTo = encodeURIComponent(`/products/${id}/review`);
      redirect(await withLocalePrefix(`/account/login?returnTo=${returnTo}`, await getLocale()));
    }
    if (eligibility.reason !== "ALREADY_REVIEWED") {
      redirect(await withLocalePrefix(`/products/${id}`, await getLocale()));
    }
  }

  return (
    <main className="mx-auto w-full max-w-2xl px-6 pt-6 pb-10">
      <h1 className="text-2xl font-extrabold tracking-tight">
        Write a review
      </h1>
      <p className="mt-1 text-sm text-muted-foreground">
        Reviewing{" "}
        <span className="font-medium text-foreground">
          {product!.name ?? product!.sku}
        </span>
      </p>
      <ReviewForm
        productId={id}
        questions={product!.reviewQuestions}
        isGuest={!customer}
      />
    </main>
  );
}
