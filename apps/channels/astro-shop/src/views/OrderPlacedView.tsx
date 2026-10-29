import { ButtonLink } from "../components/ui";

export function OrderPlacedView({ number }: { number?: string }) {
  return (
    <section className="mx-auto grid max-w-lg justify-items-center gap-3 py-16 text-center">
      <span aria-hidden="true" className="grid size-14 place-items-center rounded-full bg-success/15 text-2xl text-success" data-reveal="">
        ✓
      </span>
      <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">Thank you for your order</h1>
      {number && (
        <p className="text-lg">
          Your order number is <strong data-order-number="">{number}</strong>.
        </p>
      )}
      <p className="text-muted-foreground">We've emailed you a confirmation with the details.</p>
      <ButtonLink href="/" className="mt-6">
        Back to the shop
      </ButtonLink>
    </section>
  );
}
