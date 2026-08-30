import { Component, type ErrorInfo, type ReactNode } from 'react'

// Stops one page's render error from blanking the whole app, since React unmounts the entire tree on an uncaught render error with no
// boundary - shows the message and stack instead of a white screen, and is keyed by route path so navigating away resets it
type Props = { children: ReactNode }
type State = { error: Error | null }

export class RouteErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('[route error]', error, info.componentStack)
  }

  render() {
    const { error } = this.state
    if (!error) return this.props.children
    return (
      <div className="mx-auto max-w-2xl p-8">
        <h1 className="text-lg font-semibold text-destructive">This page hit an error</h1>
        <p className="mt-1 text-sm text-muted-foreground">{error.message}</p>
        {error.stack ? (
          <pre className="mt-3 max-h-80 overflow-auto rounded-lg border bg-muted p-3 text-xs">{error.stack}</pre>
        ) : null}
        <button
          type="button"
          onClick={() => this.setState({ error: null })}
          className="mt-4 rounded-md border px-3 py-1.5 text-sm font-medium hover:bg-muted"
        >
          Try again
        </button>
      </div>
    )
  }
}
