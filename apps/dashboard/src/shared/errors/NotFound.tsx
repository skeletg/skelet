import { lazy, Suspense, useEffect, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { ArrowLeftIcon, HomeIcon, SkullIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'

// Lazy so the easter egg never weighs down the eager bundle NotFound ships in
const SkeletonRun = lazy(() =>
  import('@/features/boneyard/SkeletonRun').then((m) => ({ default: m.SkeletonRun })),
)

export function NotFound() {
  const navigate = useNavigate()
  const location = useLocation()
  const [playing, setPlaying] = useState(false)

  useEffect(() => {
    if (playing) return
    const onKey = (e: KeyboardEvent) => {
      if (e.code !== 'Space' || e.repeat) return
      const t = e.target
      if (
        t instanceof HTMLElement &&
        (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'BUTTON' || t.tagName === 'A' || t.isContentEditable)
      ) {
        return
      }
      e.preventDefault()
      setPlaying(true)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [playing])

  return (
    <div className="relative flex min-h-[70vh] flex-col items-center justify-center gap-5 overflow-hidden px-6 text-center">
      {playing ? (
        <div className="relative z-10 w-full max-w-[600px]">
          <Suspense fallback={null}>
            <SkeletonRun autoStart />
          </Suspense>
        </div>
      ) : (
        <>
          <button
            type="button"
            aria-label="Shh"
            onClick={() => setPlaying(true)}
            className="group relative z-10 grid size-16 cursor-pointer place-items-center rounded-full bg-background ring-1 ring-border shadow-sm transition-transform hover:-translate-y-1"
          >
            <SkullIcon className="size-8 text-muted-foreground transition-colors group-hover:text-foreground" />
          </button>
          <div className="relative z-10 grid gap-1.5">
            <div className="font-mono text-xs font-medium uppercase tracking-[0.2em] text-muted-foreground">
              404
            </div>
            <h1 className="text-2xl font-bold tracking-tight">Page not found</h1>
            <p className="max-w-md text-sm text-muted-foreground">
              The page you&apos;re looking for doesn&apos;t exist or may have been moved.
            </p>
            <p className="mt-1 font-mono text-xs text-muted-foreground/80">
              {location.pathname}
            </p>
          </div>
        </>
      )}
      <div className="relative z-10 flex flex-wrap items-center justify-center gap-2">
        <Button variant="outline" onClick={() => navigate(-1)}>
          <ArrowLeftIcon className="size-4" />
          Go back
        </Button>
        <Button asChild>
          <Link to="/">
            <HomeIcon className="size-4" />
            Dashboard
          </Link>
        </Button>
      </div>
    </div>
  )
}
