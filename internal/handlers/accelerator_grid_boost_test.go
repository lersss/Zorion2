// internal/handlers/accelerator_grid_boost_test.go
// Тесты POST /api/accelerator/boost на модели v9 «Планшет» (спека
// 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута §14): гейты, валидация
// пути, положительный и отрицательный бонус, помеха разведки, пересоздание
// задачи на новом сегменте и очистка при прибытии.
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/repository"
	"zorion/internal/routegame"
	"zorion/internal/travel"
)

// ==================== ХЕЛПЕРЫ ====================

// newAccelBoostRequest — POST /api/accelerator/boost с телом {fingerprint, path}.
func newAccelBoostRequest(userID, fingerprint string, path []int) *http.Request {
	body, err := json.Marshal(acceleratorBoostRequest{Fingerprint: fingerprint, Path: path})
	if err != nil {
		panic(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/accelerator/boost", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	return withUserID(req, userID)
}

func accelSign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	default:
		return 0
	}
}

func accelAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// accelStairCells — канонический «лестничный» путь a → b (4-связный, без обхода).
func accelStairCells(n, a, b int) []int {
	path := []int{a}
	ci, cj := a%n, a/n
	bi, bj := b%n, b/n
	for ci != bi || cj != bj {
		di, dj := bi-ci, bj-cj
		switch {
		case di == 0:
			cj += accelSign(dj)
		case dj == 0:
			ci += accelSign(di)
		case accelAbs(di) >= accelAbs(dj):
			ci += accelSign(di)
		default:
			cj += accelSign(dj)
		}
		path = append(path, cj*n+ci)
	}
	return path
}

// accelBeaconsNatural — маяки в естественном порядке (по i, затем j) — как
// трасса L_naive модели.
func accelBeaconsNatural(field routegame.GridField) []int {
	order := append([]int{}, field.Beacons...)
	sort.Slice(order, func(a, b int) bool {
		ai, aj := order[a]%field.N, order[a]/field.N
		bi, bj := order[b]%field.N, order[b]/field.N
		if ai != bi {
			return ai < bi
		}
		return aj < bj
	})
	return order
}

// accelBoostStairPath — валидный путь СТАРТ → все маяки → ФИНИШ по лестницам
// (трасса L_naive): бонус ≈ +0.10.
func accelBoostStairPath(field routegame.GridField) []int {
	path := []int{field.Start}
	prev := field.Start
	for _, b := range accelBeaconsNatural(field) {
		p := accelStairCells(field.N, prev, b)
		path = append(path, p[1:]...)
		prev = b
	}
	p := accelStairCells(field.N, prev, field.Finish)
	return append(path, p[1:]...)
}

// accelBoostBouncePrefix — дорогой префикс: маятник start↔сосед (count шагов,
// возвращается в start при чётном count). Гарантированно задирает стоимость.
func accelBoostBouncePrefix(n, start, count int) []int {
	a := start + 1
	if start%n+1 >= n {
		a = start - 1
	}
	path := []int{start}
	cur := start
	for k := 0; k < count; k++ {
		if cur == start {
			cur = a
		} else {
			cur = start
		}
		path = append(path, cur)
	}
	return path
}

// accelCellNeighbors — 4-соседи клетки (для проверки «помеха на подходе»).
func accelCellNeighbors(n, c int) []int {
	i, j := c%n, c/n
	var out []int
	if i > 0 {
		out = append(out, c-1)
	}
	if i < n-1 {
		out = append(out, c+1)
	}
	if j > 0 {
		out = append(out, c-n)
	}
	if j < n-1 {
		out = append(out, c+n)
	}
	return out
}

// accelGridFieldUnstableOnPath — секрет, поле, индекс unstable-сектора и трасса
// L_naive (масштаб +0.10), где клетка сектора (или её сосед) лежит на трассе:
// помеха задевает ровно этот путь, не уходя в клампинг нижнего пола.
func accelGridFieldUnstableOnPath(t *testing.T, flight *travel.TravelInfo) ([]byte, routegame.GridField, int, []int) {
	t.Helper()
	passport := accelGridTestPassport()
	want := routegame.GridContentUnstable.String()
	for k := 0; k < 5000; k++ {
		secret := []byte(fmt.Sprintf("accelerator-grid-unstable-%04d", k))
		field, ok := routegame.GenerateGridField(acceleratorSegmentSeed(flight), secret, 10, passport)
		if !ok {
			continue
		}
		path := accelBoostStairPath(field)
		onPath := map[int]bool{}
		for _, c := range path {
			onPath[c] = true
		}
		for i := range field.Sectors {
			c, err := routegame.RevealGridSector(secret, field.Layout(), i)
			if err != nil || c != want {
				continue
			}
			touches := false
			for _, cell := range field.Sectors[i].Cells {
				if onPath[cell] {
					touches = true
				}
				for _, nb := range accelCellNeighbors(field.N, cell) {
					if onPath[nb] {
						touches = true
					}
				}
			}
			if touches {
				return secret, field, i, path
			}
		}
	}
	t.Skip("нет unstable-сектора на трассе L_naive")
	return nil, routegame.GridField{}, 0, nil
}

// ==================== ГЕЙТЫ ====================

// Устаревший fingerprint (разворот/перебазирование) → 409 changed, Booster не вызван.
func TestAcceleratorBoostFingerprintChanged(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	old := tm.GetFlight(userID)
	b := &accelFakeBooster{applied: true}
	tm.SetBooster(b)

	rec := execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, "w1:w2:0", nil))
	require.Equal(t, http.StatusConflict, rec.Code)
	m := decodeMap(t, rec)
	require.Equal(t, "changed", m["reason"])
	require.Equal(t, 0, b.calls)
	require.Same(t, old, tm.GetFlight(userID), "сегмент не менялся")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Ускорение уже действует на сегменте → 409 already_active без списания.
func TestAcceleratorBoostAlreadyActive(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)
	b := &accelFakeBooster{applied: true}
	tm.SetBooster(b)

	expectAccelState(mock, userID, flight.StartTime, 25)
	expectAccelGameUser(mock, userID, "w1")

	rec := execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, acceleratorFingerprint(flight), nil))
	require.Equal(t, http.StatusConflict, rec.Code)
	m := decodeMap(t, rec)
	require.Equal(t, "already_active", m["reason"])
	require.Equal(t, 0, b.calls)
	require.Same(t, flight, tm.GetFlight(userID))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Остаток ниже порога ОТПРАВКИ → 409 too_short без списания (мир-цель уже загружен).
func TestAcceleratorBoostTooShort(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, 60*time.Second, nil)
	flight := tm.GetFlight(userID)
	b := &accelFakeBooster{applied: true}
	tm.SetBooster(b)

	expectAccelState(mock, userID, nil, nil)
	expectAccelGameUser(mock, userID, "w1")
	expectWorld(mock, "w2", 10, 0)

	rec := execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, acceleratorFingerprint(flight), nil))
	require.Equal(t, http.StatusConflict, rec.Code)
	m := decodeMap(t, rec)
	require.Equal(t, "too_short", m["reason"])
	require.Equal(t, 0, b.calls)
	require.Same(t, flight, tm.GetFlight(userID))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Модуля нет → 409 no_module (доступность не нужна, но сегмент не трогаем).
func TestAcceleratorBoostNoModule(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)
	b := &accelFakeBooster{applied: true}
	tm.SetBooster(b)

	expectAccelState(mock, userID, nil, nil)
	expectUser(mock, userID, "w1") // оборудование без ускорителя

	rec := execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, acceleratorFingerprint(flight), nil))
	require.Equal(t, http.StatusConflict, rec.Code)
	m := decodeMap(t, rec)
	require.Equal(t, "no_module", m["reason"])
	require.Equal(t, 0, b.calls)
	require.Same(t, flight, tm.GetFlight(userID))
	require.NoError(t, mock.ExpectationsWereMet())
}

// Мир-цель съеден пакманом → 404 без списания.
func TestAcceleratorBoostTargetEaten(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)
	b := &accelFakeBooster{applied: true}
	tm.SetBooster(b)

	expectAccelState(mock, userID, nil, nil)
	expectAccelGameUser(mock, userID, "w1")
	expectWorldMissing(mock, "w2")

	rec := execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, acceleratorFingerprint(flight), nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, 0, b.calls)
	require.Same(t, flight, tm.GetFlight(userID))
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== ПУТЬ: НЕВАЛИДНЫЕ ВЕТКИ ====================

// Путь по доске v9 — невалидные ветки → 400 с кодом причины, без списания.
func TestAcceleratorBoostInvalidPathReasons(t *testing.T) {
	cases := []struct {
		name string
		path func(field routegame.GridField) []int
		want string
	}{
		{"too_few", func(f routegame.GridField) []int { return []int{f.Start} }, routegame.GridReasonTooFewCells},
		{"out_of_bounds", func(f routegame.GridField) []int { return []int{f.Start, f.N * f.N} }, routegame.GridReasonCellOutOfBounds},
		{"start_mismatch", func(f routegame.GridField) []int { return []int{f.Finish, f.Start} }, routegame.GridReasonStartMismatch},
		{"finish_mismatch", func(f routegame.GridField) []int { return []int{f.Start, f.Start} }, routegame.GridReasonFinishMismatch},
		{"not_adjacent", func(f routegame.GridField) []int { return []int{f.Start, f.Finish} }, routegame.GridReasonNotAdjacent},
		{"beacon_missing", func(f routegame.GridField) []int { return accelStairCells(f.N, f.Start, f.Finish) }, routegame.GridReasonBeaconMissing},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, tm, mock := newAccelGameHarness(t)
			const userID = "u1"
			tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
			flight := tm.GetFlight(userID)
			b := &accelFakeBooster{applied: true}
			tm.SetBooster(b)

			secret := []byte("accelerator-grid-test-secret-0001")
			field := accelGridTestField(t, flight, secret, 10, accelGridTestPassport())
			layoutJSON := accelGridTestLayoutJSON(t, field)
			expectScanSegmentQueries(t, mock, userID, secret, layoutJSON, []byte("[]"), 2, flight)

			rec := execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, acceleratorFingerprint(flight), tc.path(field)))
			require.Equal(t, http.StatusBadRequest, rec.Code)
			m := decodeMap(t, rec)
			require.Equal(t, false, m["available"])
			require.Equal(t, tc.want, m["reason"])
			require.Equal(t, 0, b.calls)
			require.Same(t, flight, tm.GetFlight(userID))
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// ==================== БОНУС ====================

// Положительный бонус: путь по маякам (трасса L_naive) → сегмент сокращён.
func TestAcceleratorBoostPositiveBonus(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)
	b := &accelFakeBooster{applied: true}
	tm.SetBooster(b)

	secret := []byte("accelerator-grid-test-secret-0001")
	field := accelGridTestField(t, flight, secret, 10, accelGridTestPassport())
	layoutJSON := accelGridTestLayoutJSON(t, field)
	expectScanSegmentQueries(t, mock, userID, secret, layoutJSON, []byte("[]"), 2, flight)

	rec := execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, acceleratorFingerprint(flight), accelBoostStairPath(field)))
	require.Equal(t, http.StatusOK, rec.Code)
	var resp acceleratorBoostResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Greater(t, resp.Bonus, 0.0, "ленивый путь ускоряет")
	require.LessOrEqual(t, resp.Bonus, 0.50)
	require.Greater(t, resp.RemainingS, 0)
	require.Less(t, resp.RemainingS, 3600, "сегмент сокращён")

	boosted := tm.GetFlight(userID)
	require.NotNil(t, boosted)
	require.Less(t, boosted.Duration, time.Hour, "остаток сегмента уменьшился")
	require.Equal(t, 1, b.calls)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Отрицательный бонус: длинный путь (маятник + лестница) → перелёт УДЛИНЯЕТСЯ.
func TestAcceleratorBoostNegativeBonusLengthens(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)
	b := &accelFakeBooster{applied: true}
	tm.SetBooster(b)

	secret := []byte("accelerator-grid-test-secret-0001")
	field := accelGridTestField(t, flight, secret, 10, accelGridTestPassport())
	layoutJSON := accelGridTestLayoutJSON(t, field)
	expectScanSegmentQueries(t, mock, userID, secret, layoutJSON, []byte("[]"), 2, flight)

	path := append(accelBoostBouncePrefix(field.N, field.Start, 400), accelBoostStairPath(field)[1:]...)
	rec := execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, acceleratorFingerprint(flight), path))
	require.Equal(t, http.StatusOK, rec.Code)
	var resp acceleratorBoostResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Less(t, resp.Bonus, 0.0, "длинный путь замедляет")
	require.GreaterOrEqual(t, resp.Bonus, -0.20-1e-9, "низ шкалы §14.2")
	require.Greater(t, resp.RemainingS, 4000, "перелёт стал длиннее")

	boosted := tm.GetFlight(userID)
	require.NotNil(t, boosted)
	require.Greater(t, boosted.Duration, time.Hour, "остаток сегмента вырос")
	require.NoError(t, mock.ExpectationsWereMet())
}

// Помеха разведки: вскрытый unstable-сектор на пути → бонус ниже (ping_destabilize).
func TestAcceleratorBoostDisturbanceLowersBonus(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)
	b := &accelFakeBooster{applied: true}
	tm.SetBooster(b)

	secret, field, idx, path := accelGridFieldUnstableOnPath(t, flight)
	layoutJSON := accelGridTestLayoutJSON(t, field)

	undisturbed, ok, _ := routegame.EvaluateGridPath(field, path)
	require.True(t, ok)
	disturbed, ok2, _ := routegame.EvaluateGridPath(routegame.DestabilizeGridField(field, []int{idx}), path)
	require.True(t, ok2)
	require.Less(t, disturbed, undisturbed, "помеха должна понижать бонус")

	revealed := []byte(fmt.Sprintf(`[{"sector":%d,"content":"unstable"}]`, idx))
	expectScanSegmentQueries(t, mock, userID, secret, layoutJSON, revealed, 1, flight)

	rec := execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, acceleratorFingerprint(flight), path))
	require.Equal(t, http.StatusOK, rec.Code)
	var resp acceleratorBoostResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.InDelta(t, disturbed, resp.Bonus, 1e-9, "помеха учтена ровно как в модели")
	require.NoError(t, mock.ExpectationsWereMet())
}

// ==================== ЖИЗНЕННЫЙ ЦИКЛ ====================

// Старая задача другого сегмента не влияет: boost пересоздаёт поле под текущий
// segment_hash (импульсы и revealed сброшены), затем валидирует путь.
func TestAcceleratorBoostRecreatesStalePuzzle(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)

	expectAccelState(mock, userID, nil, nil)
	expectAccelGameUser(mock, userID, "w1")
	expectWorld(mock, "w2", 10, 0)
	expectWorld(mock, "w1", 0, 0)
	expectBelts(mock, "w2")
	canonSecret := []byte("accelerator-grid-test-secret-0001")
	canonField := accelGridTestField(t, flight, canonSecret, 10, accelGridTestPassport())
	expectRoutePuzzle(mock, userID, []byte("stale-segment-hash"), []byte("old-secret"), []byte("{}"),
		[]byte(`[{"sector":1,"content":"unstable"}]`), 0)
	expectRoutePuzzleEnsure(mock, userID, acceleratorSegmentHash(flight), canonSecret, accelGridTestLayoutJSON(t, canonField), []byte("[]"), acceleratorGridPings)

	rec := execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, acceleratorFingerprint(flight), []int{0}))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	m := decodeMap(t, rec)
	require.Equal(t, routegame.GridReasonTooFewCells, m["reason"])
	require.NoError(t, mock.ExpectationsWereMet())
}

// Прибытие снимает задачу сегмента (player_route_puzzle) — старая доска не
// переходит на новый сегмент.
func TestArrivalHandlerClearsRoutePuzzle(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	tm := travel.NewManager(nil)
	h := NewTravelHandlers(repository.NewWorldRepository(db), repository.NewUserRepository(db), tm)
	h.SetRoutePuzzle(repository.NewRoutePuzzleRepository(db))

	const userID = "11111111-1111-1111-1111-111111111111"
	const target = "w2"
	mock.ExpectExec(`DELETE FROM player_route_puzzle WHERE user_id = \$1 AND kind = \$2`).
		WithArgs(userID, "route").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectArrivalBase(mock, userID, target, 10, 0)
	expectPendingDestination(mock, userID, `{"world_id":"w9","object_type":"planet","object_id":"p1"}`)
	expectClearPendingDestination(mock, userID)

	h.ArrivalHandler(userID, target)
	require.NoError(t, mock.ExpectationsWereMet())
}

// Анти-бот (§6.6): сверх частоты boost отвечает 429 rate_limited.
func TestAcceleratorBoostRateLimited(t *testing.T) {
	h, _, mock := newAccelGameHarness(t)
	const userID = "u1"
	h.accelRate = newAcceleratorRateLimiter(0, 1)

	rec := execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, "x", nil))
	require.Equal(t, http.StatusConflict, rec.Code)
	require.Equal(t, "no_flight", decodeMap(t, rec)["reason"], "первый запрос лимитер пропускает")

	rec = execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, "x", nil))
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	m := decodeMap(t, rec)
	require.Equal(t, "rate_limited", m["reason"])
	require.NoError(t, mock.ExpectationsWereMet())
}
