import type { ReactNode } from 'react'
import { Note, YourSections } from './welcome-step-parts'

// The first-run tour. Each step highlights the real thing on screen and says two sentences about
// it - pointing beats describing, and pointing at the LIVE element means the copy can't drift from
// the UI the way a screenshot or a drawing would
//
// `target` is a `data-tour` attribute rather than a class or structural selector, so restyling the
// shell can't quietly break the tour. A step whose target isn't on screen still shows, centered
//
// Gated like the sidebar is, so nobody is pointed at something they can't open. The list is pure
// data so the gating can be tested without mounting anything

export type WelcomeStep = {
  id: string
  title: string
  target?: string
  permission?: string
  feature?: string
  Body: () => ReactNode
}

export const WELCOME_STEPS: WelcomeStep[] = [
  {
    id: 'nav',
    title: 'This is the map',
    target: '[data-tour="nav"]',
    Body: YourSections,
  },
  {
    id: 'scope',
    title: 'Check this before you save',
    target: '[data-tour="scope"]',
    Body: () => (
      <div className="grid gap-2">
        <Note>
          This is not just the logo. Click it and you get the scope, channel and locale pickers.
        </Note>
        <Note>
          On <span className="font-medium text-foreground">Global</span> you're editing the value
          every storefront inherits. Pick a channel and you're editing that one storefront instead -
          its prices, its content, its settings. Same screen, different meaning.
        </Note>
      </div>
    ),
  },
  {
    id: 'tabs',
    title: 'Tabs, like a browser',
    target: '[data-tour="tabs"]',
    Body: () => (
      <div className="grid gap-2">
        <Note>
          Every page you open lands in the tab you're on. The + opens another one.
        </Note>
        <Note>
          Switching tabs keeps each page exactly as you left it, filters and scroll included, so you
          can park a half-finished order and come back to it.
        </Note>
      </div>
    ),
  },
  {
    id: 'search',
    title: 'The fastest way anywhere',
    target: '[data-tour="search"]',
    Body: () => (
      <div className="grid gap-2">
        <Note>
          Press <kbd className="rounded border bg-muted px-1.5 font-mono text-xs">⌘K</kbd> (or{' '}
          <kbd className="rounded border bg-muted px-1.5 font-mono text-xs">Ctrl K</kbd>) and type.
          It searches every page and every settings section by name.
        </Note>
        <Note>
          Quicker than remembering which menu something was under. Hovering a sidebar row and
          clicking its star pins the ones you use daily.
        </Note>
      </div>
    ),
  },
  {
    id: 'bell',
    title: 'What needs you',
    target: '[data-tour="bell"]',
    permission: 'notifications.view',
    Body: () => (
      <div className="grid gap-2">
        <Note>
          Failing jobs, stock running out, reviews waiting on a decision. The count is what you
          haven't read.
        </Note>
        <Note>
          You can mute whole categories in Preferences if a kind of notice isn't your job.
        </Note>
      </div>
    ),
  },
  {
    id: 'yours',
    title: 'Make it yours',
    target: '[data-tour="user"]',
    Body: () => (
      <div className="grid gap-2">
        <Note>
          Preferences lives in this menu. Light or dark, a colour palette, corner roundness, text
          size, and where you land when you sign in.
        </Note>
        <Note>
          Your dashboard is a library too - Customize on it turns sections on and off. All of it
          follows you to any device you sign in on.
        </Note>
      </div>
    ),
  },
]
