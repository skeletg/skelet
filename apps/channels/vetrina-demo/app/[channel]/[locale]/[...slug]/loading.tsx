import { ProductDetailSkeleton } from "@/app/components/Skeleton";

// The catch-all serves products, categories, collections and CMS pages from one route, and the kind is only known
// after the URL resolves - product detail is the common case, so it gets the matching shape
export default function Loading() {
  return <ProductDetailSkeleton />;
}
