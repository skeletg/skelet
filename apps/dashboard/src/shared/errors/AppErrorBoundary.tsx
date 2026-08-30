import { Component, type ErrorInfo, type ReactNode } from 'react'

// The last resort, wrapping the providers and shell chrome that RouteErrorBoundary's per-tab coverage misses -
// a throw out there blanks #root with the reason only in the console

// Inline styles and no imports beyond React: a boundary that depends on the app's CSS or providers can fail to
// render the very error it exists to report
type Props = { children: ReactNode }
type State = { error: Error | null }

export class AppErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('[app error]', error, info.componentStack)
  }

  render() {
    const { error } = this.state
    if (!error) return this.props.children
    return (
      <div style={{ maxWidth: 720, margin: '0 auto', padding: 24, fontFamily: 'system-ui, sans-serif' }}>
        <h1 style={{ fontSize: 18, fontWeight: 600, color: '#b91c1c', margin: 0 }}>
          The dashboard failed to start
        </h1>
        <p style={{ fontSize: 14, lineHeight: 1.5, marginTop: 8 }}>{error.message}</p>
        {error.stack ? (
          <pre
            style={{
              marginTop: 12,
              maxHeight: 320,
              overflow: 'auto',
              border: '1px solid #e5e7eb',
              borderRadius: 8,
              padding: 12,
              fontSize: 11,
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
            }}
          >
            {error.stack}
          </pre>
        ) : null}
        <button
          type="button"
          onClick={() => window.location.reload()}
          style={{
            marginTop: 16,
            border: '1px solid #d1d5db',
            borderRadius: 6,
            padding: '6px 12px',
            fontSize: 14,
            background: 'transparent',
          }}
        >
          Reload
        </button>
      </div>
    )
  }
}
