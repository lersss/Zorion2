// internal/routegame/grid_explain.go
// «Разбор по факторам» после необратимого boost (спека
// 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута.md, §14.13):
// именованный список того, что случилось на курсе. Чистая функция: без БД,
// HTTP, времени и глобального RNG; на bonus/применение не влияет. Детекция —
// 16 ошибок §14.13.3 + положительные/нейтральные факторы §14.13.4; полосы
// значимости — гипотеза §14.13.7 (при gain/neutral — 0).
//
// Файл держит публичный API и оркестратор; детекторы вынесены по классам —
// grid_explain_geometry.go (путь-геометрия, №1–10) и
// grid_explain_uncertainty.go (решения под неопределённостью + gain/neutral,
// №11–16), служебные помощники — grid_explain_util.go.
package routegame

// Группы факторов разбора (§14.13.5).
const (
	GridFactorGroupError   = "error"
	GridFactorGroupGain    = "gain"
	GridFactorGroupNeutral = "neutral"
)

// gridFactorMaxCells — предел подсветки клеток на фактор (§14.13.5).
const gridFactorMaxCells = 16

// GridFactor — сработавший фактор разбора (§14.13.5). Code — стабильный
// машинный код (не локализуется); Severity — 1..4 у error и 0 у gain/neutral.
type GridFactor struct {
	Code     string `json:"code"`
	Group    string `json:"group"`
	Severity int    `json:"severity"`
	Count    int    `json:"count"`
	Cells    []int  `json:"cells"`
}

// gridFactorSeverity — полоса значимости по коду (§14.13.7): гипотеза
// калибровки, только для group=error. У gain/neutral полосы нет (severity 0).
var gridFactorSeverity = map[string]int{
	"revisit": 1, "overshoot": 1, "turn_cost": 1, "mud_cost": 1,
	"gate_useless": 2, "mud_entry_repeat": 2,
	"gate_reentry": 3, "bridge_reuse": 3, "dead_end": 3,
	"current_against": 3, "decoy_penalty": 3, "ping_destabilize": 3,
	"lure_missed": 4, "hidden_trap": 4, "ping_wasted": 4, "trap_entered_known": 4,
}

// GridSectorContentFromString — обратный к GridSectorContent.String: разбор
// содержимого вскрытого сектора из player_route_puzzle.revealed.
func GridSectorContentFromString(s string) (GridSectorContent, bool) {
	switch s {
	case "empty":
		return GridContentEmpty, true
	case "lure":
		return GridContentLure, true
	case "trap":
		return GridContentTrap, true
	case "decoy":
		return GridContentDecoy, true
	case "jackpot":
		return GridContentJackpot, true
	case "unstable":
		return GridContentUnstable, true
	}
	return GridContentEmpty, false
}

// explainCtx — состояние разбора пути, разделяемое детекторами классов:
// предвычисленные посещения/входы/направления пути и накопитель факторов.
type explainCtx struct {
	field      GridField
	cells      []int
	visit      map[int]int
	enter      map[int]int
	onPath     map[int]bool
	dirAt      []int
	uniquePath []int
	revealed   map[int]GridSectorContent
	out        []GridFactor
}

// add добавляет сработавший фактор: count ≤ 0 пропускается; cells — не nil и не
// длиннее gridFactorMaxCells; severity — полоса по коду только для ошибок.
func (x *explainCtx) add(code, group string, count int, cs []int) {
	if count <= 0 {
		return
	}
	sev := 0
	if group == GridFactorGroupError {
		sev = gridFactorSeverity[code]
	}
	if cs == nil {
		cs = []int{}
	}
	if len(cs) > gridFactorMaxCells {
		cs = cs[:gridFactorMaxCells]
	}
	x.out = append(x.out, GridFactor{Code: code, Group: group, Severity: sev, Count: count, Cells: cs})
}

// ExplainGridPath — разбор провалидированного пути по факторам (§14.13.3/
// §14.13.4). field — поле УЖЕ с применённой помехой (как в boost); cells —
// присланный путь; revealed — множество вскрытых секторов (sector → content,
// пустой/повтор допустим — set-семантика). Возвращает только сработавшие
// факторы, детерминированно, порядок: error → gain → neutral (внутри —
// severity desc, count desc, code asc). Пустой результат — не-nil `[]`.
func ExplainGridPath(field GridField, cells []int, revealed map[int]GridSectorContent) []GridFactor {
	x := &explainCtx{
		field:    field,
		cells:    cells,
		revealed: revealed,
		out:      make([]GridFactor, 0),
	}
	if len(cells) < 2 {
		return x.out
	}

	x.visit = make(map[int]int, len(cells))
	x.enter = make(map[int]int, len(cells))
	x.onPath = make(map[int]bool, len(cells))
	for _, c := range cells {
		x.visit[c]++
		x.onPath[c] = true
	}
	for t := 1; t < len(cells); t++ {
		x.enter[cells[t]]++
	}
	x.dirAt = make([]int, len(cells))
	for t := 1; t < len(cells); t++ {
		x.dirAt[t] = field.dirOf(cells[t-1], cells[t])
	}

	// uniquePath — клетки пути в порядке первого появления (для фактор-агрегатов).
	x.uniquePath = make([]int, 0, len(cells))
	seen := make(map[int]bool, len(cells))
	for _, c := range cells {
		if !seen[c] {
			seen[c] = true
			x.uniquePath = append(x.uniquePath, c)
		}
	}

	explainGeometry(x)
	explainUncertainty(x)

	gridSortFactors(x.out)
	return x.out
}
