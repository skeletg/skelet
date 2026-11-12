import type { ComponentProps, ReactNode } from 'react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { FLOATING_PANEL, PANEL_TITLE } from './workbench-styles'

// The email editor's layout, for any canvas page: WORKBENCH on DetailPageLayout, and these as its sidebars

// Icons only while the editor is narrow, so the canvas keeps its width, and labelled tiles once it's wide
export function Rail({ label, title, hint, className, children, ...props }: { label: string; title: string; hint: string } & ComponentProps<'aside'>) {
  return (
    <aside {...props} aria-label={label} className={cn(FLOATING_PANEL, 'mr-0 w-16 @min-[1200px]/editor:w-56', className)}>
      <div className="hidden border-b px-4 py-3.5 @min-[1200px]/editor:block">
        <h2 className={PANEL_TITLE}>{title}</h2>
        <p className="mt-0.5 text-xs text-muted-foreground">{hint}</p>
      </div>
      <div className="flex-1 space-y-2 overflow-y-auto px-2 py-3 @min-[1200px]/editor:space-y-3 @min-[1200px]/editor:p-3">{children}</div>
    </aside>
  )
}

export function RailGroup({ label, note, first, children }: { label: string; note?: string; first?: boolean; children: ReactNode }) {
  return (
    <div className="space-y-1.5">
      <div className="hidden px-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground/70 @min-[1200px]/editor:block">
        {label}
        {note ? <span className="font-normal normal-case tracking-normal"> · {note}</span> : null}
      </div>
      {first ? null : <div className="mx-auto h-px w-8 bg-border @min-[1200px]/editor:hidden" aria-hidden />}
      <div className="grid grid-cols-1 justify-items-center gap-1 @min-[1200px]/editor:grid-cols-2 @min-[1200px]/editor:justify-items-stretch @min-[1200px]/editor:gap-2">
        {children}
      </div>
    </div>
  )
}

export function RailItem({ label, hint, icon, dragging, className, ...props }: { label: string; hint: string; icon: ReactNode; dragging?: boolean } & ComponentProps<'div'>) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div
          {...props}
          role="button"
          tabIndex={0}
          className={cn(
            'group flex cursor-grab touch-none select-none items-center justify-center rounded-md transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring active:cursor-grabbing',
            '@min-[1200px]/editor:flex-col @min-[1200px]/editor:gap-1.5 @min-[1200px]/editor:rounded-lg @min-[1200px]/editor:border @min-[1200px]/editor:bg-background @min-[1200px]/editor:p-2.5 @min-[1200px]/editor:text-center @min-[1200px]/editor:hover:-translate-y-0.5 @min-[1200px]/editor:hover:border-primary/40 @min-[1200px]/editor:hover:shadow-sm',
            dragging && 'opacity-40',
            className,
          )}
        >
          <span className="grid size-9 place-items-center rounded-md bg-muted text-muted-foreground transition-colors group-hover:bg-primary/10 group-hover:text-primary">{icon}</span>
          <span className="sr-only text-[11px] font-medium leading-tight @min-[1200px]/editor:not-sr-only">{label}</span>
        </div>
      </TooltipTrigger>
      <TooltipContent side="right" className="max-w-56">
        <p className="font-medium">{label}</p>
        <p className="opacity-80">{hint}</p>
      </TooltipContent>
    </Tooltip>
  )
}

export function SidePanel({ footer, className, children }: { footer?: ReactNode; className?: string; children: ReactNode }) {
  return (
    <aside className={cn(FLOATING_PANEL, 'ml-0 w-72 @min-[1200px]/editor:w-80', className)}>
      {children}
      {footer && <div className="shrink-0 border-t p-3">{footer}</div>}
    </aside>
  )
}

const COLUMNS: Record<number, string> = { 1: 'grid-cols-1', 2: 'grid-cols-2', 3: 'grid-cols-3', 4: 'grid-cols-4' }

export function PanelTabs<T extends string>({ value, onChange, tabs }: { value: T; onChange: (v: T) => void; tabs: Array<{ value: T; label: string; content: ReactNode }> }) {
  return (
    <Tabs value={value} onValueChange={(v) => onChange(v as T)} className="flex min-h-0 flex-1 flex-col">
      <div className="border-b px-3 py-2">
        <TabsList className={cn('grid h-9 w-full', COLUMNS[tabs.length])}>
          {tabs.map((t) => (
            <TabsTrigger key={t.value} value={t.value} className="text-xs">
              {t.label}
            </TabsTrigger>
          ))}
        </TabsList>
      </div>
      {tabs.map((t) => (
        <TabsContent key={t.value} value={t.value} className="mt-0 min-h-0 flex-1 space-y-5 overflow-y-auto p-4">
          {t.content}
        </TabsContent>
      ))}
    </Tabs>
  )
}

export function PanelHeading({ children }: { children: ReactNode }) {
  return (
    <div className="border-b px-4 py-3.5">
      <h2 className={PANEL_TITLE}>{children}</h2>
    </div>
  )
}

export function PanelTitle({ icon, title, hint }: { icon: ReactNode; title: string; hint?: string }) {
  return (
    <div className="flex items-start gap-2.5">
      <span className="grid size-8 shrink-0 place-items-center rounded-md bg-muted text-muted-foreground">{icon}</span>
      <div className="min-w-0">
        <p className="text-sm font-medium">{title}</p>
        {hint ? <p className="text-xs text-muted-foreground">{hint}</p> : null}
      </div>
    </div>
  )
}

export function ToolRow({ className, children, ...props }: ComponentProps<'div'>) {
  return (
    <div {...props} className={cn('flex items-center justify-center gap-2', className)}>
      {children}
    </div>
  )
}

export function ToolGroup({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div role="group" aria-label={label} className="inline-flex items-center gap-0.5 rounded-md border bg-background p-0.5">
      {children}
    </div>
  )
}

export function ToolButton({ label, className, children, ...props }: { label: string } & ComponentProps<'button'>) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      {...props}
      className={cn('inline-flex size-7 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground', className)}
    >
      {children}
    </button>
  )
}

export function ToolPill({ label, onClick, children }: { label: string; onClick?: () => void; children: ReactNode }) {
  const body = (
    <>
      <span className="text-muted-foreground">{label}</span>
      <span className="tabular-nums text-foreground">{children}</span>
    </>
  )
  const look = 'flex h-8 items-center gap-2 rounded-md border bg-background px-2.5 text-xs'
  if (!onClick) return <div className={look}>{body}</div>
  return (
    <button type="button" onClick={onClick} className={cn(look, 'transition-colors hover:bg-muted')}>
      {body}
    </button>
  )
}
