import { Particles, ParticlesProvider } from '@tsparticles/react'
import { loadSlim } from '@tsparticles/slim'
import type { ISourceOptions } from '@tsparticles/engine'

// Deliberately NOT the panel's linked lattice - this drops links and drifts unconnected dust, since one config in both places read as a duplicate
const BACKDROP: ISourceOptions = {
  fullScreen: { enable: false },
  fpsLimit: 60,
  detectRetina: true,
  particles: {
    number: { value: 110, density: { enable: true, width: 1600, height: 900 } },
    color: { value: '#cbd5e1' },
    links: { enable: false },
    move: {
      enable: true,
      speed: 0.16,
      direction: 'top',
      straight: false,
      random: true,
      outModes: { default: 'out' },
    },
    opacity: {
      value: { min: 0.05, max: 0.3 },
      animation: { enable: true, speed: 0.25, sync: false },
    },
    size: { value: { min: 0.6, max: 2.2 } },
  },
  interactivity: { events: { onHover: { enable: false }, onClick: { enable: false } } },
}

/** Rendered only when the visitor hasn't asked for reduced motion - dropping purely decorative motion beats freezing a half-drawn frame */
export default function AuthBackdrop() {
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return null

  return (
    <div aria-hidden className="pointer-events-none fixed inset-0 overflow-hidden">
      <ParticlesProvider init={loadSlim}>
        <Particles id="auth-backdrop" options={BACKDROP} className="size-full" />
      </ParticlesProvider>
    </div>
  )
}

