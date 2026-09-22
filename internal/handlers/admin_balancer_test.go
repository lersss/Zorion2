// internal/handlers/admin_balancer_test.go
// Тесты эндпоинтов балансировщика (спека 99.2.17 §6): GET кривой, PUT с
// валидацией 422, reset, серверный sample, эталоны. Store — in-memory,
// мутаций в БД нет; после каждого теста дефолты восстанавливаются.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/economy/settlement"
)

// resetBalancer — восстановление дефолтов всех компонент (кривые + скаляр).
func resetBalancer(t *testing.T) {
	t.Helper()
	for _, c := range []string{"heat", "cold", "gravity", "radiation", settlement.HungerCurveKey} {
		if err := settlement.ResetCurveWithScalar(c); err != nil {
			t.Fatalf("ResetCurveWithScalar(%q): %v", c, err)
		}
	}
}

func TestBalancerCurveGet(t *testing.T) {
	t.Cleanup(func() { resetBalancer(t) })
	h := &AdminHandlers{}
	req := httptest.NewRequest(http.MethodGet, "/admin/balancer/curve?component=heat", nil)
	rec := execJSON(h.GetBalancerCurve, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Component string `json:"component"`
		Nodes     []struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		} `json:"nodes"`
		Bends []float64 `json:"bends"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "heat", resp.Component)
	require.Len(t, resp.Nodes, 14, "жара: 14 узлов (дефолт §3 с уплотнением сигмоидного перехода)")
	require.Len(t, resp.Bends, 13)
	require.Equal(t, 30.0, resp.Nodes[0].X)
	require.Equal(t, 0.0, resp.Nodes[0].Y)
	require.InDelta(t, 0.98, resp.Nodes[len(resp.Nodes)-1].Y, 0.001, "4000 °C → 0.980")
}

// TestBalancerColdCelsius — холод в API — в °C (решение создателя 2026-09-15):
// диапазон узлов −273.15..14.85 °C (= 0..288 K), строго возрастает.
func TestBalancerColdCelsius(t *testing.T) {
	t.Cleanup(func() { resetBalancer(t) })
	h := &AdminHandlers{}
	req := httptest.NewRequest(http.MethodGet, "/admin/balancer/curve?component=cold", nil)
	rec := execJSON(h.GetBalancerCurve, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Nodes []struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		} `json:"nodes"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.GreaterOrEqual(t, len(resp.Nodes), 2)
	require.InDelta(t, -273.15, resp.Nodes[0].X, 1e-9, "первый узел = 0 K → −273.15 °C")
	require.InDelta(t, 14.85, resp.Nodes[len(resp.Nodes)-1].X, 1e-9, "последний узел = 288 K → 14.85 °C")
	for i := 1; i < len(resp.Nodes); i++ {
		if !(resp.Nodes[i].X > resp.Nodes[i-1].X) {
			t.Errorf("X холода не строго возрастает: узел %d (%v) ≤ %d (%v)", i, resp.Nodes[i].X, i-1, resp.Nodes[i-1].X)
		}
	}
}

func TestBalancerCurveGetUnknownComponent(t *testing.T) {
	h := &AdminHandlers{}
	req := httptest.NewRequest(http.MethodGet, "/admin/balancer/curve?component=acid", nil)
	rec := execJSON(h.GetBalancerCurve, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestBalancerCurvePutAndGet(t *testing.T) {
	t.Cleanup(func() { resetBalancer(t) })
	h := &AdminHandlers{}

	body := `{"component":"heat","nodes":[{"x":30,"y":0},{"x":100,"y":0.5},{"x":4000,"y":0.9}],"bends":[0,0]}`
	req := httptest.NewRequest(http.MethodPut, "/admin/balancer/curve", strings.NewReader(body))
	rec := execJSON(h.PutBalancerCurve, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// GET отражает сохранённое.
	req = httptest.NewRequest(http.MethodGet, "/admin/balancer/curve?component=heat", nil)
	rec = execJSON(h.GetBalancerCurve, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Nodes []struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		} `json:"nodes"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Nodes, 3)
	require.Equal(t, 0.5, resp.Nodes[1].Y, "PUT применён: y(100 °C) = 0.5")

	// Идемпотентность: повторный PUT тех же данных — тот же результат.
	req = httptest.NewRequest(http.MethodPut, "/admin/balancer/curve", strings.NewReader(body))
	rec = execJSON(h.PutBalancerCurve, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

// TestBalancerCurvePutInvalid — все ошибки валидации §6 → 422, значение не
// сохраняется (текущая кривая остаётся дефолтом).
func TestBalancerCurvePutInvalid(t *testing.T) {
	t.Cleanup(func() { resetBalancer(t) })
	h := &AdminHandlers{}

	invalid := []string{
		`{"component":"heat","nodes":[{"x":30,"y":0},{"x":100,"y":0.5}],"bends":[0]}`,                             // < 3 узлов
		`{"component":"heat","nodes":[{"x":30,"y":0},{"x":30,"y":0.5},{"x":4000,"y":0.9}],"bends":[0,0]}`,         // X не возрастают
		`{"component":"heat","nodes":[{"x":30,"y":-0.1},{"x":100,"y":0.5},{"x":4000,"y":0.9}],"bends":[0,0]}`,     // y < 0
		`{"component":"heat","nodes":[{"x":30,"y":0},{"x":100,"y":0.5},{"x":4000,"y":0.999}],"bends":[0,0]}`,      // y ≥ 1 (гвард)
		`{"component":"heat","nodes":[{"x":30,"y":0},{"x":100,"y":0.5},{"x":4000,"y":0.9}],"bends":[0,10.5]}`,     // |k| > 10
		`{"component":"heat","nodes":[{"x":20,"y":0},{"x":100,"y":0.5},{"x":4000,"y":0.9}],"bends":[0,0]}`,        // X вне 30..4000
		`{"component":"cold","nodes":[{"x":-300,"y":0.01},{"x":-100,"y":0.001},{"x":14.85,"y":0}],"bends":[0,0]}`, // холод: X = −300 °C вне −273.15..14.85
		`{"component":"acid","nodes":[{"x":30,"y":0},{"x":100,"y":0.5},{"x":4000,"y":0.9}],"bends":[0,0]}`,        // неизвестная компонента
		`not json`, // битое тело
	}
	for _, body := range invalid {
		req := httptest.NewRequest(http.MethodPut, "/admin/balancer/curve", strings.NewReader(body))
		rec := execJSON(h.PutBalancerCurve, req)
		require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "тело: %s", body)
	}

	// Значение не изменилось: дефолт жары на месте.
	nodes, _, _ := settlement.GetCurve("heat")
	require.Len(t, nodes, 14, "невалидные PUT не трогают store")
}

func TestBalancerCurveReset(t *testing.T) {
	t.Cleanup(func() { resetBalancer(t) })
	h := &AdminHandlers{}

	// Сначала изменим кривую.
	body := `{"component":"heat","nodes":[{"x":30,"y":0},{"x":100,"y":0.5},{"x":4000,"y":0.9}],"bends":[0,0]}`
	req := httptest.NewRequest(http.MethodPut, "/admin/balancer/curve", strings.NewReader(body))
	rec := execJSON(h.PutBalancerCurve, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Reset → дефолт.
	req = httptest.NewRequest(http.MethodPost, "/admin/balancer/curve/reset", strings.NewReader(`{"component":"heat"}`))
	rec = execJSON(h.HandleBalancerCurveReset, req)
	require.Equal(t, http.StatusOK, rec.Code)
	nodes, _, _ := settlement.GetCurve("heat")
	require.Len(t, nodes, 14, "после reset — дефолт §3")

	// Неизвестная компонента → 422.
	req = httptest.NewRequest(http.MethodPost, "/admin/balancer/curve/reset", strings.NewReader(`{"component":"acid"}`))
	rec = execJSON(h.HandleBalancerCurveReset, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestBalancerCurveSample(t *testing.T) {
	t.Cleanup(func() { resetBalancer(t) })
	h := &AdminHandlers{}

	body := `{"component":"heat","xs":[30,70,4000,5000]}`
	req := httptest.NewRequest(http.MethodPost, "/admin/balancer/curve/sample", strings.NewReader(body))
	rec := execJSON(h.HandleBalancerCurveSample, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Component string `json:"component"`
		Points    []struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		} `json:"points"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "heat", resp.Component)
	require.Len(t, resp.Points, 4)
	require.Equal(t, 30.0, resp.Points[0].X)
	require.Equal(t, 0.0, resp.Points[0].Y, "R_жара(30 °C) = 0")
	require.InDelta(t, 5.94e-9, resp.Points[1].Y, 1e-10, "R_жара(70 °C) ≈ 5.94e-9")
	require.InDelta(t, 0.98, resp.Points[2].Y, 0.001, "R_жара(4000 °C) ≈ 0.98")
	// Экстраполяция за диапазон — горизонтальная: R(5000 °C) = R(4000 °C).
	require.InDelta(t, resp.Points[2].Y, resp.Points[3].Y, 1e-12, "экстраполяция вправо — константа")

	// Валидация числа точек: 1 и 501 → 422.
	req = httptest.NewRequest(http.MethodPost, "/admin/balancer/curve/sample", strings.NewReader(`{"component":"heat","xs":[30]}`))
	rec = execJSON(h.HandleBalancerCurveSample, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	xs := make([]string, 501)
	for i := range xs {
		xs[i] = "1"
	}
	req = httptest.NewRequest(http.MethodPost, "/admin/balancer/curve/sample",
		strings.NewReader(`{"component":"heat","xs":[`+strings.Join(xs, ",")+`]}`))
	rec = execJSON(h.HandleBalancerCurveSample, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestBalancerEtalons(t *testing.T) {
	h := &AdminHandlers{}
	req := httptest.NewRequest(http.MethodGet, "/admin/balancer/etalons?component=heat", nil)
	rec := execJSON(h.HandleBalancerEtalons, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Component string `json:"component"`
		Etalons   []struct {
			X     float64 `json:"x"`
			Y     float64 `json:"y"`
			Label string  `json:"label"`
		} `json:"etalons"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, "heat", resp.Component)
	require.Len(t, resp.Etalons, 5, "жара: 5 маркеров §4")
	require.Equal(t, 0.4507, resp.Etalons[2].Y, "+500 °C → 95% за секунды")

	req = httptest.NewRequest(http.MethodGet, "/admin/balancer/etalons?component=acid", nil)
	rec = execJSON(h.HandleBalancerEtalons, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestBalancerCurveMethodNotAllowed(t *testing.T) {
	h := &AdminHandlers{}
	req := httptest.NewRequest(http.MethodPost, "/admin/balancer/curve", nil)
	rec := execJSON(h.HandleBalancerCurve, req)
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/admin/balancer/curve/reset", nil)
	rec = execJSON(h.HandleBalancerCurveReset, req)
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

// ==================== Пресеты (итерация 7, спека §6) ====================

// TestBalancerPresetsHandler — GET список, POST «Сохранить как», apply,
// DELETE, reset-default, 422 (default delete / пустое имя), 404 (apply
// отсутствующего), 405 (чужие методы). Файл — во временном каталоге.
func TestBalancerPresetsHandler(t *testing.T) {
	t.Cleanup(func() { resetBalancer(t) })
	if err := settlement.LoadBalancerPresets(filepath.Join(t.TempDir(), "p.json")); err != nil {
		t.Fatalf("LoadBalancerPresets: %v", err)
	}
	h := &AdminHandlers{}

	// GET: список с default.
	req := httptest.NewRequest(http.MethodGet, "/admin/balancer/presets?component=heat", nil)
	rec := execJSON(h.GetBalancerPresets, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var list struct {
		Component string `json:"component"`
		Active    string `json:"active"`
		Presets   []struct {
			Name string `json:"name"`
		} `json:"presets"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Equal(t, "heat", list.Component)
	require.Equal(t, "default", list.Active)
	require.Len(t, list.Presets, 1)

	// POST: сохранить как (кривая = дефолт жары).
	req = httptest.NewRequest(http.MethodPost, "/admin/balancer/presets",
		strings.NewReader(`{"component":"heat","name":"жесткая-жара"}`))
	rec = execJSON(h.PostBalancerPresets, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "жесткая-жара")

	// POST: пустое имя → 422.
	req = httptest.NewRequest(http.MethodPost, "/admin/balancer/presets",
		strings.NewReader(`{"component":"heat","name":""}`))
	rec = execJSON(h.PostBalancerPresets, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// Apply: пресет применяется (ответ — кривая).
	req = httptest.NewRequest(http.MethodPost, "/admin/balancer/presets/apply",
		strings.NewReader(`{"component":"heat","name":"жесткая-жара"}`))
	rec = execJSON(h.HandleBalancerPresetsApply, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Apply отсутствующего → 404.
	req = httptest.NewRequest(http.MethodPost, "/admin/balancer/presets/apply",
		strings.NewReader(`{"component":"heat","name":"nope"}`))
	rec = execJSON(h.HandleBalancerPresetsApply, req)
	require.Equal(t, http.StatusNotFound, rec.Code)

	// DELETE default → 422.
	req = httptest.NewRequest(http.MethodDelete, "/admin/balancer/presets",
		strings.NewReader(`{"component":"heat","name":"default"}`))
	rec = execJSON(h.DeleteBalancerPresets, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// DELETE обычного → 200; повторный → 404.
	req = httptest.NewRequest(http.MethodDelete, "/admin/balancer/presets",
		strings.NewReader(`{"component":"heat","name":"жесткая-жара"}`))
	rec = execJSON(h.DeleteBalancerPresets, req)
	require.Equal(t, http.StatusOK, rec.Code)
	req = httptest.NewRequest(http.MethodDelete, "/admin/balancer/presets",
		strings.NewReader(`{"component":"heat","name":"жесткая-жара"}`))
	rec = execJSON(h.DeleteBalancerPresets, req)
	require.Equal(t, http.StatusNotFound, rec.Code)

	// reset-default → 200 (кривая = дефолт).
	req = httptest.NewRequest(http.MethodPost, "/admin/balancer/presets/reset-default",
		strings.NewReader(`{"component":"heat"}`))
	rec = execJSON(h.HandleBalancerPresetsResetDefault, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Чужие методы → 405.
	req = httptest.NewRequest(http.MethodPut, "/admin/balancer/presets", nil)
	rec = execJSON(h.HandleBalancerPresets, req)
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	req = httptest.NewRequest(http.MethodGet, "/admin/balancer/presets/apply", nil)
	rec = execJSON(h.HandleBalancerPresetsApply, req)
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
