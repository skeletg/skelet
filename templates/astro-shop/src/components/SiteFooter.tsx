import { Logo } from "./SiteHeader";

// fontPicker: the dev-only font switch (theme.ts)
export function SiteFooter({ shop, fontPicker = false }: { shop: string; fontPicker?: boolean }) {
  return (
    <footer className="scheme-dark bg-inverse text-inverse-foreground">
      <div className="container-page grid gap-12 py-16 sm:grid-cols-2 sm:py-20 lg:grid-cols-4">
        <div className="lg:col-span-2">
          <Logo name={shop} size="text-lg" />
          <p className="mt-5 max-w-sm text-sm leading-relaxed text-inverse-foreground/70">
            A small, considered collection - picked to be used every day and kept for years. Small batches, shipped with care.
          </p>
        </div>
        <nav aria-label="Shop">
          <h2 className="eyebrow text-inverse-foreground">Shop</h2>
          <ul className="mt-5 space-y-3 text-sm text-inverse-foreground/70">
            <li>
              <a href="/#products" className="hover:text-inverse-foreground">
                All products
              </a>
            </li>
            <li>
              <a href="/cart" data-astro-prefetch="false" className="hover:text-inverse-foreground">
                Cart
              </a>
            </li>
            <li>
              <a href="/checkout" data-astro-prefetch="false" className="hover:text-inverse-foreground">
                Checkout
              </a>
            </li>
          </ul>
        </nav>
        <nav aria-label="Information">
          <h2 className="eyebrow text-inverse-foreground">Information</h2>
          <ul className="mt-5 space-y-3 text-sm text-inverse-foreground/70">
            <li>
              <a href="/#story" className="hover:text-inverse-foreground">
                Our story
              </a>
            </li>
            <li>
              <a href="/#promise" className="hover:text-inverse-foreground">
                Our promise
              </a>
            </li>
            <li>
              <a href="/sitemap.xml" className="hover:text-inverse-foreground">
                Sitemap
              </a>
            </li>
          </ul>
        </nav>
      </div>
      <div className="border-t border-inverse-foreground/10">
        <div className="container-page flex flex-col gap-4 py-6 text-xs text-inverse-foreground/60 sm:flex-row sm:items-center sm:justify-between">
          <p>
            © {new Date().getFullYear()} {shop}. All rights reserved.
          </p>
          {fontPicker && (
            <label className="flex items-center gap-2">
              Font
              <select data-font-picker="" className="min-h-8 w-auto py-1 text-xs">
                <option value="default">Poppins</option>
                <option value="montserrat">Montserrat</option>
                <option value="system">System</option>
              </select>
            </label>
          )}
        </div>
      </div>
    </footer>
  );
}
