//go:build !race

package galaxy

// isRace — false в обычной сборке (см. race_enabled.go).
func isRace() bool { return false }
