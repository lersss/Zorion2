// internal/handlers/settlement_owner_testhelpers_test.go
// Общие ожидания owner-прохода поселений для handler-тестов (спека
// 2026-09-22-эффекты-снабжения-задержка-голод §4.1/§4.5): attachSettlements
// зовёт SyncSettlements — каталог типов эффектов, словарь ТОВАРОВ, ветки
// (с товаром-выходом) и хранимый базис нагрузки.
package handlers

import "github.com/DATA-DOG/go-sqlmock"

// settlementSelectSQL — запрос поселений планет с типом (нормы eat + привязки
// effects, §4.2).
const settlementSelectSQL = `
	SELECT s.id, s.planet_id, s.population, s.population_exact, s.stability, s.computed_at,
	                 s.created_at, s.updated_at, s.race_id, s.settlement_type_id, pt.name,
	                 pt.params->'eat', pt.params->'effects', s.owner_type, s.owner_id
	          FROM settlements s
	          LEFT JOIN producer_types pt ON pt.id = s.settlement_type_id
	          WHERE s.planet_id = ANY($1) ORDER BY s.created_at ASC`

// settlementSelectCols — колонки выборки поселений (15 штук: +владелец).
func settlementSelectCols() []string {
	return []string{"id", "planet_id", "population", "population_exact", "stability", "computed_at", "created_at", "updated_at", "race_id", "settlement_type_id", "name", "eat", "effects", "owner_type", "owner_id"}
}

// expectOwnerPassEmpty — owner-проход без веток (путь «в памяти»): каталог
// эффектов, словарь товаров, пустые ветки, хранимая нагрузка.
func expectOwnerPassEmpty(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`
		SELECT id, name, name_norm, impact, COALESCE(params->>'curve', '')
		FROM effect_types
	`).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "name_norm", "impact", "curve"}))
	mock.ExpectQuery(`
		SELECT id, name, name_norm FROM goods
	`).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "name_norm"}))
	mock.ExpectQuery(`
		SELECT component_id, COUNT(*) FROM recipe_components
		WHERE component_id IS NOT NULL GROUP BY component_id
	`).WillReturnRows(sqlmock.NewRows([]string{"component_id", "count"}))
	mock.ExpectQuery(`
		SELECT b.id, b.settlement_id, b.recipe_id, b.processed_at,
		       r.good_id, og.name, r.complexity, og.name_norm
		FROM settlement_branches b
		JOIN recipes r ON r.id = b.recipe_id
		JOIN goods og ON og.id = r.good_id
		WHERE b.settlement_id = ANY($1)
		ORDER BY b.created_at ASC, b.id ASC
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"id", "settlement_id", "recipe_id", "processed_at", "good_id", "name", "complexity", "name_norm"}))
	mock.ExpectQuery(`
		SELECT ae.effect_type_id, COALESCE(ae.source_position, ''), ae.load, ae.load_at,
		       et.impact, COALESCE(et.params->>'curve', ''), ae.owner_id
		FROM active_effects ae
		JOIN effect_types et ON et.id = ae.effect_type_id
		WHERE ae.owner_type = 'settlement' AND ae.owner_id = ANY($1)
	`).WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"effect_type_id", "source_position", "load", "load_at", "impact", "curve", "owner_id"}))
	// Ладдера стадий пуста (нет ключа базового типа): у поселения типа нет —
	// запросы настроек типов/чисел скорости не идут (пустое объединение).
	mock.ExpectQuery(`SELECT payload FROM generation_config WHERE key = $1`).
		WillReturnRows(sqlmock.NewRows([]string{"payload"}))
	// Путь «в памяти»: чтение ячеек хранилища поселения (без записи).
	mock.ExpectQuery(`
		SELECT id, owner_type, owner_id, good_id, amount, cap_share
		FROM settlement_storage_cells
		WHERE owner_type = $1 AND owner_id = $2
		ORDER BY good_id
	`).WithArgs("settlement", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "owner_type", "owner_id", "good_id", "amount", "cap_share"}))
}
