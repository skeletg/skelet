// Browser error monitoring, opt-in via NEXT_PUBLIC_SENTRY_DSN - loaded lazily so
// DSN-less builds never download or compile it
import type * as SentryType from "@sentry/nextjs";

let sentry: typeof SentryType | null = null;

if (process.env.NEXT_PUBLIC_SENTRY_DSN) {
  void import("@sentry/nextjs").then((Sentry) => {
    Sentry.init({
      dsn: process.env.NEXT_PUBLIC_SENTRY_DSN,
      environment: process.env.NODE_ENV,
      tracesSampleRate: 0.05,
    });
    sentry = Sentry;
  });
}

export function onRouterTransitionStart(
  href: string,
  navigationType: string,
): void {
  sentry?.captureRouterTransitionStart(href, navigationType);
}
