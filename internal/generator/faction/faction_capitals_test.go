// Тесты генерации фракций (идея 2026-09-22 «Фракции: одна на расу со своей
// столицей») и столиц (спека 2026-09-21-фабрики-релиз-2-столицы-фракций §3):
// одна фракция на заселённую расу, родная планета — крупнейшее поселение расы,
// идемпотентность; EnsureCapitals — идемпотентный проход «одна столица на
// фракцию» на её родной планете.
package faction

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

// capitalsSQLPattern — регулярка запроса EnsureCapitals (sqlmock, дефолтный
// regexp-матчер): INSERT в buildings, 'capital'/'faction', planet_id =
// factions.homeworld_id, owner_id = factions.id, ON CONFLICT DO NOTHING.
const capitalsSQLPattern = `INSERT INTO buildings \(planet_id, building_type, owner_type, owner_id\)[\s\S]*SELECT f\.homeworld_id, 'capital', 'faction', f\.id[\s\S]*ON CONFLICT DO NOTHING`

// raceCandidatesPattern — регулярка Select-запроса кандидатов: поселения с
// непустым race_id и population > 0, с планетой (имя + data для ресурсов).
const raceCandidatesPattern = `SELECT s\.race_id, s\.planet_id, s\.population, p\.name, p\.data[\s\S]*FROM settlements s[\s\S]*JOIN planets p ON p\.id = s\.planet_id`

// existingRacesPattern — регулярка Select-запроса уже созданных рас
// (идемпотентность: фракция с таким race_id пропускается).
const existingRacesPattern = `SELECT race_id FROM factions WHERE race_id IS NOT NULL`

// raceCandidateRows — пустая выборка кандидатов (колонки как в запросе).
func raceCandidateRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"race_id", "planet_id", "population", "name", "data"})
}

// expectFactionInsert — ожидание INSERT фракции с конкретными именем, расой и
// родной планетой (остальные поля случайны: id/тип/ресурсы/цвет/описание).
func expectFactionInsert(mock sqlmock.Sqlmock, name, raceID, homeworldID string) {
	mock.ExpectExec(`INSERT INTO factions`).
		WithArgs(sqlmock.AnyArg(), name, sqlmock.AnyArg(), raceID, homeworldID,
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

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

// TestGenerateFactionsOneFactionPerRace — одна фракция на расу (не на планету):
// у расы humans два поселения (p1/p2), у saltfolk — одно; создаётся 2 фракции,
// родная планета humans — крупнейшее поселение (p2, population 500).
func TestGenerateFactionsOneFactionPerRace(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(raceCandidatesPattern).WillReturnRows(
		raceCandidateRows().
			AddRow("humans", "p1", 100, "Аврора", `{"resources":{"mineral":0.5}}`).
			AddRow("humans", "p2", 500, "Борей", `{"resources":{"mineral":0.9}}`).
			AddRow("saltfolk", "p3", 50, "Вега", `{"resources":{"mineral":0.1}}`))
	mock.ExpectQuery(existingRacesPattern).WillReturnRows(sqlmock.NewRows([]string{"race_id"}))
	expectFactionInsert(mock, "humans", "humans", "p2")
	expectFactionInsert(mock, "saltfolk", "saltfolk", "p3")
	mock.ExpectExec(capitalsSQLPattern).WillReturnResult(sqlmock.NewResult(0, 2))

	factions, capitals, err := NewGenerator(db, 7).GenerateFactions()
	require.NoError(t, err)
	require.Equal(t, 2, factions, "по одной фракции на расу")
	require.Equal(t, 2, capitals, "на каждую новую фракцию — столица")
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGenerateFactionsHomeworldTiebreak — при равном населении поселений расы
// родной становится планета с меньшим planet_id (детерминированно).
func TestGenerateFactionsHomeworldTiebreak(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(raceCandidatesPattern).WillReturnRows(
		raceCandidateRows().
			AddRow("humans", "p2", 100, "Борей", `{}`).
			AddRow("humans", "p1", 100, "Аврора", `{}`))
	mock.ExpectQuery(existingRacesPattern).WillReturnRows(sqlmock.NewRows([]string{"race_id"}))
	expectFactionInsert(mock, "humans", "humans", "p1")
	mock.ExpectExec(capitalsSQLPattern).WillReturnResult(sqlmock.NewResult(0, 1))

	factions, capitals, err := NewGenerator(db, 7).GenerateFactions()
	require.NoError(t, err)
	require.Equal(t, 1, factions)
	require.Equal(t, 1, capitals)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGenerateFactionsIdempotent — фракция с уже существующим race_id
// пропускается: повторный прогон создаёт 0 фракций, столицы только добиваются.
func TestGenerateFactionsIdempotent(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(raceCandidatesPattern).WillReturnRows(
		raceCandidateRows().AddRow("humans", "p1", 100, "Аврора", `{}`))
	mock.ExpectQuery(existingRacesPattern).
		WillReturnRows(sqlmock.NewRows([]string{"race_id"}).AddRow("humans"))
	// INSERT фракции не ожидается: есть фракция с race_id = humans.
	mock.ExpectExec(capitalsSQLPattern).WillReturnResult(sqlmock.NewResult(0, 0))

	factions, capitals, err := NewGenerator(db, 7).GenerateFactions()
	require.NoError(t, err)
	require.Equal(t, 0, factions, "раса уже имеет фракцию — пропуск")
	require.Equal(t, 0, capitals)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGenerateFactionsOnlySettledRaces — число фракций задаётся выборкой
// поселений, а не каталогом рас: раса без поселений кандидатом не становится
// (её просто нет в выборке settled-рас).
func TestGenerateFactionsOnlySettledRaces(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(raceCandidatesPattern).WillReturnRows(
		raceCandidateRows().AddRow("humans", "p1", 100, "Аврора", `{}`))
	mock.ExpectQuery(existingRacesPattern).WillReturnRows(sqlmock.NewRows([]string{"race_id"}))
	expectFactionInsert(mock, "humans", "humans", "p1")
	mock.ExpectExec(capitalsSQLPattern).WillReturnResult(sqlmock.NewResult(0, 1))

	factions, capitals, err := NewGenerator(db, 7).GenerateFactions()
	require.NoError(t, err)
	require.Equal(t, 1, factions, "только заселённые расы дают фракции")
	require.Equal(t, 1, capitals)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestGenerateFactionsNoRacesStillEnsuresCapitals — заселённых рас нет: фракции
// не создаются (0), но догон столиц выполняется (легаси-БД).
func TestGenerateFactionsNoRacesStillEnsuresCapitals(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(raceCandidatesPattern).
		WillReturnRows(raceCandidateRows())
	mock.ExpectQuery(existingRacesPattern).WillReturnRows(sqlmock.NewRows([]string{"race_id"}))
	mock.ExpectExec(capitalsSQLPattern).WillReturnResult(sqlmock.NewResult(0, 0))

	factions, capitals, err := NewGenerator(db, 7).GenerateFactions()
	require.NoError(t, err)
	require.Equal(t, 0, factions)
	require.Equal(t, 0, capitals)
	require.NoError(t, mock.ExpectationsWereMet())
}
