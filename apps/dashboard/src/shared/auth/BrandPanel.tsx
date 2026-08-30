import { Trans } from '@lingui/react/macro'
import { lazy, Suspense, useEffect, useRef, type RefObject } from 'react'
import { cn } from '@/lib/utils'
import { AuroraBackground } from '@/components/ui/aurora-background'

// This panel sits on a PUBLIC, pre-auth page - everything here is decorative or brand, with no
// counts, module names, version or network calls - Keep it that way

// tsParticles only ever renders here and on the page backdrop - lazy so it
// stays out of the entry chunk that every signed-in user downloads
const PanelField = lazy(() => import('./PanelField'))

function prefersReducedMotion() {
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

/** Circular cursor replacing the native one - driven by direct style writes on a ref, since a
 *  pointermove re-render every frame is pure waste and this costs one composited transform
 *  It trails slightly behind, which is what makes it read as an object rather than a cursor */
function PanelCursor({ hostRef }: { hostRef: RefObject<HTMLDivElement | null> }) {
  const ringRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    const host = hostRef.current
    const ring = ringRef.current
    if (!host || !ring) return

    const move = (e: PointerEvent) => {
      const box = host.getBoundingClientRect()
      ring.style.transform = `translate3d(${e.clientX - box.left}px, ${e.clientY - box.top}px, 0)`
    }
    const show = () => {
      ring.style.opacity = '1'
    }
    const hide = () => {
      ring.style.opacity = '0'
      ring.classList.remove('is-down')
    }
    const press = () => ring.classList.add('is-down')
    const release = () => ring.classList.remove('is-down')

    host.addEventListener('pointermove', move)
    host.addEventListener('pointerenter', show)
    host.addEventListener('pointerleave', hide)
    host.addEventListener('pointerdown', press)
    // on window, so releasing outside the panel still resets the ring
    window.addEventListener('pointerup', release)

    return () => {
      host.removeEventListener('pointermove', move)
      host.removeEventListener('pointerenter', show)
      host.removeEventListener('pointerleave', hide)
      host.removeEventListener('pointerdown', press)
      window.removeEventListener('pointerup', release)
    }
  }, [hostRef])

  return (
    <div ref={ringRef} aria-hidden className="skelet-cursor">
      <span className="skelet-cursor__ring" />
    </div>
  )
}

/** The greeting, one <span> per letter so each glyph lands on its own beat - Geist is variable,
 *  so the weight axis animates and the word arrives light, overshoots heavy, then settles */
function GlyphWord({ word }: { word: string }) {
  return (
    <h2
      role="img"
      aria-label={word}
      className="inline-flex font-sans text-5xl tracking-tight text-white xl:text-6xl"
    >
      {[...word].map((ch, i) => (
        <span
          key={`${ch}-${i}`}
          aria-hidden
          className="skelet-glyph"
          style={{ animationDelay: `${i * 45}ms` }}
        >
          {ch}
        </span>
      ))}
    </h2>
  )
}

/** The right half of the auth split - Decorative only, by design */
export function BrandPanel({ className }: { className?: string }) {
  const hostRef = useRef<HTMLDivElement | null>(null)
  const interactive = !prefersReducedMotion()

  return (
    <div
      ref={hostRef}
      className={cn(
        'relative flex-col justify-center overflow-hidden bg-neutral-900 p-12 xl:p-16',
        // The native cursor is hidden only when the ring is there to replace it
        interactive ? 'cursor-none' : 'cursor-crosshair',
        className,
      )}
    >
      <AuroraBackground className="absolute inset-0 opacity-40" />
      {interactive && (
        <>
          <Suspense fallback={null}>
            <PanelField />
          </Suspense>
          <PanelCursor hostRef={hostRef} />
        </>
      )}

      <div className="relative z-10">
        {/* lands once on mount - re-forming the same word on a loop is motion
            without information, and it sits right next to a password field */}
        <GlyphWord word="Welcome" />
        <p className="mt-5 max-w-sm text-base leading-relaxed text-neutral-400">
          <Trans>Sign in to your store to pick up where you left off.</Trans>
        </p>
      </div>

      <style>{`
        .skelet-cursor {
          position: absolute;
          top: 0;
          left: 0;
          z-index: 20;
          opacity: 0;
          pointer-events: none;
          will-change: transform;
          transition: transform 110ms cubic-bezier(0.22, 1, 0.36, 1), opacity 180ms ease;
        }
        .skelet-cursor__ring {
          display: block;
          width: 30px;
          height: 30px;
          margin: -15px 0 0 -15px;
          border-radius: 9999px;
          border: 1px solid rgba(103, 232, 249, 0.7);
          background: rgba(103, 232, 249, 0.06);
          transition: transform 140ms ease, border-color 140ms ease, background-color 140ms ease;
        }
        .skelet-cursor.is-down .skelet-cursor__ring {
          transform: scale(0.7);
          border-color: rgba(165, 243, 252, 1);
          background: rgba(103, 232, 249, 0.16);
        }
        .skelet-glyph {
          display: inline-block;
          animation: skelet-glyph 900ms cubic-bezier(0.2, 0.8, 0.2, 1) both;
        }
        @keyframes skelet-glyph {
          0%   { opacity: 0; transform: translateY(0.4em); filter: blur(8px); font-variation-settings: 'wght' 150; }
          55%  { opacity: 1; filter: blur(0); font-variation-settings: 'wght' 850; }
          100% { opacity: 1; transform: translateY(0); filter: blur(0); font-variation-settings: 'wght' 560; }
        }
        @media (prefers-reduced-motion: reduce) {
          .skelet-glyph { animation: none; font-variation-settings: 'wght' 560; }
        }
      `}</style>
    </div>
  )
}
