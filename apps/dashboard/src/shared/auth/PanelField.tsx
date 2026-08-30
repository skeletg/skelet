import { Particles, ParticlesProvider } from '@tsparticles/react'
import { loadSlim } from '@tsparticles/slim'
import type { ISourceOptions } from '@tsparticles/engine'

// Interactive lattice behind the greeting - same engine as the page backdrop, but this one answers the cursor without moving fast enough to distract
const FIELD: ISourceOptions = {
  fullScreen: { enable: false },
  fpsLimit: 60,
  detectRetina: true,
  particles: {
    number: { value: 70, density: { enable: true, width: 1000, height: 800 } },
    color: { value: '#a5f3fc' },
    links: { enable: true, color: '#94e2f5', distance: 130, opacity: 0.2, width: 1 },
    move: { enable: true, speed: 0.5, outModes: { default: 'out' } },
    opacity: { value: 0.45 },
    size: { value: { min: 1, max: 2.2 } },
  },
  interactivity: {
    // Scoped to this canvas - the pointer sits over the form half most of the time, and a window listener would tug the lattice from there
    detectsOn: 'canvas',
    events: {
      onHover: { enable: true, mode: 'grab' },
      onClick: { enable: true, mode: 'push' },
    },
    modes: {
      grab: { distance: 170, links: { opacity: 0.55, color: '#67e8f9' } },
      push: { quantity: 2 },
    },
  },
}

export default function PanelField() {
  return (
    <ParticlesProvider init={loadSlim}>
      <Particles id="auth-panel-field" options={FIELD} className="absolute inset-0 size-full" />
    </ParticlesProvider>
  )
}
