//go:build race

package galaxy

// isRace — true при сборке с -race: детектор гонок замедляет прогон в 3–10x,
// замеры производительности под ним бессмысленны (см. performance_test.go).
func isRace() bool { return true }
