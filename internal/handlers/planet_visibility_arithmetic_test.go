// internal/handlers/planet_visibility_arithmetic_test.go
// T-А3 (видимость арифметики игроку, спека 2026-09-23-стадии-поселения §8.3):
// при включённой настройке игрок в режиме presence получает блок арифметики
// БЕЗ сырого входного буфера; при выключенной — блока нет; в снимке — нет
// всегда. type_id/type_name стадии остаются в любом случае.
package handlers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
	"zorion/internal/repository"
)

// settlementArithmeticPlanet — планета с поселением, несущим блок арифметики и
// новые поля ветки (число скорости + «забираем») + сырой входной буфер.
func settlementArithmeticPlanet() models.Planet {
	rate := 650.0
	return models.Planet{
		ID: "p1",
		Settlements: []models.Settlement{{
			ID: "s1", Population: 1_000_000_000, SettlementTypeID: 148, TypeName: "Городок",
			Arithmetic: []models.SettlementPositionArithmetic{{
				Position: "продовольствие", ProducedPerDay: 650, ConsumedPerDay: 600, NetPerDay: 50,
			}},
			Branches: []models.SettlementBranch{{
				ID: "b1", RecipeID: 69, RecipeName: "Пища",
				RatePerDayPerBillion: &rate,
				Take: []models.SettlementBranchTake{{
					GoodID: 359, GoodName: "Мясо", PerDay: 650,
				}},
				DepositShare: 0.25,
				Output:       []models.BranchBufferEntry{{GoodID: 378, Amount: 12.5}},
				Input:        []models.BranchBufferEntry{{GoodID: 359, GoodName: "Мясо", Amount: 1000}},
			}},
		}},
	}
}

// presence + настройка true: блок арифметики и новые поля ветки видны, сырой
// входной буфер — нет (склад закрыт stripBranchInputs).
func TestStripPresenceArithmeticVisible(t *testing.T) {
	out := stripPlanetDetails(settlementArithmeticPlanet(), &models.PlanetKnowledgeView{Mode: knowledgeModePresence}, true)
	require.Len(t, out.Settlements, 1)
	s := out.Settlements[0]
	require.Len(t, s.Arithmetic, 1)
	assert.Equal(t, "продовольствие", s.Arithmetic[0].Position)
	assert.InDelta(t, 50, s.Arithmetic[0].NetPerDay, 1e-9)
	assert.Equal(t, int64(148), s.SettlementTypeID, "стадия (type_id) игроку остаётся")
	assert.Equal(t, "Городок", s.TypeName, "имя стадии игроку остаётся")

	require.Len(t, s.Branches, 1)
	b := s.Branches[0]
	require.NotNil(t, b.RatePerDayPerBillion)
	assert.InDelta(t, 650, *b.RatePerDayPerBillion, 1e-9)
	require.Len(t, b.Take, 1, "«забираем» — расчётная производная рецепта")
	assert.InDelta(t, 650, b.Take[0].PerDay, 1e-9)
	assert.Nil(t, b.Input, "сырой входной буфер игроку не отдаётся")
}

// presence + настройка false: блока арифметики и новых полей ветки нет —
// сервер не сериализует (не «клиент скрывает»).
func TestStripPresenceArithmeticHidden(t *testing.T) {
	out := stripPlanetDetails(settlementArithmeticPlanet(), &models.PlanetKnowledgeView{Mode: knowledgeModePresence}, false)
	require.Len(t, out.Settlements, 1)
	s := out.Settlements[0]
	assert.Nil(t, s.Arithmetic, "настройка выключена → блока нет")
	assert.Equal(t, int64(148), s.SettlementTypeID, "стадия остаётся и при выключенной настройке")
	require.Len(t, s.Branches, 1)
	b := s.Branches[0]
	assert.Nil(t, b.RatePerDayPerBillion, "число скорости скрыто")
	assert.False(t, b.NotInStageSet)
	assert.Nil(t, b.Take, "«забираем» скрыто")
	assert.Zero(t, b.DepositShare)
	assert.Nil(t, b.Input)
}

// snapshot: блок арифметики не замораживается — чистится всегда, даже при
// включённой настройке.
func TestStripSnapshotArithmeticAlwaysHidden(t *testing.T) {
	out := stripPlanetDetails(settlementArithmeticPlanet(), &models.PlanetKnowledgeView{Mode: knowledgeModeSnapshot}, true)
	require.Len(t, out.Settlements, 1)
	s := out.Settlements[0]
	assert.Nil(t, s.Arithmetic, "блок арифметики в снимке не замораживается (§8.3 п.5)")
	require.Len(t, s.Branches, 1)
	assert.Nil(t, s.Branches[0].RatePerDayPerBillion)
	assert.Nil(t, s.Branches[0].Take)
}

// Витрина presence через applyPlanetVisibility протаскивает флаг настройки:
// true — блок арифметики в ответе, false — нет (сервер не сериализует).
func TestApplyPlanetVisibilityArithmeticFlag(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	knowledge := repository.NewKnowledgeRepository(db)

	mock.ExpectQuery(`SELECT user_id, planet_id, data, scanned_at, source FROM player_planet_knowledge WHERE user_id = \$1 AND planet_id = \$2`).
		WithArgs("u1", "p1").
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "planet_id", "data", "scanned_at", "source"}))

	out := applyPlanetVisibility("u1", []models.Planet{settlementArithmeticPlanet()}, knowledge, "p1", true)
	require.Len(t, out, 1)
	require.Len(t, out[0].Settlements, 1)
	require.Len(t, out[0].Settlements[0].Arithmetic, 1, "настройка true → игрок видит блок")
	require.Len(t, out[0].Settlements[0].Branches, 1)
	assert.Nil(t, out[0].Settlements[0].Branches[0].Input, "входной буфер игроку не отдаётся")
	require.NoError(t, mock.ExpectationsWereMet())
}

// planetsHaveSettlementArithmetic — детектор «есть что скрывать»: иначе настройка
// не читается (лишний запрос на карточку без арифметики).
func TestPlanetsHaveSettlementArithmetic(t *testing.T) {
	assert.True(t, planetsHaveSettlementArithmetic([]models.Planet{settlementArithmeticPlanet()}))
	assert.False(t, planetsHaveSettlementArithmetic([]models.Planet{{
		Settlements: []models.Settlement{{ID: "s1", TypeName: "Городок"}},
	}}))
}

// Очистка витрины игрока НЕ мутирует входной срез веток: копия поселений
// поверхностная, и правка `Branches` не должна менять исходные планеты запроса
// (новая точка алиасинга — находка @reviewer).
func TestPlayerSettlementsArithmeticCleanupDoesNotMutateInput(t *testing.T) {
	p := settlementArithmeticPlanet()
	input := p.Settlements[0].Branches
	require.NotNil(t, input[0].RatePerDayPerBillion)

	out := playerSettlements(p.Settlements, false)
	require.Len(t, out, 1)
	assert.Nil(t, out[0].Branches[0].RatePerDayPerBillion, "витрина очищена")
	assert.Nil(t, out[0].Branches[0].Take)

	assert.NotNil(t, input[0].RatePerDayPerBillion, "исходный срез веток не изменён")
	assert.NotNil(t, input[0].Take, "«забираем» исходного среза не изменено")
}

// То же для снимка: stripSnapshotSettlementSecrets копирует срез веток перед
// правкой, входной срез не мутируется.
func TestSnapshotArithmeticCleanupDoesNotMutateInput(t *testing.T) {
	p := settlementArithmeticPlanet()
	input := p.Settlements[0].Branches
	require.NotNil(t, input[0].RatePerDayPerBillion)

	stripSnapshotSettlementSecrets(&p)
	assert.Nil(t, p.Settlements[0].Branches[0].RatePerDayPerBillion)
	assert.NotNil(t, input[0].RatePerDayPerBillion, "исходный срез веток не изменён")
}

// ==================== T-А3: ПРОВОДКА ФЛАГА ИЗ БД (§8.3/§10) ====================

// expectPresenceOwnerPassArithmeticBranch — owner-проход присутствия с веткой
// (рецепт 69 → позиция «продовольствие») и объявленным числом скорости пары
// (тип 148): витрина получает блок арифметики (производим 650, потребляем 600).
func expectPresenceOwnerPassArithmeticBranch(mock sqlmock.Sqlmock, now time.Time) {
	mock.ExpectQuery(`SELECT id, name, name_norm, impact, COALESCE\(params->>'curve', ''\) FROM effect_types`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "name_norm", "impact", "curve"}).
			AddRow(int64(1), "Голод", "голод", "population_rate", "hunger"))
	mock.ExpectQuery(`SELECT name_norm FROM goods`).
		WillReturnRows(sqlmock.NewRows([]string{"name_norm"}).AddRow("пища"))
	mock.ExpectQuery(`SELECT b\.id.*FROM settlement_branches b`).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "settlement_id", "recipe_id", "processed_at", "good_id", "name", "complexity", "name_norm"}).
			AddRow("b1", "s1", int64(69), now.Add(-time.Minute), int64(378), "Пища", int64(1), "пища"))
	mock.ExpectQuery(`SELECT rc\.recipe_id, rc\.component_id, rc\.quantity FROM recipe_components rc`).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"recipe_id", "component_id", "quantity"}).AddRow(int64(69), int64(359), 1))
	mock.ExpectQuery(`SELECT bb\.branch_id, bb\.direction, bb\.good_id, g\.name, bb\.amount FROM settlement_branch_buffers bb`).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"branch_id", "direction", "good_id", "name", "amount"}).
			AddRow("b1", "output", int64(378), "Пища", 0.0).
			AddRow("b1", "input", int64(359), "Мясо", 1e15))
	mock.ExpectQuery(`SELECT ae\.effect_type_id.*FROM active_effects ae`).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"effect_type_id", "source_position", "load", "load_at", "impact", "curve", "owner_id"}))
	mock.ExpectQuery(`SELECT payload FROM generation_config WHERE key = \$1`).WithArgs(models.DefaultSettlementTypeIDKey).
		WillReturnRows(sqlmock.NewRows([]string{"payload"}))
	mock.ExpectQuery(`SELECT id, params->'eat', params->'effects' FROM producer_types WHERE id = ANY\(\$1\)`).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id", "eat", "effects"}).
			AddRow(int64(148), []byte(`{"продовольствие":600}`), []byte(`{"продовольствие":"голод"}`)))
	mock.ExpectQuery(`SELECT producer_type_id, recipe_id, rate FROM producer_recipes WHERE producer_type_id = ANY\(\$1\)`).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"producer_type_id", "recipe_id", "rate"}).AddRow(int64(148), int64(69), 650.0))
	mock.ExpectQuery(`SELECT planet_id, id, good_id, amount FROM deposits`).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"planet_id", "id", "good_id", "amount"}))
}

// arithmeticPresenceHandlers — полный путь GET /api/worlds/w2/planets для игрока
// на орбите p1: присутствие с веткой и витриной арифметики; настройка
// видимости в generation_config отдаётся из settingPayload.
func arithmeticPresenceHandlers(t *testing.T, settingPayload string) (*AdminHandlers, sqlmock.Sqlmock) {
	t.Helper()
	h, mock := visAdminHandlers(t)
	const userID = "u1"
	nowT := time.Now()

	expectModesPlanets(mock, "w2")
	mock.ExpectQuery(`SELECT s\.id, s\.planet_id.*FROM settlements s LEFT JOIN producer_types pt.*WHERE s\.planet_id = ANY\(\$1\) ORDER BY s\.created_at ASC`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows(settlementSelectCols()).
			AddRow("s1", "p1", 876000000, 876000000.4, 69, nowT, nowT, nowT, "spark", int64(148), "Городок",
				[]byte(`{"пища":600}`), []byte(`{"пища":"голод"}`), nil, nil))
	expectEmptyFactionsBuildings(mock)
	expectEmptyDeposits(mock)
	expectModesWorld(mock, "w2")
	pos := `{"status":"orbit","object_type":"planet","object_id":"p1","level":"orbit"}`
	expectPlayerUserWithPosition(mock, userID, "w2", pos)
	expectKnownWorlds(mock)
	expectModesBelts(mock, "w2")
	expectModesScan(mock, userID, "w2")
	expectPresenceOwnerPassArithmeticBranch(mock, nowT)
	expectModesSettlementLog(mock)
	// Настройку видимости handler читает ДО applyPlanetVisibility (знание — внутри).
	mock.ExpectQuery(`SELECT payload FROM generation_config WHERE key = \$1`).
		WithArgs(models.SettlementArithmeticVisibleKey).
		WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow([]byte(settingPayload)))
	expectModesKnowledge(mock, userID, "p1", `{"surface_dominant":"горы","settlements_count":1}`, nowT)
	return h, mock
}

// Настройка true: handler читает её из БД и оставляет блок арифметики игроку
// (без сырого входного буфера).
func TestGetPlanetsByWorldReadsArithmeticVisibilityTrue(t *testing.T) {
	h, mock := arithmeticPresenceHandlers(t, "true")
	rec := execJSON(h.GetPlanetsByWorld, modesRequest("u1", "w2"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp modesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Planets, 1)
	require.Len(t, resp.Planets[0].Settlements, 1)
	s := resp.Planets[0].Settlements[0]
	require.Len(t, s.Arithmetic, 1, "настройка true → блок арифметики игроку есть")
	assert.Equal(t, "пища", s.Arithmetic[0].Position)
	// Население поселения 8.76e8: (650−600) × 0.876 = 43.8 ед/сутки.
	assert.InDelta(t, 43.8, s.Arithmetic[0].NetPerDay, 1e-3)
	require.Len(t, s.Branches, 1)
	require.NotNil(t, s.Branches[0].RatePerDayPerBillion)
	assert.Nil(t, s.Branches[0].Input, "сырой входной буфер игроку не отдаётся")
	assert.Equal(t, int64(148), s.SettlementTypeID, "стадия остаётся")
}

// Настройка false: handler читает её из БД и НЕ сериализует блок арифметики и
// новые поля ветки (выключение действует сразу, без кэша).
func TestGetPlanetsByWorldReadsArithmeticVisibilityFalse(t *testing.T) {
	h, mock := arithmeticPresenceHandlers(t, "false")
	rec := execJSON(h.GetPlanetsByWorld, modesRequest("u1", "w2"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())

	var resp modesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Planets, 1)
	require.Len(t, resp.Planets[0].Settlements, 1)
	s := resp.Planets[0].Settlements[0]
	assert.Nil(t, s.Arithmetic, "настройка false → блока арифметики нет")
	require.Len(t, s.Branches, 1)
	assert.Nil(t, s.Branches[0].RatePerDayPerBillion, "число скорости скрыто")
	assert.Nil(t, s.Branches[0].Take)
	assert.Nil(t, s.Branches[0].Input)
	assert.Equal(t, int64(148), s.SettlementTypeID, "стадия (type_id) остаётся")
}
