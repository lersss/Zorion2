// internal/economy/settlement/balancer_hunger_test.go
// Тесты этапа 3 «эффекты снабжения» (§7.1/§7.2, T20/T21/T30): компонента
// `hunger` в store балансировки — допустимость, ось X (нагрузка), порог
// (нулевой префикс) только у эффект-компонент, скаляр `recovery` (только
// hunger, заводское 0.25), атомарная запись кривой+скаляра, пресеты несут
// скаляр, расовый сеттер hunger не принимает.
package settlement

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// validHungerCurve — валидная кривая эффект-компоненты: первый узел Y = 0
// (порог), X в диапазоне [0, 8760].
func validHungerCurve() ComponentCurve {
	return ComponentCurve{
		Nodes: []SegmentNode{{X: 0, Y: 0}, {X: 24, Y: 0}, {X: 456, Y: 1e-7}},
		Bends: []float64{-1, -0.2},
	}
}

// TestHungerComponentClassification (T21): hunger — валидная ГЛОБАЛЬНАЯ
// компонента, но НЕ расовая; heat — расовая.
func TestHungerComponentClassification(t *testing.T) {
	if !ValidComponent(HungerCurveKey) {
		t.Error("ValidComponent(hunger) = false, хочу true (§7.1)")
	}
	if IsRaceComponent(HungerCurveKey) {
		t.Error("IsRaceComponent(hunger) = true, хочу false (§7.2)")
	}
	for _, c := range []string{"heat", "cold", "gravity", "radiation"} {
		if !IsRaceComponent(c) {
			t.Errorf("IsRaceComponent(%q) = false, хочу true", c)
		}
	}
	if ValidComponent("acid") || IsRaceComponent("acid") {
		t.Error("неизвестная компонента должна быть отвергнута обеими проверками")
	}
}

// TestHungerCurveRangeAndThreshold (T20): ось X hunger — [0, 8760]; требование
// «первый узел Y = 0» действует только для hunger (cold/gravity прошли бы
// с ненулевым первым узлом — их кривые такие и есть).
func TestHungerCurveRangeAndThreshold(t *testing.T) {
	t.Cleanup(func() {
		_ = ResetCurve(HungerCurveKey)
		_ = ResetCurve("cold")
	})

	if err := SetCurve(HungerCurveKey, validHungerCurve()); err != nil {
		t.Fatalf("валидная hunger отклонена: %v", err)
	}
	// Первый узел Y ≠ 0 → ошибка (порог обязателен для эффект-компоненты).
	bad := ComponentCurve{
		Nodes: []SegmentNode{{X: 0, Y: 0.5}, {X: 24, Y: 0}, {X: 456, Y: 1e-7}},
		Bends: []float64{-1, -0.2},
	}
	if err := SetCurve(HungerCurveKey, bad); err == nil {
		t.Error("hunger с первым узлом Y ≠ 0 принята (нужен порог)")
	}
	// X вне [0, 8760] → ошибка.
	out := ComponentCurve{
		Nodes: []SegmentNode{{X: 0, Y: 0}, {X: 24, Y: 1e-8}, {X: 9000, Y: 1e-7}},
		Bends: []float64{0, 0},
	}
	if err := SetCurve(HungerCurveKey, out); err == nil {
		t.Error("hunger с X = 9000 вне диапазона [0,8760] принята")
	}
	// cold с ненулевым первым узлом остаётся валидной (требование порога — только hunger).
	if err := SetCurve("cold", ComponentCurve{
		Nodes: []SegmentNode{{X: -273.15, Y: 0.05}, {X: -100, Y: 0.001}, {X: 14.85, Y: 0}},
		Bends: []float64{0, 0},
	}); err != nil {
		t.Errorf("валидная cold отклонена (порог не должен применяться к не-эффектам): %v", err)
	}
}

// TestHungerDefaultCurveNonNil (T20): инициализатор store и defaultCurve не
// nil; порог дефолта = 24 (нулевой префикс 0..24).
func TestHungerDefaultCurveNonNil(t *testing.T) {
	nodes, bends, ok := GetCurve(HungerCurveKey)
	if !ok || len(nodes) == 0 {
		t.Fatalf("GetCurve(hunger): ok=%v nodes=%d — кривая отсутствует", ok, len(nodes))
	}
	if len(nodes) != len(defaultHungerCurve().Nodes) || len(bends) != len(defaultHungerCurve().Bends) {
		t.Errorf("дефолт hunger не совпадает с defaultHungerCurve (%d узлов)", len(nodes))
	}
	if thr := ZeroPrefixThreshold(nodes); thr != 24 {
		t.Errorf("порог дефолта hunger = %v, хочу 24", thr)
	}
	if defaultCurve(HungerCurveKey) == nil {
		t.Error("defaultCurve(hunger) = nil")
	}
	if nodes[0].Y != 0 {
		t.Errorf("первый узел дефолта hunger Y = %v, хочу 0", nodes[0].Y)
	}
}

// TestHungerScalarOnlyForEffect (T30): скаляр recovery живёт только у hunger,
// заводское 0.25; прочие → ошибка.
func TestHungerScalarOnlyForEffect(t *testing.T) {
	t.Cleanup(func() { _ = ResetComponentScalar(HungerCurveKey) })

	v, ok := ComponentScalar(HungerCurveKey)
	if !ok || v != 0.25 {
		t.Errorf("ComponentScalar(hunger) = %v ok=%v, хочу 0.25 (заводское)", v, ok)
	}
	if _, ok := ComponentScalar("heat"); ok {
		t.Error("ComponentScalar(heat) вернул ok=true — скаляра у среды быть не должно")
	}
	if err := SetComponentScalar("heat", 0.5); err == nil {
		t.Error("SetComponentScalar(heat) — не ошибка (скаляр только для эффект-компонент)")
	}
	if err := SetComponentScalar(HungerCurveKey, 0.4); err != nil {
		t.Fatalf("SetComponentScalar(hunger, 0.4): %v", err)
	}
	if err := SetComponentScalar(HungerCurveKey, -0.1); err == nil {
		t.Error("отрицательный скаляр hunger — не ошибка")
	}
	if err := ResetComponentScalar(HungerCurveKey); err != nil {
		t.Fatalf("ResetComponentScalar(hunger): %v", err)
	}
	if v, _ := ComponentScalar(HungerCurveKey); v != 0.25 {
		t.Errorf("после reset скаляр hunger = %v, хочу 0.25", v)
	}
}

// TestSetCurveWithScalarAtomic (T30): кривая и скаляр пишутся одной операцией;
// при ошибке валидации не меняется НИ кривая, НИ скаляр.
func TestSetCurveWithScalarAtomic(t *testing.T) {
	t.Cleanup(func() { _ = ResetCurveWithScalar(HungerCurveKey) })

	// Зафиксируем исходное состояние (дефолт).
	baseNodes, _, _ := GetCurve(HungerCurveKey)
	baseScalar, _ := ComponentScalar(HungerCurveKey)

	scalar := 0.33
	if err := SetCurveWithScalar(HungerCurveKey, validHungerCurve(), &scalar); err != nil {
		t.Fatalf("SetCurveWithScalar валидных данных: %v", err)
	}
	nodes, _, _ := GetCurve(HungerCurveKey)
	if nodes[1].X != 24 || nodes[2].Y != 1e-7 {
		t.Errorf("кривая не применилась: %+v", nodes)
	}
	if v, _ := ComponentScalar(HungerCurveKey); v != 0.33 {
		t.Errorf("скаляр не применился: %v", v)
	}

	// Невалидная кривая → ни кривая, ни скаляр не изменились.
	neg := -1.0
	if err := SetCurveWithScalar(HungerCurveKey, ComponentCurve{
		Nodes: []SegmentNode{{X: 0, Y: 0.5}, {X: 24, Y: 0}, {X: 456, Y: 1e-7}},
		Bends: []float64{0, 0},
	}, &neg); err == nil {
		t.Error("SetCurveWithScalar с невалидной кривой — не ошибка")
	}
	if v, _ := ComponentScalar(HungerCurveKey); v != 0.33 {
		t.Errorf("скаляр изменён при невалидной кривой: %v", v)
	}
	if nodes, _, _ := GetCurve(HungerCurveKey); nodes[2].Y != 1e-7 {
		t.Errorf("кривая изменена при невалидном PUT: %+v", nodes)
	}

	// Отрицательный скаляр → ошибка, кривая не меняется.
	if err := SetCurveWithScalar(HungerCurveKey, validHungerCurve(), &neg); err == nil {
		t.Error("SetCurveWithScalar с recovery < 0 — не ошибка")
	}

	// Скаляр на не-эффект-компоненту → ошибка.
	okScalar := 0.5
	if err := SetCurveWithScalar("heat", ComponentCurve{
		Nodes: []SegmentNode{{X: 30, Y: 0}, {X: 100, Y: 0.5}, {X: 4000, Y: 0.9}},
		Bends: []float64{0, 0},
	}, &okScalar); err == nil {
		t.Error("SetCurveWithScalar(heat, recovery) — не ошибка (recovery только для hunger)")
	}

	// Ничего из этого не тронуло исходный (дефолтный) store — проверим reset ниже.
	_ = baseNodes
	_ = baseScalar
}

// TestResetCurveWithScalar (T30/T20): reset сбрасывает И кривую, И скаляр.
func TestResetCurveWithScalar(t *testing.T) {
	t.Cleanup(func() { _ = ResetCurveWithScalar(HungerCurveKey) })

	scalar := 0.7
	if err := SetCurveWithScalar(HungerCurveKey, validHungerCurve(), &scalar); err != nil {
		t.Fatalf("SetCurveWithScalar: %v", err)
	}
	if err := ResetCurveWithScalar(HungerCurveKey); err != nil {
		t.Fatalf("ResetCurveWithScalar: %v", err)
	}
	nodes, _, _ := GetCurve(HungerCurveKey)
	if len(nodes) != len(defaultHungerCurve().Nodes) {
		t.Errorf("после reset кривая hunger не дефолтная: %d узлов", len(nodes))
	}
	if v, _ := ComponentScalar(HungerCurveKey); v != 0.25 {
		t.Errorf("после reset скаляр hunger = %v, хочу заводское 0.25", v)
	}
}

// TestHungerPresetCarriesRecovery (T20/T30): пресет несёт скаляр, apply и
// reset-default восстанавливают его; default-пресет hunger в файле — 0.25.
func TestHungerPresetCarriesRecovery(t *testing.T) {
	path := newPresetEnv(t)
	t.Cleanup(func() { _ = ResetCurveWithScalar(HungerCurveKey) })

	// default-пресет hunger в файле несёт заводское 0.25.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("чтение файла пресетов: %v", err)
	}
	var f balancerPresetsFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("файл пресетов невалиден: %v", err)
	}
	foundDefault := false
	for _, p := range f.Presets {
		if p.Component == HungerCurveKey && p.Name == "default" {
			foundDefault = true
			if p.Recovery == nil || *p.Recovery != 0.25 {
				t.Errorf("default-пресет hunger recovery = %v, хочу 0.25", p.Recovery)
			}
		}
	}
	if !foundDefault {
		t.Fatal("default-пресет hunger отсутствует в файле пресетов")
	}

	if err := SetCurve(HungerCurveKey, validHungerCurve()); err != nil {
		t.Fatalf("SetCurve(hunger): %v", err)
	}
	if err := SetComponentScalar(HungerCurveKey, 0.35); err != nil {
		t.Fatalf("SetComponentScalar: %v", err)
	}
	if _, err := SavePreset(HungerCurveKey, "лето"); err != nil {
		t.Fatalf("SavePreset(hunger): %v", err)
	}

	// Скаляр испорчен, кривая сброшена — apply возвращает оба из пресета.
	if err := SetComponentScalar(HungerCurveKey, 0.9); err != nil {
		t.Fatalf("SetComponentScalar: %v", err)
	}
	if _, _, err := ApplyPreset(HungerCurveKey, "лето"); err != nil {
		t.Fatalf("ApplyPreset(hunger): %v", err)
	}
	if v, _ := ComponentScalar(HungerCurveKey); math.Abs(v-0.35) > 1e-12 {
		t.Errorf("после apply скаляр = %v, хочу 0.35 (из пресета)", v)
	}

	// reset-default → и кривая, и скаляр заводские.
	if _, _, err := ResetDefaultPreset(HungerCurveKey); err != nil {
		t.Fatalf("ResetDefaultPreset(hunger): %v", err)
	}
	if v, _ := ComponentScalar(HungerCurveKey); v != 0.25 {
		t.Errorf("после reset-default скаляр = %v, хочу 0.25", v)
	}
}

// TestSetRaceCurveRejectsHunger (T21): расовый сеттер hunger не принимает
// (защита в глубину, §7.2) — возвращает ошибку, а не nil-кривую.
func TestSetRaceCurveRejectsHunger(t *testing.T) {
	if err := LoadRaceBalancer(filepath.Join(t.TempDir(), "race_balancer.json")); err != nil {
		t.Fatalf("LoadRaceBalancer: %v", err)
	}
	if err := SetRaceCurve("ammonia", HungerCurveKey, validHungerCurve()); err == nil {
		t.Error("SetRaceCurve(ammonia, hunger) — не ошибка (голод не от расы)")
	}
	// Разовая среда принимается (контроль, что отказ — именно из-за hunger).
	if err := SetRaceCurve("ammonia", "heat", ComponentCurve{
		Nodes: []SegmentNode{{X: 0, Y: 0}, {X: 100, Y: 0.5}, {X: 4000, Y: 0.9}},
		Bends: []float64{0, 0},
	}); err != nil {
		t.Errorf("SetRaceCurve(ammonia, heat) отклонён: %v", err)
	}
}
