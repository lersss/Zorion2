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

	"zorion/internal/goodsstudio/ai"
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

// TestCreateGood — товар-черновик: рецепт + один пустой компонент (quantity=1).
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
	mock.ExpectQuery(`INSERT INTO goods \(name, name_norm, category_id, kind, source\) VALUES \(\$1, \$2, \$3, \$4, 'manual'\) RETURNING id, created_at`).
		WithArgs("Сталь", "сталь", int64(1), "good").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(1), time.Now()))
	mock.ExpectQuery(`INSERT INTO recipes \(good_id\) VALUES \(\$1\) RETURNING id`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(100)))
	mock.ExpectExec(`INSERT INTO recipe_components \(recipe_id, pos, quantity\) VALUES \(\$1, 0, 1\)`).
		WithArgs(int64(100)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	g, err := NewGoodsRepository(db).CreateGood("Сталь", 1, model.KindGood)
	require.NoError(t, err)
	require.Equal(t, "1", g.ID)
	require.Equal(t, model.KindGood, g.Kind)
	require.Equal(t, int64(100), g.RecipeID, "рецепт создан вместе с товаром")
	require.Len(t, g.Recipe, 1, "товар — с одним пустым компонентом")
	require.Equal(t, 1, g.Recipe[0].Quantity)
	// Р2: значение веса/объёма есть всегда — новые товары 1/1 (DEFAULT).
	require.NotNil(t, g.Volume)
	require.Equal(t, 1.0, *g.Volume)
	require.NotNil(t, g.Weight)
	require.Equal(t, 1.0, *g.Weight)
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
	mock.ExpectQuery(`INSERT INTO goods \(name, name_norm, category_id, kind, source\) VALUES \(\$1, \$2, \$3, \$4, 'manual'\) RETURNING id, created_at`).
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
	err = NewGoodsRepository(db).UpdateGood(1, nil, &catID, nil, nil, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateGoodCategoryBoundOK — смена категории товара, чей рецепт
// привязан к постройкам, больше не блокируется → 200 (мёртвый 409 снят;
// запроса producer_recipes нет).
func TestUpdateGoodCategoryBoundOK(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind FROM goods WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("good"))
	mock.ExpectQuery(`SELECT kind FROM categories WHERE id = \$1`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("good"))
	mock.ExpectExec(`UPDATE goods SET category_id = \$1 WHERE id = \$2`).
		WithArgs(int64(5), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	catID := int64(5)
	require.NoError(t, NewGoodsRepository(db).UpdateGood(1, nil, &catID, nil, nil, nil))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateGoodCategoryUnboundFree — смена категории товара без привязок
// свободна (409 не триггерится).
func TestUpdateGoodCategoryUnboundFree(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind FROM goods WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("good"))
	mock.ExpectQuery(`SELECT kind FROM categories WHERE id = \$1`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("good"))
	mock.ExpectExec(`UPDATE goods SET category_id = \$1 WHERE id = \$2`).
		WithArgs(int64(5), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	catID := int64(5)
	require.NoError(t, NewGoodsRepository(db).UpdateGood(1, nil, &catID, nil, nil, nil))
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- удаление с очисткой ссылок (решение гейта №2) ---

// TestDeleteGoodClearedLinks — на товар ссылаются 2 слота → очистка в той же
// транзакции, ответ несёт cleared_links=2 и deposits (число залежей, T14).
func TestDeleteGoodClearedLinks(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM deposits WHERE good_id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectQuery(`FROM settlement_branches b`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec(`UPDATE recipe_components SET component_id = NULL, reason = '' WHERE component_id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`DELETE FROM goods WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	res, err := NewGoodsRepository(db).DeleteGood(1)
	require.NoError(t, err)
	require.Equal(t, 2, res.ClearedLinks)
	require.Equal(t, 3, res.Deposits, "число залежей ресурса (предпроверка, T14)")
	require.Equal(t, 1, res.Branches, "число веток с этим товаром-выходом (О3 итерации 2)")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeleteGoodNoLinks — ссылок и залежей нет → cleared_links=0, deposits=0.
func TestDeleteGoodNoLinks(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM deposits WHERE good_id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`FROM settlement_branches b`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`UPDATE recipe_components SET component_id = NULL, reason = '' WHERE component_id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`DELETE FROM goods WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	res, err := NewGoodsRepository(db).DeleteGood(1)
	require.NoError(t, err)
	require.Equal(t, 0, res.ClearedLinks)
	require.Equal(t, 0, res.Deposits)
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

// --- состав рецепта (recipe_components, спека 2026-09-21-рецепт-сущность §5) ---

// TestPutRecipeComponentCycle — составляющая создаёт цикл → 409 (проверка на
// снимке графа из той же транзакции).
func TestPutRecipeComponentCycle(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT good_id FROM recipes WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"good_id"}).AddRow(int64(2)))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM recipe_components WHERE recipe_id = \$1 AND pos = \$2\)`).
		WithArgs(int64(2), 0).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM goods WHERE id = \$1\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	// граф: g1 (id=1) содержит g2 (id=2) → класть g1 в компонент g2 = цикл
	mock.ExpectQuery(`SELECT g.id, g.name, g.category_id, g.kind, g.source, r.id, r.complexity, g.created_at, g.volume, g.weight, g.description FROM goods g LEFT JOIN recipes r ON r.good_id = g.id ORDER BY g.id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "source", "recipe_id", "complexity", "created_at", "volume", "weight", "description"}).
			AddRow(int64(1), "A", int64(1), "good", "manual", nil, nil, time.Now(), nil, nil, nil).
			AddRow(int64(2), "B", int64(1), "good", "manual", nil, nil, time.Now(), nil, nil, nil))
	mock.ExpectQuery(`SELECT r\.good_id, c\.pos, c\.component_id, c\.quantity, c\.reason, c\.allow_resource FROM recipe_components c JOIN recipes r ON r\.id = c\.recipe_id ORDER BY r\.good_id, c\.pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}).
			AddRow(int64(1), 0, int64(2), 1, nil, false))

	q := 1
	err = NewGoodsRepository(db).PutRecipeComponent(2, 0, 1, &q)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestPutRecipeComponentNoRecipe — рецепта нет → 404.
func TestPutRecipeComponentNoRecipe(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT good_id FROM recipes WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{"good_id"}))

	err = NewGoodsRepository(db).PutRecipeComponent(99, 0, 1, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 404, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestAddRecipeComponentNoRecipe — рецепта нет → 404.
func TestAddRecipeComponentNoRecipe(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT good_id FROM recipes WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{"good_id"}))

	err = NewGoodsRepository(db).AddRecipeComponent(99)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 404, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestAddRecipeComponentAppendsPos — новый компонент pos = max+1.
func TestAddRecipeComponentAppendsPos(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT good_id FROM recipes WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"good_id"}).AddRow(int64(1)))
	mock.ExpectQuery(`SELECT MAX\(pos\) FROM recipe_components WHERE recipe_id = \$1`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(int64(1)))
	mock.ExpectExec(`INSERT INTO recipe_components \(recipe_id, pos, quantity\) VALUES \(\$1, \$2, 1\)`).
		WithArgs(int64(5), 2).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewGoodsRepository(db).AddRecipeComponent(5))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeleteRecipeComponentShiftsPos — удаление компонента сдвигает pos.
func TestDeleteRecipeComponentShiftsPos(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT good_id FROM recipes WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"good_id"}).AddRow(int64(1)))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM recipe_components WHERE recipe_id = \$1 AND pos = \$2\)`).
		WithArgs(int64(1), 0).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`DELETE FROM recipe_components WHERE recipe_id = \$1 AND pos = \$2`).
		WithArgs(int64(1), 0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE recipe_components SET pos = pos - 1 WHERE recipe_id = \$1 AND pos > \$2`).
		WithArgs(int64(1), 0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewGoodsRepository(db).DeleteRecipeComponent(1, 0))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestClearRecipeComponent — очистка компонента (component_id → NULL).
func TestClearRecipeComponent(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT good_id FROM recipes WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"good_id"}).AddRow(int64(1)))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM recipe_components WHERE recipe_id = \$1 AND pos = \$2\)`).
		WithArgs(int64(1), 0).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE recipe_components SET component_id = NULL, reason = '' WHERE recipe_id = \$1 AND pos = \$2`).
		WithArgs(int64(1), 0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewGoodsRepository(db).ClearRecipeComponent(1, 0))
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- сложность рецепта (PUT /recipes/{id}, спека 2026-09-21-рецепт-сущность §5) ---

// TestSetRecipeComplexity — сложность задана → UPDATE recipes.complexity.
func TestSetRecipeComplexity(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM recipes WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE recipes SET complexity = \$1 WHERE id = \$2`).
		WithArgs(3, int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	c := 3
	require.NoError(t, NewGoodsRepository(db).SetRecipeComplexity(3, &c))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSetRecipeComplexityNull — null — очистить (сложность вычисляемая).
func TestSetRecipeComplexityNull(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM recipes WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE recipes SET complexity = NULL WHERE id = \$1`).
		WithArgs(int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewGoodsRepository(db).SetRecipeComplexity(3, nil))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSetRecipeComplexityNegative — отрицательная сложность → 400.
func TestSetRecipeComplexityNegative(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM recipes WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	c := -1
	err = NewGoodsRepository(db).SetRecipeComplexity(1, &c)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSetRecipeComplexityNoRecipe — рецепта нет → 404.
func TestSetRecipeComplexityNoRecipe(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM recipes WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	c := 1
	err = NewGoodsRepository(db).SetRecipeComplexity(99, &c)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 404, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- снимок ---

// TestSnapshot — согласованный снимок: категории + товары с составом рецептов
// и привязками (спека 2026-09-21-рецепт-сущность §5).
func TestSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, name, kind, code, is_system FROM categories ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "code", "is_system"}).
			AddRow(int64(1), "Корабли", "good", nil, false).
			AddRow(int64(7), "Минералы", "resource", "mineral", true))
	mock.ExpectQuery(`SELECT g.id, g.name, g.category_id, g.kind, g.source, r.id, r.complexity, g.created_at, g.volume, g.weight, g.description FROM goods g LEFT JOIN recipes r ON r.good_id = g.id ORDER BY g.id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "source", "recipe_id", "complexity", "created_at", "volume", "weight", "description"}).
			AddRow(int64(1), "Сталь", int64(1), "good", "manual", int64(100), 3, time.Now(), nil, nil, nil).
			AddRow(int64(2), "Железо Fe", int64(7), "resource", "palette", nil, nil, time.Now(), nil, nil, nil))
	mock.ExpectQuery(`SELECT r\.good_id, c\.pos, c\.component_id, c\.quantity, c\.reason, c\.allow_resource FROM recipe_components c JOIN recipes r ON r\.id = c\.recipe_id ORDER BY r\.good_id, c\.pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}).
			AddRow(int64(1), 0, int64(2), 1, nil, false))
	mock.ExpectQuery(`SELECT id, name, kind, category_id, race_family, parent_id, race, output, input, params, hidden, created_at FROM producer_types ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "category_id", "race_family", "parent_id", "race", "output", "input", "params", "hidden", "created_at"}))
	mock.ExpectQuery(`SELECT id, name, slot_type, unlocks, params, created_at FROM items ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slot_type", "unlocks", "params", "created_at"}))
	mock.ExpectQuery(`SELECT producer_type_id, item_id, requirements FROM producer_items ORDER BY producer_type_id, item_id`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "item_id", "requirements"}))
	mock.ExpectQuery(`SELECT id, parent_id, category_id, race_family, race, hidden, created_at FROM producer_slots ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id", "category_id", "race_family", "race", "hidden", "created_at"}))
	mock.ExpectQuery(`SELECT pr.producer_type_id, pr.recipe_id, pr.rate, r.good_id FROM producer_recipes pr JOIN recipes r ON r.id = pr.recipe_id ORDER BY pr.producer_type_id, pr.recipe_id`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "recipe_id", "rate", "good_id"}).
			AddRow(int64(62), int64(100), nil, int64(1)))
	mock.ExpectCommit()

	snap, err := NewGoodsRepository(db).Snapshot()
	require.NoError(t, err)
	require.Len(t, snap.Categories, 2)
	require.Len(t, snap.Goods, 2)
	require.Len(t, snap.Goods[0].Recipe, 1, "компонент Стали ссылается на Железо")
	require.Equal(t, "2", snap.Goods[0].Recipe[0].GoodID)
	require.Equal(t, int64(100), snap.Goods[0].RecipeID, "проекция рецепта")
	require.NotNil(t, snap.Goods[0].Complexity)
	require.Equal(t, 3, *snap.Goods[0].Complexity)
	require.Len(t, snap.Bindings, 1, "привязка рецепта к фабрике")
	require.Equal(t, int64(62), snap.Bindings[0].ProducerTypeID)
	require.Equal(t, int64(1), snap.Bindings[0].GoodID)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestRealResources — real-ресурсы витрины 94a из БД (спека iterB §5.3):
// goods kind=resource с props ? 'family' (JSONB-оператор наличия ключа) +
// code категории (джойн по category_id). Источник витрины — БД (С1).
func TestRealResources(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT g\.id, g\.name, c\.code, g\.props FROM goods g JOIN categories c ON c\.id = g\.category_id WHERE g\.kind = 'resource' AND g\.props \? 'family' ORDER BY g\.id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "props"}).
			AddRow(int64(21), "Железо Fe", "mineral", `{"твёрдость":50,"family":"Металлы","t_melt_k":1811,"t_boil_k":3134}`).
			AddRow(int64(22), "Вода H₂O", "water", `{"плотность":35,"family":"Вода и растворы","t_melt_k":273,"t_boil_k":373}`))

	rows, err := NewGoodsRepository(db).RealResources()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, int64(21), rows[0].ID)
	require.Equal(t, "Железо Fe", rows[0].Name)
	require.Equal(t, "mineral", rows[0].Category, "category — code из categories.code")
	require.Contains(t, string(rows[0].Props), "family")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestResources — палитра: ресурсы kind=resource (скрытия у ресурсов нет).
func TestResources(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT id, name, category_id FROM goods WHERE kind = 'resource' ORDER BY id`).
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
	mock.ExpectQuery(`INSERT INTO goods \(name, name_norm, category_id, kind, source, description\) VALUES \(\$1, \$2, \$3, 'good', 'manual', \$4\) RETURNING id`).
		WithArgs("Товар", "товар", int64(1), nil).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(5)))
	mock.ExpectQuery(`INSERT INTO recipes \(good_id\) VALUES \(\$1\) RETURNING id`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(50)))
	mock.ExpectExec(`INSERT INTO recipe_components \(recipe_id, pos, quantity\) VALUES \(\$1, 0, 1\)`).
		WithArgs(int64(50)).
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
	mock.ExpectQuery(`INSERT INTO goods \(name, name_norm, category_id, kind, source\) VALUES \(\$1, \$2, \$3, \$4, 'manual'\) RETURNING id, created_at`).
		WithArgs("Сталь", "сталь", int64(1), "good").
		WillReturnError(&pq.Error{Code: "23505"})

	_, err = NewGoodsRepository(db).CreateGood("Сталь", 1, model.KindGood)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- ApplyProposals (спека iterC §5.4/§11) ---

// applySnapshotRows — ожидания снимка каталога в tx: категория 5 (good),
// товар 1 «Корабль» (рецепт 100) с двумя пустыми компонентами (pos 0, 1).
func applySnapshotRows(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT id, name, kind, code, is_system FROM categories ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "code", "is_system"}).
			AddRow(int64(5), "Комплектующие", "good", nil, false))
	mock.ExpectQuery(`SELECT g.id, g.name, g.category_id, g.kind, g.source, r.id, r.complexity, g.created_at, g.volume, g.weight, g.description FROM goods g LEFT JOIN recipes r ON r.good_id = g.id ORDER BY g.id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "source", "recipe_id", "complexity", "created_at", "volume", "weight", "description"}).
			AddRow(int64(1), "Корабль", int64(5), "good", "manual", int64(100), nil, time.Now(), nil, nil, nil))
	mock.ExpectQuery(`SELECT r\.good_id, c\.pos, c\.component_id, c\.quantity, c\.reason, c\.allow_resource FROM recipe_components c JOIN recipes r ON r\.id = c\.recipe_id ORDER BY r\.good_id, c\.pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}).
			AddRow(int64(1), 0, nil, 1, nil, false).
			AddRow(int64(1), 1, nil, 1, nil, false))
}

// TestApplyProposalsWriteBack — два принятых пункта kind=new: новые товары
// INSERT + рецепт (без состава, не-регресс §8.3), компоненты UPDATE по
// конкретным pos рецепта цели, маппинг "gN"→real id, applied = 2, отчёт пуст.
func TestApplyProposalsWriteBack(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	applySnapshotRows(mock)
	mock.ExpectQuery(`INSERT INTO goods \(name, name_norm, category_id, kind, source, description\) VALUES \(\$1, \$2, \$3, 'good', 'ai', \$4\) RETURNING id`).
		WithArgs("Сталь", "сталь", int64(5), nil).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(10)))
	mock.ExpectExec(`INSERT INTO recipes \(good_id\) VALUES \(\$1\)`).
		WithArgs(int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`INSERT INTO goods \(name, name_norm, category_id, kind, source, description\) VALUES \(\$1, \$2, \$3, 'good', 'ai', \$4\) RETURNING id`).
		WithArgs("Топливо", "топливо", int64(5), nil).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(11)))
	mock.ExpectExec(`INSERT INTO recipes \(good_id\) VALUES \(\$1\)`).
		WithArgs(int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE recipe_components SET component_id = \$1, reason = \$2 WHERE recipe_id = \$3 AND pos = \$4`).
		WithArgs(int64(10), "", int64(100), 0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE recipe_components SET component_id = \$1, reason = \$2 WHERE recipe_id = \$3 AND pos = \$4`).
		WithArgs(int64(11), "", int64(100), 1).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	applied, report, err := NewGoodsRepository(db).ApplyProposals(1, []ai.ProposalItem{
		{Slot: 0, Name: "Сталь", CategoryID: "5", Kind: "new"},
		{Slot: 1, Name: "Топливо", CategoryID: "5", Kind: "new"},
	})
	require.NoError(t, err)
	require.Equal(t, 2, applied)
	require.Empty(t, report)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestApplyProposalsGoodGoneM2 — товар удалён между фазами → отчёт «товар
// не найден», не ошибка (М2: ответ 200), никаких записей.
func TestApplyProposalsGoodGoneM2(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT id, name, kind, code, is_system FROM categories ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "code", "is_system"}).
			AddRow(int64(5), "Комплектующие", "good", nil, false))
	mock.ExpectQuery(`SELECT g.id, g.name, g.category_id, g.kind, g.source, r.id, r.complexity, g.created_at, g.volume, g.weight, g.description FROM goods g LEFT JOIN recipes r ON r.good_id = g.id ORDER BY g.id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "source", "recipe_id", "complexity", "created_at", "volume", "weight", "description"}).
			AddRow(int64(2), "Другой", int64(5), "good", "manual", nil, nil, time.Now(), nil, nil, nil))
	mock.ExpectQuery(`SELECT r\.good_id, c\.pos, c\.component_id, c\.quantity, c\.reason, c\.allow_resource FROM recipe_components c JOIN recipes r ON r\.id = c\.recipe_id ORDER BY r\.good_id, c\.pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}))
	mock.ExpectRollback() // ничего не записано — ранний return, rollback

	applied, report, err := NewGoodsRepository(db).ApplyProposals(1, []ai.ProposalItem{
		{Slot: 0, Name: "Сталь", CategoryID: "5", Kind: "new"},
	})
	require.NoError(t, err)
	require.Equal(t, 0, applied)
	require.Len(t, report, 1)
	require.Contains(t, report[0], "товар не найден")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestApplyProposalsLinkGoneC1 — link-цель удалена между фазами → дроп +
// отчёт «ссылка исчезла», новый товар НЕ создаётся (С1), applied = 0.
func TestApplyProposalsLinkGoneC1(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	applySnapshotRows(mock)
	mock.ExpectCommit()

	applied, report, err := NewGoodsRepository(db).ApplyProposals(1, []ai.ProposalItem{
		{Slot: 0, Name: "Сталь", Kind: "link"}, // Сталь удалена между фазами
	})
	require.NoError(t, err)
	require.Equal(t, 0, applied)
	require.Len(t, report, 1)
	require.Contains(t, report[0], "ссылка исчезла")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestApplyProposalsUniqueDrop — INSERT нового товара упал на UNIQUE (23505):
// дроп пункта + отчёт, applied уменьшен, слот не заполняется (страховка).
func TestApplyProposalsUniqueDrop(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	applySnapshotRows(mock)
	mock.ExpectQuery(`INSERT INTO goods \(name, name_norm, category_id, kind, source, description\) VALUES \(\$1, \$2, \$3, 'good', 'ai', \$4\) RETURNING id`).
		WithArgs("Сталь", "сталь", int64(5), nil).
		WillReturnError(&pq.Error{Code: "23505"})
	mock.ExpectCommit()

	applied, report, err := NewGoodsRepository(db).ApplyProposals(1, []ai.ProposalItem{
		{Slot: 0, Name: "Сталь", CategoryID: "5", Kind: "new"},
	})
	require.NoError(t, err)
	require.Equal(t, 0, applied) // 1 принят − 1 дроп (23505)
	require.Len(t, report, 1)
	require.Contains(t, report[0], "уже есть")
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- LayerResources (спека iterC §7.2) ---

// TestLayerResources — только props ? 'closes' (дискриминатор слоя), джойн
// категорий по code, порядок по id.
func TestLayerResources(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT g\.id, g\.name, c\.code, g\.props FROM goods g JOIN categories c ON c\.id = g\.category_id WHERE g\.kind = 'resource' AND g\.props \? 'closes' ORDER BY g\.id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "code", "props"}).
			AddRow(int64(1), "вода-ресурс", "water", []byte(`{"closes":["вода"],"bridge":false}`)).
			AddRow(int64(2), "сера-ресурс", "mineral", []byte(`{"closes":["сера"]}`)))

	rows, err := NewGoodsRepository(db).LayerResources()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, int64(1), rows[0].ID)
	require.Equal(t, "вода-ресурс", rows[0].Name)
	require.Equal(t, "water", rows[0].Category)
	require.Contains(t, string(rows[0].Props), "closes")
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- UpdateDescriptions: режимы onlyIfEmpty (спека 2026-09-21-каталог-описание §6.6/И4) ---

// TestUpdateDescriptionsOnlyIfEmpty — пакетный режим (true): непустое описание
// не перезаписывается — строка guard'а, UPDATE нет.
func TestUpdateDescriptionsOnlyIfEmpty(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT name, COALESCE\(description, ''\) FROM goods WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "coalesce"}).AddRow("Сталь", "уже есть"))
	mock.ExpectCommit()

	applied, report, err := NewGoodsRepository(db).UpdateDescriptions([]DescItem{{ID: 1, Text: "новое"}}, true)
	require.NoError(t, err)
	require.Equal(t, 0, applied)
	require.Len(t, report, 1)
	require.Contains(t, report[0], "описание уже заполнено")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateDescriptionsExplicitOverwrite — явный режим (false): непустое
// описание перезаписывается (guard снят), applied=1.
func TestUpdateDescriptionsExplicitOverwrite(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT name, COALESCE\(description, ''\) FROM goods WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "coalesce"}).AddRow("Сталь", "старое"))
	mock.ExpectExec(`UPDATE goods SET description = \$1 WHERE id = \$2`).
		WithArgs("новое", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	applied, report, err := NewGoodsRepository(db).UpdateDescriptions([]DescItem{{ID: 1, Text: "новое"}}, false)
	require.NoError(t, err)
	require.Equal(t, 1, applied)
	require.Empty(t, report)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateDescriptionsMissing — записи нет → строка «не найдена», оба режима
// (guard непустого не при чём).
func TestUpdateDescriptionsMissing(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT name, COALESCE\(description, ''\) FROM goods WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "coalesce"}))
	mock.ExpectCommit()

	applied, report, err := NewGoodsRepository(db).UpdateDescriptions([]DescItem{{ID: 99, Text: "новое"}}, false)
	require.NoError(t, err)
	require.Equal(t, 0, applied)
	require.Len(t, report, 1)
	require.Contains(t, report[0], "не найдена")
	require.NoError(t, mock.ExpectationsWereMet())
}
