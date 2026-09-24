// internal/repository/branch_testhelpers_test.go
// Общие ожидания моков owner-прохода поселения (спека 2026-09-22-эффекты-
// снабжения-задержка-голод §4.1/§4.5): SyncSettlements зовётся из
// attachSettlements и читает каталог типов эффектов, словарь ТОВАРОВ, ветки
// (с товаром-выходом), состав рецептов/буферы и хранимый базис нагрузки до
// выбора пути.
package repository

import (
	"database/sql/driver"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
)

// effectTypeRows — каталог типов эффектов: один «голод» (id 1, curve hunger).
func effectTypeRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "name", "name_norm", "impact", "curve"}).
		AddRow(int64(1), "Голод", "голод", "population_rate", "hunger")
}

// goodsNameRows — словарь позиций-ТОВАРОВ (goods.name_norm): включает позицию
// привязки ownerInput («пища»).
func goodsNameRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"name_norm"}).
		AddRow("пища").
		AddRow("очищенная вода")
}

// ownerTestTypeID — тип поселения тестов owner-прохода (settlement_type_id).
const ownerTestTypeID = 148

// ownerTestRate — число скорости пар тестов, ед/сутки/млрд: при населении 1e9
// даёт 27.8 батч/час (прежняя скорость от формулы k·P/complexity=1).
const ownerTestRate = 667.2

// producerRateRows — числа скорости пар (producer_type_id, recipe_id, rate) для
// веток тестов (рецепты 69/70, спека 2026-09-23 §3.3).
func producerRateRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"producer_type_id", "recipe_id", "rate"}).
		AddRow(int64(ownerTestTypeID), int64(69), ownerTestRate).
		AddRow(int64(ownerTestTypeID), int64(70), ownerTestRate)
}

// emptyProducerTypeRows — выборка настроек типов без строк (ладдера не нужна).
func emptyProducerTypeRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "eat", "effects"})
}

// emptyProducerRateRows — выборка чисел скорости без строк.
func emptyProducerRateRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"producer_type_id", "recipe_id", "rate"})
}

// emptyDefaultTypeRows — generation_config без ключа базового типа → ладдера пуста.
func emptyDefaultTypeRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"payload"})
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

// expectOwnerPassHead — каталог эффектов + словарь товаров (первый шаг
// SyncSettlements), затем выборка веток. stored == nil — хранимой нагрузки нет.
// Ладдера стадий пуста (нет ключа базового типа) — переключений нет.
func expectOwnerPassHead(mock sqlmock.Sqlmock, branchRows *sqlmock.Rows, stored *sqlmock.Rows) {
	mock.ExpectQuery(effectTypeCatalogSQL).WillReturnRows(effectTypeRows())
	mock.ExpectQuery(goodsNamesSQL).WillReturnRows(goodsNameRows())
	mock.ExpectQuery(branchSelectBySettlementsSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchRows)
	if stored == nil {
		stored = activeEffectRows()
	}
	mock.ExpectQuery(activeEffectsSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(stored)
	expectBatchStageQueries(mock)
}

// expectBatchStageQueries — запросы пачки, общие для обоих путей: ладдера
// стадий (пусто), настройки типов, числа скорости.
func expectBatchStageQueries(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(defaultSettlementTypeSelectSQL).WillReturnRows(emptyDefaultTypeRows())
	mock.ExpectQuery(producerTypesSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(emptyProducerTypeRows())
	mock.ExpectQuery(producerRatesSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(producerRateRows())
}

// expectOrphanCleanup — корневая очистка сирот active_effects в конце
// персистентного прохода (§5.4): ключ — типы эффектов текущих привязок.
func expectOrphanCleanup(mock sqlmock.Sqlmock, ownerID string, effectTypeIDs []int64) {
	v, _ := pq.Array(effectTypeIDs).Value()
	mock.ExpectExec(orphanEffectsDeleteSQL).WithArgs(ownerID, v).
		WillReturnResult(sqlmock.NewResult(0, 0))
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
	mock.ExpectQuery(goodsNamesSQL).WillReturnRows(goodsNameRows())
	mock.ExpectQuery(branchSelectBySettlementsSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(branchRows)
	mock.ExpectQuery(branchComponentsSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(componentRows)
	mock.ExpectQuery(branchBuffersSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(bufferRows)
	if stored == nil {
		stored = activeEffectRows()
	}
	mock.ExpectQuery(activeEffectsSelectSQL).WithArgs(sqlmock.AnyArg()).WillReturnRows(stored)
	expectBatchStageQueries(mock)
}
