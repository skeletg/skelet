import type { HomeProductsQuery } from "../graphql/product-list.gql";
import type { Channel } from "../lib/channel";
import { ProductCard } from "../components/ProductCard";
import { SectionHeading } from "../components/SectionHeading";

type Product = HomeProductsQuery["browseProducts"]["items"][number];

// shortcut: placeholder media - swap in the shop's own before launch. The hero clip is Mixkit 4479 "Woman weaving a
// cloth on a large wooden loom" (Mixkit Stock Video Free License - commercial use, no credit; many Mixkit clips are
// personal-use only, so check each one), cut to 10s in public/media/. The photos hotlink Unsplash (Unsplash License),
// which shows Unsplash every visitor
const unsplash = (id: string, w: number) => `https://images.unsplash.com/photo-${id}?w=${w}&q=70&auto=format&fit=crop`;
const srcset = (id: string) => [640, 960, 1400].map((w) => `${unsplash(id, w)} ${w}w`).join(", ");
const photos = {
  story: "1556905055-8f358a7a47b2",
  band: "1441986300917-64674bd600d8",
  promise: "1495121605193-b116b5b9c5fe",
};

const promises = [
  "Materials chosen to wear in, not out",
  "Fair prices, with no surprises at checkout",
  "Tracked delivery to your door",
  "Free returns within 30 days",
];

// Products arrive with a URL each - the page drops the ones with no page in this locale
export function HomeView({ products, channel }: { products: Product[]; channel: Channel }) {
  return (
    <>
      <Hero shop={channel.name} />
      <TrustStrip inStock={products.filter((p) => p.inStock).length} />

      <section id="products" aria-labelledby="products-title" className="container-page scroll-mt-28 py-20 sm:py-28">
        <SectionHeading eyebrow="The shop" title="The collection" id="products-title" center>
          {`${products.length} ${products.length === 1 ? "piece" : "pieces"}, each one picked to be used every day and kept for years.`}
        </SectionHeading>
        {products.length === 0 && <p className="mt-12 text-center text-muted-foreground">Nothing for sale yet - check back soon.</p>}
        <ul className="mt-12 grid grid-cols-2 gap-4 sm:gap-6 lg:grid-cols-3 lg:gap-8">
          {products.map((p, i) => (
            <li key={p.id}>
              <ProductCard product={p} currency={channel.currency} priority={i < 2} />
            </li>
          ))}
        </ul>
      </section>

      <Story shop={channel.name} count={products.length} />
      <Band />
      <PromiseSection />
    </>
  );
}

function Hero({ shop }: { shop: string }) {
  return (
    <section className="relative isolate flex min-h-[min(88svh,52rem)] items-center overflow-hidden scheme-dark bg-inverse text-inverse-foreground">
      <img src="/media/hero-poster.jpg" alt="" width="1280" height="720" fetchPriority="high" className="absolute inset-0 -z-30 h-full w-full object-cover opacity-55" />
      {/* Wide screens with motion allowed play the clip over its own first frame; phones and reduced motion keep the
          still, and never download the video */}
      <video aria-hidden="true" autoPlay muted loop playsInline className="absolute inset-0 -z-20 h-full w-full object-cover opacity-55">
        <source src="/media/hero.mp4" type="video/mp4" media="(min-width: 48rem) and (prefers-reduced-motion: no-preference)" />
      </video>
      <div aria-hidden="true" className="absolute inset-0 -z-10 bg-linear-to-r from-inverse via-inverse/75 to-inverse/10"></div>
      <div aria-hidden="true" className="absolute inset-x-0 bottom-0 -z-10 h-40 bg-linear-to-t from-inverse"></div>

      <div className="container-page pb-32 pt-24 sm:pb-40 sm:pt-28">
        <p className="eyebrow inline-flex items-center gap-2.5 rounded-full border border-inverse-foreground/20 bg-inverse-foreground/5 px-4 py-2 text-inverse-foreground/90 backdrop-blur-sm">
          <span className="size-1.5 rounded-full bg-accent"></span>
          Crafted goods · everyday essentials
        </p>
        <h1 className="mt-6 max-w-3xl text-5xl font-semibold leading-[1.02] tracking-tight text-balance sm:text-7xl">Good things, made to last.</h1>
        <p className="mt-6 max-w-xl text-lg leading-relaxed text-pretty text-inverse-foreground/80">
          A small, considered collection from {shop}. Pick something you'll keep using - we'll take care of the rest.
        </p>
        <div className="mt-10 flex flex-wrap gap-3">
          <a href="#products" className="inline-flex min-h-12 items-center gap-2 rounded-full bg-inverse-foreground px-7 text-sm font-semibold tracking-wide text-inverse transition duration-200 hover:gap-3 active:scale-[0.97]">
            Shop the collection <span aria-hidden="true">→</span>
          </a>
          <a href="#story" className="inline-flex min-h-12 items-center rounded-full border border-inverse-foreground/30 px-7 text-sm font-semibold tracking-wide transition-colors hover:bg-inverse-foreground/10">
            Our story
          </a>
        </div>
      </div>
    </section>
  );
}

function TrustStrip({ inStock }: { inStock: number }) {
  const trust = [
    { title: "Free returns", text: "Within 30 days", icon: "M4 12a8 8 0 1 0 2.3-5.7M4 4v4h4" },
    { title: "Secure checkout", text: "No account needed", icon: "M7 11V8a5 5 0 0 1 10 0v3M6 11h12v9H6z" },
    { title: "Small batches", text: "Shipped with care", icon: "M4 8l8-4 8 4v8l-8 4-8-4V8Zm0 0 8 4 8-4M12 12v8" },
    { title: `${inStock} in stock`, text: "Ready to ship", icon: "M5 12.5l4.5 4.5L19 7.5" },
  ];
  return (
    <div className="container-page relative z-10 -mt-14">
      <ul className="grid grid-cols-2 overflow-hidden rounded-[1.25rem] border border-border bg-card shadow-soft lg:grid-cols-4">
        {trust.map((t, i) => (
          <li
            key={t.title}
            className={["flex items-center gap-3 p-4 sm:gap-4 sm:p-6", i % 2 === 1 && "border-l border-border", i > 1 && "border-t border-border lg:border-t-0", i === 2 && "lg:border-l"]
              .filter(Boolean)
              .join(" ")}
          >
            <span className="grid size-10 shrink-0 place-items-center rounded-xl bg-accent/10 text-accent sm:size-11">
              <svg aria-hidden="true" viewBox="0 0 24 24" className="size-5" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                <path d={t.icon} />
              </svg>
            </span>
            <span className="min-w-0">
              <span className="block text-sm font-semibold">{t.title}</span>
              <span className="block text-xs text-muted-foreground">{t.text}</span>
            </span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function Story({ shop, count }: { shop: string; count: number }) {
  return (
    <section id="story" aria-labelledby="story-title" className="scroll-mt-28 bg-muted py-20 sm:py-28">
      <div className="container-page grid items-center gap-16 lg:grid-cols-2 lg:gap-20">
        <div className="relative">
          <img
            src={unsplash(photos.story, 960)}
            srcSet={srcset(photos.story)}
            sizes="(min-width: 1024px) 40vw, 90vw"
            alt="Knitwear, denim and a watch laid out on linen"
            width="960"
            height="1200"
            loading="lazy"
            decoding="async"
            className="aspect-4/5 w-full rounded-3xl bg-border object-cover"
          />
          <div className="absolute -bottom-6 right-4 rounded-[1.25rem] border border-border bg-card p-5 shadow-soft sm:-right-6">
            <p className="text-3xl font-semibold tracking-tight">{count}</p>
            <p className="mt-1 text-xs text-muted-foreground">pieces in the collection</p>
          </div>
        </div>
        <div>
          <SectionHeading eyebrow="Our story" title="Made slowly, meant to last" id="story-title">
            <p>
              {shop} started with a simple idea: buy fewer things, and buy them well. Every piece in the shop is one we'd use ourselves - chosen for the
              materials, the making and how it holds up after years of use.
            </p>
            <p className="mt-4">We keep the range small on purpose, ship in small batches and stand behind everything we sell.</p>
          </SectionHeading>
          <div className="mt-10 flex flex-wrap gap-3">
            <a href="#products" className="inline-flex min-h-12 items-center rounded-full bg-primary px-7 text-sm font-semibold tracking-wide text-primary-foreground transition-colors hover:bg-primary/85">
              Browse the shop
            </a>
            <a href="#promise" className="inline-flex min-h-12 items-center rounded-full border border-border bg-card px-7 text-sm font-semibold tracking-wide transition-colors hover:border-foreground">
              Our promise
            </a>
          </div>
        </div>
      </div>
    </section>
  );
}

function Band() {
  return (
    <section aria-labelledby="band-title" className="relative isolate overflow-hidden scheme-dark bg-inverse text-inverse-foreground">
      <img
        src={unsplash(photos.band, 1400)}
        srcSet={srcset(photos.band)}
        sizes="100vw"
        alt=""
        loading="lazy"
        decoding="async"
        className="absolute inset-0 -z-20 h-full w-full object-cover opacity-40"
      />
      <div aria-hidden="true" className="absolute inset-0 -z-10 bg-linear-to-r from-inverse via-inverse/80 to-inverse/30"></div>
      <div className="container-page py-24 sm:py-32">
        <SectionHeading eyebrow="New in" title="Find the piece you'll keep for years" id="band-title">
          <p className="text-inverse-foreground/75">Browse the full collection, pay securely without an account, and we'll ship it tracked to your door.</p>
        </SectionHeading>
        <a href="#products" className="mt-10 inline-flex min-h-12 items-center gap-2 rounded-full bg-accent px-7 text-sm font-semibold tracking-wide text-accent-foreground transition duration-200 hover:gap-3">
          Shop now <span aria-hidden="true">→</span>
        </a>
      </div>
    </section>
  );
}

function PromiseSection() {
  return (
    <section id="promise" aria-labelledby="promise-title" className="container-page grid scroll-mt-28 items-center gap-16 py-20 sm:py-28 lg:grid-cols-2 lg:gap-20">
      <div>
        <SectionHeading eyebrow="Our promise" title="Honest goods, fairly sold" id="promise-title">
          <p>No drops, no countdowns, no fake discounts - just good things at a fair price, and someone to talk to if anything goes wrong.</p>
        </SectionHeading>
        <ul className="mt-8 space-y-3">
          {promises.map((p) => (
            <li key={p} className="flex items-start gap-3 text-sm">
              <span className="mt-0.5 grid size-5 shrink-0 place-items-center rounded-full bg-accent/10 text-accent">
                <svg aria-hidden="true" viewBox="0 0 24 24" className="size-3" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M5 12.5l4.5 4.5L19 7.5" />
                </svg>
              </span>
              {p}
            </li>
          ))}
        </ul>
      </div>
      <img
        src={unsplash(photos.promise, 960)}
        srcSet={srcset(photos.promise)}
        sizes="(min-width: 1024px) 40vw, 90vw"
        alt="A rail of jackets, a blouse and denim"
        width="960"
        height="1200"
        loading="lazy"
        decoding="async"
        className="aspect-4/5 w-full rounded-3xl bg-muted object-cover lg:order-first"
      />
    </section>
  );
}
