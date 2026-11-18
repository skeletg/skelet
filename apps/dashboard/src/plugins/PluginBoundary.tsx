import { Component, type ErrorInfo, type ReactNode } from 'react'

// A plugin's section, slot or widget that throws stays its own failure: the page around it - and whatever the admin
// had typed into it - carries on, and a line says what's missing. Without it the route's own boundary takes the whole
// page, and on the home page the control that would hide the widget with it
export class PluginBoundary extends Component<{ id: string; children: ReactNode }, { failed: boolean }> {
  state = { failed: false }

  static getDerivedStateFromError() {
    return { failed: true }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error(`plugin UI "${this.props.id}" failed to render`, error, info.componentStack)
  }

  render() {
    if (!this.state.failed) return this.props.children
    return (
      <p role="alert" className="rounded-md border border-warning/40 bg-warning-soft px-3 py-2 text-xs text-muted-foreground">
        <span className="font-mono">{this.props.id}</span> couldn't be shown here.
      </p>
    )
  }
}

// A registered render function runs inside the boundary only as a component of its own - called inline it would
// throw in its host's render, above the boundary
export function PluginRender({ id, render }: { id: string; render: () => ReactNode }) {
  return (
    <PluginBoundary id={id}>
      <Rendered render={render} />
    </PluginBoundary>
  )
}

function Rendered({ render }: { render: () => ReactNode }) {
  return <>{render()}</>
}
