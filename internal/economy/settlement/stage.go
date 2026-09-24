// internal/economy/settlement/stage.go
//
// Ладдера стадий поселения (спека 2026-09-23-стадии-поселения-и-скорость-
// производства §4.3): детерминированный выбор ступени роста по населению с
// гистерезисом. Стадия несёт порог входа `Enter` (население ≥ Enter → стадия
// применима) и порог выхода `Exit` (население < Exit → стадия снимается),
// причём Exit < Enter — зазор гасит дребезг на границе. Низшая стадия ладдеры
// — пол: её Exit не читается, ниже неё поселение не опускается.
//
// Ладдера строится ТОЛЬКО из записей класса «Поселение» с заданным params.stage
// (§4.3, находка M2) — фильтр класса и резолв типа-родителя живут в
// repository; сюда приходит уже готовый набор ступеней. Сортировка — по
// (Enter, ID) детерминированно. Чистая функция, никакого доступа к БД.
package settlement

import "sort"

// Stage — ступень роста поселения: тип поселения (ID = producer_types.id) с
// порогами по населению (в людях/особях). Enter — порог входа, Exit — порог
// выхода (Exit < Enter — зазор гистерезиса). Exit низшей ступени ладдеры не
// читается (пол).
type Stage struct {
	ID    int64
	Enter float64
	Exit  float64
}

// StageLadder — упорядоченная ладдера стадий. Порядок — по (Enter, ID),
// детерминированный: сравнение идёт с одной монотонной ладдерой, а не с
// диапазонами (§4.3).
type StageLadder struct {
	stages []Stage
}

// NewStageLadder — ладдера из набора стадий. Копирует вход и сортирует по
// (Enter, ID): порядок не зависит от порядка строк БД.
func NewStageLadder(stages []Stage) StageLadder {
	out := make([]Stage, len(stages))
	copy(out, stages)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Enter != out[j].Enter {
			return out[i].Enter < out[j].Enter
		}
		return out[i].ID < out[j].ID
	})
	return StageLadder{stages: out}
}

// Len — число стадий в ладдере.
func (l StageLadder) Len() int { return len(l.stages) }

// StageView — витрина ступени для карточки поселения (спека 2026-09-23 §11.3):
// пороги текущей ступени (в людях) и порог входа ближайшей ступени ВЫШЕ.
// Exit = 0 — порог выхода не читается (пол: низшая ступень ладдеры);
// NextEnter = 0 — выше текущей ступени нет.
type StageView struct {
	Enter     float64
	Exit      float64
	NextEnter float64
}

// CurrentView — витрина ступени типа currentID: пороги текущей ступени и вход
// следующей ВЫШЕ по ладдере. Тип вне ладдеры → ok=false (витрины нет — карточка
// рисует только имя типа). Чистая: порядок уже задан NewStageLadder.
func (l StageLadder) CurrentView(currentID int64) (StageView, bool) {
	for i, s := range l.stages {
		if s.ID != currentID {
			continue
		}
		v := StageView{Enter: s.Enter, Exit: s.Exit}
		if i == 0 {
			v.Exit = 0 // пол: порог выхода низшей ступени не читается
		}
		if i+1 < len(l.stages) {
			v.NextEnter = l.stages[i+1].Enter
		}
		return v, true
	}
	return StageView{}, false
}

// Select — целевая стадия для текущего типа и населения (§4.3, алгоритм N
// стадий): upTarget — самая высокая стадия, чей вход пройден; downTarget —
// самая высокая, чей выход ещё держится. Коридор удержания — [upTarget,
// downTarget]; текущая внутри него не меняется (вот где живёт гистерезис).
// Возвращает (текущая, false), если переключения нет: ладдера из одной записи,
// текущий тип не в ладдере, население в коридоре.
func (l StageLadder) Select(currentID int64, population float64) (int64, bool) {
	if len(l.stages) < 2 {
		return currentID, false
	}
	current := -1
	for i, s := range l.stages {
		if s.ID == currentID {
			current = i
			break
		}
	}
	if current < 0 {
		return currentID, false
	}

	up := -1
	for i, s := range l.stages {
		if population >= s.Enter {
			up = i
		}
	}
	// Пол: низшая стадия (index 0) не читает Exit — её выход держится всегда,
	// поэтому downTarget не опускается ниже пола.
	down := 0
	for i := 1; i < len(l.stages); i++ {
		if population >= l.stages[i].Exit {
			down = i
		}
	}

	switch {
	case current < up:
		return l.stages[up].ID, true // прыжок ВВЕРХ через стадии разрешён
	case current > down:
		return l.stages[down].ID, true // спуск, в т.ч. через несколько
	default:
		return currentID, false
	}
}
