export const SECURE_COOKIES = process.env.NODE_ENV === "production";

export function cookieOptions(maxAge: number, httpOnly = true) {
  return { path: "/", httpOnly, sameSite: "lax" as const, secure: SECURE_COOKIES, maxAge };
}
