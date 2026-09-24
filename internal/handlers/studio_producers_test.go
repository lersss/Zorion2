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
	// суженный С4 (итерация 4 §5.1): у родителя есть слоты
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	// слот-инвариант С4: применяемый слот родителя
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1 AND category_id = \$2 AND \(race_family IS NULL OR \(race_family = \$3 AND race IS NULL\) OR race = \$4\)\)`).
		WithArgs(int64(1), int64(3), nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE name_norm = \$1\)`).
		WithArgs("фабрика продовольствия").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE parent_id = \$1 AND category_id IS NOT DISTINCT FROM \$2 AND race_family IS NOT DISTINCT FROM \$3 AND race IS NOT DISTINCT FROM \$4\)`).
		WithArgs(int64(1), int64(3), nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`INSERT INTO producer_types \(name, name_norm, kind, category_id, race_family, parent_id, race\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7\) RETURNING id, name, kind, category_id, race_family, parent_id, race, output, input, params, hidden, created_at`).
		WithArgs("Фабрика продовольствия", "фабрика продовольствия", "goods", int64(3), nil, int64(1), nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "category_id", "race_family", "parent_id", "race", "output", "input", "params", "hidden", "created_at"}).
			AddRow(int64(2), "Фабрика продовольствия", "goods", int64(3), nil, int64(1), nil, nil, nil, nil, false, time.Now()))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
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

// TestStudioProducerHidden — POST /studio/api/producers/1/hidden {hidden:true}:
// единственный носитель скрытия (запись-фабрика).
func TestStudioProducerHidden(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE producer_types SET hidden = \$1 WHERE id = \$2`).
		WithArgs(true, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/producers/1/hidden",
		strings.NewReader(`{"hidden":true}`))
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

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
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

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
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

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
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
	mock.ExpectQuery(`INSERT INTO items \(name, name_norm, slot_type\) VALUES \(\$1, \$2, \$3\) RETURNING id, name, slot_type, unlocks, params, created_at`).
		WithArgs("Чертёж", "чертёж", "чертёж").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slot_type", "unlocks", "params", "created_at"}).
			AddRow(int64(1), "Чертёж", "чертёж", nil, nil, time.Now()))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
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
	mock.ExpectQuery(`SELECT g.id, g.name, g.category_id, g.kind, g.source, r.id, r.complexity, g.created_at, g.volume, g.weight, g.description, g.code FROM goods g LEFT JOIN recipes r ON r.good_id = g.id ORDER BY g.id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "source", "recipe_id", "complexity", "created_at", "volume", "weight", "description", "code"}))
	mock.ExpectQuery(`SELECT r\.good_id, c\.pos, c\.component_id, c\.quantity, c\.reason, c\.allow_resource FROM recipe_components c JOIN recipes r ON r\.id = c\.recipe_id ORDER BY r\.good_id, c\.pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}))
	mock.ExpectQuery(`SELECT id, name, kind, category_id, race_family, parent_id, race, output, input, params, hidden, created_at, code FROM producer_types ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "category_id", "race_family", "parent_id", "race", "output", "input", "params", "hidden", "created_at", "code"}).
			AddRow(int64(1), "Лаборатория исследовательская", "items", nil, nil, nil, nil, []byte(`{"items":["Чертёж"]}`), []byte(`{}`), []byte(`{}`), false, time.Now(), "p_0001").
			AddRow(int64(2), "Фабрика", "goods", int64(1), nil, nil, nil, nil, []byte(`{"energy": true}`), []byte(`{}`), false, time.Now(), "p_0002"))
	mock.ExpectQuery(`SELECT id, name, slot_type, unlocks, params, created_at, code FROM items ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slot_type", "unlocks", "params", "created_at", "code"}).
			AddRow(int64(1), "Чертёж", "чертёж", []byte(`[]`), []byte(`{}`), time.Now(), "i_0001"))
	mock.ExpectQuery(`SELECT producer_type_id, item_id, requirements FROM producer_items ORDER BY producer_type_id, item_id`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "item_id", "requirements"}).
			AddRow(int64(1), int64(1), nil))
	mock.ExpectQuery(`SELECT id, parent_id, category_id, race_family, race, hidden, created_at FROM producer_slots ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id", "category_id", "race_family", "race", "hidden", "created_at"}).
			AddRow(int64(5), int64(2), int64(1), nil, nil, false, time.Now()))
	mock.ExpectQuery(`SELECT pr.producer_type_id, pr.recipe_id, pr.rate, r.good_id FROM producer_recipes pr JOIN recipes r ON r.id = pr.recipe_id ORDER BY pr.producer_type_id, pr.recipe_id`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "recipe_id", "rate", "good_id"}))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodGet, "/studio/api/state", nil)
	rec := httptest.NewRecorder()
	h.State(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var view StateView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &view))
	require.Len(t, view.ProducerTypes, 2)
	require.Len(t, view.Items, 1)
	// лаборатория: kind=items + привязанный предмет; hidden — носитель скрытия
	require.Equal(t, "items", view.ProducerTypes[0].Kind)
	require.False(t, view.ProducerTypes[0].Hidden)
	require.Len(t, view.ProducerTypes[0].Items, 1)
	require.Equal(t, "Чертёж", view.ProducerTypes[0].Items[0].Name)
	// фабрика: kind=goods + категория
	require.Equal(t, "goods", view.ProducerTypes[1].Kind)
	require.NotNil(t, view.ProducerTypes[1].CategoryID)
	require.Equal(t, "Корабли", view.ProducerTypes[1].CategoryName)
	// слоты родителя: в state, с именами родителя/категории из снимка
	require.Len(t, view.ProducerSlots, 1)
	require.Equal(t, int64(2), view.ProducerSlots[0].ParentID)
	require.Equal(t, "Фабрика", view.ProducerSlots[0].ParentName)
	require.Equal(t, "Корабли", view.ProducerSlots[0].CategoryName)
	require.False(t, view.ProducerSlots[0].Hidden)
}

// --- слоты родителя (спека 2026-09-21-студия-скрытые-категории-строений §4) ---

// TestStudioCreateSlot — POST /studio/api/slots {parent_id, category_id}:
// создание слота (переопределение уровня / база) → 201.
func TestStudioCreateProducerSlot(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT kind, parent_id IS NULL FROM producer_types WHERE id = \$1`).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id_is_null"}).AddRow("goods", true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM categories WHERE id = \$1\)`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1 AND category_id = \$2 AND race_family IS NOT DISTINCT FROM \$3 AND race IS NOT DISTINCT FROM \$4\)`).
		WithArgs(int64(2), int64(3), nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`INSERT INTO producer_slots \(parent_id, category_id, race_family, race, hidden\) VALUES \(\$1, \$2, \$3, \$4, \$5\) RETURNING id, parent_id, category_id, race_family, race, hidden, created_at`).
		WithArgs(int64(2), int64(3), nil, nil, false).
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id", "category_id", "race_family", "race", "hidden", "created_at"}).
			AddRow(int64(10), int64(2), int64(3), nil, nil, false, time.Now()))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/slots",
		strings.NewReader(`{"parent_id":2,"category_id":3}`))
	rec := httptest.NewRecorder()
	h.Slots(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var sv ProducerSlotView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &sv))
	require.Equal(t, int64(10), sv.ID)
	require.Equal(t, int64(2), sv.ParentID)
	require.False(t, sv.Hidden)
}

// TestStudioSlotHidden — PUT /studio/api/slots/10 {hidden:true} → 200.
func TestStudioSlotHidden(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE producer_slots SET hidden = \$1 WHERE id = \$2`).
		WithArgs(true, int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPut, "/studio/api/slots/10",
		strings.NewReader(`{"hidden":true}`))
	rec := httptest.NewRecorder()
	h.SlotByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioSlotDelete — DELETE /studio/api/slots/10 (снять переопределение /
// убрать из базы) → 200.
func TestStudioSlotDelete(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT parent_id, category_id, race_family, race FROM producer_slots WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"parent_id", "category_id", "race_family", "race"}).
			AddRow(int64(2), int64(3), nil, nil))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types pt WHERE pt.parent_id = \$1 AND pt.category_id = \$2 AND pt.kind = 'goods' AND \(\$3::text IS NULL OR \(\(\$4::text IS NOT NULL AND pt.race = \$4::text\) OR \(\$4::text IS NULL AND pt.race_family = \$3::text\)\)\)\)`).
		WithArgs(int64(2), int64(3), nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`DELETE FROM producer_slots WHERE id = \$1`).
		WithArgs(int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodDelete, "/studio/api/slots/10", nil)
	rec := httptest.NewRecorder()
	h.SlotByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioSlotDeleteRestrict — DELETE слота с заводами категории → 409
// (RESTRICT, §1.4 п.2).
func TestStudioSlotDeleteRestrict(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT parent_id, category_id, race_family, race FROM producer_slots WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"parent_id", "category_id", "race_family", "race"}).
			AddRow(int64(2), int64(3), nil, nil))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types pt WHERE pt.parent_id = \$1 AND pt.category_id = \$2 AND pt.kind = 'goods' AND \(\$3::text IS NULL OR \(\(\$4::text IS NOT NULL AND pt.race = \$4::text\) OR \(\$4::text IS NULL AND pt.race_family = \$3::text\)\)\)\)`).
		WithArgs(int64(2), int64(3), nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodDelete, "/studio/api/slots/10", nil)
	rec := httptest.NewRecorder()
	h.SlotByID(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioCreateProducerNoSlot — POST подтипа kind=goods без применяемого
// слота родителя → 400 «категория не настроена у родителя на этом уровне»
// (инвариант С4, §1.4 п.1).
func TestStudioCreateProducerNoSlot(t *testing.T) {
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
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1 AND category_id = \$2 AND \(race_family IS NULL OR \(race_family = \$3 AND race IS NULL\) OR race = \$4\)\)`).
		WithArgs(int64(1), int64(3), nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/producers",
		strings.NewReader(`{"name":"Фабрика продовольствия","kind":"goods","category_id":3,"parent_id":1}`))
	rec := httptest.NewRecorder()
	h.Producers(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var errBody map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &errBody))
	require.Contains(t, errBody["error"], "категория не настроена у родителя на этом уровне")
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- привязки рецептов к фабрикам (спека 2026-09-21-рецепт-сущность §5) ---

// TestStudioBindRecipe — POST /studio/api/producers/5/recipes {recipe_id:10}
// → 200.
func TestStudioBindRecipe(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT parent_id FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(int64(2)))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM recipes WHERE id = \$1\)`).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_recipes WHERE producer_type_id = \$1 AND recipe_id = \$2\)`).
		WithArgs(int64(5), int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`INSERT INTO producer_recipes \(producer_type_id, recipe_id\) VALUES \(\$1, \$2\)`).
		WithArgs(int64(5), int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/producers/5/recipes", strings.NewReader(`{"recipe_id":10}`))
	rec := httptest.NewRecorder()
	h.ProducerByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioUnbindRecipe — DELETE /studio/api/producers/5/recipes/10 → 200.
func TestStudioUnbindRecipe(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_recipes WHERE producer_type_id = \$1 AND recipe_id = \$2\)`).
		WithArgs(int64(5), int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`DELETE FROM producer_recipes WHERE producer_type_id = \$1 AND recipe_id = \$2`).
		WithArgs(int64(5), int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodDelete, "/studio/api/producers/5/recipes/10", nil)
	rec := httptest.NewRecorder()
	h.ProducerByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestStudioCopyUniversalRecipes — POST
// /studio/api/producers/5/recipes/copy-universal → 200 {added, skipped}.
func TestStudioCopyUniversalRecipes(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectStudioMutation(mock)
	mock.ExpectQuery(`SELECT kind, parent_id, category_id FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id"}).
			AddRow("goods", int64(2), int64(8)))
	mock.ExpectQuery(`SELECT kind FROM categories WHERE id = \$1`).
		WithArgs(int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("good"))
	mock.ExpectQuery(`SELECT DISTINCT pr\.recipe_id FROM producer_recipes pr JOIN producer_types pt ON pt\.id = pr\.producer_type_id WHERE pt\.kind = 'goods' AND pt\.parent_id IS NOT NULL AND pt\.category_id = \$1 AND pt\.race_family IS NULL AND pt\.race IS NULL AND pt\.id <> \$2`).
		WithArgs(int64(8), int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"recipe_id"}).AddRow(int64(10)))
	mock.ExpectExec(`INSERT INTO producer_recipes \(producer_type_id, recipe_id\) VALUES \(\$1, \$2\) ON CONFLICT \(producer_type_id, recipe_id\) DO NOTHING`).
		WithArgs(int64(5), int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	h := NewStudioHandlers(db, ai.NewClient("http://127.0.0.1:1", "test-model", "build", time.Second, 0), "test-model")
	req := httptest.NewRequest(http.MethodPost, "/studio/api/producers/5/recipes/copy-universal", nil)
	rec := httptest.NewRecorder()
	h.ProducerByID(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, mock.ExpectationsWereMet())

	var body map[string]int
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 1, body["added"])
	require.Equal(t, 0, body["skipped"])
}
