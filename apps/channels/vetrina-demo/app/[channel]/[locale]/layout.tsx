import type { Metadata } from "next";
import { getSiteUrl } from "@/app/lib/channel/site";
import { notFound } from "next/navigation";
import { NextIntlClientProvider } from "next-intl";
import { getTranslations, setRequestLocale } from "next-intl/server";
import { DefaultLocaleProvider } from "@/app/components/LocaleLink";
import { Geist_Mono, Inter } from "next/font/google";
import "@/app/globals.css";
// Side-effect import: installed storefront plugins register their nav items and slots here, before
// any page renders - see app/plugins/loaded.ts
import "@/app/plugins/loaded";

import { AuthUIProvider } from "@/app/components/AuthUIProvider";
import { CartUIProvider } from "@/app/components/CartUIProvider";
import { CartHydrator } from "@/app/components/CartHydrator";
import { LensGate } from "@/app/components/LensGate";
import { CustomerProvider } from "@/app/lib/customer/client";
import { CartMergeNotice } from "@/app/components/CartMergeNotice";
import { CartStoreProvider } from "@/app/lib/cart/store";
import { CompareTray } from "@/app/components/CompareTray";
import { CookieConsentBanner } from "@/app/components/CookieConsentBanner";
import { Footer } from "@/app/components/Footer";
import { Header } from "@/app/components/Header";
import { SignupForms } from "@/app/components/SignupForms";
import { ChannelProvider } from "@/app/components/ChannelProvider";
import { JsonLd } from "@/app/components/JsonLd";
import { getChannel, getStoreName } from "@/app/lib/channel/read";
import { organizationJsonLd, webSiteJsonLd } from "@/app/lib/json-ld";
import { getBranding, getSignalsEnabled } from "@/app/lib/config/read";
import { getCurrentChannelLocales, getCurrentLocale } from "@/app/lib/locale/read";
import { BUILD_PROBE_CHANNEL } from "@/app/lib/channel";
import { htmlLangTag } from "@/app/lib/locale/lang";
import { getPresentmentOptions } from "@/app/lib/presentment/read";
import { MasqueradeBanner } from "@/app/components/MasqueradeBanner";
import { NavProgress } from "@/app/components/NavProgress";
import { PageTransition } from "@/app/components/PageTransition";
import { PresentmentProvider } from "@/app/components/PresentmentProvider";

// Variable font - the full weight axis, so headlines can mix 400/700/800 spans
const inter = Inter({
  variable: "--font-body",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

// A function rather than a static object so the tab icon can come from the branding setting - an
// operator uploading a favicon shouldn't need a redeploy
//
// The bundled default lives in public/, NOT app/. Next auto-emits a <link rel="icon"> for an
// app/favicon.ico on top of whatever metadata declares, which left two competing icon links and the
// browser choosing between them - serving the default from public/ means exactly one link either way
export async function generateMetadata({
  params,
}: LayoutProps<"/[channel]/[locale]">): Promise<Metadata> {
  const { locale } = await params;
  const [{ faviconUrl }, t] = await Promise.all([
    getBranding(),
    getTranslations({ locale, namespace: "meta" }),
  ]);
  return {
    title: "Vetrina",
    description: t("description"),
    icons: { icon: faviconUrl ?? "/favicon.ico" },
  };
}

// This layout reads no cookie, so it never forces every route dynamic - a route is static
// only once its own fetches are cached too, and personalized state hydrates client-side

// A locale added in admin must start working without a rebuild - generateStaticParams only seeds
// what to prerender, it must not be the gate. The guard below is the gate, and it re-reads the
// channel every render
export const dynamicParams = true;

// The host picks the channel at runtime, so the build renders one unreachable probe to learn which routes read the request
export async function generateStaticParams() {
  return [{ channel: BUILD_PROBE_CHANNEL, locale: "en" }];
}

export default async function RootLayout({
  children,
  params,
}: Readonly<{
  children: React.ReactNode;
  params: Promise<{ channel: string; locale: string }>;
}>) {
  const { channel: channelId, locale: requested } = await params;
  // Against the CHANNEL's list - a channel with no German must not answer /de/ at all, or it
  // serves clamped English content under a url claiming to be German. An empty list means the
  // backend is unreachable; render rather than turn a blip into 404s a crawler will believe
  const { codes, defaultLocale } = await getCurrentChannelLocales();
  if (codes.length > 0 && !codes.includes(requested)) notFound();
  // Makes the locale available to every server component below WITHOUT reading a request, which is
  // what lets these pages stay statically rendered
  setRequestLocale(requested);
  const [channel, storeName, branding, locale, presentment, signalsEnabled, siteUrl] = await Promise.all([
    getChannel(),
    getStoreName(),
    getBranding(),
    getCurrentLocale(),
    getPresentmentOptions(),
    getSignalsEnabled(),
    getSiteUrl(),
  ]);
  // The palette is already funnelled through two custom properties, so overriding them here is the
  // whole of theming: --primary is the solid fill buttons use, --signal the accent on links, eyebrows
  // and badges. Both values are re-validated as #rrggbb in getBranding before they reach this string
  const brandVars = [
    branding.primaryColor ? `--primary:${branding.primaryColor};--ring:${branding.primaryColor}` : "",
    branding.secondaryColor ? `--signal:${branding.secondaryColor}` : "",
  ]
    .filter(Boolean)
    .join(";");

  const langTag = await htmlLangTag(locale); // loadHeader is per-request cached, so this costs no extra fetch

  return (
    <html
      lang={langTag}
      // Channel id stamped server-side so client SDKs can send it as X-Channel-Id regardless of consent - the backend
      // resolves the channel from that header (the /t proxy can't inject it)
      data-channel-id={channel?.id ?? ""}
      className={`${inter.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="min-h-full flex flex-col">
        {/* After the stylesheet so it wins, and scoped to :root so both light and dark inherit it */}
        {brandVars ? (
          <style dangerouslySetInnerHTML={{ __html: `:root{${brandVars}}` }} />
        ) : null}
        {/* Sitewide brand identity + sitelinks-search-box markup - the customer-facing name, so
            search results carry the branding setting rather than the operator's internal label */}
        {storeName && (
          <>
            <JsonLd data={organizationJsonLd({ name: storeName }, siteUrl)} />
            <JsonLd data={webSiteJsonLd({ name: storeName }, siteUrl)} />
          </>
        )}
        {/* overflow-x-clip contains descendant overflow (carousel rails) HERE, before it reaches
            body - mobile Chrome sizes the layout viewport off body's overflow region even though
            body itself clips, and the page rendered zoomed out on phones without this */}
        <div id="page-shell" className="flex min-h-full flex-1 flex-col overflow-x-clip">
        <NextIntlClientProvider>
        <DefaultLocaleProvider value={defaultLocale} channelId={channelId}>
        <ChannelProvider value={channel}>
          <PresentmentProvider options={presentment.options}>
          <CartUIProvider>
            <CartStoreProvider initial={null}>
            <AuthUIProvider>
              <CustomerProvider>
              {/* Outside the page slot so a navigation can't fade the thing reporting on it */}
              <NavProgress />
              <MasqueradeBanner />
              <CartMergeNotice />
              <CartHydrator />
              <Header />
              <PageTransition>{children}</PageTransition>
              <Footer />
              <CompareTray />
              <CookieConsentBanner />
              <LensGate signalsEnabled={signalsEnabled} channelId={channel?.id ?? ""} />
              {/* Shown regardless of marketing consent - the form is how the store earns it */}
              {signalsEnabled && <SignupForms channelId={channel?.id ?? ""} />}
              </CustomerProvider>
            </AuthUIProvider>
            </CartStoreProvider>
          </CartUIProvider>
          </PresentmentProvider>
        </ChannelProvider>
        </DefaultLocaleProvider>
        </NextIntlClientProvider>
        </div>
      </body>
    </html>
  );
}
