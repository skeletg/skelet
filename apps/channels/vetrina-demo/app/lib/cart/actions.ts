"use server";

import { cookies } from "next/headers";

import {
  AddBundleToCartDocument,
  AddConfiguredToCartDocument,
  AddToCartDocument,
  ApplyCouponDocument,
  CartProbeDocument,
  ClearCartDocument,
  CreateGuestCartDocument,
  MoveSavedToCartDocument,
  RemoveCouponDocument,
  RemoveSavedItemDocument,
  SaveForLaterDocument,
  SetCartNoteDocument,
  RemoveFromCartDocument,
  RequestProductOptionFileUploadDocument,
  UpdateCartLineDocument,
} from "@/app/lib/cart/mutations.gql";
import { CART_COOKIE } from "@/app/lib/cart/read";
import { couponSchema } from "@/app/lib/cart/schemas";
import { parseForm, replyReset, type FormResult } from "@/app/lib/forms/schema";
import { getAccessToken } from "@/app/lib/auth/session";
import { getCartRecommendations } from "@/app/lib/catalog/read";
import { getCart } from "@/app/lib/cart/read";
import { productHref } from "@/app/lib/entity-href";
import {
  CartByIdDocument,
  CartEstimateDocument,
  MyCartDocument,
} from "@/app/lib/cart/queries.gql";
import type { Cart } from "@/app/lib/cart/types";
import { gqlFetch, GraphQLRequestError } from "@/app/lib/graphql";

// Resolves the active cart ID, creating a guest cart server-side (and writing the
// cookie) on first use
async function ensureCartID(): Promise<string> {
  const store = await cookies();
  // Signed in: always the customer's own cart (get-or-create server side)
  if (await getAccessToken()) {
    try {
      const mine = await gqlFetch(MyCartDocument);
      const id = mine.data.myCart.id;
      if (store.get(CART_COOKIE)?.value !== id) setCartCookie(store, id);
      return id;
    } catch {
      // Token expired mid-session - fall back to the guest path so the add still
      // succeeds
    }
  }
  const existing = store.get(CART_COOKIE)?.value;
  if (existing) {
    const probe = await gqlFetch(CartProbeDocument, { id: existing });
    if (probe.data.cart) return existing;
  }
  const res = await gqlFetch(CreateGuestCartDocument);
  const id = res.data.createGuestCart.id;
  setCartCookie(store, id);
  return id;
}

// Shared so the signed-in and guest paths can't drift on cookie flags
function setCartCookie(
  store: Awaited<ReturnType<typeof cookies>>,
  id: string,
) {
  store.set(CART_COOKIE, id, {
    path: "/",
    httpOnly: false,
    sameSite: "lax",
    // Match the auth-cookie pattern: in production the cart cookie is Secure-only so it
    // can't leak over a non-TLS hop
    secure: process.env.NODE_ENV === "production",
    // Match the locale cookie - guest carts are sticky for as long as the browser is
    // around
    maxAge: 60 * 60 * 24 * 365,
  });
}

const PROMO_PENDING_COOKIE = "storefront_pending_promo";

async function consumePendingPromo(cartId: string): Promise<Cart | null> {
  const store = await cookies();
  const pending = store.get(PROMO_PENDING_COOKIE)?.value?.trim();
  if (!pending) return null;
  store.delete(PROMO_PENDING_COOKIE);
  try {
    const res = await gqlFetch(ApplyCouponDocument, { cartId, code: pending });
    return res.data.applyCoupon ?? null;
  } catch {
    // Code expired / unknown / cart-conditions don't qualify - no UI here, just drop
    // the cookie
    return null;
  }
}

// getCartFull re-reads the whole cart for the active cookie
async function getCartFull(cartId: string): Promise<Cart | null> {
  try {
    const res = await gqlFetch(CartByIdDocument, { id: cartId });
    return res.data.cart ?? null;
  } catch {
    return null;
  }
}
export type AddToCartResult = {
  ok: boolean;
  error?: string;
  // Backend error code (e.g. PRODUCT_OPTIONS_REQUIRED) so clients can branch on the
  // typed failure instead of parsing the message
  code?: string;
  cart?: Cart | null;
};
function formatAddError(err: unknown): string {
  if (err instanceof GraphQLRequestError) {
    const msg = err.errors[0]?.message ?? err.message;
    return msg.replace(/^add to cart(?: rejected)?:\s*/i, "");
  }
  return err instanceof Error ? err.message : "addFailed";
}
function addErrorCode(err: unknown): string | undefined {
  if (err instanceof GraphQLRequestError) {
    const code = err.errors[0]?.extensions?.code;
    return typeof code === "string" ? code : undefined;
  }
  return undefined;
}

export async function addSimpleToCart(
  _prev: AddToCartResult | null,
  formData: FormData,
): Promise<AddToCartResult> {
  const sku = String(formData.get("productSku") ?? "").trim();
  const rawQty = Number(formData.get("quantity") ?? 1);
  const quantity = rawQty > 0 ? rawQty : 1;
  if (!sku) return { ok: false, error: "missingProduct" };
  try {
    const cartId = await ensureCartID();
    const res = await gqlFetch(AddToCartDocument, { cartId, sku, quantity });
    const promoCart = await consumePendingPromo(cartId);
    return { ok: true, cart: promoCart ?? res.data.addToCart };
  } catch (err) {
    return { ok: false, error: formatAddError(err), code: addErrorCode(err) };
  }
}

export async function addBundleToCart(
  _prev: AddToCartResult | null,
  formData: FormData,
): Promise<AddToCartResult> {
  const sku = String(formData.get("productSku") ?? "").trim();
  const quantity = Math.max(1, Number(formData.get("quantity") ?? 1));
  // The client encodes the picks as JSON to avoid wrestling with FormData's repeated-key handling for the option →
  // [selectionIds] shape. Shape: [{ optionId, selectionIds: [...] }]
  const selectionsJson = String(formData.get("selections") ?? "[]");
  let selections: { optionId: string; selectionIds: string[] }[];
  try {
    selections = JSON.parse(selectionsJson);
  } catch {
    return { ok: false, error: "invalidBundle" };
  }
  if (!sku) return { ok: false, error: "missingProduct" };
  try {
    const cartId = await ensureCartID();
    const res = await gqlFetch(AddBundleToCartDocument, {
      input: { cartId, sku, quantity, selections },
    });
    return { ok: true, cart: res.data.addBundleToCart };
  } catch (err) {
    return { ok: false, error: formatAddError(err) };
  }
}

export async function addConfiguredToCart(
  _prev: AddToCartResult | null,
  formData: FormData,
): Promise<AddToCartResult> {
  const sku = String(formData.get("productSku") ?? "").trim();
  const rawQty = Number(formData.get("quantity") ?? 1);
  const quantity = rawQty > 0 ? rawQty : 1;
  // The client encodes the picks as JSON to avoid FormData's repeated-key handling for the option → [valueIds] shape.
  // Shape: [{ optionId, valueIds: [...], textValue }]
  const optionsJson = String(formData.get("options") ?? "[]");
  let options: {
    optionId: string;
    valueIds?: string[];
    textValue?: string;
    fileAssetId?: string;
  }[];
  try {
    options = JSON.parse(optionsJson);
  } catch {
    return { ok: false, error: "invalidOption" };
  }
  if (!sku) return { ok: false, error: "missingProduct" };
  try {
    const cartId = await ensureCartID();
    const res = await gqlFetch(AddConfiguredToCartDocument, {
      input: { cartId, sku, quantity, options },
    });
    const promoCart = await consumePendingPromo(cartId);
    return { ok: true, cart: promoCart ?? res.data.addConfiguredToCart };
  } catch (err) {
    return { ok: false, error: formatAddError(err) };
  }
}

export async function requestProductOptionFileUploadAction(input: {
  productSku: string;
  optionId: string;
  filename: string;
  mimeType: string;
  sizeBytes: number;
}): Promise<
  | { ok: true; assetId: string; uploadUrl: string }
  | { ok: false; error: string }
> {
  try {
    const res = await gqlFetch(RequestProductOptionFileUploadDocument, {
      sku: input.productSku,
      optionId: input.optionId,
      filename: input.filename,
      mimeType: input.mimeType,
      sizeBytes: input.sizeBytes,
    });
    return {
      ok: true,
      assetId: res.data.requestProductOptionFileUpload.assetId,
      uploadUrl: res.data.requestProductOptionFileUpload.uploadUrl,
    };
  } catch (err) {
    if (err instanceof GraphQLRequestError) {
      return { ok: false, error: err.errors[0]?.message ?? err.message };
    }
    return {
      ok: false,
      error: err instanceof Error ? err.message : "uploadFailed",
    };
  }
}

// Grouped products don't have a single "addGroupedToCart" mutation - each member
// becomes its own cart line
export async function addGroupedToCart(
  _prev: AddToCartResult | null,
  formData: FormData,
): Promise<AddToCartResult> {
  let lines: {
    productSku: string;
    productName: string;
    quantity: number;
    hasOptions?: boolean;
    options?: {
      optionId: string;
      valueIds?: string[];
      textValue?: string;
      fileAssetId?: string;
    }[];
    missingRequired?: string[];
    uploading?: boolean;
  }[];
  try {
    lines = formData.getAll("line").map((raw) => JSON.parse(String(raw)));
  } catch {
    return { ok: false, error: "invalidSelection" };
  }
  const real = lines.filter((l) => l.productSku && l.quantity > 0);
  if (real.length === 0) return { ok: false, error: "chooseAtLeastOne" };
  // Pre-flight guards for friendlier errors - the backend re-validates required/hidden
  // options authoritatively on the configured add
  for (const l of real) {
    if (l.missingRequired && l.missingRequired.length > 0) {
      return {
        ok: false,
        error: `Pick the required options for ${l.productName}: ${l.missingRequired.join(", ")}.`,
      };
    }
    if (l.uploading) {
      return {
        ok: false,
        error: `A file for ${l.productName} is still uploading - try again in a moment.`,
      };
    }
  }
  // Members are added one at a time, so a mid-batch failure can leave earlier members
  // already on the cart
  let last: Cart | null = null;
  try {
    const cartId = await ensureCartID();
    for (const l of real) {
      if (l.hasOptions) {
        const res = await gqlFetch(AddConfiguredToCartDocument, {
          input: {
            cartId,
            sku: l.productSku,
            quantity: l.quantity,
            options: l.options ?? [],
          },
        });
        last = res.data.addConfiguredToCart;
      } else {
        const res = await gqlFetch(AddToCartDocument, { cartId, sku: l.productSku, quantity: l.quantity });
        last = res.data.addToCart;
      }
    }
    return { ok: true, cart: last };
  } catch (err) {
    return { ok: false, error: formatAddError(err), cart: last };
  }
}
// Line edits reconcile to server truth on failure but also carry the rejection
// message - a bounds/forced-qty snap-back with zero feedback reads as a broken stepper
export type CartLineEditResult = { cart: Cart | null; error?: string };

export async function setCartLineQuantity(
  lineId: string,
  quantity: number,
): Promise<CartLineEditResult> {
  if (!lineId) return { cart: null };
  const store = await cookies();
  const cartId = store.get(CART_COOKIE)?.value;
  if (!cartId) return { cart: null };
  const qty = quantity > 0 ? quantity : 0;
  try {
    if (qty === 0) {
      const res = await gqlFetch(RemoveFromCartDocument, { cartId, lineId });
      return { cart: res.data.removeFromCart };
    }
    const res = await gqlFetch(UpdateCartLineDocument, { cartId, lineId, quantity: qty });
    return { cart: res.data.updateCartLine };
  } catch (err) {
    return { cart: await getCartFull(cartId), error: formatAddError(err) };
  }
}
export async function removeCartLine(lineId: string): Promise<Cart | null> {
  if (!lineId) return null;
  const store = await cookies();
  const cartId = store.get(CART_COOKIE)?.value;
  if (!cartId) return null;
  try {
    const res = await gqlFetch(RemoveFromCartDocument, { cartId, lineId });
    return res.data.removeFromCart;
  } catch {
    return getCartFull(cartId);
  }
}

export async function clearCart(): Promise<Cart | null> {
  const store = await cookies();
  const cartId = store.get(CART_COOKIE)?.value;
  if (!cartId) return null;
  try {
    const res = await gqlFetch(ClearCartDocument, { cartId });
    return res.data.clearCart;
  } catch {
    return getCartFull(cartId);
  }
}
export type RestoreLinePayload =
  | { kind: "simple"; productSku: string; quantity: number }
  | {
      kind: "bundle";
      productSku: string;
      quantity: number;
      selections: { optionId: string; selectionIds: string[] }[];
    }
  | {
      kind: "configured";
      productSku: string;
      quantity: number;
      options: { optionId: string; valueIds?: string[]; textValue?: string }[];
    };

// restoreCartLineAction re-adds a removed line via the matching add mutation and
// returns the fully-hydrated cart for the store to adopt
export async function restoreCartLineAction(
  payload: RestoreLinePayload,
): Promise<CartLineEditResult> {
  const cartId = await ensureCartID();
  try {
    if (payload.kind === "bundle") {
      const res = await gqlFetch(AddBundleToCartDocument, {
        input: {
          cartId,
          sku: payload.productSku,
          quantity: payload.quantity,
          selections: payload.selections,
        },
      });
      return { cart: res.data.addBundleToCart };
    }
    if (payload.kind === "configured") {
      const res = await gqlFetch(AddConfiguredToCartDocument, {
        input: {
          cartId,
          sku: payload.productSku,
          quantity: payload.quantity,
          options: payload.options,
        },
      });
      return { cart: res.data.addConfiguredToCart };
    }
    const res = await gqlFetch(AddToCartDocument, { cartId, sku: payload.productSku, quantity: payload.quantity });
    return { cart: res.data.addToCart };
  } catch (err) {
    return { cart: await getCartFull(cartId), error: formatAddError(err) };
  }
}
// cart is set only on success - the form pushes it into the store
export type CouponActionResult = FormResult & { cart?: Cart | null };

// The backend's typed promotion errors as forms.errors keys; an untyped one keeps its own message
function formatCouponError(err: unknown): string {
  if (err instanceof GraphQLRequestError) {
    const code = err.errors[0]?.extensions?.code;
    switch (code) {
      case "PROMOTION_CODE_NOT_FOUND":
        return "couponNotFound";
      case "PROMOTION_RULE_INACTIVE":
      case "PROMOTION_CODE_INACTIVE":
        return "couponInactive";
      case "PROMOTION_RULE_MAX_USES":
      case "PROMOTION_CODE_MAX_USES":
        return "couponMaxUses";
      case "PROMOTION_RULE_NOT_YET_VALID":
      case "PROMOTION_CODE_NOT_YET_VALID":
        return "couponNotYetValid";
      case "PROMOTION_RULE_EXPIRED":
      case "PROMOTION_CODE_EXPIRED":
        return "couponExpired";
      case "PROMOTION_CODE_ON_AUTO_APPLY_RULE":
        return "couponAutoApply";
      case "PROMOTION_PER_CUSTOMER_LIMIT_RULE":
      case "PROMOTION_PER_CUSTOMER_LIMIT_CODE":
        return "couponAlreadyUsed";
    }
    return err.errors[0]?.message ?? "couponFailed";
  }
  return err instanceof Error ? err.message : "couponFailed";
}

export async function applyCoupon(
  _prevState: CouponActionResult | undefined,
  formData: FormData,
): Promise<CouponActionResult> {
  const submission = parseForm(formData, couponSchema);
  if (submission.status !== "success") return submission.reply();
  const cartId = await ensureCartID();
  try {
    const res = await gqlFetch(ApplyCouponDocument, {
      cartId,
      code: submission.value.code,
    });
    return {
      ...replyReset(submission),
      cart: res.data.applyCoupon,
    };
  } catch (err) {
    return submission.reply({ fieldErrors: { code: [formatCouponError(err)] } });
  }
}

// removeCouponAction drops a manually-applied code; returns the hydrated cart for the
// store
export async function removeCouponAction(code: string): Promise<Cart | null> {
  const trimmed = code.trim();
  if (!trimmed) return null;
  const store = await cookies();
  const cartId = store.get(CART_COOKIE)?.value;
  if (!cartId) return null;
  try {
    const res = await gqlFetch(RemoveCouponDocument, { cartId, code: trimmed });
    return res.data.removeCoupon;
  } catch {
    return getCartFull(cartId);
  }
}

// cart is the cart after the adds - the header badge reads the client store, so the caller applies it rather than
// the action revalidating every page
export type QuickOrderResult = {
  ok: boolean;
  added: number;
  errors: Array<{ sku: string; message: string }>;
  cart?: Cart | null;
};

export async function quickOrderAction(
  _prev: QuickOrderResult | null,
  formData: FormData,
): Promise<QuickOrderResult> {
  const rows: Array<{ sku: string; qty: number }> = [];
  for (let i = 0; i < 20; i++) {
    const sku = String(formData.get(`sku:${i}`) ?? "").trim();
    const qty = Number(formData.get(`qty:${i}`) ?? 0);
    if (sku && Number.isFinite(qty) && qty > 0) rows.push({ sku, qty });
  }
  if (rows.length === 0) {
    return { ok: false, added: 0, errors: [{ sku: "", message: "Enter at least one SKU and quantity." }] };
  }
  const cartId = await ensureCartID();
  let added = 0;
  const errors: Array<{ sku: string; message: string }> = [];
  for (const row of rows) {
    try {
      await gqlFetch(AddToCartDocument, { cartId, sku: row.sku, quantity: row.qty });
      added++;
    } catch (err) {
      errors.push({
        sku: row.sku,
        message:
          err instanceof GraphQLRequestError
            ? err.message
            : "Could not add this item.",
      });
    }
  }
  return {
    ok: errors.length === 0,
    added,
    errors,
    cart: added > 0 ? await getCartFull(cartId) : undefined,
  };
}

export async function saveForLaterAction(lineId: string): Promise<Cart | null> {
  const store = await cookies();
  const cartId = store.get(CART_COOKIE)?.value;
  if (!cartId || !lineId) return null;
  try {
    const res = await gqlFetch(SaveForLaterDocument, { cartId, lineId });
    return res.data.saveForLater;
  } catch {
    // Bundle/configured lines aren't save-able yet; reconcile to server truth
    return getCartFull(cartId);
  }
}

export async function moveSavedToCartAction(savedItemId: string): Promise<Cart | null> {
  const store = await cookies();
  const cartId = store.get(CART_COOKIE)?.value;
  if (!cartId || !savedItemId) return null;
  try {
    const res = await gqlFetch(MoveSavedToCartDocument, { cartId, savedItemId });
    return res.data.moveSavedToCart;
  } catch {
    return getCartFull(cartId);
  }
}

export async function removeSavedItemAction(savedItemId: string): Promise<Cart | null> {
  const store = await cookies();
  const cartId = store.get(CART_COOKIE)?.value;
  if (!cartId || !savedItemId) return null;
  try {
    const res = await gqlFetch(RemoveSavedItemDocument, { cartId, savedItemId });
    return res.data.removeSavedItem;
  } catch {
    return getCartFull(cartId);
  }
}

export type CartEstimate = {
  subtotalCents: number;
  cheapestShippingCents: number;
  taxCents: number;
  estimatedTotalCents: number;
  shippingOptions: { carrierName: string; name: string; priceCents: number }[];
};

export async function cartEstimateAction(
  countryCode: string,
  regionCode: string,
): Promise<CartEstimate | null> {
  const store = await cookies();
  const cartId = store.get(CART_COOKIE)?.value;
  if (!cartId || !countryCode.trim()) return null;
  try {
    const res = await gqlFetch(CartEstimateDocument, {
      cartId,
      countryCode: countryCode.trim().toUpperCase(),
      regionCode: regionCode.trim() ? regionCode.trim().toUpperCase() : null,
    });
    return res.data.cartEstimate as CartEstimate;
  } catch {
    return null;
  }
}

export async function setCartNoteAction(note: string): Promise<void> {
  const store = await cookies();
  const cartId = store.get(CART_COOKIE)?.value;
  if (!cartId) return;
  try {
    await gqlFetch(SetCartNoteDocument, { cartId, note });
  } catch {
  }
}
export type CartRecoItem = {
  id: string;
  name: string;
  href: string;
  imageUrl: string | null;
  priceCents: number;
};
export async function cartRecommendationsAction(): Promise<CartRecoItem[]> {
  const store = await cookies();
  const cartId = store.get(CART_COOKIE)?.value;
  if (!cartId) return [];
  try {
    const recs = await getCartRecommendations(cartId);
    return recs.map((p) => ({
      id: p.id,
      name: p.name ?? p.sku,
      href: productHref(p),
      imageUrl: p.mainImage?.url ?? null,
      priceCents: p.defaultPrice,
    }));
  } catch {
    return [];
  }
}

// The cart, fetched after hydration by CartHydrator - reading it during render forces the whole
// route dynamic, and a cached page must carry no visitor's cart in its HTML
export async function loadCartAction(): Promise<Cart | null> {
  return getCart();
}
