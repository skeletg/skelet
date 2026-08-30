"use server";

import { revalidatePath } from "next/cache";

import { getProduct } from "@/app/lib/catalog/read";
import { productHref } from "@/app/lib/entity-href";
import { gqlFetch } from "@/app/lib/graphql";
import {
  ProductQuestionsDocument,
  SubmitProductQuestionDocument,
} from "@/app/lib/qna/operations.gql";

export type ProductQuestion = {
  id: string;
  authorName: string;
  question: string;
  answer: string;
  answeredAt: string | null;
  createdAt: string;
};

export async function getProductQuestions(
  productId: string,
): Promise<ProductQuestion[]> {
  try {
    const res = await gqlFetch(ProductQuestionsDocument, {
      productId,
      limit: 20,
    });
    return res.data.productQuestions;
  } catch {
    return [];
  }
}

export type AskResult = { ok: boolean; error?: string };

export async function askQuestionAction(
  productId: string,
  formData: FormData,
): Promise<AskResult> {
  const question = String(formData.get("question") ?? "").trim();
  const authorName = String(formData.get("authorName") ?? "").trim();
  const authorEmail = String(formData.get("authorEmail") ?? "").trim();
  if (question.length < 10) {
    return { ok: false, error: "Questions need at least 10 characters." };
  }
  try {
    await gqlFetch(SubmitProductQuestionDocument, {
      productId,
      question,
      authorName: authorName || null,
      authorEmail: authorEmail || null,
    });
  } catch {
    return {
      ok: false,
      error:
        "Could not submit your question. Add a display name if you're not signed in, then try again.",
    };
  }
  const { product } = await getProduct(productId);
  if (product) revalidatePath(productHref(product));
  return { ok: true };
}
