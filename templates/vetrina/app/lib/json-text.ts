// The backend's content fields are typed as the GraphQL `JSON` scalar because they can
// hold either plain strings or a structured rich-text document
export function renderJsonText(value: unknown): string | null {
  if (value == null) return null;
  if (typeof value === "string") return value.trim() || null;
  if (typeof value === "number" || typeof value === "boolean") {
    return String(value);
  }
  // Seed/admin code wraps plain copy as `{ text, plain }` (both strings, same value) -
  // return one and don't visit the rest, so we don't render the same sentence twice
  if (typeof value === "object" && !Array.isArray(value)) {
    const obj = value as Record<string, unknown>;
    if (typeof obj.plain === "string") {
      const s = obj.plain.trim();
      if (s) return s;
    }
    if (typeof obj.text === "string") {
      const s = obj.text.trim();
      if (s) return s;
    }
  }
  // Fallback: walk the tree and concatenate every string leaf
  const parts: string[] = [];
  const visit = (n: unknown) => {
    if (n == null) return;
    if (typeof n === "string") {
      const s = n.trim();
      if (s && parts[parts.length - 1] !== s) parts.push(s);
      return;
    }
    if (Array.isArray(n)) {
      n.forEach(visit);
      return;
    }
    if (typeof n === "object") {
      Object.values(n as Record<string, unknown>).forEach(visit);
    }
  };
  visit(value);
  return parts.length ? parts.join(" ") : null;
}
