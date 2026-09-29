import { z } from "zod";

import {
  emailField,
  newPasswordField,
  optionalText,
  passwordField,
  requiredText,
} from "@/app/lib/forms/schema";

// The auth and account forms' shapes, validated on the client first and again in the action. returnTo and the reset
// token stay out of these on purpose - returnTo goes through safeReturnTo, the token is a bound argument

export const loginSchema = z.object({
  email: emailField(),
  password: passwordField(),
});

export const registerSchema = z
  .object({
    firstName: requiredText(),
    lastName: requiredText(),
    email: emailField(),
    password: newPasswordField(),
    confirmPassword: passwordField(),
    marketingOptIn: z.boolean().optional(),
    referralCode: optionalText(),
  })
  .refine((v) => v.password === v.confirmPassword, {
    path: ["confirmPassword"],
    error: "passwordMismatch",
  });

export const forgotSchema = z.object({
  email: emailField(),
});

export const resetSchema = z
  .object({
    newPassword: newPasswordField(),
    confirmPassword: passwordField(),
  })
  .refine((v) => v.newPassword === v.confirmPassword, {
    path: ["confirmPassword"],
    error: "passwordMismatch",
  });

export const changePasswordSchema = z
  .object({
    currentPassword: passwordField(),
    newPassword: newPasswordField(),
    confirmPassword: passwordField(),
  })
  .refine((v) => v.newPassword === v.confirmPassword, {
    path: ["confirmPassword"],
    error: "passwordMismatch",
  });

export const profileSchema = z.object({
  firstName: requiredText(),
  lastName: requiredText(),
  phone: optionalText(),
});

export const addressSchema = z.object({
  firstName: requiredText(),
  lastName: requiredText(),
  company: optionalText(),
  street1: requiredText(),
  street2: optionalText(),
  city: requiredText(),
  state: optionalText(),
  postalCode: requiredText(),
  // 2-letter ISO 3166 - the backend checks the same shape, but its answer is a generic "invalid input"
  countryCode: requiredText()
    .toUpperCase()
    .regex(/^[A-Z]{2}$/, "countryCode"),
  phone: optionalText(),
});
