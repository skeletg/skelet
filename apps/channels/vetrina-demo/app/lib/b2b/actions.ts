"use server";

import { revalidatePath } from "next/cache";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { CART_COOKIE } from "@/app/lib/cart/read";
import { gqlFetch } from "@/app/lib/graphql";
import {
  MyCompanyDocument,
  MyCompanyInvoicesDocument,
  MyCompanyOrdersDocument,
  MyQuotesDocument,
  RequestQuoteDocument,
  RespondToQuoteDocument,
} from "@/app/lib/b2b/operations.gql";

export type MyCompany = {
  name: string;
  status: string;
  role: string;
  creditLimitCents: number;
  creditUsedCents: number;
  paymentTermsDays: number;
};

export type MyQuote = {
  id: string;
  status: string;
  note: string;
  sellerNote: string;
  totalCents: number;
  validUntil: string | null;
  createdAt: string;
  items: { sku: string; name: string; quantity: number; unitPriceCents: number; optionsSummary: string }[];
};

export type CompanyOrder = {
  id: string;
  number: string;
  status: string;
  paymentStatus: string;
  totalCents: number;
  currencyCode: string;
  memberEmail: string;
  createdAt: string;
};

export async function getMyCompany(): Promise<MyCompany | null> {
  try {
    const res = await gqlFetch(MyCompanyDocument, {});
    return res.data.myCompany ?? null;
  } catch {
    return null;
  }
}

export async function getMyQuotes(): Promise<MyQuote[]> {
  try {
    const res = await gqlFetch(MyQuotesDocument, {});
    return res.data.myQuotes;
  } catch {
    return [];
  }
}

export async function getMyCompanyOrders(): Promise<CompanyOrder[]> {
  try {
    const res = await gqlFetch(MyCompanyOrdersDocument, {});
    return res.data.myCompanyOrders;
  } catch {
    return [];
  }
}

export type CompanyInvoice = Awaited<
  ReturnType<typeof getMyCompanyInvoices>
>[number];

export async function getMyCompanyInvoices() {
  try {
    const res = await gqlFetch(MyCompanyInvoicesDocument, {});
    return res.data.myCompanyInvoices;
  } catch {
    return [];
  }
}

export type QuoteActionResult = { ok: boolean; error?: string };

export async function requestQuoteAction(
  formData: FormData,
): Promise<QuoteActionResult> {
  const note = String(formData.get("note") ?? "").trim();
  const cookieStore = await cookies();
  const cartId = cookieStore.get(CART_COOKIE)?.value;
  if (!cartId) {
    return { ok: false, error: "Your cart is empty." };
  }
  try {
    await gqlFetch(RequestQuoteDocument, { cartId, note: note || null });
  } catch {
    return {
      ok: false,
      error: "Could not send the quote request. Sign in and try again.",
    };
  }
  revalidatePath("/account/company");
  return { ok: true };
}

export async function respondToQuoteAction(formData: FormData): Promise<void> {
  const id = String(formData.get("id") ?? "");
  const accept = String(formData.get("accept") ?? "") === "1";
  if (!id) return;
  let ok = true;
  try {
    await gqlFetch(RespondToQuoteDocument, { id, accept });
  } catch (err) {
    // Quote not open / expired - say so rather than re-rendering an unchanged page
     
    console.error("quote response failed:", err);
    ok = false;
  }
  revalidatePath("/account/company");
  redirect(ok ? "/account/company?done=quote" : "/account/company?error=quote");
}
