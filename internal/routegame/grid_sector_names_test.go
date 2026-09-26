// internal/routegame/grid_sector_names_test.go
// Player-facing словарь публичных полей сектора (§1 интерфейсной спеки
// 2026-09-25-маршрут-мини-игра-интерфейс.md / §14.12): гулкость σ —
// Тихий/Ровный/Гулкий, окружение — Ничего/Кордон/Обрыв/Мгла/Течение.
package routegame

import "testing"

func TestGridSectorSig_String_PlayerFacing(t *testing.T) {
	want := map[GridSectorSig]string{
		GridSigQuiet:  "Тихий",
		GridSigMedium: "Ровный",
		GridSigNoisy:  "Гулкий",
	}
	for sig, w := range want {
		if got := sig.String(); got != w {
			t.Errorf("GridSectorSig(%d).String() = %q, want %q", sig, got, w)
		}
	}
}

func TestGridSectorSurround_String_PlayerFacing(t *testing.T) {
	want := map[GridSectorSurround]string{
		GridSurroundPlain:   "Ничего",
		GridSurroundGate:    "Кордон",
		GridSurroundDeadEnd: "Обрыв",
		GridSurroundMud:     "Мгла",
		GridSurroundCurrent: "Течение",
	}
	for sur, w := range want {
		if got := sur.String(); got != w {
			t.Errorf("GridSectorSurround(%d).String() = %q, want %q", sur, got, w)
		}
	}
}
