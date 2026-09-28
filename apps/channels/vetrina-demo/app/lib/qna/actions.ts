"use server";

import { revalidatePublicPath } from "@/app/lib/revalidate";

import { getAccessToken } from "@/app/lib/auth/session";
import { getProduct } from "@/app/lib/catalog/read";
import { productHref } from "@/app/lib/entity-href";
import { parseForm, type FormResult } from "@/app/lib/forms/schema";
import { replyError } from "@/app/lib/forms/reply";
import { gqlFetch } from "@/app/lib/graphql";
import {
  ProductQuestionsDocument,
  SubmitProductQuestionDocument,
} from "@/app/lib/qna/operations.gql";
import { askQuestionSchema } from "@/app/lib/qna/schemas";

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

// productId arrives bound, like the reset token - the form only carries what the shopper typed
export async function askQuestionAction(
  productId: string,
  _prev: FormResult | undefined,
  formData: FormData,
): Promise<FormResult> {
  const guest = !(await getAccessToken());
  const submission = parseForm(formData, askQuestionSchema({ guest }));
  if (submission.status !== "success") return submission.reply();
  const { question, authorName, authorEmail } = submission.value;
  try {
    await gqlFetch(SubmitProductQuestionDocument, {
      productId,
      question,
      authorName: authorName || null,
      authorEmail: authorEmail ?? null,
    });
  } catch (err) {
    return replyError(submission, err, "questionFailed");
  }
  const { product } = await getProduct(productId);
  if (product) await revalidatePublicPath(productHref(product));
  return submission.reply({ resetForm: true });
}
