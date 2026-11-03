// Server error monitoring, opt-in - without SENTRY_DSN the SDK is never imported, so
// unconfigured environments pay nothing. onRequestError forwards errors Next only logs
import type { captureRequestError } from "@sentry/nextjs";

export async function register() {
  if (!process.env.SENTRY_DSN) return;
  const Sentry = await import("@sentry/nextjs");
  Sentry.init({
    dsn: process.env.SENTRY_DSN,
    environment: process.env.NODE_ENV,
    tracesSampleRate: 0.05,
  });
}

export const onRequestError = async (
  ...args: Parameters<typeof captureRequestError>
) => {
  if (!process.env.SENTRY_DSN) return;
  const Sentry = await import("@sentry/nextjs");
  return Sentry.captureRequestError(...args);
};
