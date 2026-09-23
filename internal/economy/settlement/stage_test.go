// internal/economy/settlement/stage_test.go
//
// Юнит-тесты ладдеры стадий (спека 2026-09-23-стадии-поселения-и-скорость-
// производства §4.3, §15.2 T-С1…С3): порядок по (Enter, ID), коридор удержания
// [upTarget, downTarget], пол (низшая стадия не читает Exit), тип вне ладдеры,
// одна стадия, прыжок через ступень.
package settlement

import "testing"

func TestStageLadderSortsByEnterThenID(t *testing.T) {
	l := NewStageLadder([]Stage{
		{ID: 30, Enter: 200, Exit: 150},
		{ID: 10, Enter: 0, Exit: 0},
		{ID: 20, Enter: 100, Exit: 50},
	})
	if l.Len() != 3 {
		t.Fatalf("Len = %d, want 3", l.Len())
	}
	// Пол — низшая по (Enter, ID) = id 10.
	if id, changed := l.Select(10, 1_000); !changed || id != 30 {
		t.Fatalf("Select(пол, много) = (%d,%v), want (30,true)", id, changed)
	}
}

func TestStageLadderSelectControlExample(t *testing.T) {
	// Контрольный пример §4.3: две стадии, одинаковые пороги (S1 — пол).
	l := NewStageLadder([]Stage{
		{ID: 1, Enter: 500, Exit: 300},
		{ID: 2, Enter: 500, Exit: 300},
	})

	// pop = 400, current = S1: upTarget = 0, downTarget = 1 → коридор [0,1],
	// current внутри → остаётся на S1 (дребезга нет).
	if id, changed := l.Select(1, 400); changed || id != 1 {
		t.Fatalf("Select(S1, 400) = (%d,%v), want (1,false)", id, changed)
	}
	// pop = 600 → переход на S2.
	if id, changed := l.Select(1, 600); !changed || id != 2 {
		t.Fatalf("Select(S1, 600) = (%d,%v), want (2,true)", id, changed)
	}
	// pop = 250, current = S2 → возврат на S1.
	if id, changed := l.Select(2, 250); !changed || id != 1 {
		t.Fatalf("Select(S2, 250) = (%d,%v), want (1,true)", id, changed)
	}
}

func TestStageLadderHysteresis(t *testing.T) {
	l := NewStageLadder([]Stage{
		{ID: 1, Enter: 0, Exit: 0},
		{ID: 2, Enter: 100, Exit: 50},
	})

	// Население в коридоре [50,100): ни вверх, ни вниз.
	if id, changed := l.Select(1, 70); changed || id != 1 {
		t.Fatalf("Select(S1, 70) = (%d,%v), want (1,false) — коридор", id, changed)
	}
	// Выше входа — вверх.
	if id, changed := l.Select(1, 120); !changed || id != 2 {
		t.Fatalf("Select(S1, 120) = (%d,%v), want (2,true)", id, changed)
	}
	// Ниже выхода — вниз.
	if id, changed := l.Select(2, 40); !changed || id != 1 {
		t.Fatalf("Select(S2, 40) = (%d,%v), want (1,true)", id, changed)
	}
}

func TestStageLadderFloorExitNotRead(t *testing.T) {
	// У пола Exit заведомо больше населения: он не читается — поселение всё
	// равно опускается на пол, а не застревает/уходит в невалидный индекс.
	l := NewStageLadder([]Stage{
		{ID: 1, Enter: 0, Exit: 999},
		{ID: 2, Enter: 100, Exit: 50},
	})
	if id, changed := l.Select(2, 10); !changed || id != 1 {
		t.Fatalf("Select(S2, 10) = (%d,%v), want (1,true) — пол держит выход", id, changed)
	}
	// На самом полу ниже входа некуда — остаётся.
	if id, changed := l.Select(1, 10); changed || id != 1 {
		t.Fatalf("Select(S1, 10) = (%d,%v), want (1,false)", id, changed)
	}
}

func TestStageLadderJump(t *testing.T) {
	l := NewStageLadder([]Stage{
		{ID: 1, Enter: 0, Exit: 0},
		{ID: 2, Enter: 100, Exit: 50},
		{ID: 3, Enter: 200, Exit: 150},
	})
	// Скачок населения через ступень → одно переключение на целевую.
	if id, changed := l.Select(1, 500); !changed || id != 3 {
		t.Fatalf("Select(S1, 500) = (%d,%v), want (3,true) — прыжок", id, changed)
	}
	// Прыжок вниз через ступень.
	if id, changed := l.Select(3, 10); !changed || id != 1 {
		t.Fatalf("Select(S3, 10) = (%d,%v), want (1,true)", id, changed)
	}
}

func TestStageLadderCurrentOutside(t *testing.T) {
	l := NewStageLadder([]Stage{
		{ID: 1, Enter: 0, Exit: 0},
		{ID: 2, Enter: 100, Exit: 50},
	})
	// Текущий тип не в ладдере (нет params.stage) → переключений нет.
	if id, changed := l.Select(999, 1_000); changed || id != 999 {
		t.Fatalf("Select(вне ладдеры, 1000) = (%d,%v), want (999,false)", id, changed)
	}
}

func TestStageLadderSingleStage(t *testing.T) {
	l := NewStageLadder([]Stage{{ID: 1, Enter: 0, Exit: 0}})
	if id, changed := l.Select(1, 1_000); changed || id != 1 {
		t.Fatalf("Select(единственная, 1000) = (%d,%v), want (1,false)", id, changed)
	}
}

func TestStageLadderEmpty(t *testing.T) {
	l := NewStageLadder(nil)
	if id, changed := l.Select(148, 1_000); changed || id != 148 {
		t.Fatalf("Select(пустая ладдера) = (%d,%v), want (148,false)", id, changed)
	}
}
