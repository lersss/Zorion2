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

	"zorion/internal/goodsstudio/graph"
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
	// суженный С4 (итерация 4 §5.1): у родителя есть слоты
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	// слот-инвариант С4: применяемый слот родителя (parent, category, уровень)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1 AND category_id = \$2 AND \(race_family IS NULL OR \(race_family = \$3 AND race IS NULL\) OR race = \$4\)\)`).
		WithArgs(int64(1), int64(3), nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE name_norm = \$1\)`).
		WithArgs("фабрика продовольствия").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	// уникальность подтипа: (parent_id, category_id, race_family, race)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE parent_id = \$1 AND category_id IS NOT DISTINCT FROM \$2 AND race_family IS NOT DISTINCT FROM \$3 AND race IS NOT DISTINCT FROM \$4\)`).
		WithArgs(int64(1), int64(3), nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`INSERT INTO producer_types \(name, name_norm, kind, category_id, race_family, parent_id, race\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7\) RETURNING id, name, kind, category_id, race_family, parent_id, race, output, input, params, hidden, created_at`).
		WithArgs("Фабрика продовольствия", "фабрика продовольствия", "goods", int64(3), nil, int64(1), nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "category_id", "race_family", "parent_id", "race", "output", "input", "params", "hidden", "created_at"}).
			AddRow(int64(2), "Фабрика продовольствия", "goods", int64(3), nil, int64(1), nil, nil, nil, nil, false, time.Now()))
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
	require.False(t, p.Hidden, "новая запись-фабрика живая и видимая (hidden из дефолта)")
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
	// суженный С4: у родителя есть слоты — категория подтипа обязательна
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

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
	// суженный С4 (итерация 4 §5.1): у родителя есть слоты
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1 AND category_id = \$2 AND \(race_family IS NULL OR \(race_family = \$3 AND race IS NULL\) OR race = \$4\)\)`).
		WithArgs(int64(1), int64(3), nil, nil).
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
	// слот-инвариант С4: применяемый слот родителя по ИТОГОВОМУ уровню
	// (parent=2, cat=8, fam nil, race nil) — проверяется до EXISTS категории
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1\)`).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1 AND category_id = \$2 AND \(race_family IS NULL OR \(race_family = \$3 AND race IS NULL\) OR race = \$4\)\)`).
		WithArgs(int64(2), int64(8), nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
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
	// итерация 4 §5.3 п.3: ссылающихся поселений нет — удаление свободно
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM settlements WHERE settlement_type_id = \$1\)`).
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

// TestSetProducerHidden — обратимое скрытие записи-фабрики: hidden=true,
// затем hidden=false (единственный носитель скрытия, спека 2026-09-21 §1.2).
func TestSetProducerHidden(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE producer_types SET hidden = \$1 WHERE id = \$2`).
		WithArgs(true, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewGoodsRepository(db).SetProducerHidden(1, true))
	require.NoError(t, mock.ExpectationsWereMet())

	// обратимость: показать обратно
	db2, mock2, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db2.Close()
	expectMutationBegin(mock2)
	mock2.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock2.ExpectExec(`UPDATE producer_types SET hidden = \$1 WHERE id = \$2`).
		WithArgs(false, int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock2.ExpectCommit()

	require.NoError(t, NewGoodsRepository(db2).SetProducerHidden(1, false))
	require.NoError(t, mock2.ExpectationsWereMet())
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
	mock.ExpectQuery(`INSERT INTO items \(name, name_norm, slot_type\) VALUES \(\$1, \$2, \$3\) RETURNING id, name, slot_type, unlocks, params, created_at`).
		WithArgs("Чертёж", "чертёж", "чертёж").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slot_type", "unlocks", "params", "created_at"}).
			AddRow(int64(1), "Чертёж", "чертёж", nil, nil, time.Now()))
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
	mock.ExpectQuery(`SELECT g.id, g.name, g.category_id, g.kind, g.source, r.id, r.complexity, g.created_at, g.volume, g.weight, g.description, g.code FROM goods g LEFT JOIN recipes r ON r.good_id = g.id ORDER BY g.id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "category_id", "kind", "source", "recipe_id", "complexity", "created_at", "volume", "weight", "description", "code"}))
	mock.ExpectQuery(`SELECT r\.good_id, c\.pos, c\.component_id, c\.quantity, c\.reason, c\.allow_resource FROM recipe_components c JOIN recipes r ON r\.id = c\.recipe_id ORDER BY r\.good_id, c\.pos`).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "pos", "component_id", "quantity", "reason", "allow_resource"}))
	mock.ExpectQuery(`SELECT id, name, kind, category_id, race_family, parent_id, race, output, input, params, hidden, created_at, code FROM producer_types ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "category_id", "race_family", "parent_id", "race", "output", "input", "params", "hidden", "created_at", "code"}).
			AddRow(int64(1), "Лаборатория исследовательская", "items", nil, nil, nil, nil, []byte(`{"items":["Чертёж"]}`), []byte(`{}`), []byte(`{}`), false, time.Now(), "p_0001"))
	mock.ExpectQuery(`SELECT id, name, slot_type, unlocks, params, created_at, code FROM items ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "slot_type", "unlocks", "params", "created_at", "code"}).
			AddRow(int64(1), "Чертёж", "чертёж", []byte(`[]`), []byte(`{}`), time.Now(), "i_0001"))
	mock.ExpectQuery(`SELECT producer_type_id, item_id, requirements FROM producer_items ORDER BY producer_type_id, item_id`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "item_id", "requirements"}).
			AddRow(int64(1), int64(1), nil))
	mock.ExpectQuery(`SELECT id, parent_id, category_id, race_family, race, hidden, created_at FROM producer_slots ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id", "category_id", "race_family", "race", "hidden", "created_at"}).
			AddRow(int64(1), int64(2), int64(3), nil, nil, true, time.Now()))
	mock.ExpectQuery(`SELECT pr.producer_type_id, pr.recipe_id, pr.rate, r.good_id FROM producer_recipes pr JOIN recipes r ON r.id = pr.recipe_id ORDER BY pr.producer_type_id, pr.recipe_id`).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "recipe_id", "rate", "good_id"}))
	mock.ExpectCommit()

	snap, err := NewGoodsRepository(db).Snapshot()
	require.NoError(t, err)
	require.Len(t, snap.ProducerTypes, 1)
	require.Equal(t, "items", snap.ProducerTypes[0].Kind)
	require.Equal(t, "p_0001", snap.ProducerTypes[0].Code.String, "снимок несёт метку переноса (code)")
	require.Len(t, snap.Items, 1)
	require.Equal(t, "чертёж", snap.Items[0].SlotType)
	require.Equal(t, "i_0001", snap.Items[0].Code.String, "снимок несёт метку переноса (code)")
	require.Len(t, snap.ProducerItems, 1)
	require.Equal(t, int64(1), snap.ProducerItems[0].ProducerTypeID)
	require.Len(t, snap.ProducerSlots, 1)
	require.True(t, snap.ProducerSlots[0].Hidden)
	require.Equal(t, int64(2), snap.ProducerSlots[0].ParentID)
	require.Empty(t, snap.Bindings)
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- слоты родителя (спека 2026-09-21-студия-скрытые-категории-строений §1.1/§1.4/§4) ---

// TestCreateSlot — создание слота универсального уровня (база): родитель —
// тип kind=goods, категория существует, INSERT RETURNING несёт hidden.
func TestCreateProducerSlot(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
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
		WithArgs(int64(2), int64(3), nil, nil, true).
		WillReturnRows(sqlmock.NewRows([]string{"id", "parent_id", "category_id", "race_family", "race", "hidden", "created_at"}).
			AddRow(int64(10), int64(2), int64(3), nil, nil, true, time.Now()))
	mock.ExpectCommit()

	s, err := NewGoodsRepository(db).CreateProducerSlot(2, 3, nil, nil, true)
	require.NoError(t, err)
	require.Equal(t, int64(10), s.ID)
	require.Equal(t, int64(2), s.ParentID)
	require.Equal(t, int64(3), s.CategoryID)
	require.True(t, s.Hidden)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateSlotParentNotGoods — слот только у типа kind=goods (§1.4 п.3):
// родитель kind=items → 400.
func TestCreateSlotParentNotGoods(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id IS NULL FROM producer_types WHERE id = \$1`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id_is_null"}).AddRow("items", true))

	_, err = NewGoodsRepository(db).CreateProducerSlot(5, 3, nil, nil, false)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.Contains(t, ce.Msg, "слоты только у типов kind=goods")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateSlotDuplicate — дубликат кортежа (parent, category, family, race),
// NULL-safe → 409.
func TestCreateSlotDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id IS NULL FROM producer_types WHERE id = \$1`).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id_is_null"}).AddRow("goods", true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM categories WHERE id = \$1\)`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1 AND category_id = \$2 AND race_family IS NOT DISTINCT FROM \$3 AND race IS NOT DISTINCT FROM \$4\)`).
		WithArgs(int64(2), int64(3), nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	_, err = NewGoodsRepository(db).CreateProducerSlot(2, 3, nil, nil, false)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateSlotHidden — PUT /slots/{id} {hidden:true}: UPDATE hidden,
// кортеж уникальности не трогается (hidden вне кортежа, §1.4 п.4).
func TestUpdateProducerSlotHidden(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE producer_slots SET hidden = \$1 WHERE id = \$2`).
		WithArgs(true, int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewGoodsRepository(db).UpdateProducerSlotHidden(10, true))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateSlotHiddenNotFound — несуществующий слот → 404.
func TestUpdateSlotHiddenNotFound(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(99)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	err = NewGoodsRepository(db).UpdateProducerSlotHidden(99, true)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 404, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeleteSlot — удаление слота без заводов категории (снятие
// переопределения / убрать из базы).
func TestDeleteProducerSlot(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
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

	require.NoError(t, NewGoodsRepository(db).DeleteProducerSlot(10))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeleteSlotRestrict — RESTRICT (§1.4 п.2): при существующих заводах
// категории (применяемых к слоту) → 409 «сначала удалите заводы категории».
func TestDeleteSlotRestrict(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT parent_id, category_id, race_family, race FROM producer_slots WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"parent_id", "category_id", "race_family", "race"}).
			AddRow(int64(2), int64(3), nil, nil))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types pt WHERE pt.parent_id = \$1 AND pt.category_id = \$2 AND pt.kind = 'goods' AND \(\$3::text IS NULL OR \(\(\$4::text IS NOT NULL AND pt.race = \$4::text\) OR \(\$4::text IS NULL AND pt.race_family = \$3::text\)\)\)\)`).
		WithArgs(int64(2), int64(3), nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	err = NewGoodsRepository(db).DeleteProducerSlot(10)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.Contains(t, ce.Msg, "сначала удалите заводы категории")
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- слот-инвариант С4 (спека скрытых §1.4 п.1) ---

// TestCreateProducerTypeNoSlot — подтип kind=goods без применяемого слота
// родителя → 400 «категория не настроена у родителя на этом уровне».
func TestCreateProducerTypeNoSlot(t *testing.T) {
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
	// суженный С4 (итерация 4 §5.1): у родителя есть слоты
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	// слот не настроен — 400
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1 AND category_id = \$2 AND \(race_family IS NULL OR \(race_family = \$3 AND race IS NULL\) OR race = \$4\)\)`).
		WithArgs(int64(1), int64(3), nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	catID := int64(3)
	parentID := int64(1)
	_, err = NewGoodsRepository(db).CreateProducerType("Фабрика продовольствия", "goods", &catID, &parentID, nil, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.Contains(t, ce.Msg, "категория не настроена у родителя на этом уровне")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateProducerTypeFamilySlotOK — семейный слот засчитывается для
// семейной записи: слот (parent, cat, F4) покрывает запись race_family=F4.
func TestCreateProducerTypeFamilySlotOK(t *testing.T) {
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
	// суженный С4 (итерация 4 §5.1): у родителя есть слоты
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	// слот с race_family=F4 применим к записи семейства F4
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1 AND category_id = \$2 AND \(race_family IS NULL OR \(race_family = \$3 AND race IS NULL\) OR race = \$4\)\)`).
		WithArgs(int64(1), int64(3), "F4", nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE name_norm = \$1\)`).
		WithArgs("фабрика топлива f4").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE parent_id = \$1 AND category_id IS NOT DISTINCT FROM \$2 AND race_family IS NOT DISTINCT FROM \$3 AND race IS NOT DISTINCT FROM \$4\)`).
		WithArgs(int64(1), int64(3), "F4", nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`INSERT INTO producer_types \(name, name_norm, kind, category_id, race_family, parent_id, race\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7\) RETURNING id, name, kind, category_id, race_family, parent_id, race, output, input, params, hidden, created_at`).
		WithArgs("Фабрика топлива F4", "фабрика топлива f4", "goods", int64(3), "F4", int64(1), nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "category_id", "race_family", "parent_id", "race", "output", "input", "params", "hidden", "created_at"}).
			AddRow(int64(3), "Фабрика топлива F4", "goods", int64(3), "F4", int64(1), nil, nil, nil, nil, false, time.Now()))
	mock.ExpectCommit()

	catID := int64(3)
	parentID := int64(1)
	fam := "F4"
	p, err := NewGoodsRepository(db).CreateProducerType("Фабрика топлива F4", "goods", &catID, &parentID, &fam, nil)
	require.NoError(t, err)
	require.Equal(t, int64(3), p.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateProducerTypeFamilyRecordRacialSlotOnly — С4-строгость (спека
// 2026-09-21 §5.6): семейная запись (race_family=F4, race NULL) требует
// применяемого слота; расовый слот семейства (F4, race=humans) к ней НЕ
// применяется (уровень семейства не видит расовых слотов) → 400.
func TestCreateProducerTypeFamilyRecordRacialSlotOnly(t *testing.T) {
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
	// суженный С4 (итерация 4 §5.1): у родителя есть слоты
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	// применяемого слота для семейной записи нет (расовый слот не считается)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1 AND category_id = \$2 AND \(race_family IS NULL OR \(race_family = \$3 AND race IS NULL\) OR race = \$4\)\)`).
		WithArgs(int64(1), int64(3), "F4", nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	catID := int64(3)
	parentID := int64(1)
	fam := "F4"
	_, err = NewGoodsRepository(db).CreateProducerType("Фабрика топлива F4", "goods", &catID, &parentID, &fam, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.Contains(t, ce.Msg, "категория не настроена у родителя на этом уровне")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateProducerTypeFamilyNoSlot — C4 при смене семейства (ревью 2026-09-21):
// подтип kind=goods, смена race_family на F1 при отсутствии слота F1 у родителя
// → 400 «категория не настроена у родителя на этом уровне» (итоговый уровень
// записи не покрыт слотами).
func TestUpdateProducerTypeFamilyNoSlot(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id, category_id, race_family, race FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(15)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id", "race_family", "race"}).
			AddRow("goods", int64(2), int64(7), nil, nil))
	// суженный С4 (итерация 4 §5.1): у родителя есть слоты
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1\)`).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	// C4 по итоговому уровню (fam="F1"): слота нет → 400
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1 AND category_id = \$2 AND \(race_family IS NULL OR \(race_family = \$3 AND race IS NULL\) OR race = \$4\)\)`).
		WithArgs(int64(2), int64(7), "F1", nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	fam := "F1"
	err = NewGoodsRepository(db).UpdateProducerType(15, nil, nil, nil, &fam, nil, nil, nil, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.Contains(t, ce.Msg, "категория не настроена у родителя на этом уровне")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateProducerTypeFamilyUniversalSlotOK — C4 при смене семейства на F1:
// универсальный слот (база) покрывает уровень F1 — 400 не возникает
// (сид-типы с universal-слотами не ломаются, ревью п.3).
func TestUpdateProducerTypeFamilyUniversalSlotOK(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id, category_id, race_family, race FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(15)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id", "race_family", "race"}).
			AddRow("goods", int64(2), int64(7), nil, nil))
	// суженный С4 (итерация 4 §5.1): у родителя есть слоты
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1\)`).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	// C4: универсальный слот (race_family IS NULL) покрывает итоговый уровень F1
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1 AND category_id = \$2 AND \(race_family IS NULL OR \(race_family = \$3 AND race IS NULL\) OR race = \$4\)\)`).
		WithArgs(int64(2), int64(7), "F1", nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE producer_types SET race_family = \$1 WHERE id = \$2`).
		WithArgs("F1", int64(15)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// кортеж подтипа изменился (family nil→F1) — проверка уникальности
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE parent_id = \$1 AND category_id IS NOT DISTINCT FROM \$2 AND race_family IS NOT DISTINCT FROM \$3 AND race IS NOT DISTINCT FROM \$4 AND id <> \$5\)`).
		WithArgs(int64(2), int64(7), "F1", nil, int64(15)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectCommit()

	fam := "F1"
	err = NewGoodsRepository(db).UpdateProducerType(15, nil, nil, nil, &fam, nil, nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeleteProducerSlotRaceNoFamilyRecords — RESTRICT расового слота (ревью п.2):
// расовый слот (F0, humans) НЕ применяется к семейной записи (race_family=F0,
// race NULL) — удаление свободно (ложного 409 нет).
func TestDeleteProducerSlotRaceNoFamilyRecords(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT parent_id, category_id, race_family, race FROM producer_slots WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"parent_id", "category_id", "race_family", "race"}).
			AddRow(int64(2), int64(3), "F0", "humans"))
	// предикат: $4='humans' задан → только pt.race='humans'; семейная запись
	// (race NULL) не матчится → завода нет
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types pt WHERE pt.parent_id = \$1 AND pt.category_id = \$2 AND pt.kind = 'goods' AND \(\$3::text IS NULL OR \(\(\$4::text IS NOT NULL AND pt.race = \$4::text\) OR \(\$4::text IS NULL AND pt.race_family = \$3::text\)\)\)\)`).
		WithArgs(int64(2), int64(3), "F0", "humans").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`DELETE FROM producer_slots WHERE id = \$1`).
		WithArgs(int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewGoodsRepository(db).DeleteProducerSlot(10))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeleteProducerSlotRaceRaceRecord — RESTRICT расового слота: расовая запись
// (race=humans) применяется к расовому слоту → 409.
func TestDeleteProducerSlotRaceRaceRecord(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT parent_id, category_id, race_family, race FROM producer_slots WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"parent_id", "category_id", "race_family", "race"}).
			AddRow(int64(2), int64(3), "F0", "humans"))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types pt WHERE pt.parent_id = \$1 AND pt.category_id = \$2 AND pt.kind = 'goods' AND \(\$3::text IS NULL OR \(\(\$4::text IS NOT NULL AND pt.race = \$4::text\) OR \(\$4::text IS NULL AND pt.race_family = \$3::text\)\)\)\)`).
		WithArgs(int64(2), int64(3), "F0", "humans").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	err = NewGoodsRepository(db).DeleteProducerSlot(10)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.Contains(t, ce.Msg, "сначала удалите заводы категории")
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- привязки рецептов к фабрикам (producer_recipes, спека 2026-09-21-рецепт-сущность §5) ---

// TestBindRecipe — привязка рецепта к записи-подтипу постройки:
// привязки нет → INSERT (запроса к goods нет).
func TestBindRecipe(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
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

	require.NoError(t, NewGoodsRepository(db).BindRecipe(5, 10))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestBindRecipeWrongCategoryOK — выход рецепта «чужой» категории больше не
// ограничивает привязку (BindRecipe не читает goods) → 200.
func TestBindRecipeWrongCategoryOK(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
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

	require.NoError(t, NewGoodsRepository(db).BindRecipe(5, 10))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestBindRecipeNotConcrete400 — тип любого kind (parent_id NULL) → 400.
func TestBindRecipeNotConcrete400(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT parent_id FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))

	err = NewGoodsRepository(db).BindRecipe(2, 10)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.Contains(t, ce.Msg, "записи-подтипу")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestBindRecipeStillRejectsType400 — регресс-гейт: абстрактный тип/класс
// (parent_id IS NULL) привязку не принимает → 400 (мёртвая привязка
// невозможна и при появлении наследования наборов).
func TestBindRecipeStillRejectsType400(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT parent_id FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(nil))

	err = NewGoodsRepository(db).BindRecipe(1, 10)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.Contains(t, ce.Msg, "не типу/классу")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestBindRecipeDuplicate409 — повторная привязка → 409.
func TestBindRecipeDuplicate409(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT parent_id FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(int64(2)))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM recipes WHERE id = \$1\)`).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_recipes WHERE producer_type_id = \$1 AND recipe_id = \$2\)`).
		WithArgs(int64(5), int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	err = NewGoodsRepository(db).BindRecipe(5, 10)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestBindRecipeResourceGoodOK — рецепт на ресурс (выход — resource) больше
// не отбивается → 200.
func TestBindRecipeResourceGoodOK(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
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

	require.NoError(t, NewGoodsRepository(db).BindRecipe(5, 10))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestBindRecipeSettlementSubtypeOK — запись-подтип без товарной категории
// (тип поселения, аналог dev id=148) держит рецепт → 200.
func TestBindRecipeSettlementSubtypeOK(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT parent_id FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(148)).
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(int64(1)))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM recipes WHERE id = \$1\)`).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_recipes WHERE producer_type_id = \$1 AND recipe_id = \$2\)`).
		WithArgs(int64(148), int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`INSERT INTO producer_recipes \(producer_type_id, recipe_id\) VALUES \(\$1, \$2\)`).
		WithArgs(int64(148), int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewGoodsRepository(db).BindRecipe(148, 10))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestBindRecipeItemsKindSubtypeOK — подтип kind=items держит рецепт
// (универсальность: kind подтипа не ограничивает) → 200.
func TestBindRecipeItemsKindSubtypeOK(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT parent_id FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(30)).
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(int64(29)))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM recipes WHERE id = \$1\)`).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_recipes WHERE producer_type_id = \$1 AND recipe_id = \$2\)`).
		WithArgs(int64(30), int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`INSERT INTO producer_recipes \(producer_type_id, recipe_id\) VALUES \(\$1, \$2\)`).
		WithArgs(int64(30), int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewGoodsRepository(db).BindRecipe(30, 10))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUnbindRecipe — отвязка существующей привязки → DELETE.
func TestUnbindRecipe(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_recipes WHERE producer_type_id = \$1 AND recipe_id = \$2\)`).
		WithArgs(int64(5), int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`DELETE FROM producer_recipes WHERE producer_type_id = \$1 AND recipe_id = \$2`).
		WithArgs(int64(5), int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewGoodsRepository(db).UnbindRecipe(5, 10))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUnbindRecipeNotFound404 — привязки нет → 404.
func TestUnbindRecipeNotFound404(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_recipes WHERE producer_type_id = \$1 AND recipe_id = \$2\)`).
		WithArgs(int64(5), int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	err = NewGoodsRepository(db).UnbindRecipe(5, 10)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 404, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// copyUniversalRows — общие ожидания цели copy-universal (фабрика 5,
// категория 8, товарная).
func copyUniversalRows(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT kind, parent_id, category_id FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id"}).
			AddRow("goods", int64(2), int64(8)))
	mock.ExpectQuery(`SELECT kind FROM categories WHERE id = \$1`).
		WithArgs(int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("good"))
}

// TestCopyUniversalRecipes — added/skipped: рецепт 10 вставлен (added),
// рецепт 11 уже привязан (skipped).
func TestCopyUniversalRecipes(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	copyUniversalRows(mock)
	mock.ExpectQuery(`SELECT DISTINCT pr\.recipe_id FROM producer_recipes pr JOIN producer_types pt ON pt\.id = pr\.producer_type_id WHERE pt\.kind = 'goods' AND pt\.parent_id IS NOT NULL AND pt\.category_id = \$1 AND pt\.race_family IS NULL AND pt\.race IS NULL AND pt\.id <> \$2`).
		WithArgs(int64(8), int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"recipe_id"}).AddRow(int64(10)).AddRow(int64(11)))
	mock.ExpectExec(`INSERT INTO producer_recipes \(producer_type_id, recipe_id\) VALUES \(\$1, \$2\) ON CONFLICT \(producer_type_id, recipe_id\) DO NOTHING`).
		WithArgs(int64(5), int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO producer_recipes \(producer_type_id, recipe_id\) VALUES \(\$1, \$2\) ON CONFLICT \(producer_type_id, recipe_id\) DO NOTHING`).
		WithArgs(int64(5), int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	added, skipped, err := NewGoodsRepository(db).CopyUniversalRecipes(5)
	require.NoError(t, err)
	require.Equal(t, 1, added)
	require.Equal(t, 1, skipped)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCopyUniversalRecipesIdempotent — повторный вызов: added=0, skipped=N.
func TestCopyUniversalRecipesIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	copyUniversalRows(mock)
	mock.ExpectQuery(`SELECT DISTINCT pr\.recipe_id FROM producer_recipes pr JOIN producer_types pt ON pt\.id = pr\.producer_type_id WHERE pt\.kind = 'goods' AND pt\.parent_id IS NOT NULL AND pt\.category_id = \$1 AND pt\.race_family IS NULL AND pt\.race IS NULL AND pt\.id <> \$2`).
		WithArgs(int64(8), int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"recipe_id"}).AddRow(int64(10)))
	mock.ExpectExec(`INSERT INTO producer_recipes \(producer_type_id, recipe_id\) VALUES \(\$1, \$2\) ON CONFLICT \(producer_type_id, recipe_id\) DO NOTHING`).
		WithArgs(int64(5), int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	added, skipped, err := NewGoodsRepository(db).CopyUniversalRecipes(5)
	require.NoError(t, err)
	require.Equal(t, 0, added)
	require.Equal(t, 1, skipped)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCopyUniversalRecipesEmptySource — пустой источник — не ошибка (0,0).
func TestCopyUniversalRecipesEmptySource(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	copyUniversalRows(mock)
	mock.ExpectQuery(`SELECT DISTINCT pr\.recipe_id FROM producer_recipes pr JOIN producer_types pt ON pt\.id = pr\.producer_type_id WHERE pt\.kind = 'goods' AND pt\.parent_id IS NOT NULL AND pt\.category_id = \$1 AND pt\.race_family IS NULL AND pt\.race IS NULL AND pt\.id <> \$2`).
		WithArgs(int64(8), int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"recipe_id"}))
	mock.ExpectCommit()

	added, skipped, err := NewGoodsRepository(db).CopyUniversalRecipes(5)
	require.NoError(t, err)
	require.Equal(t, 0, added)
	require.Equal(t, 0, skipped)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCopyUniversalRecipesResourceCategory400 — категория ресурсная → 400.
func TestCopyUniversalRecipesResourceCategory400(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id, category_id FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id"}).
			AddRow("goods", int64(2), int64(3)))
	mock.ExpectQuery(`SELECT kind FROM categories WHERE id = \$1`).
		WithArgs(int64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow("resource"))

	_, _, err = NewGoodsRepository(db).CopyUniversalRecipes(5)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCopyUniversalRecipesNotConcrete400 — цель не конкретная фабрика → 400.
func TestCopyUniversalRecipesNotConcrete400(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id, category_id FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id"}).
			AddRow("goods", nil, nil))

	_, _, err = NewGoodsRepository(db).CopyUniversalRecipes(5)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateProducerTypeCategoryBoundOK — смена категории подтипа, у
// которого есть привязанные рецепты, больше не блокируется → 200 (мёртвый
// 409 снят; запроса producer_recipes нет).
func TestUpdateProducerTypeCategoryBoundOK(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id, category_id, race_family, race FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(15)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id", "race_family", "race"}).
			AddRow("goods", int64(2), int64(7), nil, nil))
	// суженный С4 (итерация 4 §5.1): у родителя есть слоты
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1\)`).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1 AND category_id = \$2 AND \(race_family IS NULL OR \(race_family = \$3 AND race IS NULL\) OR race = \$4\)\)`).
		WithArgs(int64(2), int64(8), nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM categories WHERE id = \$1\)`).
		WithArgs(int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE producer_types SET category_id = \$1 WHERE id = \$2`).
		WithArgs(int64(8), int64(15)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE parent_id = \$1 AND category_id IS NOT DISTINCT FROM \$2 AND race_family IS NOT DISTINCT FROM \$3 AND race IS NOT DISTINCT FROM \$4 AND id <> \$5\)`).
		WithArgs(int64(2), int64(8), nil, nil, int64(15)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectCommit()

	catID := int64(8)
	require.NoError(t, NewGoodsRepository(db).UpdateProducerType(15, nil, &catID, nil, nil, nil, nil, nil, nil))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateProducerTypeCategoryWithBindingOK — регресс-гейт: подтип с
// привязками и семейством меняет категорию без отвязки рецептов → 200
// (запрос producer_recipes в UpdateProducerType отсутствует).
func TestUpdateProducerTypeCategoryWithBindingOK(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id, category_id, race_family, race FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(16)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id", "race_family", "race"}).
			AddRow("goods", int64(2), int64(7), "F2", nil))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1\)`).
		WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1 AND category_id = \$2 AND \(race_family IS NULL OR \(race_family = \$3 AND race IS NULL\) OR race = \$4\)\)`).
		WithArgs(int64(2), int64(8), "F2", nil).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM categories WHERE id = \$1\)`).
		WithArgs(int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec(`UPDATE producer_types SET category_id = \$1 WHERE id = \$2`).
		WithArgs(int64(8), int64(16)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE parent_id = \$1 AND category_id IS NOT DISTINCT FROM \$2 AND race_family IS NOT DISTINCT FROM \$3 AND race IS NOT DISTINCT FROM \$4 AND id <> \$5\)`).
		WithArgs(int64(2), int64(8), "F2", nil, int64(16)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectCommit()

	catID := int64(8)
	require.NoError(t, NewGoodsRepository(db).UpdateProducerType(16, nil, &catID, nil, nil, nil, nil, nil, nil))
	require.NoError(t, mock.ExpectationsWereMet())
}

// --- тип поселения: суженный С4 без категорий (итерация 4 §5.1, вариант B) ---

// TestCreateProducerTypeNoSlotParentNoCategory — T3: подтип kind=goods под
// типом БЕЗ слотов (Поселение) без категории создаётся → 200 (миграция
// 000067/сид заводят «Обычное поселение» именно так).
func TestCreateProducerTypeNoSlotParentNoCategory(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id IS NULL FROM producer_types WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id_is_null"}).AddRow("goods", true))
	// у родителя слотов нет → категория подтипа обязана быть NULL
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE name_norm = \$1\)`).
		WithArgs("обычное поселение").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	// уникальность кортежа НЕ проверяется (категория пустая) — нет мока
	mock.ExpectQuery(`INSERT INTO producer_types \(name, name_norm, kind, category_id, race_family, parent_id, race\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7\) RETURNING id, name, kind, category_id, race_family, parent_id, race, output, input, params, hidden, created_at`).
		WithArgs("Обычное поселение", "обычное поселение", "goods", nil, nil, int64(1), nil).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "category_id", "race_family", "parent_id", "race", "output", "input", "params", "hidden", "created_at"}).
			AddRow(int64(9), "Обычное поселение", "goods", nil, nil, int64(1), nil, nil, nil, nil, false, time.Now()))
	mock.ExpectCommit()

	parentID := int64(1)
	p, err := NewGoodsRepository(db).CreateProducerType("Обычное поселение", "goods", nil, &parentID, nil, nil)
	require.NoError(t, err)
	require.Equal(t, int64(9), p.ID)
	require.False(t, p.CategoryID.Valid, "у типа без слотов категории нет")
	require.True(t, p.ParentID.Valid)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateProducerTypeNoSlotParentWithCategory400 — T3: категория у подтипа
// под типом без слотов запрещена → 400 «у типа без слотов не бывает категории товаров».
func TestCreateProducerTypeNoSlotParentWithCategory400(t *testing.T) {
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
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	catID := int64(3)
	parentID := int64(1)
	_, err = NewGoodsRepository(db).CreateProducerType("Поселение крупное", "goods", &catID, &parentID, nil, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.Contains(t, ce.Msg, "у типа без слотов не бывает категории товаров")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestCreateProducerTypeTwoNoCategorySubtypes — T3: два подтипа без категории
// под одним родителем создаются (уникальность кортежа не применяется при
// пустой категории); проверка существования кортежа НЕ вызывается.
func TestCreateProducerTypeTwoNoCategorySubtypes(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	parentID := int64(1)
	for i, name := range []string{"Обычное поселение", "Крупное поселение"} {
		expectMutationBegin(mock)
		mock.ExpectQuery(`SELECT kind, parent_id IS NULL FROM producer_types WHERE id = \$1`).
			WithArgs(int64(1)).
			WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id_is_null"}).AddRow("goods", true))
		mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1\)`).
			WithArgs(int64(1)).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE name_norm = \$1\)`).
			WithArgs(graph.NormalizeName(name)).
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectQuery(`INSERT INTO producer_types \(name, name_norm, kind, category_id, race_family, parent_id, race\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7\) RETURNING id, name, kind, category_id, race_family, parent_id, race, output, input, params, hidden, created_at`).
			WithArgs(name, graph.NormalizeName(name), "goods", nil, nil, int64(1), nil).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name", "kind", "category_id", "race_family", "parent_id", "race", "output", "input", "params", "hidden", "created_at"}).
				AddRow(int64(9+i), name, "goods", nil, nil, int64(1), nil, nil, nil, nil, false, time.Now()))
		mock.ExpectCommit()

		p, err := NewGoodsRepository(db).CreateProducerType(name, "goods", nil, &parentID, nil, nil)
		require.NoError(t, err)
		require.False(t, p.CategoryID.Valid)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDeleteProducerTypeSettlements409 — T4: тип, на который ссылаются
// поселения, не удаляется → 409 «тип используется поселениями» (не сырая
// FK-ошибка ON DELETE RESTRICT).
func TestDeleteProducerTypeSettlements409(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE parent_id = \$1\)`).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM settlements WHERE settlement_type_id = \$1\)`).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	err = NewGoodsRepository(db).DeleteProducerType(9)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.Contains(t, ce.Msg, "тип используется поселениями")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestBindRecipeNoCategorySubtypeOK — T15 (перевёрнут): запись-подтип без
// товарной категории (тип поселения) держит рецепт → 200.
func TestBindRecipeNoCategorySubtypeOK(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT parent_id FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"parent_id"}).AddRow(int64(1)))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM recipes WHERE id = \$1\)`).
		WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_recipes WHERE producer_type_id = \$1 AND recipe_id = \$2\)`).
		WithArgs(int64(9), int64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`INSERT INTO producer_recipes \(producer_type_id, recipe_id\) VALUES \(\$1, \$2\)`).
		WithArgs(int64(9), int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewGoodsRepository(db).BindRecipe(9, 10))
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateProducerTypeMoveToSlotParentNoCategory400 — симметрия С4
// (итерация 4 §5.1): подтип kind=goods без категории (тип поселения) нельзя
// перевести под родителя СО слотами — иначе С4 обойдён (CreateProducerType
// такое запрещает). → 400 «для подтипа kind=goods обязательна категория товаров».
func TestUpdateProducerTypeMoveToSlotParentNoCategory400(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	// «Обычное поселение»: kind=goods, родитель «Поселение» (id=9), категории нет
	mock.ExpectQuery(`SELECT kind, parent_id, category_id, race_family, race FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(20)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id", "race_family", "race"}).
			AddRow("goods", int64(9), nil, nil, nil))
	// смена родителя на тип со слотами (id=5): родитель существует, глубина 1
	mock.ExpectQuery(`SELECT kind, parent_id IS NULL FROM producer_types WHERE id = \$1`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id_is_null"}).AddRow("goods", true))
	// у записи подтипов нет — смена родителя не запрещена глубиной 2
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE parent_id = \$1\)`).
		WithArgs(int64(20)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	// у итогового родителя слоты ЕСТЬ, а итоговая категория пустая → 400
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1\)`).
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	newParent := int64(5)
	parentID := &newParent
	err = NewGoodsRepository(db).UpdateProducerType(20, nil, nil, &parentID, nil, nil, nil, nil, nil)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
	require.Contains(t, ce.Msg, "для подтипа kind=goods обязательна категория товаров")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestUpdateProducerTypeNoSlotParentNoCategoryRename200 — контроль: подтип без
// категории у родителя БЕЗ слотов обновляется без смены родителя (родитель
// без слотов, категория NULL) → 200, симметричная проверка не ломает штатный
// путь типа поселения.
func TestUpdateProducerTypeNoSlotParentNoCategoryRename200(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectMutationBegin(mock)
	mock.ExpectQuery(`SELECT kind, parent_id, category_id, race_family, race FROM producer_types WHERE id = \$1 FOR UPDATE`).
		WithArgs(int64(20)).
		WillReturnRows(sqlmock.NewRows([]string{"kind", "parent_id", "category_id", "race_family", "race"}).
			AddRow("goods", int64(9), nil, nil, nil))
	// итоговый родитель «Поселение» (id=9) — слотов нет, 400 не возникает
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_slots WHERE parent_id = \$1\)`).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM producer_types WHERE name_norm = \$1 AND id <> \$2\)`).
		WithArgs("крупное поселение", int64(20)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(`UPDATE producer_types SET name = \$1, name_norm = \$2 WHERE id = \$3`).
		WithArgs("Крупное поселение", "крупное поселение", int64(20)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	name := "Крупное поселение"
	err = NewGoodsRepository(db).UpdateProducerType(20, &name, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
