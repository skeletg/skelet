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

// DerefOr reads through a pointer, giving def for nil - for an optional argument whose absence means something other than the zero value
func DerefOr[T any](p *T, def T) T {
	if p == nil {
		return def
	}
	return *p
}

// NonZero points at v, or is nil when v is the zero value - an empty string written to a nullable column reads as unset
func NonZero[T comparable](v T) *T {
	var zero T
	if v == zero {
		return nil
	}
	return &v
}
