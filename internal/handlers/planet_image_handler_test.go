// internal/handlers/planet_image_handler_test.go
//
// Тесты авторизованного эндпоинта картинки планеты (спека 2026-09-20 §8.2):
// режимы видимости (честная/заглушка), 401 без JWT, 404 неизвестной планеты.
package handlers

import (
	"bytes"
	"database/sql"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/auth"
	"zorion/internal/generator/planet"
	"zorion/internal/models"
	"zorion/internal/repository"
)

// planetImageHarness — PlanetImageHandler с sqlmock-БД (без диск-кэша).
func planetImageHarness(t *testing.T) (*PlanetImageHandler, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	h := NewPlanetImageHandler(
		repository.NewPlanetRepository(db),
		repository.NewUserRepository(db),
		repository.NewKnowledgeRepository(db),
		"",
	)
	return h, mock
}

// planetImageRow — строка планеты для sqlmock (без поселений).
func planetImageRow(id, worldID, data string) *sqlmock.Rows {
	now := time.Now()
	return sqlmock.NewRows([]string{"id", "world_id", "name", "orbit_index", "data", "created_at", "updated_at"}).
		AddRow(id, worldID, "Планета", 1, data, now, now)
}

// expectPlanetImageFetch — ожидания GetPlanetByID: планета + поселения (пусто).
func expectPlanetImageFetch(mock sqlmock.Sqlmock, id, worldID, data string) {
	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(planetImageRow(id, worldID, data))
	mock.ExpectQuery(`FROM settlements WHERE planet_id = ANY\(\$1\) ORDER BY created_at ASC`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id"}))
}

// expectUserFetch — ожидания GetByID пользователя.
func expectUserFetch(mock sqlmock.Sqlmock, id, world string) {
	mock.ExpectQuery(`SELECT id, username, password_hash, email, agent_id, current_world_id, ship_icon, ship_color, ship_model_id, equipment, role, created_at, updated_at FROM users WHERE id = \$1`).
		WithArgs(id).
		WillReturnRows(visUserRow(id, world, `{}`))
}

// expectNoKnowledge — знание о планете отсутствует.
func expectNoKnowledge(mock sqlmock.Sqlmock, userID, planetID string) {
	mock.ExpectQuery(`SELECT user_id, planet_id, data, scanned_at, source FROM player_planet_knowledge WHERE user_id = \$1 AND planet_id = \$2`).
		WithArgs(userID, planetID).
		WillReturnError(sql.ErrNoRows)
}

// ==================== №5: РЕЖИМЫ ВИДИМОСТИ ====================

func TestResolveMode(t *testing.T) {
	h, mock := planetImageHarness(t)
	p := &models.Planet{ID: "p1", WorldID: "w1"}

	// admin / skycomposer → честная всегда (И7).
	req := httptest.NewRequest(http.MethodGet, "/api/planet-image?planet_id=p1", nil)
	req = withRole(req, "admin")
	require.Equal(t, planet.ImageModeHonest, h.resolveMode(req, p))
	req = withRole(req, "skycomposer")
	require.Equal(t, planet.ImageModeHonest, h.resolveMode(req, p))

	// player в своей системе (current_world_id == world_id) → честная.
	req = withRole(req, "player")
	req = withUserID(req, "u1")
	expectUserFetch(mock, "u1", "w1")
	require.Equal(t, planet.ImageModeHonest, h.resolveMode(req, p))
	require.NoError(t, mock.ExpectationsWereMet())

	// player в чужой системе без знания → заглушка.
	expectUserFetch(mock, "u1", "w2")
	expectNoKnowledge(mock, "u1", "p1")
	require.Equal(t, planet.ImageModeStub, h.resolveMode(req, p))
	require.NoError(t, mock.ExpectationsWereMet())

	// player в чужой системе со знанием (любая запись, включая протухшую) → честная.
	expectUserFetch(mock, "u1", "w2")
	mock.ExpectQuery(`SELECT user_id, planet_id, data, scanned_at, source FROM player_planet_knowledge WHERE user_id = \$1 AND planet_id = \$2`).
		WithArgs("u1", "p1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "planet_id", "data", "scanned_at", "source"}).
			AddRow("u1", "p1", `{"surface_dominant":"горы"}`, now(), "scanner"))
	require.Equal(t, planet.ImageModeHonest, h.resolveMode(req, p))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPlanetImageNoJWT(t *testing.T) {
	h, _ := planetImageHarness(t)
	// Эндпоинт авторизован: без JWT — 401 (AuthMiddleware на роуте).
	handler := auth.AuthMiddleware(h.ServeHTTP)
	req := httptest.NewRequest(http.MethodGet, "/api/planet-image?planet_id=p1", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestPlanetImageNotFound(t *testing.T) {
	h, mock := planetImageHarness(t)
	req := httptest.NewRequest(http.MethodGet, "/api/planet-image?planet_id=missing", nil)
	req = withRole(req, "admin")
	req = withUserID(req, "u1")

	mock.ExpectQuery(`SELECT id, world_id, name, orbit_index, data, created_at, updated_at FROM planets WHERE id = \$1`).
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	rec := execJSON(h.ServeHTTP, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPlanetImageStubForForeign(t *testing.T) {
	h, mock := planetImageHarness(t)
	req := httptest.NewRequest(http.MethodGet, "/api/planet-image?planet_id=p1&size=small", nil)
	req = withRole(req, "player")
	req = withUserID(req, "u1")

	// Чужая система без знания → заглушка (200, валидный PNG).
	expectPlanetImageFetch(mock, "p1", "w1", `{"temperature":288,"water_percent":60,"life":true}`)
	expectUserFetch(mock, "u1", "w2")
	expectNoKnowledge(mock, "u1", "p1")

	rec := execJSON(h.ServeHTTP, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "image/png", rec.Header().Get("Content-Type"))
	_, err := png.Decode(bytes.NewReader(rec.Body.Bytes()))
	require.NoError(t, err, "ответ — валидный PNG")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPlanetImageHonestForOwn(t *testing.T) {
	h, mock := planetImageHarness(t)
	req := httptest.NewRequest(http.MethodGet, "/api/planet-image?planet_id=p1&size=small", nil)
	req = withRole(req, "player")
	req = withUserID(req, "u1")

	// Своя система → честная (200, валидный PNG).
	expectPlanetImageFetch(mock, "p1", "w1",
		`{"temperature":288,"water_percent":60,"life":true,"biomes":[{"form":"горы","share":40},{"form":"океаны","share":35},{"form":"леса","share":25}]}`)
	expectUserFetch(mock, "u1", "w1")

	rec := execJSON(h.ServeHTTP, req)
	require.Equal(t, http.StatusOK, rec.Code)
	_, err := png.Decode(bytes.NewReader(rec.Body.Bytes()))
	require.NoError(t, err, "ответ — валидный PNG")
	require.NoError(t, mock.ExpectationsWereMet())
}