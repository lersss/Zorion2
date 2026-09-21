// Тесты столиц фракций (спека 2026-09-21-фабрики-релиз-2-столицы-фракций §3):
// EnsureCapitals — идемпотентный проход «одна столица на фракцию» на её
// родной планете; GenerateFactions возвращает число созданных столиц.
package faction

import (
	"math/rand"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// capitalsSQLPattern — регулярка запроса EnsureCapitals (sqlmock, дефолтный
// regexp-матчер): INSERT в buildings, 'capital'/'faction', planet_id =
// factions.homeworld_id, owner_id = factions.id, ON CONFLICT DO NOTHING.
const capitalsSQLPattern = `INSERT INTO buildings \(planet_id, building_type, owner_type, owner_id\)[\s\S]*SELECT f\.homeworld_id, 'capital', 'faction', f\.id[\s\S]*ON CONFLICT DO NOTHING`

// Select-запрос генератора фракций (обитаемые планеты).
const settledPlanetsPattern = `SELECT p\.id, p\.name, p\.data FROM planets p`

// TestEnsureCapitalsInsertsOnePerFaction — EnsureCapitals вставляет по одной
// столице на фракцию без столицы (building_type='capital',
// owner_type='faction', owner_id=factions.id, planet_id=homeworld_id — заданы
// самим запросом INSERT ... SELECT); возвращает число созданных.
func TestEnsureCapitalsInsertsOnePerFaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(capitalsSQLPattern).WillReturnResult(sqlmock.NewResult(0, 2))

	n, err := NewGenerator(db, 1).EnsureCapitals()
	require.NoError(t, err)
	require.Equal(t, 2, n, "число столиц = RowsAffected INSERT ... SELECT")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestEnsureCapitalsIdempotent — при наличии столиц у всех фракций запрос не
// создаёт записей (0); повторный вызов не дублирует (NOT EXISTS + частичный
// UNIQUE uq_buildings_capital_owner + ON CONFLICT DO NOTHING).
func TestEnsureCapitalsIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(capitalsSQLPattern).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(capitalsSQLPattern).WillReturnResult(sqlmock.NewResult(0, 0))

	g := NewGenerator(db, 1)
	first, err := g.EnsureCapitals()
	require.NoError(t, err)
	require.Equal(t, 0, first, "у всех фракций есть столица — 0 новых")

	second, err := g.EnsureCapitals()
	require.NoError(t, err)
	require.Equal(t, 0, second, "повторный прогон не дублирует столицы")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestEnsureCapitalsNoFactions — фракций 0 → 0 столиц, без ошибки.
func TestEnsureCapitalsNoFactions(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectExec(capitalsSQLPattern).WillReturnResult(sqlmock.NewResult(0, 0))

	n, err := NewGenerator(db, 1).EnsureCapitals()
	require.NoError(t, err)
	require.Equal(t, 0, n)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGenerateFactionsReturnsCapitals — GenerateFactions завершается проходом
// столиц: возвращает (фракции, столицы) — оба числа идут в лог/отчёт джоба
// («N factions, M capitals», §3/§7 п.2).
func TestGenerateFactionsReturnsCapitals(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	planetRows := sqlmock.NewRows([]string{"id", "name", "data"}).
		AddRow("p1", "Аврора", `{"resources":{"mineral":0.5}}`)
	mock.ExpectQuery(settledPlanetsPattern).WillReturnRows(planetRows)

	// count — число фракций на обитаемую планету (1–3). Генератор берёт его
	// первым вызовом rng — повторяем тем же seed, чтобы мок знал число INSERT-ов.
	count := 1 + rand.New(rand.NewSource(7)).Intn(3)
	for i := 0; i < count; i++ {
		mock.ExpectExec(`INSERT INTO factions`).WillReturnResult(sqlmock.NewResult(1, 1))
	}
	mock.ExpectExec(capitalsSQLPattern).WillReturnResult(sqlmock.NewResult(0, int64(count)))

	factions, capitals, err := NewGenerator(db, 7).GenerateFactions()
	require.NoError(t, err)
	require.Equal(t, count, factions, "на одну обитаемую планету — 1–3 фракции")
	require.Equal(t, count, capitals, "на каждую новую фракцию — столица")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGenerateFactionsNoPlanetsStillEnsuresCapitals — обитаемых планет нет:
// фракции не создаются (0), но догон столиц выполняется (легаси-БД, §3).
func TestGenerateFactionsNoPlanetsStillEnsuresCapitals(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(settledPlanetsPattern).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "data"}))
	mock.ExpectExec(capitalsSQLPattern).WillReturnResult(sqlmock.NewResult(0, 0))

	factions, capitals, err := NewGenerator(db, 7).GenerateFactions()
	require.NoError(t, err)
	require.Equal(t, 0, factions)
	require.Equal(t, 0, capitals)
	require.NoError(t, mock.ExpectationsWereMet())
}
