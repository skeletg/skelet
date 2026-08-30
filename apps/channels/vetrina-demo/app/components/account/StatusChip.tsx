import type { ReactNode } from "react";

// One chip for every order/payment status the account area shows, so the
// list, the overview and the receipt can't drift apart

export function humanizeStatus(status: string) {
  return status.replace(/_/g, " ").toLowerCase();
}

export function StatusChip({
  children,
  tone = "muted",
}: {
  children: ReactNode;
  tone?: "muted" | "positive";
}) {
  return (
    <span
      className={`inline-block whitespace-nowrap rounded-sm px-2.5 py-0.5 text-[0.65rem] font-semibold uppercase tracking-wider ${
        tone === "positive"
          ? "bg-emerald-500/10 text-emerald-700"
          : "bg-muted text-muted-foreground"
      }`}
    >
      {children}
    </span>
  );
}

export function PaymentChip({ status }: { status: string }) {
  return (
    <StatusChip tone={status === "PAID" ? "positive" : "muted"}>
      {humanizeStatus(status)}
    </StatusChip>
  );
}
