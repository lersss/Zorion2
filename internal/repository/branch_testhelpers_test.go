// internal/repository/branch_testhelpers_test.go
// Общие ожидания моков для веток поселения (спека 2026-09-22-поселение-
// ветка-буферы-переработка §4.2): attachBranches зовётся внутри
// attachSettlements после пересчёта населения, поэтому тесты планеты мира
// должны предусмотреть чтение веток.
package repository

import "github.com/DATA-DOG/go-sqlmock"

// expectEmptyBranches — ожидание attachBranches: поселения есть, веток нет
// (пустая выборка веток; запрос буферов/компонентов не выполняется).
func expectEmptyBranches(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`
		SELECT b.id, b.settlement_id, b.recipe_id, b.processed_at, r.good_id, og.name, r.complexity
		FROM settlement_branches b
		JOIN recipes r ON r.id = b.recipe_id
		JOIN goods og ON og.id = r.good_id
		WHERE b.settlement_id = ANY($1)
		ORDER BY b.created_at ASC, b.id ASC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "settlement_id", "recipe_id", "processed_at", "good_id", "name", "complexity"}))
}
