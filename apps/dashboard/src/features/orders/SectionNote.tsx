import { InfoIcon } from 'lucide-react'

// Why a section has nothing to offer yet - without it they just disappear, leaving people hunting for a block that was never going to render
export function SectionNote({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex items-start gap-2 rounded-md border border-dashed bg-muted/30 px-3 py-2 text-sm text-muted-foreground">
      <InfoIcon className="mt-0.5 size-4 shrink-0" aria-hidden />
      <span>{children}</span>
    </div>
  )
}
