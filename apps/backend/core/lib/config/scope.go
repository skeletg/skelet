package config

import "fmt"

// FormatScope returns a stable human-readable scope string for CLI/admin use
func FormatScope(scope Scope) string {
	switch scope.Kind {
	case ScopeDefault:
		return ScopeDefault
	case ScopeChannel:
		if scope.ChannelID == nil {
			return ScopeChannel
		}
		if scope.Locale != nil {
			return fmt.Sprintf("%s:%s:%s", ScopeChannel, scope.ChannelID.String(), *scope.Locale)
		}
		return fmt.Sprintf("%s:%s", ScopeChannel, scope.ChannelID.String())
	default:
		return scope.Kind
	}
}
