"use server";
import "../setup";

import * as core from "@skelet/storefront-core/auth/actions";

export async function registerAction(...args: Parameters<typeof core.registerAction>) {
  return core.registerAction(...args);
}

export async function loginAction(...args: Parameters<typeof core.loginAction>) {
  return core.loginAction(...args);
}

export async function requestPasswordResetAction(...args: Parameters<typeof core.requestPasswordResetAction>) {
  return core.requestPasswordResetAction(...args);
}

export async function resetPasswordAction(...args: Parameters<typeof core.resetPasswordAction>) {
  return core.resetPasswordAction(...args);
}

export async function resendVerificationEmailAction(...args: Parameters<typeof core.resendVerificationEmailAction>) {
  return core.resendVerificationEmailAction(...args);
}

export async function resendVerificationAction(...args: Parameters<typeof core.resendVerificationAction>) {
  return core.resendVerificationAction(...args);
}

export async function changePasswordAction(...args: Parameters<typeof core.changePasswordAction>) {
  return core.changePasswordAction(...args);
}

export async function updateProfileAction(...args: Parameters<typeof core.updateProfileAction>) {
  return core.updateProfileAction(...args);
}

export async function addAddressAction(...args: Parameters<typeof core.addAddressAction>) {
  return core.addAddressAction(...args);
}

export async function updateAddressAction(...args: Parameters<typeof core.updateAddressAction>) {
  return core.updateAddressAction(...args);
}

export async function deleteAddressAction(...args: Parameters<typeof core.deleteAddressAction>) {
  return core.deleteAddressAction(...args);
}

export async function setDefaultBillingAction(...args: Parameters<typeof core.setDefaultBillingAction>) {
  return core.setDefaultBillingAction(...args);
}

export async function setDefaultShippingAction(...args: Parameters<typeof core.setDefaultShippingAction>) {
  return core.setDefaultShippingAction(...args);
}

export async function logoutAction(...args: Parameters<typeof core.logoutAction>) {
  return core.logoutAction(...args);
}
