import { useTranslations } from "next-intl";

import { cn } from "@/app/lib/cn";

// Loading placeholders for loading.tsx and <Suspense> fallbacks. Theme tokens only (bg-muted), so a recoloured store
// still gets matching skeletons, and the whole block announces itself once as busy rather than every bar separately

export function Skeleton({ className }: { className?: string }) {
  return <div aria-hidden="true" className={cn("animate-pulse rounded-md bg-muted", className)} />;
}

// The busy region every skeleton sits in - screen readers hear "Loading" once, the bars stay silent
export function SkeletonRegion({
  className,
  children,
}: {
  className?: string;
  children: React.ReactNode;
}) {
  const t = useTranslations("common");
  return (
    <div role="status" aria-busy="true" className={className}>
      <span className="sr-only">{t("loading")}</span>
      {children}
    </div>
  );
}

export function PageHeaderSkeleton() {
  return (
    <div className="space-y-2">
      <Skeleton className="h-3 w-24" />
      <Skeleton className="h-8 w-64" />
    </div>
  );
}

export function ProductCardSkeleton() {
  return (
    <div className="space-y-3">
      <Skeleton className="aspect-square w-full rounded-xl" />
      <Skeleton className="h-4 w-3/4" />
      <Skeleton className="h-4 w-1/3" />
    </div>
  );
}

export function ProductGridSkeleton({ count = 8 }: { count?: number }) {
  return (
    <div className="grid grid-cols-2 gap-6 sm:grid-cols-3 lg:grid-cols-4">
      {Array.from({ length: count }, (_, i) => (
        <ProductCardSkeleton key={i} />
      ))}
    </div>
  );
}

// A text-first page: header and a stack of rows (account pages, cart, blog index). contained=false drops the page
// padding for pages that render inside a layout which already provides it (the account area)
export function ContentSkeleton({ rows = 5, contained = true }: { rows?: number; contained?: boolean }) {
  return (
    <SkeletonRegion className={contained ? "mx-auto w-full max-w-3xl px-6 pt-6 pb-12" : undefined}>
      <PageHeaderSkeleton />
      <div className="mt-8 space-y-4">
        {Array.from({ length: rows }, (_, i) => (
          <Skeleton key={i} className="h-16 w-full rounded-xl" />
        ))}
      </div>
    </SkeletonRegion>
  );
}
