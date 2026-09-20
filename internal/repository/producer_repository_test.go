// internal/repository/producer_repository_test.go
// Тесты репозитория каталога типов производителей и предметов (спека
// 2026-09-20-фабрики §3.1/§4): CRUD producer_types/items/producer_items,
// дубликат имени (409), kind=goods без категории (400), привязка предмета
// только к kind=items (400), снимок с производителями/предметами.
package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// --- типы производителей ---

// TestCreateProducerType — создание подтипа kind=goods с категорией и
// родителем (дерево построек 2026-09-21 §1.2: категория — только у подтипов).
func TestCreateProducerType(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	// родитель: kind=goods, тип (parent_id IS NULL) — глубина 1, kind наследуется
	mock.ExpectQuery(`SELECT kind, parent_id IS NULL FROM producer_types WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id_is_null"}).AddRow("goods", true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM categories WHERE id = \$1\)`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE name_norm = \$1\)`).
		WithArgs("фабрика продовольствия").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	// уникальность подтипа: (parent_id, category_id, race_family, race)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE parent_id = \$1 AND category_id IS NOT DISTINCT FROM \$2 AND race_family IS NOT DISTINCT FROM \$3 AND race IS NOT DISTINCT FROM \$4\)`).
		WithArgs(int64(1), int64(3), nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`INSERT INTO producer_types \(name, name_norm, kind, category_id, race_family, parent_id, race, status\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, 'draft'\) RETURNING id, name, kind, category_id, race_family, parent_id, race, output, input, params, status, created_at`).
		WithArgs("Фабрика продовольствия", "фабрика продовольствия", "goods", int64(3), nil, int64(1), nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "category_id", "race_family", "parent_id", "race", "output", "input", "params", "status", "created_at"}).
			AddRow(int64(2), "Фабрика продовольствия", "goods", int64(3), nil, int64(1), nil, nil, nil, nil, "draft", time.Now()))
	mock.ExpectCommit()

	catID := int64(3)
	parentID := int64(1)
	p, err := NewGoodsRepository(db).CreateProducerType("Фабрика продовольствия", "goods", &catID, &parentID, nil, nil)
	require.NoError(t, err)
	require.Equal(t, int64(2), p.ID)
	require.Equal(t, "goods", p.Kind)
	require.True(t, p.CategoryID.Valid)
	require.True(t, p.ParentID.Valid)
	require.Equal(t, int64(1), p.ParentID.Int64)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateProducerTypeGoodsNoCategory — подтип kind=goods без категории → 400.
func TestCreateProducerTypeGoodsNoCategory(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id IS NULL FROM producer_types WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id_is_null"}).AddRow("goods", true))

	parentID := int64(1)
	_, err = NewGoodsRepository(db).CreateProducerType("Фабрика", "goods", nil, &parentID, nil, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateProducerTypeGoodsTypeWithCategory — тип (без родителя) kind=goods
// с категорией → 400: тип абстрактен, категории живут в подтипах (§1.2 п.4).
func TestCreateProducerTypeGoodsTypeWithCategory(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)

	catID := int64(3)
	_, err = NewGoodsRepository(db).CreateProducerType("Фабрика", "goods", &catID, nil, nil, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateProducerTypeBadKind — неизвестный kind → 400.
func TestCreateProducerTypeBadKind(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)

	_, err = NewGoodsRepository(db).CreateProducerType("Что-то", "other", nil, nil, nil, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateProducerTypeDuplicate — дубликат нормализованного имени → 409.
func TestCreateProducerTypeDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id IS NULL FROM producer_types WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id_is_null"}).AddRow("goods", true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM categories WHERE id = \$1\)`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE name_norm = \$1\)`).
		WithArgs("фабрика продовольствия").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	catID := int64(3)
	parentID := int64(1)
	_, err = NewGoodsRepository(db).CreateProducerType("Фабрика продовольствия", "goods", &catID, &parentID, nil, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateProducerTypeSubtypeDup — дубликат подтипа (родитель, категория,
// семейство, раса) → 409.
func TestCreateProducerTypeSubtypeDup(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
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
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	catID := int64(3)
	parentID := int64(1)
	_, err = NewGoodsRepository(db).CreateProducerType("Фабрика продовольствия", "goods", &catID, &parentID, nil, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateProducerTypeSubtypeOfSubtype — родитель сам подтип → 400 (глубина 1).
func TestCreateProducerTypeSubtypeOfSubtype(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id IS NULL FROM producer_types WHERE id = \$1`).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id_is_null"}).AddRow("goods", false))

	parentID := int64(2)
	_, err = NewGoodsRepository(db).CreateProducerType("Ещё фабрика", "goods", nil, &parentID, nil, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateProducerTypeKindMismatch — kind подтипа ≠ kind родителя → 400.
func TestCreateProducerTypeKindMismatch(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id IS NULL FROM producer_types WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id_is_null"}).AddRow("items", true))

	parentID := int64(1)
	_, err = NewGoodsRepository(db).CreateProducerType("Фабрика", "goods", nil, &parentID, nil, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateProducerTypeRaceNoFamily — раса задана, семейство нет → 400.
func TestCreateProducerTypeRaceNoFamily(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)

	race := "humans"
	_, err = NewGoodsRepository(db).CreateProducerType("Фабрика", "goods", nil, nil, nil, &race)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateProducerType — переименование + JSON-поля (input/output/params).
func TestUpdateProducerType(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id, category_id, race_family, race FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id", "race_family", "race"}).
			AddRow("goods", nil, nil, nil, nil))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE name_norm = \$1 AND id <> \$2\)`).
		WithArgs("автофабрика", int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`UPDATE producer_types SET name = \$1, name_norm = \$2 WHERE id = \$3`).
		WithArgs("Автофабрика", "автофабрика", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE producer_types SET input = \$1 WHERE id = \$2`).
		WithArgs(`{"robots": true}`, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	name := "Автофабрика"
	input := `{"robots": true}`
	err = NewGoodsRepository(db).UpdateProducerType(1, &name, nil, nil, nil, nil, nil, &input, nil)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateProducerTypeBadJSON — невалидный JSON в input → 400.
func TestUpdateProducerTypeBadJSON(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id, category_id, race_family, race FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id", "race_family", "race"}).
			AddRow("goods", nil, nil, nil, nil))

	bad := `{not json`
	err = NewGoodsRepository(db).UpdateProducerType(1, nil, nil, nil, nil, nil, nil, &bad, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateProducerTypeCategoryOnType — категория типу kind=goods (без
// родителя) → 400 (категории живут в подтипах, §1.2 п.4).
func TestUpdateProducerTypeCategoryOnType(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id, category_id, race_family, race FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id", "race_family", "race"}).
			AddRow("goods", nil, nil, nil, nil))

	catID := int64(3)
	err = NewGoodsRepository(db).UpdateProducerType(1, nil, &catID, nil, nil, nil, nil, nil, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateProducerTypeRace — смена расы с семейством (соответствие каталогу
// проверяет хендлер; здесь — структурная проверка и UPDATE).
func TestUpdateProducerTypeRace(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id, category_id, race_family, race FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id", "race_family", "race"}).
			AddRow("items", nil, nil, nil, nil))
	mock.ExpectExec(`UPDATE producer_types SET race_family = \$1 WHERE id = \$2`).
		WithArgs("F1", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE producer_types SET race = \$1 WHERE id = \$2`).
		WithArgs("humans", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	fam := "F1"
	race := "humans"
	err = NewGoodsRepository(db).UpdateProducerType(1, nil, nil, nil, &fam, &race, nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateProducerTypeClearParentRejected — снятие родителя через API не
// поддерживается (UI родителя не редактирует; JSON null = «не менять»):
// parentID = &nil → 400.
func TestUpdateProducerTypeClearParentRejected(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id, category_id, race_family, race FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id", "race_family", "race"}).
			AddRow("items", nil, nil, nil, nil))

	var nilParent *int64
	err = NewGoodsRepository(db).UpdateProducerType(1, nil, nil, &nilParent, nil, nil, nil, nil, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateProducerTypeParentWithSubtypes — тип с подтипами нельзя сделать
// подтипом (глубина 2, §1.2 п.2): смена parent_id у записи с детьми → 409.
func TestUpdateProducerTypeParentWithSubtypes(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id, category_id, race_family, race FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id", "race_family", "race"}).
			AddRow("goods", nil, nil, nil, nil))
	// новый родитель — тип (глубина 1, kind совпадает)
	mock.ExpectQuery(`SELECT kind, parent_id IS NULL FROM producer_types WHERE id = \$1`).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id_is_null"}).AddRow("goods", true))
	// у самой записи есть подтипы → 409 (глубина 2 запрещена)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE parent_id = \$1\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	newParent := int64(2)
	parentID := &newParent
	err = NewGoodsRepository(db).UpdateProducerType(1, nil, nil, &parentID, nil, nil, nil, nil, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateProducerTypeRenameSubtype — чистое переименование подтипа
// (PUT {name}) → 200: кортеж (parent_id, category_id, race_family, race) не
// меняется — уникальность НЕ проверяется (лаборатории легитимно делят кортеж,
// B1). Отсутствие мока на EXISTS уникальности = проверка «кортеж не трогаем».
func TestUpdateProducerTypeRenameSubtype(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id, category_id, race_family, race FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(6)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id", "race_family", "race"}).
			AddRow("items", int64(18), nil, nil, nil))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE name_norm = \$1 AND id <> \$2\)`).
		WithArgs("лаборатория космических технологий", int64(6)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`UPDATE producer_types SET name = \$1, name_norm = \$2 WHERE id = \$3`).
		WithArgs("Лаборатория космических технологий", "лаборатория космических технологий", int64(6)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	name := "Лаборатория космических технологий"
	err = NewGoodsRepository(db).UpdateProducerType(6, &name, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateProducerTypeSubtypeTupleConflict — смена кортежа подтипа в
// конфликт с соседом (та же (parent, category, family, race)) → 409
// (регрессия: проверка уникальности при фактическом изменении кортежа).
func TestUpdateProducerTypeSubtypeTupleConflict(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id, category_id, race_family, race FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(15)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id", "race_family", "race"}).
			AddRow("goods", int64(2), int64(7), nil, nil))
	// категория существует (kind=goods, подтип)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM categories WHERE id = \$1\)`).
		WithArgs(int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE producer_types SET category_id = \$1 WHERE id = \$2`).
		WithArgs(int64(8), int64(15)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// уникальность: кортеж изменился (category 7→8) — сосед с (2, 8, NULL, NULL) есть
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE parent_id = \$1 AND category_id IS NOT DISTINCT FROM \$2 AND race_family IS NOT DISTINCT FROM \$3 AND race IS NOT DISTINCT FROM \$4 AND id <> \$5\)`).
		WithArgs(int64(2), int64(8), nil, nil, int64(15)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	catID := int64(8)
	err = NewGoodsRepository(db).UpdateProducerType(15, nil, &catID, nil, nil, nil, nil, nil, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeleteProducerType — удаление типа без подтипов.
func TestDeleteProducerType(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE parent_id = \$1\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`DELETE FROM producer_types WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewGoodsRepository(db).DeleteProducerType(1))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeleteProducerTypeHasSubtypes — тип с подтипами → 409 (RESTRICT, §1.2 п.7).
func TestDeleteProducerTypeHasSubtypes(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE parent_id = \$1\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	err = NewGoodsRepository(db).DeleteProducerType(1)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSetProducerTypeStatus — смена статуса типа.
func TestSetProducerTypeStatus(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE producer_types SET status = \$1 WHERE id = \$2`).
		WithArgs("approved", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewGoodsRepository(db).SetProducerTypeStatus(1, "approved"))
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- связи производитель ↔ предмет ---

// TestLinkProducerItem — привязка предмета к лаборатории (kind=items).
func TestLinkProducerItem(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
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

	require.NoError(t, NewGoodsRepository(db).LinkProducerItem(6, 1, nil))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestLinkProducerItemNotItems — привязка к производителю kind=goods → 400.
func TestLinkProducerItemNotItems(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("goods"))

	err = NewGoodsRepository(db).LinkProducerItem(2, 1, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUnlinkProducerItem — отвязка предмета.
func TestUnlinkProducerItem(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_items WHERE producer_type_id = \$1 AND item_id = \$2\)`).
		WithArgs(int64(6), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`DELETE FROM producer_items WHERE producer_type_id = \$1 AND item_id = \$2`).
		WithArgs(int64(6), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewGoodsRepository(db).UnlinkProducerItem(6, 1))
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- предметы ---

// TestCreateItem — создание предмета.
func TestCreateItem(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM items WHERE name_norm = \$1\)`).
		WithArgs("чертёж").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`INSERT INTO items \(name, name_norm, slot_type, status\) VALUES \(\$1, \$2, \$3, 'draft'\) RETURNING id, name, slot_type, status, unlocks, params, created_at`).
		WithArgs("Чертёж", "чертёж", "чертёж").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slot_type", "status", "unlocks", "params", "created_at"}).
			AddRow(int64(1), "Чертёж", "чертёж", "draft", nil, nil, time.Now()))
	mock.ExpectCommit()

	it, err := NewGoodsRepository(db).CreateItem("Чертёж", "чертёж")
	require.NoError(t, err)
	require.Equal(t, int64(1), it.ID)
	require.Equal(t, "чертёж", it.SlotType)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateItemDuplicate — дубликат имени → 409.
func TestCreateItemDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM items WHERE name_norm = \$1\)`).
		WithArgs("чертёж").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	_, err = NewGoodsRepository(db).CreateItem("Чертёж", "чертёж")
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateItem — переименование + unlocks.
func TestUpdateItem(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM items WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM items WHERE name_norm = \$1 AND id <> \$2\)`).
		WithArgs("чертёж электроники", int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`UPDATE items SET name = \$1, name_norm = \$2 WHERE id = \$3`).
		WithArgs("Чертёж электроники", "чертёж электроники", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE items SET unlocks = \$1 WHERE id = \$2`).
		WithArgs(`[{"producer_type_id": 2}]`, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	name := "Чертёж электроники"
	unlocks := `[{"producer_type_id": 2}]`
	err = NewGoodsRepository(db).UpdateItem(1, &name, nil, &unlocks, nil)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeleteItem — удаление предмета.
func TestDeleteItem(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM items WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`DELETE FROM items WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewGoodsRepository(db).DeleteItem(1))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestSetItemStatus — смена статуса предмета.
func TestSetItemStatus(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM items WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE items SET status = 'banned' WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewGoodsRepository(db).SetItemStatus(1, "banned"))
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- снимок ---

// TestSnapshotWithProducers — снимок несёт типы производителей, предметы и
// связи (спека §3.1): producer_types/items/producer_items в одной транзакции.
func TestSnapshotWithProducers(t *testing.T) {
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
			AddRow(int64(1), "Лаборатория исследовательская", "items", nil, nil, nil, nil, []byte(`{"items":["Чертёж"]}`), []byte(`{}`), []byte(`{}`), "approved", time.Now()))
	mock.ExpectQuery(`SELECT id, name, slot_type, status, unlocks, params, created_at FROM items ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slot_type", "status", "unlocks", "params", "created_at"}).
			AddRow(int64(1), "Чертёж", "чертёж", "approved", []byte(`[]`), []byte(`{}`), time.Now()))
	mock.ExpectQuery(`SELECT producer_type_id, item_id, requirements FROM producer_items ORDER BY producer_type_id, item_id`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "item_id", "requirements"}).
			AddRow(int64(1), int64(1), nil))
	mock.ExpectCommit()

	snap, err := NewGoodsRepository(db).Snapshot()
	require.NoError(t, err)
	require.Len(t, snap.ProducerTypes, 1)
	require.Equal(t, "items", snap.ProducerTypes[0].Kind)
	require.Len(t, snap.Items, 1)
	require.Equal(t, "чертёж", snap.Items[0].SlotType)
	require.Len(t, snap.ProducerItems, 1)
	require.Equal(t, int64(1), snap.ProducerItems[0].ProducerTypeID)
	require.NoError(t, mock.ExpectationsWereMet())
}