import {
  ProductGridSkeleton,
  Skeleton,
  SkeletonRegion,
} from "@/app/components/Skeleton";

// The fallback for every route under the locale without a loading.tsx of its own - shaped like the home page, which
// is the one route it is mostly seen on
export default function Loading() {
  return (
    <SkeletonRegion className="mx-auto w-full max-w-8xl px-6 pt-10 pb-14 md:px-10">
      <div className="max-w-2xl space-y-4">
        <Skeleton className="h-3 w-40" />
        <Skeleton className="h-14 w-full" />
        <Skeleton className="h-5 w-3/4" />
        <Skeleton className="h-11 w-44" />
      </div>
      <div className="mt-14">
        <ProductGridSkeleton count={4} />
      </div>
    </SkeletonRegion>
  );
}
