"use server";
import "../setup";

import * as core from "@skelet/storefront-core/cart/actions";

export type * from "@skelet/storefront-core/cart/actions";

export async function addSimpleToCart(...args: Parameters<typeof core.addSimpleToCart>) {
  return core.addSimpleToCart(...args);
}

export async function addBundleToCart(...args: Parameters<typeof core.addBundleToCart>) {
  return core.addBundleToCart(...args);
}

export async function addConfiguredToCart(...args: Parameters<typeof core.addConfiguredToCart>) {
  return core.addConfiguredToCart(...args);
}

export async function requestProductOptionFileUploadAction(...args: Parameters<typeof core.requestProductOptionFileUploadAction>) {
  return core.requestProductOptionFileUploadAction(...args);
}

export async function addGroupedToCart(...args: Parameters<typeof core.addGroupedToCart>) {
  return core.addGroupedToCart(...args);
}

export async function setCartLineQuantity(...args: Parameters<typeof core.setCartLineQuantity>) {
  return core.setCartLineQuantity(...args);
}

export async function removeCartLine(...args: Parameters<typeof core.removeCartLine>) {
  return core.removeCartLine(...args);
}

export async function clearCart(...args: Parameters<typeof core.clearCart>) {
  return core.clearCart(...args);
}

export async function restoreCartLineAction(...args: Parameters<typeof core.restoreCartLineAction>) {
  return core.restoreCartLineAction(...args);
}

export async function applyCoupon(...args: Parameters<typeof core.applyCoupon>) {
  return core.applyCoupon(...args);
}

export async function removeCouponAction(...args: Parameters<typeof core.removeCouponAction>) {
  return core.removeCouponAction(...args);
}

export async function quickOrderAction(...args: Parameters<typeof core.quickOrderAction>) {
  return core.quickOrderAction(...args);
}

export async function saveForLaterAction(...args: Parameters<typeof core.saveForLaterAction>) {
  return core.saveForLaterAction(...args);
}

export async function moveSavedToCartAction(...args: Parameters<typeof core.moveSavedToCartAction>) {
  return core.moveSavedToCartAction(...args);
}

export async function removeSavedItemAction(...args: Parameters<typeof core.removeSavedItemAction>) {
  return core.removeSavedItemAction(...args);
}

export async function cartEstimateAction(...args: Parameters<typeof core.cartEstimateAction>) {
  return core.cartEstimateAction(...args);
}

export async function setCartNoteAction(...args: Parameters<typeof core.setCartNoteAction>) {
  return core.setCartNoteAction(...args);
}

export async function cartRecommendationsAction(...args: Parameters<typeof core.cartRecommendationsAction>) {
  return core.cartRecommendationsAction(...args);
}

export async function loadCartAction(...args: Parameters<typeof core.loadCartAction>) {
  return core.loadCartAction(...args);
}
