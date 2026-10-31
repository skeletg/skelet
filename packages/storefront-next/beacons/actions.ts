"use server";

import { parseForm, replyReset, type FormResult } from "../forms/schema";
import { BEACON } from "./paths";
import { backInStockSchema, signupSchema } from "./schemas";
import { postBeacon } from "./server";

// "Notify me when it's back". The backend answers 204 whatever it decided, so only a transport failure is an error
export async function notifyBackInStockAction(_prev: FormResult | undefined, formData: FormData): Promise<FormResult> {
  const submission = parseForm(formData, backInStockSchema);
  if (submission.status !== "success") return submission.reply();
  try {
    const res = await postBeacon(BEACON.backInStock, submission.value);
    if (res.ok) return replyReset(submission);
  } catch (err) {
    console.error("back-in-stock request failed", err);
  }
  return submission.reply({ formErrors: ["notifyFailed"] });
}

export type SignupResult = FormResult & { message?: string };

// A signup form lead. A 400 carries one of the backend's shopper-facing sentinels (invalid email, unknown form);
// anything else is ours to word
export async function submitSignupAction(_prev: SignupResult | undefined, formData: FormData): Promise<SignupResult> {
  const submission = parseForm(formData, signupSchema);
  if (submission.status !== "success") return submission.reply();
  const { form, email, name } = submission.value;
  try {
    const res = await postBeacon(BEACON.signupSubmit, { form, email, name: name ?? "" });
    const body = (await res.json().catch(() => ({}))) as { message?: string; error?: string };
    if (res.ok) return { ...replyReset(submission), message: body.message };
    if (res.status === 400 && body.error) return submission.reply({ formErrors: [body.error] });
  } catch (err) {
    console.error("signup form submit failed", err);
  }
  return submission.reply({ formErrors: ["signupFailed"] });
}
