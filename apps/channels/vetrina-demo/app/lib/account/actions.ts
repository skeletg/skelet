"use server";

// Reorder and RMA actions for the customer's past orders
// Both are reachable by direct POST, so the resolvers re-verify order ownership

import { revalidatePath } from "next/cache";

import { gqlFetch, GraphQLRequestError } from "@/app/lib/graphql";
import { ReorderDocument, RequestReturnDocument } from "@/app/lib/checkout/operations.gql";

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
  if (!orderId) return { ok: false, error: "Missing order." };
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
          : "Could not reorder. Please try again.",
    };
  }
}

export type ReturnResult = { ok: boolean; rmaNumber?: string; error?: string };

export async function requestReturnAction(
  _prev: ReturnResult | null,
  formData: FormData,
): Promise<ReturnResult> {
  const orderId = String(formData.get("orderId") ?? "");
  const reason = String(formData.get("reason") ?? "").trim();
  const resolution =
    formData.get("resolution") === "exchange" ? "exchange" : "refund";
  if (!orderId) return { ok: false, error: "Missing order." };

  const lines: Array<{ orderLineId: string; quantity: number }> = [];
  for (const [key, value] of formData.entries()) {
    if (!key.startsWith("qty:")) continue;
    const qty = Number(value);
    if (qty === 0) continue;
    // Order lines are whole units - half a returned item is a rejected RMA at best
    if (!Number.isInteger(qty) || qty < 0) {
      return { ok: false, error: "Return quantities must be whole numbers." };
    }
    lines.push({ orderLineId: key.slice(4), quantity: qty });
  }
  if (lines.length === 0) {
    return { ok: false, error: "Enter a quantity for at least one item." };
  }

  try {
    const res = await gqlFetch(RequestReturnDocument, {
      input: { orderId, lines, reason: reason || null, resolution },
    });
    revalidatePath(`/account/orders/${orderId}`);
    return { ok: true, rmaNumber: res.data.requestReturn.rmaNumber };
  } catch (err) {
    return {
      ok: false,
      error:
        err instanceof GraphQLRequestError
          ? err.message
          : "Could not submit the return. Please try again.",
    };
  }
}
