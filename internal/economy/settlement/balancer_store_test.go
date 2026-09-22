// internal/economy/settlement/balancer_store_test.go
// Тесты store кривых (спека 99.2.17 §5/§6/§9): горячая подмена (SetCurve →
// evaluateCurve считает по новой), сброс на дефолт, валидация 422 (ошибки
// store), параллельный доступ (замена -race: локально go test ./…).
package settlement

import (
	"fmt"
	"math"
	"sync"
	"testing"
)

// resetAllCurves — восстановление дефолтов всех компонент (t.Cleanup).
func resetAllCurves(t *testing.T) {
	t.Helper()
	for _, c := range []string{"heat", "cold", "gravity", "radiation", HungerCurveKey} {
		if err := ResetCurve(c); err != nil {
			t.Fatalf("ResetCurve(%q): %v", c, err)
		}
	}
}

// TestHotSwap — SetCurve ⇒ GetCurve возвращает новую кривую; следующий
// evaluateCurve считает по ней (чувствительность: R(100 °C) изменился).
func TestHotSwap(t *testing.T) {
	t.Cleanup(func() { resetAllCurves(t) })

	before := 0.0
	if nodes, bends, ok := GetCurve("heat"); ok {
		before = evaluateCurve(nodes, bends, 100)
	}
	curve := ComponentCurve{
		Nodes: []SegmentNode{{X: 30, Y: 0}, {X: 100, Y: 0.5}, {X: 4000, Y: 0.9}},
		Bends: []float64{0, 0},
	}
	if err := SetCurve("heat", curve); err != nil {
		t.Fatalf("SetCurve: %v", err)
	}
	nodes, bends, ok := GetCurve("heat")
	if !ok {
		t.Fatal("GetCurve(heat) не нашёл кривую")
	}
	if len(nodes) != 3 || len(bends) != 2 {
		t.Fatalf("GetCurve вернул %d узлов / %d изгибов, хочу 3/2", len(nodes), len(bends))
	}
	if nodes[1].Y != 0.5 {
		t.Errorf("GetCurve вернул старую кривую: y(100) = %v, хочу 0.5", nodes[1].Y)
	}
	after := evaluateCurve(nodes, bends, 100)
	if math.Abs(after-before) < 1e-9 {
		t.Errorf("R(100 °C) не изменился после SetCurve: %v", after)
	}
	if math.Abs(after-0.5) > 1e-12 {
		t.Errorf("R(100 °C) = %v, хочу 0.5 (кривая из store)", after)
	}
}

// TestReset — SetCurve → ResetCurve → GetCurve = дефолт §3.
func TestReset(t *testing.T) {
	t.Cleanup(func() { resetAllCurves(t) })

	curve := ComponentCurve{
		Nodes: []SegmentNode{{X: 30, Y: 0}, {X: 100, Y: 0.5}, {X: 4000, Y: 0.9}},
		Bends: []float64{0, 0},
	}
	if err := SetCurve("heat", curve); err != nil {
		t.Fatalf("SetCurve: %v", err)
	}
	if err := ResetCurve("heat"); err != nil {
		t.Fatalf("ResetCurve: %v", err)
	}
	nodes, bends, ok := GetCurve("heat")
	if !ok {
		t.Fatal("GetCurve(heat) не нашёл кривую после reset")
	}
	def := defaultHeatCurve()
	if len(nodes) != len(def.Nodes) || len(bends) != len(def.Bends) {
		t.Fatalf("после reset: %d узлов/%d изгибов, хочу дефолт %d/%d",
			len(nodes), len(bends), len(def.Nodes), len(def.Bends))
	}
	for i := range def.Nodes {
		if nodes[i] != def.Nodes[i] {
			t.Errorf("узел %d после reset = %+v, хочу дефолт %+v", i, nodes[i], def.Nodes[i])
		}
	}
	for i := range def.Bends {
		if bends[i] != def.Bends[i] {
			t.Errorf("изгиб %d после reset = %v, хочу дефолт %v", i, bends[i], def.Bends[i])
		}
	}
}

// TestValidation422 — невалидные данные → error (spec §6, 422 на уровне
// хендлера; store возвращает ошибку, значение не сохраняется).
func TestValidation422(t *testing.T) {
	t.Cleanup(func() { resetAllCurves(t) })

	valid := ComponentCurve{
		Nodes: []SegmentNode{{X: 30, Y: 0}, {X: 100, Y: 0.1}, {X: 4000, Y: 0.9}},
		Bends: []float64{0, 0},
	}
	if err := SetCurve("heat", valid); err != nil {
		t.Fatalf("валидная кривая отклонена: %v", err)
	}

	// 1. Узлов < 3.
	if err := SetCurve("heat", ComponentCurve{
		Nodes: []SegmentNode{{X: 30, Y: 0}, {X: 100, Y: 0.1}},
		Bends: []float64{0},
	}); err == nil {
		t.Error("2 узла — не ошибка (мин 3)")
	}
	// 2. Узлов > 16.
	tooMany := ComponentCurve{Bends: make([]float64, 16)}
	for i := 0; i < 17; i++ {
		tooMany.Nodes = append(tooMany.Nodes, SegmentNode{X: float64(30 + i), Y: 0.001})
	}
	if err := SetCurve("heat", tooMany); err == nil {
		t.Error("17 узлов — не ошибка (макс 16)")
	}
	// 3. Изгибов ≠ узлов − 1.
	if err := SetCurve("heat", ComponentCurve{
		Nodes: []SegmentNode{{X: 30, Y: 0}, {X: 100, Y: 0.1}, {X: 4000, Y: 0.9}},
		Bends: []float64{0},
	}); err == nil {
		t.Error("изгибов 1 при 3 узлах — не ошибка")
	}
	// 4. X не возрастают (дубликат).
	if err := SetCurve("heat", ComponentCurve{
		Nodes: []SegmentNode{{X: 30, Y: 0}, {X: 30, Y: 0.1}, {X: 4000, Y: 0.9}},
		Bends: []float64{0, 0},
	}); err == nil {
		t.Error("дубликат X — не ошибка")
	}
	// 5. y < 0.
	if err := SetCurve("heat", ComponentCurve{
		Nodes: []SegmentNode{{X: 30, Y: -0.01}, {X: 100, Y: 0.1}, {X: 4000, Y: 0.9}},
		Bends: []float64{0, 0},
	}); err == nil {
		t.Error("y < 0 — не ошибка")
	}
	// 6. y ≥ 0.999 (гвард R < 1).
	if err := SetCurve("heat", ComponentCurve{
		Nodes: []SegmentNode{{X: 30, Y: 0}, {X: 100, Y: 0.1}, {X: 4000, Y: 0.999}},
		Bends: []float64{0, 0},
	}); err == nil {
		t.Error("y = 0.999 — не ошибка (гвард R < 1)")
	}
	// 7. |k| > 10.
	if err := SetCurve("heat", ComponentCurve{
		Nodes: []SegmentNode{{X: 30, Y: 0}, {X: 100, Y: 0.1}, {X: 4000, Y: 0.9}},
		Bends: []float64{0, 10.5},
	}); err == nil {
		t.Error("k = 10.5 — не ошибка")
	}
	// 8. X вне диапазона компоненты (жара: 30..4000 °C).
	if err := SetCurve("heat", ComponentCurve{
		Nodes: []SegmentNode{{X: 20, Y: 0}, {X: 100, Y: 0.1}, {X: 4000, Y: 0.9}},
		Bends: []float64{0, 0},
	}); err == nil {
		t.Error("X = 20 °C вне диапазона жары — не ошибка")
	}
	// 8.1. X вне диапазона холод (в °C, −273.15..14.85 = 0..288 K; решение
	// создателя 2026-09-15 — холод везде в °C).
	if err := SetCurve("cold", ComponentCurve{
		Nodes: []SegmentNode{{X: -300, Y: 0.01}, {X: -100, Y: 0.001}, {X: 14.85, Y: 0}},
		Bends: []float64{0, 0},
	}); err == nil {
		t.Error("X = −300 °C вне диапазона холода — не ошибка")
	}
	// 8.2. Валидная холод в °C принимается (0..288 K = −273.15..14.85 °C).
	if err := SetCurve("cold", ComponentCurve{
		Nodes: []SegmentNode{{X: -273.15, Y: 0.05}, {X: -100, Y: 0.001}, {X: 14.85, Y: 0}},
		Bends: []float64{0, 0},
	}); err != nil {
		t.Errorf("валидная холод (в °C) отклонена: %v", err)
	}
	// 9. Неизвестная компонента.
	if err := SetCurve("acid", valid); err == nil {
		t.Error("неизвестная компонента — не ошибка")
	}

	// Значение не меняется при ошибке (идемпотентность хранения).
	nodes, _, _ := GetCurve("heat")
	if len(nodes) != 3 || nodes[1].Y != 0.1 {
		t.Errorf("store изменился после невалидных SetCurve: %+v", nodes)
	}
}

// TestConcurrentAccess — параллельные Get/Set/Sample из нескольких горутин.
// Локально без -race (нет gcc) — тест гоняет код-пути; в CI — под -race
// (AGENTS.md §2). Хранимый инвариант: ни паники, ни повреждения store.
func TestConcurrentAccess(t *testing.T) {
	t.Cleanup(func() { resetAllCurves(t) })

	const workers = 16
	const iters = 100
	var wg sync.WaitGroup
	errCh := make(chan error, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				comp := []string{"heat", "cold", "gravity", "radiation"}[w%4]
				switch i % 5 {
				case 0, 1, 2:
					nodes, bends, ok := GetCurve(comp)
					if !ok {
						errCh <- fmt.Errorf("GetCurve(%q): !ok", comp)
						return
					}
					_ = evaluateCurve(nodes, bends, nodes[0].X)
				case 3:
					xs := []float64{1, 2, 3}
					if _, ok := SampleCurve(comp, xs); !ok {
						errCh <- fmt.Errorf("SampleCurve(%q): !ok", comp)
						return
					}
				case 4:
					curve := ComponentCurve{
						Nodes: []SegmentNode{{X: 30, Y: 0}, {X: 100, Y: 0.1}, {X: 4000, Y: 0.9}},
						Bends: []float64{0, 0},
					}
					if err := SetCurve("heat", curve); err != nil {
						errCh <- fmt.Errorf("SetCurve: %v", err)
						return
					}
				}
			}
		}(w)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
	// После гонки store читается (не повреждён): дефолт-форма жары на месте
	// после сброса в cleanup; здесь просто проверяем, что Get работает.
	if _, _, ok := GetCurve("heat"); !ok {
		t.Error("store повреждён после параллельного доступа")
	}
}
