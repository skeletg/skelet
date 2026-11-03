import { StarIcon } from "lucide-react";

export function RatingStars({
  value,
  count,
  className = "",
}: {
  value: number;
  count?: number;
  className?: string;
}) {
  const pct = Math.max(0, Math.min(100, (value / 5) * 100));
  return (
    <span className={`inline-flex items-center gap-1.5 ${className}`}>
      <span className="relative inline-flex" aria-hidden="true">
        <span className="flex text-border">
          {Array.from({ length: 5 }, (_, i) => (
            <StarIcon key={i} className="h-3.5 w-3.5 shrink-0 fill-current" strokeWidth={0} />
          ))}
        </span>
        <span
          className="absolute inset-y-0 left-0 flex overflow-hidden text-amber-400"
          style={{ width: `${pct}%` }}
        >
          {Array.from({ length: 5 }, (_, i) => (
            <StarIcon key={i} className="h-3.5 w-3.5 shrink-0 fill-current" strokeWidth={0} />
          ))}
        </span>
      </span>
      <span className="sr-only">{value.toFixed(1)} out of 5 stars</span>
      {count != null && (
        <span className="text-xs text-muted-foreground">({count})</span>
      )}
    </span>
  );
}
