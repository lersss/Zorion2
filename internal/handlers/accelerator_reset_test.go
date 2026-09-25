// internal/handlers/accelerator_reset_test.go
// Тесты админского сброса СВОЕГО таймера ускорителя (идея ускорителя §13):
// успех (Reset вызван, 200 {ok:true}), нет репозитория → 500, нет userID в
// контексте → 401. Ролевой гейт admin + skycomposer — auth.AdminAuth
// (регистрация в cmd/server/main.go), здесь не воспроизводится.
package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/repository"
	"zorion/internal/travel"
)

const accelResetSQL = `UPDATE player_accelerator SET last_boost_at = NULL, last_cooldown_min = NULL, updated_at = NOW\(\) WHERE user_id = \$1`

func TestAcceleratorResetSelfSuccess(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	h := NewTravelHandlers(nil, nil, travel.NewManager(nil))
	h.SetAccelerator(repository.NewPlayerAcceleratorRepository(db))

	mock.ExpectExec(accelResetSQL).WithArgs("u1").WillReturnResult(sqlmock.NewResult(0, 0))

	req := withUserID(httptest.NewRequest(http.MethodPost, "/admin/accelerator/reset-self", nil), "u1")
	rec := execJSON(h.AcceleratorResetSelf, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, true, decodeMap(t, rec)["ok"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAcceleratorResetSelfMethodNotAllowed(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	h := NewTravelHandlers(nil, nil, travel.NewManager(nil))
	h.SetAccelerator(repository.NewPlayerAcceleratorRepository(db))

	// Ожидание обязано остаться невыполненным — доказывает, что Reset не вызван.
	mock.ExpectExec(accelResetSQL).WithArgs("u1").WillReturnResult(sqlmock.NewResult(0, 0))

	req := withUserID(httptest.NewRequest(http.MethodGet, "/admin/accelerator/reset-self", nil), "u1")
	rec := execJSON(h.AcceleratorResetSelf, req)
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	require.Error(t, mock.ExpectationsWereMet(), "GET не должен вызывать Reset")
}

func TestAcceleratorResetSelfNoRepo(t *testing.T) {
	h := NewTravelHandlers(nil, nil, travel.NewManager(nil))

	req := withUserID(httptest.NewRequest(http.MethodPost, "/admin/accelerator/reset-self", nil), "u1")
	rec := execJSON(h.AcceleratorResetSelf, req)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestAcceleratorResetSelfUnauthorized(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	h := NewTravelHandlers(nil, nil, travel.NewManager(nil))
	h.SetAccelerator(repository.NewPlayerAcceleratorRepository(db))

	rec := execJSON(h.AcceleratorResetSelf, httptest.NewRequest(http.MethodPost, "/admin/accelerator/reset-self", nil))
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet(), "без userID БД не трогаем")
}
