"use server";
import "../setup";

import * as core from "@skelet/storefront-core/checkout/actions";

export async function saveDestinationStep(...args: Parameters<typeof core.saveDestinationStep>) {
  return core.saveDestinationStep(...args);
}

export async function saveSplitDeliveryAction(...args: Parameters<typeof core.saveSplitDeliveryAction>) {
  return core.saveSplitDeliveryAction(...args);
}

export async function saveGroupDeliveryAction(...args: Parameters<typeof core.saveGroupDeliveryAction>) {
  return core.saveGroupDeliveryAction(...args);
}

export async function saveDeliveryPointAction(...args: Parameters<typeof core.saveDeliveryPointAction>) {
  return core.saveDeliveryPointAction(...args);
}

export async function saveDetailsStep(...args: Parameters<typeof core.saveDetailsStep>) {
  return core.saveDetailsStep(...args);
}

export async function applyGiftCardAction(...args: Parameters<typeof core.applyGiftCardAction>) {
  return core.applyGiftCardAction(...args);
}

export async function removeGiftCardAction(...args: Parameters<typeof core.removeGiftCardAction>) {
  return core.removeGiftCardAction(...args);
}

export async function setStoreCreditAction(...args: Parameters<typeof core.setStoreCreditAction>) {
  return core.setStoreCreditAction(...args);
}

export async function placeOrderAction(...args: Parameters<typeof core.placeOrderAction>) {
  return core.placeOrderAction(...args);
}

export async function applyCheckoutCouponAction(...args: Parameters<typeof core.applyCheckoutCouponAction>) {
  return core.applyCheckoutCouponAction(...args);
}
