import type { ReactNode } from "react";

import { AccountNav } from "@/app/components/account/AccountNav";
import { SubmitButton } from "@/app/components/account/SubmitButton";
import { getCurrentCustomer } from "@/app/lib/customer/read";
import { logoutAction } from "@/app/lib/auth/actions";

export default async function AccountLayout({
  children,
}: {
  children: ReactNode;
}) {
  const customer = await getCurrentCustomer();

  if (!customer) {
    return (
      <main className="mx-auto w-full max-w-md px-6 pt-10 pb-20">
        {children}
      </main>
    );
  }
  const initials =
    `${customer.firstName.charAt(0)}${customer.lastName.charAt(0)}`.toUpperCase();

  return (
    <main className="mx-auto w-full max-w-8xl px-6 pt-6 pb-16 md:px-10">
      {/* Mobile nav scrolls horizontally so content doesn't start below eleven stacked links */}
      <div className="lg:hidden">
        <div className="flex items-center justify-between gap-3 pb-4">
          <div className="flex min-w-0 items-center gap-3">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-foreground text-xs font-semibold text-background">
              {initials}
            </span>
            <p className="truncate text-sm font-semibold">
              {customer.firstName} {customer.lastName}
            </p>
          </div>
          <form action={logoutAction}>
            <SubmitButton
              className="shrink-0 text-sm text-muted-foreground hover:text-foreground"
              pendingLabel="Signing out…"
            >
              Sign out
            </SubmitButton>
          </form>
        </div>
        <AccountNav variant="strip" />
      </div>

      <div className="lg:grid lg:grid-cols-[16rem_1fr] lg:gap-10">
        <aside className="hidden lg:sticky lg:top-24 lg:block lg:self-start">
          <div className="glass rounded-xl p-4">
            <div className="flex items-center gap-3 px-1 pb-4">
              <span className="flex size-10 shrink-0 items-center justify-center rounded-full bg-foreground text-sm font-semibold text-background">
                {initials}
              </span>
              <div className="min-w-0">
                <p className="truncate text-sm font-semibold">
                  {customer.firstName} {customer.lastName}
                </p>
                <p className="truncate text-xs text-muted-foreground">
                  {customer.email}
                </p>
              </div>
            </div>
            <AccountNav />
            <form action={logoutAction} className="mt-6 border-t border-border pt-3">
              <SubmitButton
                className="w-full rounded-lg px-3 py-2 text-left text-sm text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
                pendingLabel="Signing out…"
              >
                Sign out
              </SubmitButton>
            </form>
          </div>
        </aside>

        <div className="mt-8 min-w-0 lg:mt-0">{children}</div>
      </div>
    </main>
  );
}
