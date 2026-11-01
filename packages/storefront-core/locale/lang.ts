import { loadHeader } from "../header/read";

// Codes are stored lowercase, so a de-DE store would otherwise render lang="de-de" - hreflangValue carries the conventional form, and the schema documents "" as "fall back to the code"
export async function htmlLangTag(locale: string): Promise<string> {
  try {
    const { locales } = await loadHeader();
    const match = locales.find((l) => l.code === locale);
    return match?.hreflangValue?.trim() || locale;
  } catch {
    return locale; // the header query failing must not cost the page its lang attribute
  }
}
