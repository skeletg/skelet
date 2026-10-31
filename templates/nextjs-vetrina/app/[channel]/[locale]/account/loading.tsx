import { ContentSkeleton } from "@/app/components/Skeleton";

// Rendered inside the account layout, which already draws the nav and page padding
export default function Loading() {
  return <ContentSkeleton contained={false} />;
}
