// Package graphqlx wires apperr into gqlgen - an error presenter turning a Go error into GraphQL extensions, plus a panic recover func, one Surface per endpoint
package graphqlx

type Surface int

const (
	SurfaceAdmin Surface = iota
	SurfaceFront
	SurfaceSignals // the Signals admin API, served by cmd/signals - operator-facing like admin, so it keeps the detailed errors
)

func (s Surface) String() string {
	switch s {
	case SurfaceFront:
		return "front"
	case SurfaceSignals:
		return "signals"
	}
	return "admin"
}
