import { useTranslations } from "next-intl";

import { Link } from "@/app/components/LocaleLink";

// Global 404 fallback. The title stays static - not-found takes a metadata object, not generateMetadata

export const metadata = {
  title: "Page not found",
};

export default function NotFound() {
  const t = useTranslations("errors");
  const tc = useTranslations("common");
  return (
    <main className="mx-auto flex min-h-[60vh] max-w-xl flex-col items-center justify-center gap-4 px-6 py-16 text-center">
      <p className="text-xs uppercase tracking-widest text-muted-foreground">
        404
      </p>
      <h1 className="text-3xl font-extrabold tracking-tight">
        {t("notFoundTitle")}
      </h1>
      <p className="text-sm text-muted-foreground">
        {t("notFoundBody")}
      </p>
      <div className="mt-4 flex flex-wrap justify-center gap-3">
        <Link
          href="/"
          className="inline-flex h-10 items-center rounded-md bg-primary px-5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
        >
          {t("goHome")}
        </Link>
        <Link
          href="/products"
          className="inline-flex h-10 items-center rounded-md border border-border px-5 text-sm font-medium hover:bg-accent"
        >
          {tc("browseProducts")}
        </Link>
        <Link
          href="/blog"
          className="inline-flex h-10 items-center rounded-md border border-border px-5 text-sm font-medium hover:bg-accent"
        >
          {t("readBlog")}
        </Link>
      </div>
    </main>
  );
}
