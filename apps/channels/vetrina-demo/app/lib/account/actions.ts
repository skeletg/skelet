"use server";

// Reorder and RMA actions for the customer's past orders
// Both are reachable by direct POST, so the resolvers re-verify order ownership

import { revalidatePublicPath } from "@/app/lib/revalidate";

import { gqlFetch, GraphQLRequestError } from "@/app/lib/graphql";
import { ReorderDocument, RequestReturnDocument } from "@/app/lib/checkout/operations.gql";
import { returnRequestSchema } from "@/app/lib/account/schemas";
import { parseForm, replyReset, type FormResult } from "@/app/lib/forms/schema";
import { replyError } from "@/app/lib/forms/reply";

export type ReorderResult = {
  ok: boolean;
  added?: number;
  skipped?: Array<{ name: string; sku: string; reason: string }>;
  error?: string;
};

export async function reorderAction(
  _prev: ReorderResult | null,
  formData: FormData,
): Promise<ReorderResult> {
  const orderId = String(formData.get("orderId") ?? "");
  if (!orderId) return { ok: false, error: "orderMissing" };
  try {
    const res = await gqlFetch(ReorderDocument, { orderId });
    const r = res.data.reorder;
    return { ok: true, added: r.added, skipped: r.skipped };
  } catch (err) {
    return {
      ok: false,
      error:
        err instanceof GraphQLRequestError
          ? err.message
          : "reorderFailed",
    };
  }
}

export type ReturnResult = FormResult & { rmaNumber?: string };

export async function requestReturnAction(
  _prev: ReturnResult | undefined,
  formData: FormData,
): Promise<ReturnResult> {
  const submission = parseForm(formData, returnRequestSchema);
  if (submission.status !== "success") return submission.reply();
  const { orderId, reason, resolution } = submission.value;
  const lines = submission.value.lines.filter((l) => l.quantity > 0);

  try {
    const res = await gqlFetch(RequestReturnDocument, {
      input: { orderId, lines, reason, resolution },
    });
    await revalidatePublicPath(`/account/orders/${orderId}`);
    return {
      ...replyReset(submission),
      rmaNumber: res.data.requestReturn.rmaNumber,
    };
  } catch (err) {
    return replyError(submission, err, "returnFailed");
  }
}
