"use server";
import "../setup";

import * as core from "@skelet/storefront-core/qna/actions";

export type * from "@skelet/storefront-core/qna/actions";

export async function getProductQuestions(...args: Parameters<typeof core.getProductQuestions>) {
  return core.getProductQuestions(...args);
}

export async function askQuestionAction(...args: Parameters<typeof core.askQuestionAction>) {
  return core.askQuestionAction(...args);
}
