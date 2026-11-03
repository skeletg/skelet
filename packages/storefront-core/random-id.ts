// crypto.randomUUID exists only in a SECURE CONTEXT - https, or localhost. Reaching the dev server
// over the LAN (http://192.168.x.x:3000, e.g. from a phone) is not one, so the bare call is
// undefined there and throws, taking Lens tracking and Prism assignment down with it
//
// Client code must route through this rather than calling crypto.randomUUID directly. Server code
// need not - Node always has it - but nothing breaks if it does
export function randomId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  // Not cryptographically strong, and it does not need to be - every caller is minting a local
  // correlation id for an analytics event or an experiment bucket, never a secret
  return `id_${Math.random().toString(36).slice(2)}${Date.now().toString(36)}`;
}
