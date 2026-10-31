export const CONSENT_COOKIE = "storefront_consent"; // split from read.ts so a client component can name it without importing next/headers

// Not httpOnly: the banner and the trackers resolve consent in the browser, so cached pages need no server render.
// 180 days is the usual GDPR re-prompt window
export const CONSENT_COOKIE_OPTS = {
  path: "/",
  httpOnly: false,
  sameSite: "lax" as const,
  secure: process.env.NODE_ENV === "production",
  maxAge: 60 * 60 * 24 * 180,
};
