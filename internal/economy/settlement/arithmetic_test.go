// internal/economy/settlement/arithmetic_test.go
// T-А2 (витрина арифметики, спека 2026-09-23 §8.1/§8.2): «производим/потребляем/
// сверх» считается по ПОЗИЦИИ (сумма источников − спрос), «потребляем» — только
// по позициям из effects (позиция только в eat спроса не создаёт); «забираем» —
// по ветке (выход × quantity), доля залежи — по факту прохода.
package settlement

import (
	"math"
	"testing"
)

// T-А2: сверх потребления — по позиции (сумма веток позиции − спрос); позиция
// только в eat спроса не создаёт; норма отсутствующей в eat позиции — DefaultEatK.
func TestComputePositionArithmetic(t *testing.T) {
	effects := map[string]string{"продовольствие": "голод"}
	eat := map[string]float64{"продовольствие": 600, "вода": 500}
	sources := []ArithmeticSource{
		{Position: "продовольствие", RatePerDayPerBillion: 400},
		{Position: "продовольствие", RatePerDayPerBillion: 200},
		{Position: "вода", RatePerDayPerBillion: 100},
	}
	got := ComputePositionArithmetic(1e9, effects, eat, sources)
	if len(got) != 2 {
		t.Fatalf("ожидали 2 позиции, получили %d: %+v", len(got), got)
	}
	// Порядок — по позиции: «вода» < «продовольствие».
	if got[0].Position != "вода" || got[1].Position != "продовольствие" {
		t.Fatalf("порядок позиций неверен: %+v", got)
	}
	// «вода» есть в eat, но не в effects → спроса НЕ создаёт (мёртвый ключ нормы):
	// производим 100, потребляем 0, сверх +100.
	if d := math.Abs(got[0].ProducedPerDay - 100); d > 1e-9 {
		t.Fatalf("вода: производим = %v, ждали 100", got[0].ProducedPerDay)
	}
	if got[0].ConsumedPerDay != 0 {
		t.Fatalf("вода: потребляем = %v, ждали 0 (позиция только в eat)", got[0].ConsumedPerDay)
	}
	if d := math.Abs(got[0].NetPerDay - 100); d > 1e-9 {
		t.Fatalf("вода: сверх = %v, ждали +100", got[0].NetPerDay)
	}
	// «продовольствие»: производим 400+200 = 600, потребляем 600, сверх 0.
	if d := math.Abs(got[1].ProducedPerDay - 600); d > 1e-9 {
		t.Fatalf("продовольствие: производим = %v, ждали 600", got[1].ProducedPerDay)
	}
	if d := math.Abs(got[1].ConsumedPerDay - 600); d > 1e-9 {
		t.Fatalf("продовольствие: потребляем = %v, ждали 600", got[1].ConsumedPerDay)
	}
	if d := math.Abs(got[1].NetPerDay); d > 1e-9 {
		t.Fatalf("продовольствие: сверх = %v, ждали 0", got[1].NetPerDay)
	}
}

// Позиция из effects без записи в eat → норма DefaultEatK (600); дефицит
// (net < 0) выводится знаком.
func TestComputePositionArithmeticDefaultNorm(t *testing.T) {
	got := ComputePositionArithmetic(1e9,
		map[string]string{"продовольствие": "голод"}, nil,
		[]ArithmeticSource{{Position: "продовольствие", RatePerDayPerBillion: 100}})
	if len(got) != 1 {
		t.Fatalf("ожидали одну позицию: %+v", got)
	}
	if d := math.Abs(got[0].ConsumedPerDay - DefaultEatK); d > 1e-9 {
		t.Fatalf("норма по умолчанию: потребляем = %v, ждали %v", got[0].ConsumedPerDay, float64(DefaultEatK))
	}
	if d := math.Abs(got[0].NetPerDay + (DefaultEatK - 100)); d > 1e-9 {
		t.Fatalf("дефицит: сверх = %v, ждали %v", got[0].NetPerDay, -(DefaultEatK - 100))
	}
}

// Пустые sources/effects → пустой блок (не nil-панику).
func TestComputePositionArithmeticEmpty(t *testing.T) {
	if got := ComputePositionArithmetic(1e9, nil, nil, nil); got != nil {
		t.Fatalf("пустой вход → nil, получили %+v", got)
	}
}

// T12: строка нужды (§10.2) — позиция из effects получает норму и покрытие/
// дефицит из слоя потребности (w): covered = 1 − w, кламп [0,1]; позиция без
// эффекта (только источник) нужды не несёт.
func TestAttachNeedArithmetic(t *testing.T) {
	positions := ComputePositionArithmetic(1e9,
		map[string]string{"очищенная вода": "жажда", "вода": "жажда"},
		map[string]float64{"очищенная вода": 2e7},
		[]ArithmeticSource{
			{Position: "очищенная вода", RatePerDayPerBillion: 0},
			{Position: "вода", RatePerDayPerBillion: 0},
		})
	// «вода» в effects есть, но в этот эффект не привязана (нет bindings) —
	// берём только «очищенную воду» как привязанную позицию.
	bindings := []NeedsBinding{{
		Position: "очищенная вода", EffectTypeName: "жажда", EffectTypeID: 5, NormPerDayPerBillion: 2e7,
	}}
	effects := []EffectRun{{EffectTypeID: 5, W: 0.58}}

	got := AttachNeedArithmetic(positions, bindings, effects)
	if len(got) != 2 {
		t.Fatalf("ожидали 2 позиции, получили %d: %+v", len(got), got)
	}
	var water *PositionArithmetic
	for i := range got {
		if got[i].Position == "очищенная вода" {
			water = &got[i]
		}
	}
	if water == nil {
		t.Fatalf("позиция «очищенная вода» не найдена: %+v", got)
	}
	if water.Need != "жажда" || water.Effect != "жажда" {
		t.Fatalf("нужда позиции: need=%q effect=%q, ждали «жажда»/«жажда»", water.Need, water.Effect)
	}
	if d := math.Abs(water.NormPerDayPerBillion - 2e7); d > 1e-9 {
		t.Fatalf("норма позиции = %v, ждали 2e7", water.NormPerDayPerBillion)
	}
	if d := math.Abs(water.DeficitShare - 0.58); d > 1e-9 {
		t.Fatalf("дефицит = %v, ждали 0.58", water.DeficitShare)
	}
	if d := math.Abs(water.CoveredShare - 0.42); d > 1e-9 {
		t.Fatalf("покрытие = %v, ждали 0.42", water.CoveredShare)
	}
	// Позиция без привязки — без нужды (нули).
	for i := range got {
		if got[i].Position == "вода" && (got[i].Need != "" || got[i].CoveredShare != 0) {
			t.Fatalf("позиция без привязки не должна нести нужду: %+v", got[i])
		}
	}
}

// Дефицит клампится в [0,1]: w < 0 → покрытие 1, дефицит 0.
func TestAttachNeedArithmeticClamp(t *testing.T) {
	positions := ComputePositionArithmetic(1e9,
		map[string]string{"пища": "голод"}, nil,
		[]ArithmeticSource{{Position: "пища", RatePerDayPerBillion: 600}})
	bindings := []NeedsBinding{{Position: "пища", EffectTypeName: "голод", EffectTypeID: 1, NormPerDayPerBillion: 600}}
	effects := []EffectRun{{EffectTypeID: 1, W: -0.5}}

	got := AttachNeedArithmetic(positions, bindings, effects)
	if len(got) != 1 {
		t.Fatalf("ожидали одну позицию: %+v", got)
	}
	if got[0].DeficitShare != 0 || got[0].CoveredShare != 1 {
		t.Fatalf("кламп: дефицит %v, покрытие %v (ждали 0/1)", got[0].DeficitShare, got[0].CoveredShare)
	}
}

// «Забираем» — по ветке: выход × quantity_i; дубликат компонента сводится.
func TestBranchTakePerDay(t *testing.T) {
	takes := BranchTakePerDay(600, 1e9, []BranchComponent{
		{GoodID: 359, Quantity: 3},
		{GoodID: 360, Quantity: 1},
		{GoodID: 359, Quantity: 1},
	})
	if len(takes) != 2 {
		t.Fatalf("ожидали 2 агрегированных компонента: %+v", takes)
	}
	if takes[0].GoodID != 359 || takes[0].Quantity != 4 {
		t.Fatalf("359 агрегируется: %+v", takes[0])
	}
	if d := math.Abs(takes[0].PerDay - 2400); d > 1e-9 {
		t.Fatalf("359: забираем = %v, ждали 2400 (600×4)", takes[0].PerDay)
	}
	if d := math.Abs(takes[1].PerDay - 600); d > 1e-9 {
		t.Fatalf("360: забираем = %v, ждали 600", takes[1].PerDay)
	}
}

// Доля добора из залежей — по факту прохода: need минус фактически съеденный вход.
func TestBranchDepositShare(t *testing.T) {
	components := []BranchComponent{{GoodID: 359, Quantity: 3}, {GoodID: 359, Quantity: 1}}
	// produced 10 батчей → need = 40; входа было 30, стало 0 → из залежей 10.
	share := BranchDepositShare(10, components,
		map[int64]float64{359: 30}, map[int64]float64{359: 0})
	if d := math.Abs(share - 0.25); d > 1e-9 {
		t.Fatalf("доля залежи = %v, ждали 0.25", share)
	}
	// Входа хватило целиком → доля 0.
	if s := BranchDepositShare(10, components,
		map[int64]float64{359: 40}, map[int64]float64{359: 0}); s != 0 {
		t.Fatalf("входа хватило → доля 0, получили %v", s)
	}
	// Нет производства → 0.
	if s := BranchDepositShare(0, components, map[int64]float64{359: 30}, nil); s != 0 {
		t.Fatalf("produced 0 → доля 0, получили %v", s)
	}
}
