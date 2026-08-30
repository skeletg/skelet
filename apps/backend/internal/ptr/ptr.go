// Package ptr holds the pointer helpers that optional GraphQL arguments and nullable columns need
// Four packages had grown their own byte-identical string version under three different names
package ptr

// Deref reads through a pointer, giving the zero value for nil - the shape a nullable input arrives in
func Deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
