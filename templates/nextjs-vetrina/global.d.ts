import type en from "./messages/en.json";

// Types every t("...") call against the English catalogue - a key that isn't in en.json fails typecheck instead of
// throwing MISSING_MESSAGE at render. The other locales are deep-merged over English (i18n/request.ts), so they can
// only ever be a subset
declare module "next-intl" {
  interface AppConfig {
    Messages: typeof en;
  }
}
