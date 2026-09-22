// internal/handlers/admin_balancer_hunger_test.go
// Тесты этапа 3 (T20/T21/T30): компонента hunger в админ-ручках «Балансировки»
// — GET/PUT несут скаляр recovery, PUT пишет кривую+скаляр атомарно, reset
// сбрасывает и скаляр, sample/etalons работают (эталонов нет — пустой список),
// расовые ручки и Sample?race_id= hunger → 422 (не 500).
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

// hungerCurveJSON — валидная кривая hunger (первый узел Y = 0, X в [0,8760]).
const hungerCurveJSON = `{"component":"hunger","nodes":[{"x":0,"y":0},{"x":24,"y":0},{"x":456,"y":0.0000001}],"bends":[-1,-0.2]}`

// T20: GET/PUT/reset hunger несут скаляр recovery; PUT пишет оба; reset сбрасывает оба.
func TestBalancerHungerCurveAndScalar(t *testing.T) {
	t.Cleanup(func() { resetBalancer(t) })
	h := &AdminHandlers{}

	// GET: кривая + скаляр (заводское 0.25).
	rec := execJSON(h.GetBalancerCurve, httptest.NewRequest(http.MethodGet, "/admin/balancer/curve?component=hunger", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var got struct {
		Component string   `json:"component"`
		Recovery  *float64 `json:"recovery"`
		Nodes     []struct {
			X float64 `json:"x"`
		} `json:"nodes"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "hunger", got.Component)
	require.NotNil(t, got.Recovery, "у hunger поле recovery присутствует всегда")
	require.Equal(t, 0.25, *got.Recovery)
	require.Len(t, got.Nodes, 8)

	// PUT: кривая + скаляр за один вызов.
	putBody := strings.Replace(hungerCurveJSON, `"component":"hunger"`, `"component":"hunger","recovery":0.4`, 1)
	rec = execJSON(h.PutBalancerCurve, httptest.NewRequest(http.MethodPut, "/admin/balancer/curve", strings.NewReader(putBody)))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"recovery":0.4`)

	// GET отражает оба.
	rec = execJSON(h.GetBalancerCurve, httptest.NewRequest(http.MethodGet, "/admin/balancer/curve?component=hunger", nil))
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.NotNil(t, got.Recovery)
	require.Equal(t, 0.4, *got.Recovery)
	require.Len(t, got.Nodes, 3)

	// PUT с отрицательным recovery → 422; ни скаляр, ни кривая не изменились.
	negBody := strings.Replace(hungerCurveJSON, `"component":"hunger"`, `"component":"hunger","recovery":-0.1`, 1)
	rec = execJSON(h.PutBalancerCurve, httptest.NewRequest(http.MethodPut, "/admin/balancer/curve", strings.NewReader(negBody)))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	v, ok := settlement.ComponentScalar(settlement.HungerCurveKey)
	require.True(t, ok)
	require.Equal(t, 0.4, v, "отрицательный recovery не должен менять store")

	// PUT recovery на не-эффект-компоненту (heat) → 422.
	heatBody := `{"component":"heat","nodes":[{"x":30,"y":0},{"x":100,"y":0.5},{"x":4000,"y":0.9}],"bends":[0,0],"recovery":0.5}`
	rec = execJSON(h.PutBalancerCurve, httptest.NewRequest(http.MethodPut, "/admin/balancer/curve", strings.NewReader(heatBody)))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// У heat `recovery` в ответе отсутствует.
	rec = execJSON(h.GetBalancerCurve, httptest.NewRequest(http.MethodGet, "/admin/balancer/curve?component=heat", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), `"recovery"`)

	// reset сбрасывает и кривую, и скаляр.
	rec = execJSON(h.HandleBalancerCurveReset, httptest.NewRequest(http.MethodPost, "/admin/balancer/curve/reset", strings.NewReader(`{"component":"hunger"}`)))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"recovery":0.25`)
	v, _ = settlement.ComponentScalar(settlement.HungerCurveKey)
	require.Equal(t, 0.25, v)
	nodes, _, _ := settlement.GetCurve(settlement.HungerCurveKey)
	require.Len(t, nodes, 8, "после reset — дефолтная кривая hunger")
}

// T20: sample/etalons работают для hunger; эталонов нет — пустой список (не null).
func TestBalancerHungerSampleAndEtalons(t *testing.T) {
	t.Cleanup(func() { resetBalancer(t) })
	h := &AdminHandlers{}

	rec := execJSON(h.HandleBalancerCurveSample,
		httptest.NewRequest(http.MethodPost, "/admin/balancer/curve/sample", strings.NewReader(`{"component":"hunger","xs":[0,24,100,9000]}`)))
	require.Equal(t, http.StatusOK, rec.Code)
	var sample struct {
		Points []struct {
			Y float64 `json:"y"`
		} `json:"points"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &sample))
	require.Len(t, sample.Points, 4)
	require.Equal(t, 0.0, sample.Points[0].Y, "R(0) = 0 (нулевой префикс)")
	require.Equal(t, 0.0, sample.Points[1].Y, "R(24) = 0 (порог)")
	require.Greater(t, sample.Points[2].Y, 0.0, "R(100) > 0 (выше порога)")

	rec = execJSON(h.HandleBalancerEtalons, httptest.NewRequest(http.MethodGet, "/admin/balancer/etalons?component=hunger", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"etalons":[]`, "у hunger эталонов нет — пустой список, не null")
}

// T20: пресеты hunger несут recovery — apply/reset-default возвращают скаляр.
func TestBalancerHungerPresetsRecovery(t *testing.T) {
	t.Cleanup(func() { resetBalancer(t) })
	require.NoError(t, settlement.LoadBalancerPresets(filepath.Join(t.TempDir(), "p.json")))
	h := &AdminHandlers{}

	// PUT кривой + скаляра, затем «Сохранить как пресет».
	putBody := strings.Replace(hungerCurveJSON, `"component":"hunger"`, `"component":"hunger","recovery":0.35`, 1)
	rec := execJSON(h.PutBalancerCurve, httptest.NewRequest(http.MethodPut, "/admin/balancer/curve", strings.NewReader(putBody)))
	require.Equal(t, http.StatusOK, rec.Code)

	rec = execJSON(h.PostBalancerPresets, httptest.NewRequest(http.MethodPost, "/admin/balancer/presets",
		strings.NewReader(`{"component":"hunger","name":"сухо"}`)))
	require.Equal(t, http.StatusOK, rec.Code)

	// reset-default отдаёт заводской скаляр.
	rec = execJSON(h.HandleBalancerPresetsResetDefault, httptest.NewRequest(http.MethodPost, "/admin/balancer/presets/reset-default",
		strings.NewReader(`{"component":"hunger"}`)))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"recovery":0.25`)

	// apply возвращает восстановленный скаляр 0.35.
	rec = execJSON(h.HandleBalancerPresetsApply, httptest.NewRequest(http.MethodPost, "/admin/balancer/presets/apply",
		strings.NewReader(`{"component":"hunger","name":"сухо"}`)))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"recovery":0.35`)
}

// T21: расовые ручки hunger не принимают — 422 (не 500), включая Sample?race_id=.
func TestBalancerRaceRejectsHunger(t *testing.T) {
	require.NoError(t, settlement.LoadRaceBalancer(filepath.Join(t.TempDir(), "race_balancer.json")))
	h := &AdminHandlers{}
	const race = "ammonia"

	rec := execJSON(h.GetRaceBalancerCurve, httptest.NewRequest(http.MethodGet,
		"/admin/race-balancer/curve?race_id="+race+"&component=hunger", nil))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	rec = execJSON(h.PutRaceBalancerCurve, httptest.NewRequest(http.MethodPut,
		"/admin/race-balancer/curve?race_id="+race+"&component=hunger", strings.NewReader(`{"nodes":[],"bends":[]}`)))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	rec = execJSON(h.HandleRaceBalancerFactory, httptest.NewRequest(http.MethodGet,
		"/admin/race-balancer/factory?race_id="+race+"&component=hunger", nil))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// Sample с race_id и component=hunger → 422 (иначе nil-кривая → 500).
	rec = execJSON(h.HandleBalancerCurveSample, httptest.NewRequest(http.MethodPost,
		"/admin/balancer/curve/sample?race_id="+race, strings.NewReader(`{"component":"hunger","xs":[0,10]}`)))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// Расовый PUT с полем recovery → 422 (скаляр — глобальной компоненты).
	rec = execJSON(h.PutRaceBalancerCurve, httptest.NewRequest(http.MethodPut,
		"/admin/race-balancer/curve?race_id="+race+"&component=heat",
		strings.NewReader(`{"nodes":[{"x":0,"y":0},{"x":100,"y":0.5},{"x":4000,"y":0.9}],"bends":[0,0],"recovery":0.5}`)))
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}
