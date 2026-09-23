// internal/repository/branch_testhelpers_test.go
// Общие ожидания моков owner-прохода поселения (спека 2026-09-22-эффекты-
// снабжения-задержка-голод §4.1/§4.5): SyncSettlements зовётся из
// attachSettlements и читает каталог типов эффектов, словарь категорий, ветки
// (с категорией выхода), состав рецептов/буферы и хранимый базис нагрузки до
// выбора пути.
package repository

import (
	"database/sql/driver"

	"github.com/DATA-DOG/go-sqlmock"
)

// effectTypeRows — каталог типов эффектов: один «голод» (id 1, curve hunger).
func effectTypeRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "name", "name_norm", "impact", "curve"}).
		AddRow(int64(1), "Голод", "голод", "population_rate", "hunger")
}

// categoryNameRows — словарь позиций корзины (categories.name_norm).
func categoryNameRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"name_norm"}).
		AddRow("продовольствие").
		AddRow("металлы")
}

// emptyBranchRows — выборка веток без строк (поселения без веток).
func emptyBranchRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "settlement_id", "recipe_id", "processed_at", "good_id", "name", "complexity", "name_norm"})
}

// activeEffectRows — строки хранимого базиса нагрузки (effect_type_id,
// source_position, load, load_at, impact, curve, owner_id); values — семёрками.
func activeEffectRows(values ...driver.Value) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{"effect_type_id", "source_position", "load", "load_at", "impact", "curve", "owner_id"})
	for i := 0; i+6 < len(values); i += 7 {
		rows.AddRow(values[i], values[i+1], values[i+2], values[i+3], values[i+4], values[i+5], values[i+6])
	}
	return rows
}

// expectOwnerPassHead — каталог эффектов + словарь категорий (первый шаг
// SyncSettlements), затем выборка веток. stored == nil — хранимой нагрузки нет.
func expectOwnerPassHead(mock sqlmock.Sqlmock, branchRows *sqlmock.Rows, stored *sqlmock.Rows) {
	mock.ExpectQuery(effectTypeCatalogSQL).WillReturnRows(effectTypeRows())
	mock.ExpectQuery(categoryNamesSQL).WillReturnRows(categoryNameRows())
	mock.ExpectQuery(branchSelectBySettlementsSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchRows)
	if stored == nil {
		stored = activeEffectRows()
	}
	mock.ExpectQuery(activeEffectsSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(stored)
}

// expectOwnerPassNoBranches — owner-проход без веток: ветки пусты, составы и
// буферы не запрашиваются.
func expectOwnerPassNoBranches(mock sqlmock.Sqlmock, stored *sqlmock.Rows) {
	expectOwnerPassHead(mock, emptyBranchRows(), stored)
}

// expectOwnerPassWithBranches — owner-проход с ветками: после выборки веток
// идут состав рецептов и буферы (порядок loadBranches).
func expectOwnerPassWithBranches(mock sqlmock.Sqlmock, branchRows, componentRows, bufferRows, stored *sqlmock.Rows) {
	mock.ExpectQuery(effectTypeCatalogSQL).WillReturnRows(effectTypeRows())
	mock.ExpectQuery(categoryNamesSQL).WillReturnRows(categoryNameRows())
	mock.ExpectQuery(branchSelectBySettlementsSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchRows)
	mock.ExpectQuery(branchComponentsSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(componentRows)
	mock.ExpectQuery(branchBuffersSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(bufferRows)
	if stored == nil {
		stored = activeEffectRows()
	}
	mock.ExpectQuery(activeEffectsSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(stored)
}
