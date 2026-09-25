import { Link } from "@/app/components/LocaleLink";

// Global 404 fallback

export const metadata = {
  title: "Page not found",
};

export default function NotFound() {
  return (
    <main className="mx-auto flex min-h-[60vh] max-w-xl flex-col items-center justify-center gap-4 px-6 py-16 text-center">
      <p className="text-xs uppercase tracking-widest text-muted-foreground">
        404
      </p>
      <h1 className="text-3xl font-extrabold tracking-tight">
        Page not found
      </h1>
      <p className="text-sm text-muted-foreground">
        We couldn&rsquo;t find what you were looking for. Try one of the links
        below or head back to the shop.
      </p>
      <div className="mt-4 flex flex-wrap justify-center gap-3">
        <Link
          href="/"
          className="inline-flex h-10 items-center rounded-md bg-primary px-5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
        >
          Go home
        </Link>
        <Link
          href="/products"
          className="inline-flex h-10 items-center rounded-md border border-border px-5 text-sm font-medium hover:bg-accent"
        >
          Browse products
        </Link>
        <Link
          href="/blog"
          className="inline-flex h-10 items-center rounded-md border border-border px-5 text-sm font-medium hover:bg-accent"
        >
          Read the blog
        </Link>
      </div>
    </main>
  );
}
