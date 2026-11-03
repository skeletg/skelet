// Which pages session replay (LensReplay) and heatmap DOM snapshots (LensTracker) must never capture. Page views,
// clicks and scroll depth still flow there - only the rendered DOM stays out

// The rrweb class both recorders blank. Private page slots carry it from their first render, so the commit that
// swaps a private page in during client navigation is already blocked before either recorder reacts to the route
export const RECORDING_BLOCK_CLASS = "lens-block";

// Every path under these segments shows a shopper's own data - verify-email prints the signed-in address
const PRIVATE_ROOTS = new Set(["account", "cart", "checkout", "orders", "track-order", "gift-card", "verify-email"]);
// A locale prefix (as-needed scheme, so absent for the default locale): en, de, pt-br, zh-hant-tw
const LOCALE_SEGMENT = /^[a-z]{2,3}(-[a-z0-9]{2,8})*$/i;

function privateFrom(segs: string[]): boolean {
  const [first = "", second = ""] = segs;
  if (PRIVATE_ROOTS.has(first)) return true;
  if (first === "wishlist" && second) return true; // /wishlist/<token> is someone's shared list
  return (first === "cms" || first === "blog") && second === "preview";
}

export function isPrivateRecordingPath(path: string): boolean {
  const segs = (path || "/").split(/[?#]/)[0].split("/").filter(Boolean);
  // Checked unprefixed first: "cms" is itself locale-shaped
  return privateFrom(segs) || (LOCALE_SEGMENT.test(segs[0] ?? "") && privateFrom(segs.slice(1)));
}
