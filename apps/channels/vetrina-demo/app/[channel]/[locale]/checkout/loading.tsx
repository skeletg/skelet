// The checkout's shape while the cart, its quotes and its totals are read - three step cards and the summary, so the
// page doesn't jump when they arrive
export default function CheckoutLoading() {
  return (
    <main
      className="mx-auto grid w-full max-w-8xl gap-10 px-4 pt-6 pb-12 sm:px-6 md:grid-cols-[1fr_22rem] md:px-10"
      aria-busy="true"
    >
      <div className="animate-pulse">
        <div className="h-9 w-40 rounded-md bg-muted" />
        <div className="mt-3 h-4 w-64 rounded bg-muted" />
        <div className="mt-8 grid gap-4">
          <div className="glass h-48 rounded-xl" />
          <div className="glass h-16 rounded-xl" />
          <div className="glass h-16 rounded-xl" />
        </div>
      </div>
      <div className="glass hidden h-80 animate-pulse self-start rounded-xl md:block" />
    </main>
  );
}
