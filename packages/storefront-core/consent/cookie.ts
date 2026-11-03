import { cookieOptions } from "../cookie-policy";

export const CONSENT_COOKIE = "storefront_consent"; // split from read.ts so a client component can name it without importing the server read

// Not httpOnly: the banner and the trackers resolve consent in the browser, so cached pages need no server render.
// 180 days is the usual GDPR re-prompt window
export const CONSENT_COOKIE_OPTS = cookieOptions(60 * 60 * 24 * 180, false);
