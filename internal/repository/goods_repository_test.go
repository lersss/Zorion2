// internal/repository/goods_repository_test.go
// Тесты репозитория каталога товаров/ресурсов (спека
// переноса-студии-товаров-iterA §11): CRUD категорий/товаров/слотов,
// дубликат нормализованного имени (409), цикл при PUT slot (409),
// удаление ссылаемого товара с очисткой ссылок (cleared_links),
// слот ресурсу (403), категория не соответствует kind (400).
package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"zorion/internal/goodsstudio/model"
)

// beginMutation — ожидания транзакции мутации: Begin + advisory lock.
func expectMutationBegin(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(\$1\)`).
		WithArgs(catalogLockKey).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

// --- категории ---

func TestCreateCategory(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM categories WHERE kind = 'good' AND name_norm = \$1\)`).
		WithArgs("корабли").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`INSERT INTO categories \(name, name_norm, kind\) VALUES \(\$1, \$2, 'good'\) RETURNING id, name, kind, code, is_system`).
		WithArgs("Корабли", "корабли").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "code", "is_system"}).
			AddRow(int64(1), "Корабли", "good", nil, false))
	mock.ExpectCommit()

	c, err := NewGoodsRepository(db).CreateCategory("Корабли")
	require.NoError(t, err)
	require.Equal(t, int64(1), c.ID)
	require.Equal(t, "Корабли", c.Name)
	require.Equal(t, "good", c.Kind)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateCategoryDuplicate — дубликат нормализованного имени → 409
// (приложение; UNIQUE (kind, name_norm) — страховка на уровне БД).
func TestCreateCategoryDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM categories WHERE kind = 'good' AND name_norm = \$1\)`).
		WithArgs("корабли").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	_, err = NewGoodsRepository(db).CreateCategory("  Корабли ")
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeleteCategorySystem — системная ресурсная категория → 403.
func TestDeleteCategorySystem(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT is_system FROM categories WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"is_system"}).AddRow(true))

	err = NewGoodsRepository(db).DeleteCategory(1)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 403, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeleteCategoryNotEmpty — категория с товарами → 409.
func TestDeleteCategoryNotEmpty(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT is_system FROM categories WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"is_system"}).AddRow(false))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE category_id = \$1\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	err = NewGoodsRepository(db).DeleteCategory(1)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- товары ---

// TestCreateGood — товар-черновик с одним пустым слотом (quantity=1).
func TestCreateGood(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind FROM categories WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("good"))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE name_norm = \$1\)`).
		WithArgs("сталь").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`INSERT INTO goods \(name, name_norm, category_id, kind, status, source\) VALUES \(\$1, \$2, \$3, \$4, 'draft', 'manual'\) RETURNING id, created_at`).
		WithArgs("Сталь", "сталь", int64(1), "good").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(1), time.Now()))
	mock.ExpectExec(`INSERT INTO goods_slots \(good_id, pos, quantity\) VALUES \(\$1, 0, 1\)`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	g, err := NewGoodsRepository(db).CreateGood("Сталь", 1, model.KindGood)
	require.NoError(t, err)
	require.Equal(t, "1", g.ID)
	require.Equal(t, model.KindGood, g.Kind)
	require.Len(t, g.Recipe, 1, "товар — с одним пустым слотом")
	require.Equal(t, 1, g.Recipe[0].Quantity)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateGoodResourceNoSlots — ресурс создаётся без слотов.
func TestCreateGoodResourceNoSlots(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind FROM categories WHERE id = \$1`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("resource"))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE name_norm = \$1\)`).
		WithArgs("новый ресурс").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`INSERT INTO goods \(name, name_norm, category_id, kind, status, source\) VALUES \(\$1, \$2, \$3, \$4, 'draft', 'manual'\) RETURNING id, created_at`).
		WithArgs("Новый ресурс", "новый ресурс", int64(7), "resource").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(2), time.Now()))
	mock.ExpectCommit()

	g, err := NewGoodsRepository(db).CreateGood("Новый ресурс", 7, model.KindResource)
	require.NoError(t, err)
	require.Equal(t, model.KindResource, g.Kind)
	require.Empty(t, g.Recipe, "у ресурса слотов нет")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateGoodCategoryMismatch — ресурс в товарную категорию → 400.
func TestCreateGoodCategoryMismatch(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind FROM categories WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("good"))

	_, err = NewGoodsRepository(db).CreateGood("Ресурс", 1, model.KindResource)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateGoodDuplicateName — дубликат нормализованного имени → 409
// («Железо»/«железо» — одно имя).
func TestCreateGoodDuplicateName(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind FROM categories WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("good"))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE name_norm = \$1\)`).
		WithArgs("железо").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	_, err = NewGoodsRepository(db).CreateGood("железо", 1, model.KindGood)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateGoodCategoryMismatch — товар в ресурсную категорию → 400.
func TestUpdateGoodCategoryMismatch(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind FROM goods WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("good"))
	mock.ExpectQuery(`SELECT kind FROM categories WHERE id = \$1`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("resource"))

	catID := int64(7)
	err = NewGoodsRepository(db).UpdateGood(1, nil, &catID)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- удаление с очисткой ссылок (решение гейта №2) ---

// TestDeleteGoodClearedLinks — на товар ссылаются 2 слота → очистка в той же
// транзакции, ответ несёт cleared_links=2.
func TestDeleteGoodClearedLinks(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE goods_slots SET component_id = NULL, reason = '' WHERE component_id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`DELETE FROM goods WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	cleared, err := NewGoodsRepository(db).DeleteGood(1)
	require.NoError(t, err)
	require.Equal(t, 2, cleared)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeleteGoodNoLinks — ссылок нет → cleared_links=0.
func TestDeleteGoodNoLinks(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE goods_slots SET component_id = NULL, reason = '' WHERE component_id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DELETE FROM goods WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	cleared, err := NewGoodsRepository(db).DeleteGood(1)
	require.NoError(t, err)
	require.Equal(t, 0, cleared)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeleteGoodNotFound — нет товара → 404.
func TestDeleteGoodNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	_, err = NewGoodsRepository(db).DeleteGood(99)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 404, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- слоты ---

// TestPutSlotCycle — составляющая создаёт цикл → 409 (проверка на снимке
// графа из той же транзакции, §9.2).
func TestPutSlotCycle(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind FROM goods WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("good"))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods_slots WHERE good_id = \$1 AND pos = \$2\)`).
		WithArgs(int64(2), 0).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE id = \$1\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	// граф: g1 (id=1) содержит g2 (id=2) → класть g1 в слот g2 = цикл
	mock.ExpectQuery(`SELECT id, name, category_id, kind, status, source, tier_override, banned_at, created_at FROM goods ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "status", "source", "tier_override", "banned_at", "created_at"}).
			AddRow(int64(1), "A", int64(1), "good", "draft", "manual", nil, nil, time.Now()).
			AddRow(int64(2), "B", int64(1), "good", "draft", "manual", nil, nil, time.Now()))
	mock.ExpectQuery(`SELECT good_id, pos, component_id, quantity, reason, allow_resource FROM goods_slots ORDER BY good_id, pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}).
			AddRow(int64(1), 0, int64(2), 1, nil, false))

	q := 1
	err = NewGoodsRepository(db).PutSlot(2, 0, 1, &q)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestPutSlotResource — слот ресурсу → 403 (решение №1).
func TestPutSlotResource(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind FROM goods WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("resource"))

	err = NewGoodsRepository(db).PutSlot(3, 0, 1, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 403, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestAddSlotResource — добавить слот ресурсу → 403.
func TestAddSlotResource(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind FROM goods WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("resource"))

	err = NewGoodsRepository(db).AddSlot(3)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 403, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeleteSlotShiftsPos — удаление слота сдвигает pos в той же транзакции.
func TestDeleteSlotShiftsPos(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind FROM goods WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("good"))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods_slots WHERE good_id = \$1 AND pos = \$2\)`).
		WithArgs(int64(1), 0).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`DELETE FROM goods_slots WHERE good_id = \$1 AND pos = \$2`).
		WithArgs(int64(1), 0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE goods_slots SET pos = pos - 1 WHERE good_id = \$1 AND pos > \$2`).
		WithArgs(int64(1), 0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewGoodsRepository(db).DeleteSlot(1, 0))
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- статусы и тир ---

// TestSetStatusBanned — бан: banned_at=NOW().
func TestSetStatusBanned(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE goods SET status = 'banned', banned_at = NOW\(\) WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewGoodsRepository(db).SetStatus(1, "banned"))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSetStatusUnban — разбан: draft + banned_at=NULL.
func TestSetStatusUnban(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE goods SET status = 'draft', banned_at = NULL WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewGoodsRepository(db).SetStatus(1, "unban"))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSetStatusUnknown — неизвестный статус → 400.
func TestSetStatusUnknown(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	err = NewGoodsRepository(db).SetStatus(1, "resource")
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSetTierResource — тир ресурсу разрешён (С3).
func TestSetTierResource(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE goods SET tier_override = \$1 WHERE id = \$2`).
		WithArgs(3, int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	tier := 3
	require.NoError(t, NewGoodsRepository(db).SetTier(3, &tier))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSetTierNegative — отрицательный тир → 400.
func TestSetTierNegative(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	tier := -1
	err = NewGoodsRepository(db).SetTier(1, &tier)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- снимок ---

// TestSnapshot — согласованный снимок: категории + товары со слотами.
func TestSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, name, kind, code, is_system FROM categories ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "code", "is_system"}).
			AddRow(int64(1), "Корабли", "good", nil, false).
			AddRow(int64(7), "Минералы", "resource", "mineral", true))
	mock.ExpectQuery(`SELECT id, name, category_id, kind, status, source, tier_override, banned_at, created_at FROM goods ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "status", "source", "tier_override", "banned_at", "created_at"}).
			AddRow(int64(1), "Сталь", int64(1), "good", "approved", "manual", nil, nil, time.Now()).
			AddRow(int64(2), "Железо Fe", int64(7), "resource", "approved", "palette", nil, nil, time.Now()))
	mock.ExpectQuery(`SELECT good_id, pos, component_id, quantity, reason, allow_resource FROM goods_slots ORDER BY good_id, pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}).
			AddRow(int64(1), 0, int64(2), 1, nil, false))
	mock.ExpectCommit()

	snap, err := NewGoodsRepository(db).Snapshot()
	require.NoError(t, err)
	require.Len(t, snap.Categories, 2)
	require.Len(t, snap.Goods, 2)
	require.Len(t, snap.Goods[0].Recipe, 1, "слот Стали ссылается на Железо")
	require.Equal(t, "2", snap.Goods[0].Recipe[0].GoodID)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestResources — палитра: ресурсы, не banned.
func TestResources(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT id, name, category_id FROM goods WHERE kind = 'resource' AND status <> 'banned' ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id"}).
			AddRow(int64(2), "Железо Fe", int64(7)).
			AddRow(int64(3), "Вода H₂O", int64(8)))

	res, err := NewGoodsRepository(db).Resources()
	require.NoError(t, err)
	require.Len(t, res, 2)
	require.Equal(t, "Железо Fe", res[0].Name)
	require.Equal(t, "resource", res[0].Kind)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- кириллическая нормализация (BUG-1, @tester iterA) ---
// PostgreSQL lower() при datcollate = C не приводит кириллицу; name_norm
// пишется из Go (graph.NormalizeName) — проверки дубликатов и UNIQUE
// работают на единой нормализации.

// TestCreateCategoryCyrillicDuplicate — «Категория» + «категория» → 409
// (раньше обе создавались: SQL lower не приводил кириллицу).
func TestCreateCategoryCyrillicDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM categories WHERE kind = 'good' AND name_norm = \$1\)`).
		WithArgs("категория").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	_, err = NewGoodsRepository(db).CreateCategory("категория")
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateGoodCyrillicDuplicate — «Товар» + «товар» → 409.
func TestCreateGoodCyrillicDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind FROM categories WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("good"))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE name_norm = \$1\)`).
		WithArgs("товар").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	_, err = NewGoodsRepository(db).CreateGood("товар", 1, model.KindGood)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateGoodWhitespaceDuplicate — « X » + «x» → 409 (trim + схлопывание).
func TestCreateGoodWhitespaceDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind FROM categories WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("good"))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE name_norm = \$1\)`).
		WithArgs("x").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	_, err = NewGoodsRepository(db).CreateGood("x", 1, model.KindGood)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestBulkCyrillicCaseDuplicate — bulk: «Товар» и «товар» в одной пачке —
// второй пропускается (skip), не 500.
func TestBulkCyrillicCaseDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT id, name FROM categories WHERE kind = 'good'`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(int64(1), "Корабли"))
	mock.ExpectQuery(`SELECT name FROM goods`).
		WillReturnRows(sqlmock.NewRows([]string{"name"}))
	mock.ExpectQuery(`INSERT INTO goods \(name, name_norm, category_id, kind, status, source\) VALUES \(\$1, \$2, \$3, 'good', 'draft', 'manual'\) RETURNING id`).
		WithArgs("Товар", "товар", int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(5)))
	mock.ExpectExec(`INSERT INTO goods_slots \(good_id, pos, quantity\) VALUES \(\$1, 0, 1\)`).
		WithArgs(int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	rep, err := NewGoodsRepository(db).BulkCreateGoods([]string{"Товар | Корабли", "товар | Корабли"})
	require.NoError(t, err)
	require.Len(t, rep.Created, 1)
	require.Len(t, rep.Skipped, 1, "кириллический дубль регистра — пропуск, не 500")
	require.Equal(t, 2, rep.Skipped[0].Line)
	require.Empty(t, rep.Errors)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateGoodUniqueViolation409 — страховка от гонки двух вставок:
// UNIQUE-нарушение (код 23505) → 409, не 500 (спека §9.1).
func TestCreateGoodUniqueViolation409(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind FROM categories WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("good"))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE name_norm = \$1\)`).
		WithArgs("сталь").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`INSERT INTO goods \(name, name_norm, category_id, kind, status, source\) VALUES \(\$1, \$2, \$3, \$4, 'draft', 'manual'\) RETURNING id, created_at`).
		WithArgs("Сталь", "сталь", int64(1), "good").
		WillReturnError(&pq.Error{Code: "23505"})

	_, err = NewGoodsRepository(db).CreateGood("Сталь", 1, model.KindGood)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}