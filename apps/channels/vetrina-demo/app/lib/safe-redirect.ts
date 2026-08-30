// Only same-origin relative redirects - never an absolute URL, so redirect endpoints can't be used as open redirectors.
// Rejects absolute, protocol-relative, and backslash ("/\evil.com" - browsers fold it to //) targets
export function safeRelativePath(to: string | null): string {
  if (!to || !to.startsWith("/") || to.startsWith("//") || to.includes("\\")) return "/";
  return to;
}
