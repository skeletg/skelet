// The one channel this storefront serves, decided ONCE - three modules read the env directly and
// only one of them enforced the rule below, so the other two shipped an empty header instead
//
// No imports on purpose: proxy.ts runs as middleware, and pulling a server-only module in there
// (read.ts imports next-intl/server) is not an option

// Channel uuid, not the slug - ids don't reveal sibling storefronts the way guessable slugs do
export const DEFAULT_CHANNEL_ID = process.env.STOREFRONT_CHANNEL_ID ?? "";

// `next build` runs with NODE_ENV=production and none of the deployment's env, so the check below
// has to let the build through - it is about serving traffic, not compiling
const BUILDING = process.env.NEXT_PHASE === "phase-production-build";

// There is no such thing as a storefront request without a channel. Sending an empty header lets
// the backend fall back to Host and then to whichever channel is flagged default, which renders
// someone else's market and looks like it worked - so the absence is an error in every
// environment, dev included
export function requireChannelId(): string {
  if (!DEFAULT_CHANNEL_ID && !BUILDING) {
    throw new Error(
      "STOREFRONT_CHANNEL_ID is not set - a storefront is pinned to exactly one channel (admin > channels)",
    );
  }
  return DEFAULT_CHANNEL_ID;
}
