package routegame

// grid_explain_test.go — тесты «разбора по факторам» (§14.13): детекция каждой
// из 16 ошибок и положительных/нейтральных факторов на синтетических полях,
// порядок и полосы значимости, пустой разбор, отсутствие влияния на bonus.

import (
	"reflect"
	"testing"
)

// explainBase — пустое синтетическое поле N×N для разбора (Start (0,2),
// Finish (4,2) при N=5; путь — прямая строка j=2: 10,11,12,13,14).
func explainBase(n int) GridField {
	return GridField{
		N: n,
		Mud: map[int]float64{}, Lane: map[int]bool{}, Wall: map[int]bool{}, Gate: map[int]bool{},
		CurrentDir: map[int]int{}, Bridge: map[int]bool{}, DeadEnd: map[int]bool{},
		Bottleneck: map[int]bool{}, TurnCost: 1.9,
	}
}

func explainFind(fs []GridFactor, code string) *GridFactor {
	for i := range fs {
		if fs[i].Code == code {
			return &fs[i]
		}
	}
	return nil
}

// Прямая строка j=2: Start 10 → Finish 14.
func explainStraight() []int { return []int{10, 11, 12, 13, 14} }

func TestExplain_EmptyForCleanPath(t *testing.T) {
	f := explainBase(5)
	f.Start, f.Finish = 10, 14
	got := ExplainGridPath(f, explainStraight(), nil)
	if got == nil {
		t.Fatal("пустой разбор должен быть не-nil []")
	}
	if len(got) != 0 {
		t.Fatalf("чистый путь дал факторы: %+v", got)
	}
}

func TestExplain_TooFewCells(t *testing.T) {
	f := explainBase(5)
	f.Start, f.Finish = 10, 14
	if got := ExplainGridPath(f, []int{10}, nil); len(got) != 0 {
		t.Fatalf("короткий путь должен дать пустой разбор, got %+v", got)
	}
}

func TestExplain_TurnCost(t *testing.T) {
	f := explainBase(5)
	f.Start, f.Finish = 10, 14
	path := []int{10, 11, 12, 7, 12, 13, 14}
	got := ExplainGridPath(f, path, nil)
	fc := explainFind(got, "turn_cost")
	if fc == nil || fc.Group != GridFactorGroupError || fc.Count != 3 || fc.Severity != 1 {
		t.Fatalf("turn_cost: %+v", fc)
	}
}

func TestExplain_Revisit(t *testing.T) {
	f := explainBase(5)
	f.Start, f.Finish = 10, 14
	got := ExplainGridPath(f, []int{10, 11, 12, 7, 12, 13, 14}, nil)
	fc := explainFind(got, "revisit")
	if fc == nil || fc.Count != 1 || len(fc.Cells) != 1 || fc.Cells[0] != 12 {
		t.Fatalf("revisit: %+v", fc)
	}
}

func TestExplain_Overshoot(t *testing.T) {
	f := explainBase(5)
	f.Start, f.Finish = 10, 14
	got := ExplainGridPath(f, []int{10, 11, 12, 13, 14, 9, 14}, nil)
	fc := explainFind(got, "overshoot")
	if fc == nil || fc.Count != 1 || len(fc.Cells) != 1 || fc.Cells[0] != 14 {
		t.Fatalf("overshoot: %+v", fc)
	}
}

func TestExplain_MudCostAndRepeat(t *testing.T) {
	f := explainBase(5)
	f.Start, f.Finish = 10, 14
	f.Mud[12] = 2
	got := ExplainGridPath(f, []int{10, 11, 12, 11, 12, 13, 14}, nil)
	fc := explainFind(got, "mud_cost")
	if fc == nil || fc.Count != 2 || fc.Severity != 1 {
		t.Fatalf("mud_cost: %+v", fc)
	}
	fr := explainFind(got, "mud_entry_repeat")
	if fr == nil || fr.Count != 1 || len(fr.Cells) != 1 || fr.Cells[0] != 12 || fr.Severity != 2 {
		t.Fatalf("mud_entry_repeat: %+v", fr)
	}
}

func TestExplain_GateReentry(t *testing.T) {
	f := explainBase(5)
	f.Start, f.Finish = 10, 14
	f.Gate[12] = true
	got := ExplainGridPath(f, []int{10, 11, 12, 11, 12, 13, 14}, nil)
	fc := explainFind(got, "gate_reentry")
	if fc == nil || fc.Count != 1 || fc.Severity != 3 {
		t.Fatalf("gate_reentry: %+v", fc)
	}
}

func TestExplain_GateUseless(t *testing.T) {
	f := explainBase(5)
	f.Start, f.Finish = 10, 19
	f.Gate[12] = true
	got := ExplainGridPath(f, []int{10, 11, 12, 11, 16, 17, 18, 19}, nil)
	fc := explainFind(got, "gate_useless")
	if fc == nil || fc.Count != 1 || len(fc.Cells) != 1 || fc.Cells[0] != 12 || fc.Severity != 2 {
		t.Fatalf("gate_useless: %+v", fc)
	}
	if fc := explainFind(got, "gate_reentry"); fc != nil {
		t.Fatalf("gate_useless не должен давать gate_reentry: %+v", fc)
	}
}

func TestExplain_BridgeReuse(t *testing.T) {
	f := explainBase(5)
	f.Start, f.Finish = 10, 14
	f.Bridge[12] = true
	got := ExplainGridPath(f, []int{10, 11, 12, 11, 12, 13, 14}, nil)
	fc := explainFind(got, "bridge_reuse")
	if fc == nil || fc.Count != 1 || fc.Severity != 3 {
		t.Fatalf("bridge_reuse: %+v", fc)
	}
}

func TestExplain_DeadEnd(t *testing.T) {
	f := explainBase(5)
	f.Start, f.Finish = 10, 14
	f.DeadEnd[12] = true
	got := ExplainGridPath(f, explainStraight(), nil)
	fc := explainFind(got, "dead_end")
	if fc == nil || fc.Count != 1 || len(fc.Cells) != 1 || fc.Cells[0] != 12 || fc.Severity != 3 {
		t.Fatalf("dead_end: %+v", fc)
	}
}

func TestExplain_CurrentAgainstAndAlong(t *testing.T) {
	f := explainBase(5)
	f.Start, f.Finish = 10, 14
	f.CurrentDir[12] = 1 // течение −i; вход 11→12 идёт +i — против
	got := ExplainGridPath(f, explainStraight(), nil)
	fc := explainFind(got, "current_against")
	if fc == nil || fc.Count != 1 || fc.Severity != 3 {
		t.Fatalf("current_against: %+v", fc)
	}

	f2 := explainBase(5)
	f2.Start, f2.Finish = 10, 14
	f2.CurrentDir[12] = 0 // течение +i; вход 11→12 совпадает
	got2 := ExplainGridPath(f2, explainStraight(), nil)
	fa := explainFind(got2, "current_along")
	if fa == nil || fa.Group != GridFactorGroupGain || fa.Count != 1 || fa.Severity != 0 {
		t.Fatalf("current_along: %+v", fa)
	}
	if explainFind(got2, "current_against") != nil {
		t.Error("current_along не должен давать current_against")
	}
}

func TestExplain_WallCostNeutral(t *testing.T) {
	f := explainBase(5)
	f.Start, f.Finish = 10, 14
	f.Wall[12] = true
	got := ExplainGridPath(f, explainStraight(), nil)
	fc := explainFind(got, "wall_cost")
	if fc == nil || fc.Group != GridFactorGroupNeutral || fc.Count != 1 || fc.Severity != 0 {
		t.Fatalf("wall_cost: %+v", fc)
	}
}

func TestExplain_TrapHiddenAndKnown(t *testing.T) {
	// Воронка не вскрыта → hidden_trap.
	f := explainBase(5)
	f.Start, f.Finish = 10, 14
	f.Sectors = []GridSector{{Cells: []int{12}, content: GridContentTrap}}
	got := ExplainGridPath(f, explainStraight(), nil)
	fh := explainFind(got, "hidden_trap")
	if fh == nil || fh.Count != 1 || fh.Severity != 4 || len(fh.Cells) != 1 || fh.Cells[0] != 12 {
		t.Fatalf("hidden_trap: %+v", fh)
	}
	if explainFind(got, "trap_entered_known") != nil {
		t.Error("невскрытая Воронка не даёт trap_entered_known")
	}

	// Вскрыта → trap_entered_known.
	revealed := map[int]GridSectorContent{0: GridContentTrap}
	got2 := ExplainGridPath(f, explainStraight(), revealed)
	fk := explainFind(got2, "trap_entered_known")
	if fk == nil || fk.Count != 1 || fk.Severity != 4 {
		t.Fatalf("trap_entered_known: %+v", fk)
	}
	if explainFind(got2, "hidden_trap") != nil {
		t.Error("вскрытая Воронка не даёт hidden_trap")
	}
}

func TestExplain_Decoy(t *testing.T) {
	f := explainBase(5)
	f.Start, f.Finish = 10, 14
	f.Sectors = []GridSector{{Cells: []int{12}, content: GridContentDecoy}}
	got := ExplainGridPath(f, explainStraight(), nil)
	fc := explainFind(got, "decoy_penalty")
	if fc == nil || fc.Count != 1 || fc.Severity != 3 {
		t.Fatalf("decoy_penalty: %+v", fc)
	}
}

func TestExplain_LureMissed(t *testing.T) {
	f := explainBase(5)
	f.Start, f.Finish = 10, 14
	f.Sectors = []GridSector{{Cells: []int{0}, content: GridContentLure}}
	revealed := map[int]GridSectorContent{0: GridContentLure}
	got := ExplainGridPath(f, explainStraight(), revealed)
	fc := explainFind(got, "lure_missed")
	if fc == nil || fc.Count != 1 || fc.Severity != 4 || len(fc.Cells) != 1 || fc.Cells[0] != 0 {
		t.Fatalf("lure_missed: %+v", fc)
	}
	// Зов использован → find_used, не lure_missed.
	f.Sectors[0].Cells = []int{12}
	got2 := ExplainGridPath(f, explainStraight(), revealed)
	if explainFind(got2, "lure_missed") != nil {
		t.Error("использованный Зов не даёт lure_missed")
	}
	fu := explainFind(got2, "find_used")
	if fu == nil || fu.Group != GridFactorGroupGain || fu.Count != 1 || fu.Severity != 0 {
		t.Fatalf("find_used (lure): %+v", fu)
	}
}

func TestExplain_PingWasted(t *testing.T) {
	// Вскрытый Вакуум.
	f := explainBase(5)
	f.Start, f.Finish = 10, 14
	f.Sectors = []GridSector{{Cells: []int{0}, content: GridContentEmpty}}
	revealed := map[int]GridSectorContent{0: GridContentEmpty}
	got := ExplainGridPath(f, explainStraight(), revealed)
	fc := explainFind(got, "ping_wasted")
	if fc == nil || fc.Count != 1 || fc.Severity != 4 {
		t.Fatalf("ping_wasted (empty): %+v", fc)
	}

	// Вскрытый Просвет мимо пути.
	f2 := explainBase(5)
	f2.Start, f2.Finish = 10, 14
	f2.Sectors = []GridSector{{Cells: []int{0}, content: GridContentJackpot}}
	revealed2 := map[int]GridSectorContent{0: GridContentJackpot}
	got2 := ExplainGridPath(f2, explainStraight(), revealed2)
	if fc2 := explainFind(got2, "ping_wasted"); fc2 == nil || fc2.Count != 1 {
		t.Fatalf("ping_wasted (jackpot off path): %+v", fc2)
	}

	// Вскрытый Просвет НА пути → find_used, не ping_wasted.
	f2.Sectors[0].Cells = []int{12}
	got3 := ExplainGridPath(f2, explainStraight(), revealed2)
	if explainFind(got3, "ping_wasted") != nil {
		t.Error("Просвет на пути не даёт ping_wasted")
	}
	if explainFind(got3, "find_used") == nil {
		t.Error("Просвет на пути должен дать find_used")
	}
}

func TestExplain_PingDestabilizeErrorAndNeutral(t *testing.T) {
	// Сбой на пути → error, полоса 3.
	f := explainBase(5)
	f.Start, f.Finish = 10, 14
	f.Sectors = []GridSector{{Cells: []int{12}, content: GridContentUnstable}}
	revealed := map[int]GridSectorContent{0: GridContentUnstable}
	got := ExplainGridPath(f, explainStraight(), revealed)
	fc := explainFind(got, "ping_destabilize")
	if fc == nil || fc.Group != GridFactorGroupError || fc.Count != 1 || fc.Severity != 3 {
		t.Fatalf("ping_destabilize error: %+v", fc)
	}
	if len(fc.Cells) == 0 || fc.Cells[0] != 12 {
		t.Fatalf("ping_destabilize error cells: %+v", fc.Cells)
	}

	// Сбой в стороне (ни сектор, ни соседи не на пути) → neutral, severity 0.
	f2 := explainBase(5)
	f2.Start, f2.Finish = 10, 14
	f2.Sectors = []GridSector{{Cells: []int{0}, content: GridContentUnstable}}
	got2 := ExplainGridPath(f2, explainStraight(), map[int]GridSectorContent{0: GridContentUnstable})
	fn := explainFind(got2, "ping_destabilize")
	if fn == nil || fn.Group != GridFactorGroupNeutral || fn.Severity != 0 || fn.Count != 1 {
		t.Fatalf("ping_destabilize neutral: %+v", fn)
	}
}

func TestExplain_GroupOrderAndSeverity(t *testing.T) {
	f := explainBase(5)
	f.Start, f.Finish = 10, 14
	f.Mud[11] = 2          // error
	f.Wall[13] = true      // neutral
	f.CurrentDir[12] = 0   // gain (current_along)
	got := ExplainGridPath(f, explainStraight(), nil)
	if len(got) < 3 {
		t.Fatalf("ожидались 3 группы, got %+v", got)
	}
	if got[0].Group != GridFactorGroupError {
		t.Fatalf("первый — не error: %+v", got)
	}
	if got[len(got)-1].Group != GridFactorGroupNeutral {
		t.Fatalf("последний — не neutral: %+v", got)
	}
	sawGain := false
	for _, x := range got {
		sawGain = sawGain || x.Group == GridFactorGroupGain
		if x.Group != GridFactorGroupError && x.Severity != 0 {
			t.Fatalf("gain/neutral должен иметь severity 0: %+v", x)
		}
	}
	if !sawGain {
		t.Fatalf("нет gain-фактора: %+v", got)
	}
}

// Разбор — чистая функция: bonus до и после одинаков, поле не мутируется.
func TestExplain_DoesNotAffectBonus(t *testing.T) {
	var f GridField
	found := false
	for seed := int64(0); seed < 50 && !found; seed++ {
		if ff, ok := GenerateGridField(seed, []byte("explain"), 20000, calmPassport()); ok {
			f, found = ff, true
		}
	}
	if !found {
		t.Skip("нет принятого поля")
	}
	path := f.visitAllGrid(f.Visible, true).path
	if len(path) < 2 {
		t.Skip("нет пути")
	}
	before, ok, _ := EvaluateGridPath(f, path)
	if !ok {
		t.Fatal("путь невалиден")
	}
	snapshot := f
	_ = ExplainGridPath(f, path, nil)
	after, ok2, _ := EvaluateGridPath(f, path)
	if !ok2 {
		t.Fatal("путь перестал быть валидным")
	}
	if before != after {
		t.Fatalf("bonus изменился: %v → %v", before, after)
	}
	if !reflect.DeepEqual(snapshot, f) {
		t.Fatal("поле мутировано разбором")
	}
}

func TestExplain_SortSeverityThenCount(t *testing.T) {
	// Два error-фактора: создаём gate_reentry (sev3) и mud_cost (sev1).
	f := explainBase(5)
	f.Start, f.Finish = 10, 14
	f.Gate[12] = true
	f.Mud[11] = 2
	got := ExplainGridPath(f, []int{10, 11, 12, 11, 12, 13, 14}, nil)
	iGate, iMud := -1, -1
	for i, x := range got {
		if x.Code == "gate_reentry" {
			iGate = i
		}
		if x.Code == "mud_cost" {
			iMud = i
		}
	}
	if iGate < 0 || iMud < 0 || iGate > iMud {
		t.Fatalf("порядок severity desc нарушен: %+v", got)
	}
}
