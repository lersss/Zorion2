// internal/handlers/studio_producers_test.go
// Тесты хендлеров веток «Производители»/«Предметы» студии (спека
// 2026-09-20-фабрики §4): POST /studio/api/producers, статусы, привязка
// предметов, POST /studio/api/items, статусы предметов.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/goodsstudio/ai"
)

// TestStudioCreateProducer — POST /studio/api/producers {name, kind,
// category_id, parent_id}: создание подтипа kind=goods с категорией и
// родителем (дерево построек 2026-09-21 §1.2) → 201.
func TestStudioCreateProducer(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT kind, parent_id IS NULL FROM producer_types WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id_is_null"}).AddRow("goods", true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM categories WHERE id = \$1\)`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE name_norm = \$1\)`).
		WithArgs("фабрика продовольствия").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE parent_id = \$1 AND category_id IS NOT DISTINCT FROM \$2 AND race_family IS NOT DISTINCT FROM \$3 AND race IS NOT DISTINCT FROM \$4\)`).
		WithArgs(int64(1), int64(3), nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`INSERT INTO producer_types \(name, name_norm, kind, category_id, race_family, parent_id, race, status\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, 'draft'\) RETURNING id, name, kind, category_id, race_family, parent_id, race, output, input, params, status, created_at`).
		WithArgs("Фабрика продовольствия", "фабрика продовольствия", "goods", int64(3), nil, int64(1), nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "category_id", "race_family", "parent_id", "race", "output", "input", "params", "status", "created_at"}).
			AddRow(int64(2), "Фабрика продовольствия", "goods", int64(3), nil, int64(1), nil, nil, nil, nil, "draft", time.Now()))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/producers",
		strings.NewReader(`{"name":"Фабрика продовольствия","kind":"goods","category_id":3,"parent_id":1}`))
	rec := httptest.NewRecorder()
	h.Producers(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var pv ProducerTypeView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &pv))
	require.Equal(t, "Фабрика продовольствия", pv.Name)
	require.Equal(t, "goods", pv.Kind)
	require.NotNil(t, pv.CategoryID)
	require.NotNil(t, pv.ParentID)
	require.Equal(t, int64(1), *pv.ParentID)
}

// TestStudioProducerStatus — POST /studio/api/producers/1/status {approved}.
func TestStudioProducerStatus(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE producer_types SET status = \$1 WHERE id = \$2`).
		WithArgs("approved", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/producers/1/status",
		strings.NewReader(`{"status":"approved"}`))
	rec := httptest.NewRecorder()
	h.ProducerByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioLinkItem — POST /studio/api/producers/6/items {item_id: 1}:
// привязка предмета к лаборатории (kind=items).
func TestStudioLinkItem(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT kind FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(6)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("items"))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM items WHERE id = \$1\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_items WHERE producer_type_id = \$1 AND item_id = \$2\)`).
		WithArgs(int64(6), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`INSERT INTO producer_items \(producer_type_id, item_id, requirements\) VALUES \(\$1, \$2, \$3\)`).
		WithArgs(int64(6), int64(1), nil).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/producers/6/items",
		strings.NewReader(`{"item_id":1}`))
	rec := httptest.NewRecorder()
	h.ProducerByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioLinkItemNotItems — привязка к производителю kind=goods → 400.
func TestStudioLinkItemNotItems(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT kind FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("goods"))

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/producers/2/items",
		strings.NewReader(`{"item_id":1}`))
	rec := httptest.NewRecorder()
	h.ProducerByID(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioUnlinkItem — DELETE /studio/api/producers/6/items/1.
func TestStudioUnlinkItem(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_items WHERE producer_type_id = \$1 AND item_id = \$2\)`).
		WithArgs(int64(6), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`DELETE FROM producer_items WHERE producer_type_id = \$1 AND item_id = \$2`).
		WithArgs(int64(6), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodDelete, "/studio/api/producers/6/items/1", nil)
	rec := httptest.NewRecorder()
	h.ProducerByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioCreateItem — POST /studio/api/items {name, slot_type} → 201.
func TestStudioCreateItem(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM items WHERE name_norm = \$1\)`).
		WithArgs("чертёж").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`INSERT INTO items \(name, name_norm, slot_type, status\) VALUES \(\$1, \$2, \$3, 'draft'\) RETURNING id, name, slot_type, status, unlocks, params, created_at`).
		WithArgs("Чертёж", "чертёж", "чертёж").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slot_type", "status", "unlocks", "params", "created_at"}).
			AddRow(int64(1), "Чертёж", "чертёж", "draft", nil, nil, time.Now()))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/items",
		strings.NewReader(`{"name":"Чертёж","slot_type":"чертёж"}`))
	rec := httptest.NewRecorder()
	h.Items(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var iv ItemView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &iv))
	require.Equal(t, "Чертёж", iv.Name)
	require.Equal(t, "чертёж", iv.SlotType)
}

// TestStudioItemStatus — POST /studio/api/items/1/status {banned}.
func TestStudioItemStatus(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM items WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE items SET status = 'banned' WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/items/1/status",
		strings.NewReader(`{"status":"banned"}`))
	rec := httptest.NewRecorder()
	h.ItemByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioStateProducersFields — StateView несёт producer_types/items
// (спека 2026-09-20-фабрики §4, аддитивно к iterA §7): карточка типа с
// kind/категорией/входом/выходом и привязанными предметами.
func TestStudioStateProducersFields(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, name, kind, code, is_system FROM categories ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "code", "is_system"}).
			AddRow(int64(1), "Корабли", "good", nil, false))
	mock.ExpectQuery(`SELECT id, name, category_id, kind, status, source, tier_override, banned_at, created_at, volume, weight FROM goods ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "status", "source", "tier_override", "banned_at", "created_at", "volume", "weight"}))
	mock.ExpectQuery(`SELECT good_id, pos, component_id, quantity, reason, allow_resource FROM goods_slots ORDER BY good_id, pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}))
	mock.ExpectQuery(`SELECT id, name, kind, category_id, race_family, parent_id, race, output, input, params, status, created_at FROM producer_types ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "category_id", "race_family", "parent_id", "race", "output", "input", "params", "status", "created_at"}).
			AddRow(int64(1), "Лаборатория исследовательская", "items", nil, nil, nil, nil, []byte(`{"items":["Чертёж"]}`), []byte(`{}`), []byte(`{}`), "approved", time.Now()).
			AddRow(int64(2), "Фабрика", "goods", int64(1), nil, nil, nil, nil, []byte(`{"energy": true}`), []byte(`{}`), "approved", time.Now()))
	mock.ExpectQuery(`SELECT id, name, slot_type, status, unlocks, params, created_at FROM items ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slot_type", "status", "unlocks", "params", "created_at"}).
			AddRow(int64(1), "Чертёж", "чертёж", "approved", []byte(`[]`), []byte(`{}`), time.Now()))
	mock.ExpectQuery(`SELECT producer_type_id, item_id, requirements FROM producer_items ORDER BY producer_type_id, item_id`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "item_id", "requirements"}).
			AddRow(int64(1), int64(1), nil))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodGet, "/studio/api/state", nil)
	rec := httptest.NewRecorder()
	h.State(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var view StateView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &view))
	require.Len(t, view.ProducerTypes, 2)
	require.Len(t, view.Items, 1)
	// лаборатория: kind=items + привязанный предмет
	require.Equal(t, "items", view.ProducerTypes[0].Kind)
	require.Len(t, view.ProducerTypes[0].Items, 1)
	require.Equal(t, "Чертёж", view.ProducerTypes[0].Items[0].Name)
	// фабрика: kind=goods + категория
	require.Equal(t, "goods", view.ProducerTypes[1].Kind)
	require.NotNil(t, view.ProducerTypes[1].CategoryID)
	require.Equal(t, "Корабли", view.ProducerTypes[1].CategoryName)
}