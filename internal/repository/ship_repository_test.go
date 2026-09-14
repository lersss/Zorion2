// internal/repository/ship_repository_test.go
// Тесты репозитория каталога деталей и схемы игрока (спека 99.2.15 §2):
// UpsertPart (ON CONFLICT), ListParts/PartsByCategory/GetPart/DeletePart,
// SaveShipVisual/GetShipVisual (JSONB, NULL = «ещё не собирал»).
package repository

import (
	"encoding/json"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

func newShipRepoHarness(t *testing.T) (*ShipRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return NewShipRepository(db), mock
}

func TestUpsertPartInsert(t *testing.T) {
	r, mock := newShipRepoHarness(t)
	params := map[string]interface{}{"w": 84.0, "h": 48.0}
	paramsJSON, _ := json.Marshal(params)
	mock.ExpectExec(`INSERT INTO ship_parts \(id, category, name, svg, params, created_at\) VALUES \(\$1, \$2, \$3, \$4, \$5, NOW\(\)\) ON CONFLICT \(id\) DO UPDATE SET category = EXCLUDED.category, name = EXCLUDED.name, svg = EXCLUDED.svg, params = EXCLUDED.params`).
		WithArgs("hull_w=84_h=48", "hull", "Стрела", "<path .../>", paramsJSON).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := r.UpsertPart(&models.ShipPart{
		ID: "hull_w=84_h=48", Category: "hull", Name: "Стрела", SVG: "<path .../>", Params: params,
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListParts(t *testing.T) {
	r, mock := newShipRepoHarness(t)
	mock.ExpectQuery(`SELECT id, category, name, svg, params, created_at FROM ship_parts ORDER BY category, id`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "category", "name", "svg", "params", "created_at",
		}).AddRow("hull_a", "hull", "Стрела", "<path/>", `{"w":84}`, now()))

	parts, err := r.ListParts()
	require.NoError(t, err)
	require.Len(t, parts, 1)
	require.Equal(t, "hull_a", parts[0].ID)
	require.Equal(t, 84.0, parts[0].Params["w"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPartsByCategory(t *testing.T) {
	r, mock := newShipRepoHarness(t)
	mock.ExpectQuery(`SELECT id, category, name, svg, params, created_at FROM ship_parts WHERE category = \$1 ORDER BY id`).
		WithArgs("nose").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "category", "name", "svg", "params", "created_at",
		}).AddRow("nose_a", "nose", "Шпиль", "<path/>", `{}`, now()))

	parts, err := r.PartsByCategory("nose")
	require.NoError(t, err)
	require.Len(t, parts, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetPartNotFound(t *testing.T) {
	r, mock := newShipRepoHarness(t)
	mock.ExpectQuery(`SELECT id, category, name, svg, params, created_at FROM ship_parts WHERE id = \$1`).
		WithArgs("none").
		WillReturnRows(sqlmock.NewRows([]string{"id", "category", "name", "svg", "params", "created_at"}))

	p, err := r.GetPart("none")
	require.NoError(t, err)
	require.Nil(t, p)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeletePart(t *testing.T) {
	r, mock := newShipRepoHarness(t)
	mock.ExpectExec(`DELETE FROM ship_parts WHERE id = \$1`).
		WithArgs("hull_a").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, r.DeletePart("hull_a"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveShipVisual(t *testing.T) {
	r, mock := newShipRepoHarness(t)
	v := &models.ShipVisual{Color: "#3b82f6", Parts: map[string]string{"hull": "hull_a"}}
	data, _ := json.Marshal(v)
	mock.ExpectExec(`UPDATE users SET ship_visual = \$1, updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs(data, "u1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, r.SaveShipVisual("u1", v))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetShipVisual(t *testing.T) {
	r, mock := newShipRepoHarness(t)
	mock.ExpectQuery(`SELECT ship_visual FROM users WHERE id = \$1`).
		WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"ship_visual"}).
			AddRow(`{"color":"#3b82f6","parts":{"hull":"hull_a"}}`))

	v, err := r.GetShipVisual("u1")
	require.NoError(t, err)
	require.NotNil(t, v)
	require.Equal(t, "#3b82f6", v.Color)
	require.Equal(t, "hull_a", v.Parts["hull"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetShipVisualNull(t *testing.T) {
	r, mock := newShipRepoHarness(t)
	mock.ExpectQuery(`SELECT ship_visual FROM users WHERE id = \$1`).
		WithArgs("u1").
		WillReturnRows(sqlmock.NewRows([]string{"ship_visual"}).AddRow(nil))

	v, err := r.GetShipVisual("u1")
	require.NoError(t, err)
	require.Nil(t, v, "NULL = «ещё не собирал» (мост, спека §8)")
	require.NoError(t, mock.ExpectationsWereMet())
}