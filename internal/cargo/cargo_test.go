// internal/cargo/cargo_test.go
//
// Трюм игрока (спека 2026-09-22-трюм-грузоподъёмность-корабля §14 п.2–5, 7):
// кламп пополнения по свободной ёмкости, снятие до нуля удаляет строку,
// «груз за борт» только уменьшает, занятая масса = Σ quantity × weight,
// форма view (limits.mass used/total, items).
package cargo

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"zorion/internal/ship"
)

const testUser = "11111111-1111-1111-1111-111111111111"

// newCargoDB — sqlmock с regexp-матчером.
func newCargoDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *Service) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db, mock, NewService(db)
}

// loadShip — дефолты ship: starter 20 + cargo_1 30 = 50 т.
func loadShip() {
	ship.LoadDefaults()
	ship.LoadModelDefaults()
}

// T-1: TryAddCargo клампит по свободной ёмкости (§14 п.2): 45/50 и запрос 20
// → принято 5; остаток у источника не списывается.
func TestTryAddCargoClamps(t *testing.T) {
	loadShip()
	_, mock, svc := newCargoDB(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT ship_model_id, equipment FROM users WHERE id = \$1 FOR UPDATE`).
		WithArgs(testUser).
		WillReturnRows(sqlmock.NewRows([]string{"ship_model_id", "equipment"}).
			AddRow("starter", `{"universal":"cargo_1"}`))
	mock.ExpectQuery(`SELECT weight FROM goods WHERE id = \$1`).
		WithArgs(int64(21)).
		WillReturnRows(sqlmock.NewRows([]string{"weight"}).AddRow(1.0))
	mock.ExpectQuery(`SELECT COALESCE\(SUM\(pc\.quantity \* g\.weight\), 0\)`).
		WithArgs(testUser).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(45.0))
	mock.ExpectExec(`INSERT INTO player_cargo \(user_id, good_id, quantity, updated_at\)`).
		WithArgs(testUser, int64(21), 5.0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	accepted, err := svc.TryAddCargo(testUser, 21, 20)
	require.NoError(t, err)
	require.Equal(t, 5.0, accepted, "принято ровно свободное (50-45)")
	require.NoError(t, mock.ExpectationsWereMet())
}

// T-1b: полный трюм (used = total) → ничего не принимаем, INSERT не идёт.
func TestTryAddCargoFullHold(t *testing.T) {
	loadShip()
	_, mock, svc := newCargoDB(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT ship_model_id, equipment FROM users WHERE id = \$1 FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"ship_model_id", "equipment"}).
			AddRow("starter", `{"universal":"cargo_1"}`))
	mock.ExpectQuery(`SELECT weight FROM goods WHERE id = \$1`).
		WithArgs(int64(21)).
		WillReturnRows(sqlmock.NewRows([]string{"weight"}).AddRow(1.0))
	mock.ExpectQuery(`SELECT COALESCE\(SUM\(pc\.quantity \* g\.weight\), 0\)`).
		WithArgs(testUser).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(50.0))
	mock.ExpectRollback()

	accepted, err := svc.TryAddCargo(testUser, 21, 20)
	require.NoError(t, err)
	require.Equal(t, 0.0, accepted, "полный трюм — принять нечего")
	require.NoError(t, mock.ExpectationsWereMet())
}

// T-1c: qty ≤ 0 — no-op без обращения к БД.
func TestTryAddCargoNonPositive(t *testing.T) {
	_, mock, svc := newCargoDB(t)
	accepted, err := svc.TryAddCargo(testUser, 21, 0)
	require.NoError(t, err)
	require.Equal(t, 0.0, accepted)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T-1d: неизвестный товар → ErrGoodNotFound (транзакция откатывается).
func TestTryAddCargoUnknownGood(t *testing.T) {
	loadShip()
	_, mock, svc := newCargoDB(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT ship_model_id, equipment FROM users WHERE id = \$1 FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"ship_model_id", "equipment"}).
			AddRow("starter", `{"universal":"cargo_1"}`))
	mock.ExpectQuery(`SELECT weight FROM goods WHERE id = \$1`).
		WithArgs(int64(999)).
		WillReturnRows(sqlmock.NewRows([]string{"weight"}))
	mock.ExpectRollback()

	_, err := svc.TryAddCargo(testUser, 999, 5)
	require.ErrorIs(t, err, ErrGoodNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T-1e (ревью): при дробном весе произведение accepted×weight не превышает
// свободную ёмкость — инвариант И1 «строго used ≤ total» (эпсилон-защита:
// free/weight × weight может дать лишний ULP сверх free).
func TestTryAddCargoFractionalWeightStrictInvariant(t *testing.T) {
	loadShip()
	_, mock, svc := newCargoDB(t)

	const weight = 0.3
	const free = 50.0 // used = 0, total = 50

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT ship_model_id, equipment FROM users WHERE id = \$1 FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"ship_model_id", "equipment"}).
			AddRow("starter", `{"universal":"cargo_1"}`))
	mock.ExpectQuery(`SELECT weight FROM goods WHERE id = \$1`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"weight"}).AddRow(weight))
	mock.ExpectQuery(`SELECT COALESCE\(SUM\(pc\.quantity \* g\.weight\), 0\)`).
		WithArgs(testUser).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(0.0))
	mock.ExpectExec(`INSERT INTO player_cargo \(user_id, good_id, quantity, updated_at\)`).
		WithArgs(testUser, int64(7), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	accepted, err := svc.TryAddCargo(testUser, 7, 1000)
	require.NoError(t, err)
	require.Greater(t, accepted, 0.0)
	require.LessOrEqual(t, accepted*weight, free, "accepted×weight ≤ free строго (И1)")
	require.NoError(t, mock.ExpectationsWereMet())
}

// T-1f (ревью): нулевой вес — битые данные каталога (спека §6, CHECK weight>0
// нет): явный отказ, а не «бесконечный трюм». Принято 0.
func TestTryAddCargoZeroWeightRejected(t *testing.T) {
	loadShip()
	_, mock, svc := newCargoDB(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT ship_model_id, equipment FROM users WHERE id = \$1 FOR UPDATE`).
		WillReturnRows(sqlmock.NewRows([]string{"ship_model_id", "equipment"}).
			AddRow("starter", `{"universal":"cargo_1"}`))
	mock.ExpectQuery(`SELECT weight FROM goods WHERE id = \$1`).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"weight"}).AddRow(0.0))
	mock.ExpectRollback()

	accepted, err := svc.TryAddCargo(testUser, 7, 50)
	require.NoError(t, err)
	require.Equal(t, 0.0, accepted, "вес 0 — пополнение отклонено")
	require.NoError(t, mock.ExpectationsWereMet())
}

// T-2: TakeCargo снимает не больше, чем есть; строка с нулём удаляется
// (§14 п.3) — нулевых строк не держим.
func TestTakeCargoToZeroDeletesRow(t *testing.T) {
	_, mock, svc := newCargoDB(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT quantity FROM player_cargo WHERE user_id = \$1 AND good_id = \$2 FOR UPDATE`).
		WithArgs(testUser, int64(21)).
		WillReturnRows(sqlmock.NewRows([]string{"quantity"}).AddRow(3.0))
	mock.ExpectExec(`DELETE FROM player_cargo WHERE user_id = \$1 AND good_id = \$2`).
		WithArgs(testUser, int64(21)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	taken, err := svc.TakeCargo(testUser, 21, 5) // есть 3 → снимаем 3, строка удаляется
	require.NoError(t, err)
	require.Equal(t, 3.0, taken)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T-2b: частичное снятие — строка остаётся с уменьшенным количеством.
func TestTakeCargoPartialUpdatesRow(t *testing.T) {
	_, mock, svc := newCargoDB(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT quantity FROM player_cargo WHERE user_id = \$1 AND good_id = \$2 FOR UPDATE`).
		WithArgs(testUser, int64(21)).
		WillReturnRows(sqlmock.NewRows([]string{"quantity"}).AddRow(10.0))
	mock.ExpectExec(`UPDATE player_cargo SET quantity = \$1, updated_at = NOW\(\) WHERE user_id = \$2 AND good_id = \$3`).
		WithArgs(6.0, testUser, int64(21)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	taken, err := svc.TakeCargo(testUser, 21, 4)
	require.NoError(t, err)
	require.Equal(t, 4.0, taken)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T-3: JettisonCargo только уменьшает (§14 п.4): запрошено больше, чем есть
// → сбрасывается ровно имеющееся, строка удаляется.
func TestJettisonCargoOnlyDecreases(t *testing.T) {
	_, mock, svc := newCargoDB(t)

	mock.ExpectQuery(`SELECT 1 FROM goods WHERE id = \$1`).
		WithArgs(int64(21)).
		WillReturnRows(sqlmock.NewRows([]string{"one"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT quantity FROM player_cargo WHERE user_id = \$1 AND good_id = \$2 FOR UPDATE`).
		WithArgs(testUser, int64(21)).
		WillReturnRows(sqlmock.NewRows([]string{"quantity"}).AddRow(8.0))
	mock.ExpectExec(`DELETE FROM player_cargo WHERE user_id = \$1 AND good_id = \$2`).
		WithArgs(testUser, int64(21)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	thrown, err := svc.JettisonCargo(testUser, 21, 20, false)
	require.NoError(t, err)
	require.Equal(t, 8.0, thrown, "сброшено не больше, чем есть")
	require.NoError(t, mock.ExpectationsWereMet())
}

// T-3b: JettisonAll — весь трюм; возвращает суммарно сброшенное.
func TestJettisonAll(t *testing.T) {
	_, mock, svc := newCargoDB(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`DELETE FROM player_cargo WHERE user_id = \$1 RETURNING quantity`).
		WithArgs(testUser).
		WillReturnRows(sqlmock.NewRows([]string{"quantity"}).AddRow(4.0).AddRow(6.0))
	mock.ExpectCommit()

	total, err := svc.JettisonAll(testUser)
	require.NoError(t, err)
	require.Equal(t, 10.0, total)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T-3c: JettisonCargo неизвестного товара → ErrGoodNotFound (§9.2: валидация).
func TestJettisonCargoUnknownGood(t *testing.T) {
	_, mock, svc := newCargoDB(t)

	mock.ExpectQuery(`SELECT 1 FROM goods WHERE id = \$1`).
		WithArgs(int64(999)).
		WillReturnRows(sqlmock.NewRows([]string{"one"}))

	_, err := svc.JettisonCargo(testUser, 999, 1, false)
	require.ErrorIs(t, err, ErrGoodNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T-4: занятая масса = Σ quantity × weight (§14 п.5).
func TestCargoMass(t *testing.T) {
	_, mock, svc := newCargoDB(t)

	mock.ExpectQuery(`SELECT COALESCE\(SUM\(pc\.quantity \* g\.weight\), 0\)`).
		WithArgs(testUser).
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(42.0))

	mass, err := svc.CargoMass(testUser)
	require.NoError(t, err)
	require.Equal(t, 42.0, mass)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T-5: View — форма §9.1: limits.mass {used,total} + items; mass считает
// сервер (quantity × weight); used ≤ total.
func TestViewForm(t *testing.T) {
	loadShip()
	_, mock, svc := newCargoDB(t)

	mock.ExpectQuery(`SELECT pc\.good_id, g\.name, g\.kind, pc\.quantity, g\.weight`).
		WithArgs(testUser).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "name", "kind", "quantity", "weight"}).
			AddRow(int64(21), "Железо Fe", "resource", 12.0, 1.0))
	mock.ExpectQuery(`SELECT ship_model_id, equipment FROM users WHERE id = \$1`).
		WithArgs(testUser).
		WillReturnRows(sqlmock.NewRows([]string{"ship_model_id", "equipment"}).
			AddRow("starter", `{"universal":"cargo_1"}`))

	view, err := svc.View(testUser)
	require.NoError(t, err)
	require.Equal(t, 50.0, view.Limits.Mass.Total, "starter 20 + cargo_1 30")
	require.Equal(t, 12.0, view.Limits.Mass.Used)
	require.LessOrEqual(t, view.Limits.Mass.Used, view.Limits.Mass.Total)
	require.Len(t, view.Items, 1)
	require.Equal(t, int64(21), view.Items[0].GoodID)
	require.Equal(t, "Железо Fe", view.Items[0].Name)
	require.Equal(t, "resource", view.Items[0].Kind)
	require.Equal(t, 12.0, view.Items[0].Quantity)
	require.Equal(t, 1.0, view.Items[0].Weight)
	require.Equal(t, 12.0, view.Items[0].Mass, "mass = quantity × weight (сервер)")
	require.NoError(t, mock.ExpectationsWereMet())
}

// T-5b: пустой трюм — items: [] (не null), used 0 / total 50.
func TestViewEmptyHold(t *testing.T) {
	loadShip()
	_, mock, svc := newCargoDB(t)

	mock.ExpectQuery(`SELECT pc\.good_id, g\.name, g\.kind, pc\.quantity, g\.weight`).
		WithArgs(testUser).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "name", "kind", "quantity", "weight"}))
	mock.ExpectQuery(`SELECT ship_model_id, equipment FROM users WHERE id = \$1`).
		WithArgs(testUser).
		WillReturnRows(sqlmock.NewRows([]string{"ship_model_id", "equipment"}).
			AddRow("starter", `{"universal":"cargo_1"}`))

	view, err := svc.View(testUser)
	require.NoError(t, err)
	require.Equal(t, 0.0, view.Limits.Mass.Used)
	require.Equal(t, 50.0, view.Limits.Mass.Total)
	require.NotNil(t, view.Items, "items должен быть массивом, не null")
	require.Empty(t, view.Items)
	require.NoError(t, mock.ExpectationsWereMet())
}

// T-6: битый params/equipment не ломает расчёт — только врождённая ёмкость.
func TestViewBrokenEquipment(t *testing.T) {
	loadShip()
	_, mock, svc := newCargoDB(t)

	mock.ExpectQuery(`SELECT pc\.good_id, g\.name, g\.kind, pc\.quantity, g\.weight`).
		WithArgs(testUser).
		WillReturnRows(sqlmock.NewRows([]string{"good_id", "name", "kind", "quantity", "weight"}))
	mock.ExpectQuery(`SELECT ship_model_id, equipment FROM users WHERE id = \$1`).
		WithArgs(testUser).
		WillReturnRows(sqlmock.NewRows([]string{"ship_model_id", "equipment"}).
			AddRow("starter", []byte("не-json")))

	view, err := svc.View(testUser)
	require.NoError(t, err)
	require.Equal(t, 20.0, view.Limits.Mass.Total, "битый equipment → только врождённая 20 т")
	require.NoError(t, mock.ExpectationsWereMet())
}

// T-7: FK player_cargo → goods/users с ON DELETE CASCADE (спека §8.1/§14 п.6):
// удаление товара/игрока чистит трюм. БД-независимая проверка по тексту
// миграции (живая проверка FK — живой прогон).
func TestPlayerCargoCascadeFKs(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000068_cargo_hold.sql"))
	require.NoError(t, err)
	s := string(src)
	require.Contains(t, s, "REFERENCES users (id) ON DELETE CASCADE",
		"трюм чистится при удалении игрока")
	require.Contains(t, s, "REFERENCES goods (id) ON DELETE CASCADE",
		"трюм чистится каскадом при удалении товара (решение создателя №7)")
}
