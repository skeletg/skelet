// Signed anon-voter token `<uuid>.<hmac>`, the HMAC keyed with the one secret the storefront and core share for it
import { createHmac } from "node:crypto";

// The storefront holds this key and no session key at all - it was once derived from JWT_SECRET, which put the admin signing key in the most exposed container
export function voterTokenSecret(): Buffer {
  const secret = process.env.REVIEW_VOTER_TOKEN_SECRET?.trim();
  if (!secret) {
    throw new Error("Voter token secret missing: set REVIEW_VOTER_TOKEN_SECRET");
  }
  return Buffer.from(secret, "utf8");
}

function uuidToBytes(id: string): Buffer {
  // Parse "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" → 16 raw bytes,
  // matching how the Go side feeds id[:] into the HMAC
  return Buffer.from(id.replace(/-/g, ""), "hex");
}

export function signVoterToken(id: string): string {
  const mac = createHmac("sha256", voterTokenSecret());
  mac.update(uuidToBytes(id));
  // base64url with no padding - matches Go's base64.RawURLEncoding
  const sig = mac
    .digest("base64")
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
  return `${id}.${sig}`;
}
