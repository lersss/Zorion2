// internal/handlers/accelerator_grid_breakdown_test.go
// Контракт «разбора по факторам» (§14.13.5): поле `breakdown` аддитивно в
// ответе boost (всегда присутствует) и НЕ утекает в offer/scan до отправки.
package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/routegame"
)

// offer: поля breakdown нет (разбор только после необратимого boost).
func TestAcceleratorOfferNoBreakdownField(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)
	secret := []byte("accelerator-grid-test-secret-0001")
	field := accelGridTestField(t, flight, secret, 10, accelGridTestPassport())
	layoutJSON := accelGridTestLayoutJSON(t, field)

	expectScanSegmentQueries(t, mock, userID, secret, layoutJSON, []byte("[]"), 2, flight)

	rec := execJSON(h.AcceleratorOffer, accelOfferRequest(userID))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "breakdown", "offer не отдаёт разбор до отправки")
	require.NoError(t, mock.ExpectationsWereMet())
}

// scan: поля breakdown нет.
func TestAcceleratorScanNoBreakdownField(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)
	secret := []byte("accelerator-grid-test-secret-0001")
	field := accelGridTestField(t, flight, secret, 10, accelGridTestPassport())
	layoutJSON := accelGridTestLayoutJSON(t, field)
	sector := 0
	wantContent, err := routegame.RevealGridSector(secret, field.Layout(), sector)
	require.NoError(t, err)
	contentArg, err := json.Marshal(wantContent)
	require.NoError(t, err)

	expectScanSegmentQueries(t, mock, userID, secret, layoutJSON, []byte("[]"), 2, flight)
	mock.ExpectQuery(`UPDATE player_route_puzzle`).
		WithArgs(userID, "route", sector, string(contentArg)).
		WillReturnRows(sqlmock.NewRows([]string{"revealed", "pings_left"}).
			AddRow([]byte(`[{"sector":0,"content":`+string(contentArg)+`}]`), 1))

	rec := execJSON(h.AcceleratorScan, newAccelScanRequest(userID, acceleratorFingerprint(flight), sector))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), "breakdown", "scan не отдаёт разбор до отправки")
	require.NoError(t, mock.ExpectationsWereMet())
}

// boost: поле breakdown всегда присутствует; путь-маятник даёт сработавшие
// факторы, severity > 0 только у ошибок.
func TestAcceleratorBoostBreakdownField(t *testing.T) {
	h, tm, mock := newAccelGameHarness(t)
	const userID = "u1"
	tm.StartFlight(userID, "w1", "w2", 0, 0, time.Hour, nil)
	flight := tm.GetFlight(userID)
	tm.SetBooster(&accelFakeBooster{applied: true})

	secret := []byte("accelerator-grid-test-secret-0001")
	field := accelGridTestField(t, flight, secret, 10, accelGridTestPassport())
	layoutJSON := accelGridTestLayoutJSON(t, field)
	expectScanSegmentQueries(t, mock, userID, secret, layoutJSON, []byte("[]"), 2, flight)

	path := append(accelBoostBouncePrefix(field.N, field.Start, 400), accelBoostStairPath(field)[1:]...)
	rec := execJSON(h.AcceleratorBoost, newAccelBoostRequest(userID, acceleratorFingerprint(flight), path))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "\"breakdown\"")

	var resp acceleratorBoostResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Breakdown, "breakdown присутствует всегда (пустой — [])")
	require.NotEmpty(t, resp.Breakdown, "маятник даёт сработавшие факторы")
	sawError := false
	for _, f := range resp.Breakdown {
		require.NotEmpty(t, f.Code)
		switch f.Group {
		case "error":
			sawError = true
			require.GreaterOrEqual(t, f.Severity, 1)
			require.LessOrEqual(t, f.Severity, 4)
		default:
			require.Equal(t, 0, f.Severity, "gain/neutral: severity 0")
		}
		require.Positive(t, f.Count)
		require.LessOrEqual(t, len(f.Cells), 16)
	}
	require.True(t, sawError, "маятник должен дать ошибки")
	require.NoError(t, mock.ExpectationsWereMet())
}
