// internal/repository/effect_repository_test.go
// Тесты репозитория типов эффектов (спека 2026-09-22-эффекты-снабжения-
// задержка-голод §3.1/§7.4): CRUD типа (T1), удаление типа с действующими
// эффектами — 409 (T16), счётчики привязок (T16), смена params.curve через
// jsonb_set (чужие ключи params сохраняются), админ-инструмент «задать load».
package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// expectEffectMutation — Begin + advisory lock каталога эффектов.
func expectEffectMutation(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(\$1\)`).
		WithArgs(effectCatalogLockKey).
		WillReturnResult(sqlmock.NewResult(0, 0))
}

func TestEffectTypes(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`SELECT id, name, name_norm, impact, COALESCE\(params->>'curve', ''\), created_at FROM effect_types ORDER BY id`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "name_norm", "impact", "curve", "created_at"}).
			AddRow(int64(1), "Голод", "голод", "population_rate", "hunger", time.Now()))
	out, err := NewEffectRepository(db).EffectTypes()
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, "голод", out[0].NameNorm)
	require.Equal(t, "hunger", out[0].Curve)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateEffectType(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectEffectMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM effect_types WHERE name_norm = \$1\)`).
		WithArgs("жажда").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(`INSERT INTO effect_types \(name, name_norm, impact, params\) VALUES \(\$1, \$2, \$3, \$4\) RETURNING id, name, name_norm, impact, COALESCE\(params->>'curve', ''\), created_at`).
		WithArgs("Жажда", "жажда", "population_rate", `{"curve":"thirst"}`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "name_norm", "impact", "curve", "created_at"}).
			AddRow(int64(2), "Жажда", "жажда", "population_rate", "thirst", time.Now()))
	mock.ExpectCommit()

	e, err := NewEffectRepository(db).CreateEffectType("Жажда", "population_rate", "thirst")
	require.NoError(t, err)
	require.Equal(t, int64(2), e.ID)
	require.Equal(t, "thirst", e.Curve)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCreateEffectTypeDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectEffectMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM effect_types WHERE name_norm = \$1\)`).
		WithArgs("голод").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	_, err = NewEffectRepository(db).CreateEffectType("Голод", "population_rate", "hunger")
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateEffectTypeCurve(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectEffectMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM effect_types WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	// curve меняется jsonb_set на уровне ключа — чужие ключи params сохраняются.
	mock.ExpectExec(`UPDATE effect_types SET params = jsonb_set\(COALESCE\(params, '\{\}'::jsonb\), '\{curve\}', to_jsonb\(\$1::text\), true\), updated_at = NOW\(\) WHERE id = \$2`).
		WithArgs("thirst", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	curve := "thirst"
	require.NoError(t, NewEffectRepository(db).UpdateEffectType(1, nil, nil, &curve))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteEffectTypeRestrict(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectEffectMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM effect_types WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM active_effects WHERE effect_type_id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	err = NewEffectRepository(db).DeleteEffectType(1)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 409, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteEffectType(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	expectEffectMutation(mock)
	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM effect_types WHERE id = \$1 FOR UPDATE\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM active_effects WHERE effect_type_id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(`DELETE FROM effect_types WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	require.NoError(t, NewEffectRepository(db).DeleteEffectType(1))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestEffectBindingsCount(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	// Счётчики — в одной транзакции под advisory-локом каталога (замечание
	// @reviewer: без транзакции счётчики могут разъехаться).
	expectEffectMutation(mock)
	mock.ExpectQuery(`SELECT name_norm FROM effect_types WHERE id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"name_norm"}).AddRow("голод"))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM active_effects WHERE effect_type_id = \$1`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM producer_types pt WHERE EXISTS`).
		WithArgs("голод").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectCommit()

	active, producers, err := NewEffectRepository(db).EffectBindingsCount(1)
	require.NoError(t, err)
	require.Equal(t, 2, active)
	require.Equal(t, 1, producers)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSetSettlementEffectLoad(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	// Правка админа — под ТЕМ ЖЕ advisory-ключом, что owner-проход (§4.5):
	// pg_advisory_xact_lock(hashtext(settlement_id)).
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(hashtext\(\$1\)\)`).
		WithArgs("s1").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE active_effects SET load = \$1, load_at = NOW\(\), updated_at = NOW\(\) WHERE owner_type = 'settlement' AND owner_id = \$2 AND effect_type_id = \$3`).
		WithArgs(24.0, "s1", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	require.NoError(t, NewEffectRepository(db).SetSettlementEffectLoad("s1", 1, 24))
	require.NoError(t, mock.ExpectationsWereMet())

	// строки нет → 404 (транзакция откатывается)
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(hashtext\(\$1\)\)`).
		WithArgs("s2").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE active_effects SET load = \$1`).
		WithArgs(5.0, "s2", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	err = NewEffectRepository(db).SetSettlementEffectLoad("s2", 1, 5)
	var ce *ErrCatalog
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 404, ce.Status)
	require.NoError(t, mock.ExpectationsWereMet())

	// отрицательная нагрузка → 400 (SQL не выполняется)
	err = NewEffectRepository(db).SetSettlementEffectLoad("s1", 1, -1)
	require.True(t, errors.As(err, &ce))
	require.Equal(t, 400, ce.Status)
}
