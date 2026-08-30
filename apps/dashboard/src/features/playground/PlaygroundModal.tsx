import {
  createContext,
  lazy,
  Suspense,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import { Dialog, DialogContent, DialogTitle } from '@/components/ui/dialog'
import { setNavAction } from '@/nav/actions'
import { usePermissions } from '@/shared/auth/usePermissions'

// GraphiQL drags in Monaco and three web workers - lazy so only an admin who actually opens it pays for them
const Playground = lazy(() =>
  import('./Playground').then((m) => ({ default: m.Playground })),
)

type PlaygroundCtx = { open: boolean; setOpen: (open: boolean) => void; toggle: () => void }
const Ctx = createContext<PlaygroundCtx | null>(null)

export function PlaygroundProvider({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false)
  const value = useMemo<PlaygroundCtx>(
    () => ({ open, setOpen, toggle: () => setOpen((o) => !o) }),
    [open],
  )
  // The sidebar renders outside this provider and its leaves register at module load, so the nav item reaches the modal through the action registry
  useEffect(() => setNavAction('playground', () => setOpen(true)), [])
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

function usePlayground(): PlaygroundCtx {
  const c = useContext(Ctx)
  if (!c) throw new Error('usePlayground must be used within PlaygroundProvider')
  return c
}

function useCanUsePlayground(): boolean {
  const { has, loading, isSuperuser } = usePermissions()
  return !loading && (isSuperuser || has('developer.playground'))
}

export function PlaygroundModal() {
  const allowed = useCanUsePlayground()
  const { open, setOpen, toggle } = usePlayground()

  // Cmd/Ctrl+B is the sidebar, so this takes Shift too - ignored while typing, or it fires inside the query editor
  useEffect(() => {
    if (!allowed) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key.toLowerCase() !== 'g' || !e.shiftKey || !(e.metaKey || e.ctrlKey)) return
      const el = document.activeElement
      const typing =
        el instanceof HTMLElement &&
        (el.isContentEditable || ['INPUT', 'TEXTAREA'].includes(el.tagName))
      if (typing) return
      e.preventDefault()
      toggle()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [allowed, toggle])

  if (!allowed) return null
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent
        className="flex h-[92vh] w-[96vw] max-w-[96vw] flex-col gap-3 p-4 sm:max-w-[96vw]"
        // GraphiQL owns its own focus and shortcuts - autofocusing the dialog steals the first keystroke
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <DialogTitle className="text-sm font-medium">GraphQL playground</DialogTitle>
        <div className="min-h-0 flex-1">
          <Suspense
            fallback={<p className="text-sm text-muted-foreground">Loading playground…</p>}
          >
            <Playground />
          </Suspense>
        </div>
      </DialogContent>
    </Dialog>
  )
}
