import type { useTranslations } from "next-intl";

import type { ProductCustomOption } from "@skelet/storefront-next/catalog/read";

export type FileEntry =
  | { status: "uploading"; filename: string }
  | { status: "ready"; assetId: string; filename: string }
  | { status: "error"; error: string };

export function acceptAttr(fileExtensions: string | null): string | undefined {
  if (!fileExtensions) return undefined;
  const exts = fileExtensions
    .split(",")
    .map((e) => e.trim().replace(/^\./, ""))
    .filter(Boolean)
    .map((e) => `.${e}`);
  return exts.length > 0 ? exts.join(",") : undefined;
}

function fileExt(name: string): string {
  const i = name.lastIndexOf(".");
  return i >= 0 ? name.slice(i + 1).toLowerCase() : "";
}

export function formatBytes(n: number): string {
  if (n >= 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  if (n >= 1024) return `${Math.round(n / 1024)} KB`;
  return `${n} bytes`;
}

// Mirror of the server-side constraint check so the customer gets instant feedback
export function validateFile(
  option: ProductCustomOption,
  file: File,
  t: ReturnType<typeof useTranslations<"product.options">>,
): string | null {
  if (option.maxFileBytes != null && file.size > option.maxFileBytes) {
    return t("fileTooLarge", { max: formatBytes(option.maxFileBytes) });
  }
  if (option.fileExtensions) {
    const allow = option.fileExtensions
      .split(",")
      .map((e) => e.trim().replace(/^\./, "").toLowerCase())
      .filter(Boolean);
    if (allow.length > 0 && !allow.includes(fileExt(file.name))) {
      return t("allowedTypes", { types: allow.join(", ") });
    }
  }
  return null;
}
