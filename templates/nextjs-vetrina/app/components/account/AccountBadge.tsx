export function AccountBadge({ count, label, newLabel }: { count: number; label: string; newLabel?: string }) {
  if (count === 0) return null;
  return (
    <span aria-label={newLabel ? `${label}, ${newLabel}` : label} className="relative ml-auto inline-flex min-w-5 shrink-0 items-center justify-center rounded-full bg-foreground px-1.5 text-[11px] font-semibold text-background ring-1 ring-background">
      <span aria-hidden="true">{count}</span>
      {newLabel ? <span aria-hidden="true" className="absolute -right-0.5 -top-0.5 size-2 rounded-full bg-primary ring-2 ring-background" /> : null}
    </span>
  );
}
