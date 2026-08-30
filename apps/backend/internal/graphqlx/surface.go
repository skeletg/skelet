// Package graphqlx wires apperr into gqlgen - an error presenter turning a Go error into GraphQL extensions, plus a panic recover func, one Surface per endpoint
package graphqlx

type Surface int

const (
	SurfaceAdmin Surface = iota
	SurfaceFront
)

func (s Surface) String() string {
	if s == SurfaceFront {
		return "front"
	}
	return "admin"
}
