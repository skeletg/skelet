import { lazy, Suspense, type ReactNode } from 'react'
import { Outlet, useLocation } from 'react-router-dom'
import { BoxIcon } from 'lucide-react'
import { BrandPanel } from './BrandPanel'

// tsParticles is heavy and only ever renders on these public pages - keep it
// out of the entry chunk so signed-in users never pay for it
const AuthBackdrop = lazy(() => import('./AuthBackdrop'))
/** Split shell for the public auth pages - the form sits on a fixed light surface since the app theme doesn't apply before sign-in, and the brand panel drops below lg
 *  A layout route, so moving between /login and /reset-password swaps only the outlet and the panel keeps animating rather than restarting */
export function AuthShell() {
  const { pathname } = useLocation()

  return (
    <div className="relative flex min-h-screen items-center justify-center bg-neutral-950 p-4 sm:p-6 lg:p-8">
      <Suspense fallback={null}>
        <AuthBackdrop />
      </Suspense>

      <div className="relative z-10 grid w-full max-w-6xl overflow-hidden rounded-2xl bg-white shadow-2xl ring-1 ring-white/10 animate-in fade-in-0 slide-in-from-bottom-2 duration-700 lg:min-h-[42rem] lg:grid-cols-[23rem_1fr]">
        <div className="flex flex-col gap-10 px-8 py-12 text-neutral-900 sm:px-10">
          <div className="flex items-center gap-2.5">
            <div className="flex size-7 items-center justify-center rounded-md bg-neutral-900 text-white">
              <BoxIcon className="size-4" />
            </div>
            <span className="text-sm font-semibold tracking-tight">Skelet</span>
          </div>

          <div
            key={pathname}
            className="auth-outlet flex flex-1 flex-col justify-center animate-in fade-in-0 slide-in-from-bottom-1 duration-300"
          >
            <Outlet />
          </div>
        </div>

        <div className="hidden p-3 lg:block">
          <BrandPanel className="flex size-full rounded-xl" />
        </div>
      </div>

      <style>{`
        @media (prefers-reduced-motion: reduce) {
          .auth-outlet { animation: none; }
        }
      `}</style>
    </div>
  )
}

export function AuthHeader({ title, subtitle }: { title: string; subtitle?: ReactNode }) {
  return (
    <div>
      <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
      {subtitle && <p className="mt-2 text-sm text-neutral-500">{subtitle}</p>}
    </div>
  )
}
