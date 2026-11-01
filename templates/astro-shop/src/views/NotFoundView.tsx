import { ButtonLink } from "../components/ui";

export function NotFoundView() {
  return (
    <section className="mx-auto max-w-md py-16 text-center">
      <p className="text-sm font-semibold text-accent">404</p>
      <h1 className="mt-2 text-3xl font-semibold tracking-tight">We couldn't find that page</h1>
      <p className="mt-3 text-muted-foreground">It may have moved, or it's no longer for sale.</p>
      <ButtonLink href="/" className="mt-8">
        Back to the shop
      </ButtonLink>
    </section>
  );
}
