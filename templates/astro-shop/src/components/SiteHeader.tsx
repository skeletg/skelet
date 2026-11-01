const strip = ["Free returns within 30 days", "Secure checkout, no account needed", "Small batches, shipped with care"];
const links = [
  { href: "/#products", label: "Shop" },
  { href: "/#story", label: "Our story" },
  { href: "/#promise", label: "Our promise" },
];

// The header's and the footer's, which differ only in size
export function Logo({ name, size }: { name: string; size: string }) {
  return (
    <a href="/" className={`flex items-center gap-2.5 ${size} font-semibold tracking-tight`}>
      <span aria-hidden="true" className="grid size-8 place-items-center rounded-lg bg-inverse-foreground">
        <span className="size-3 rounded-full bg-accent"></span>
      </span>
      {name}
    </a>
  );
}

export function SiteHeader({ shop }: { shop: string }) {
  return (
    <header className="sticky top-0 z-40 scheme-dark bg-inverse/95 text-inverse-foreground backdrop-blur-md [view-transition-name:site-header]">
      <div className="border-b border-inverse-foreground/10">
        <ul className="container-page flex h-9 items-center justify-center gap-4 text-[0.7rem] tracking-wide text-inverse-foreground/70">
          {strip.map((item, i) => (
            <li key={item} className={`items-center gap-4 ${i === 0 ? "flex" : "hidden sm:flex"}`}>
              {i > 0 && (
                <span aria-hidden="true" className="text-inverse-foreground/25">
                  |
                </span>
              )}
              {item}
            </li>
          ))}
        </ul>
      </div>
      <div className="container-page flex h-16 items-center justify-between gap-4 sm:h-[4.5rem]">
        <Logo name={shop} size="text-[1.05rem]" />
        <nav aria-label="Main" className="hidden items-center gap-7 text-[0.8125rem] font-medium md:flex">
          {links.map((l) => (
            <a key={l.href} href={l.href} className="text-inverse-foreground/80 transition-colors hover:text-inverse-foreground">
              {l.label}
            </a>
          ))}
        </nav>
        <div className="flex items-center gap-1">
          <a href="/#products" className="mr-2 hidden rounded-full bg-accent px-4 py-2 text-xs font-semibold tracking-wide text-accent-foreground transition-opacity hover:opacity-90 sm:inline-flex">
            Shop now
          </a>
          <a href="/cart" data-astro-prefetch="false" className="relative grid size-10 place-items-center rounded-full transition-colors hover:bg-inverse-foreground/10">
            <span className="sr-only">Cart</span>
            <svg aria-hidden="true" viewBox="0 0 24 24" className="size-5" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
              <path d="M6 7h12l-1 13H7L6 7Z" />
              <path d="M9 7a3 3 0 0 1 6 0" />
            </svg>
            {/* Filled from the cart_count cookie by Base's inline script, so cached HTML stays the same for everyone */}
            <span data-cart-count="" hidden className="absolute -right-0.5 -top-0.5 grid h-5 min-w-5 place-items-center rounded-full bg-accent px-1.5 text-[0.7rem] font-bold text-accent-foreground"></span>
          </a>
          {/* Scripts off, the system theme applies and there's nothing to toggle */}
          <button
            type="button"
            data-theme-toggle=""
            aria-pressed="false"
            className="hidden size-10 place-items-center rounded-full text-inverse-foreground/80 transition-colors hover:bg-inverse-foreground/10 hover:text-inverse-foreground js:grid"
          >
            <span className="sr-only">Dark theme</span>
            <svg aria-hidden="true" viewBox="0 0 24 24" className="size-5 dark:hidden" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
              <path d="M20 14.5A8 8 0 0 1 9.5 4a8 8 0 1 0 10.5 10.5Z" />
            </svg>
            <svg aria-hidden="true" viewBox="0 0 24 24" className="hidden size-5 dark:block" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round">
              <circle cx="12" cy="12" r="4" />
              <path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
            </svg>
          </button>
        </div>
      </div>
    </header>
  );
}
