"use server";

import { revalidatePublicPath } from "../revalidate";

import { getAccessToken } from "../auth/session";
import { getProduct } from "../catalog/read";
import { productHref } from "../entity-href";
import { parseForm, replyReset, type FormResult } from "../forms/schema";
import { replyError } from "../forms/reply";
import { gqlFetch } from "../graphql";
import {
  ProductQuestionsDocument,
  SubmitProductQuestionDocument,
} from "./operations.gql";
import { askQuestionSchema } from "./schemas";

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
  return replyReset(submission);
}
