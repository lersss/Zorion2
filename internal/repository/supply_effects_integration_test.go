// internal/repository/supply_effects_integration_test.go
//
// Интеграционные тесты модели эффектов снабжения на НАСТОЯЩЕЙ PostgreSQL
// (спека 2026-09-22-эффекты-снабжения-задержка-голод §12: T2/T3/T4/T6/T8/
// T13/T24/T25/T31/T33/T36 + приёмка «голод работает»). Мок не воспроизводит
// ни типы параметров, ни advisory-локи, ни поведение буферов веток — слой
// потребности/owner-проход проверяются на живой БД.
//
// Живая dev-БД пуста по веткам производства (settlement_branches=0) — здесь
// контент-фикстура строится руками в изолированной схеме (по образцу
// contract_board_integration_test.go: своя случайная схема zorion_it_<rand>,
// migrations.Apply, DROP SCHEMA CASCADE). Без TEST_DATABASE_URL/DATABASE_URL
// тесты скипаются — обычный DoD-прогон остаётся зелёным.
package repository

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/rand"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"zorion/internal/economy/settlement"
	"zorion/migrations"
)

// supplyITPosition — позиция потребления пилота: name_norm ТОВАРА «пища»
// (спека 2026-09-24-потребление-по-товарам §6.1/§7.1), ключ params.effects/eat.
const supplyITPosition = "пища"

// supplyITOpenMigrated — случайная схема с накатанными миграциями; скип без
// TEST_DATABASE_URL/DATABASE_URL (обычный прогон остаётся зелёным).
func supplyITOpenMigrated(t *testing.T) *sql.DB {
	t.Helper()

	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		base = os.Getenv("DATABASE_URL")
	}
	if base == "" {
		t.Skip("TEST_DATABASE_URL/DATABASE_URL не задан — интеграционные тесты на БД пропущены")
	}

	admin, err := sql.Open("postgres", base)
	if err != nil {
		t.Skipf("не удалось открыть соединение с БД: %v", err)
	}
	if err := admin.Ping(); err != nil {
		admin.Close()
		t.Skipf("БД недоступна (%v) — интеграционные тесты на БД пропущены", err)
	}

	schema := fmt.Sprintf("zorion_it_%d", rand.Int63())
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		admin.Close()
		t.Fatalf("create schema %s: %v", schema, err)
	}

	dsn, err := supplyITDSN(base, schema)
	if err != nil {
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		admin.Close()
		t.Fatalf("scratch DSN: %v", err)
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		admin.Exec("DROP SCHEMA " + schema + " CASCADE")
		admin.Close()
		t.Skipf("не удалось открыть соединение со scratch-схемой: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
		if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
		admin.Close()
	})

	if err := migrations.Apply(db); err != nil {
		t.Fatalf("migrations.Apply: %v", err)
	}
	return db
}

// supplyITDSN — search_path через options DSN (на каждое соединение пула).
func supplyITDSN(base, schema string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("options", "-csearch_path="+schema)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func supplyITExec(t *testing.T, db *sql.DB, query string, args ...interface{}) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func supplyITScalarInt(t *testing.T, db *sql.DB, query string, args ...interface{}) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return n
}

func supplyITScalarFloat(t *testing.T, db *sql.DB, query string, args ...interface{}) float64 {
	t.Helper()
	var f float64
	if err := db.QueryRow(query, args...).Scan(&f); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return f
}

// supplyITSeedPlanet — минимальные world+planet (FK поселений/залежей).
func supplyITSeedPlanet(t *testing.T, db *sql.DB) string {
	t.Helper()
	worldID := uuid.New().String()
	planetID := uuid.New().String()
	supplyITExec(t, db, `INSERT INTO worlds (id, name, coord_x, coord_y) VALUES ($1, 'IT', 0, 0)`, worldID)
	supplyITExec(t, db, `INSERT INTO planets (id, world_id, name, orbit_index) VALUES ($1, $2, 'IT', 1)`, planetID, worldID)
	return planetID
}

// supplyITSeedCategory — позиция корзины (categories, kind='good').
func supplyITSeedCategory(t *testing.T, db *sql.DB, name, nameNorm string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(
		`INSERT INTO categories (name, name_norm, kind) VALUES ($1, $2, 'good') RETURNING id`,
		name, nameNorm,
	).Scan(&id); err != nil {
		t.Fatalf("insert category %q: %v", nameNorm, err)
	}
	return id
}

// supplyITSeedGood — товар каталога в категории (kind='good', props NULL).
func supplyITSeedGood(t *testing.T, db *sql.DB, name, nameNorm string, categoryID int64) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(
		`INSERT INTO goods (name, name_norm, category_id, kind) VALUES ($1, $2, $3, 'good') RETURNING id`,
		name, nameNorm, categoryID,
	).Scan(&id); err != nil {
		t.Fatalf("insert good %q: %v", nameNorm, err)
	}
	return id
}

// supplyITSeedRecipe — рецепт «товар-выход → один компонент», сложность c.
func supplyITSeedRecipe(t *testing.T, db *sql.DB, outputGoodID, componentGoodID int64, complexity int) int64 {
	t.Helper()
	var recipeID int64
	if err := db.QueryRow(
		`INSERT INTO recipes (good_id, complexity) VALUES ($1, $2) RETURNING id`,
		outputGoodID, complexity,
	).Scan(&recipeID); err != nil {
		t.Fatalf("insert recipe: %v", err)
	}
	supplyITExec(t, db, `INSERT INTO recipe_components (recipe_id, pos, component_id, quantity) VALUES ($1, 0, $2, 1)`,
		recipeID, componentGoodID)
	return recipeID
}

// supplyITSeedSettlement — поселение с чек-точкой населения.
func supplyITSeedSettlement(t *testing.T, db *sql.DB, planetID string, population int, computedAt time.Time) string {
	t.Helper()
	id := uuid.New().String()
	supplyITExec(t, db,
		`INSERT INTO settlements (id, planet_id, population, population_exact, computed_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $5)`,
		id, planetID, population, float64(population), computedAt)
	return id
}

// supplyITSeedProducerType — тип-постройка (producer_types) как множитель «чей
// рецепт» для числа скорости (И1, спека 2026-09-23 §3.3).
func supplyITSeedProducerType(t *testing.T, db *sql.DB, nameNorm string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(
		`INSERT INTO producer_types (name, name_norm, kind) VALUES ($1, $2, 'goods') RETURNING id`,
		nameNorm, nameNorm).Scan(&id); err != nil {
		t.Fatalf("insert producer_type %q: %v", nameNorm, err)
	}
	return id
}

// supplyITBindRate — число скорости пары (тип, рецепт), ед/сутки/млрд (§3.1).
func supplyITBindRate(t *testing.T, db *sql.DB, typeID, recipeID int64, rate float64) {
	t.Helper()
	supplyITExec(t, db,
		`INSERT INTO producer_recipes (producer_type_id, recipe_id, rate) VALUES ($1, $2, $3)`,
		typeID, recipeID, rate)
}

// supplyITSetSettlementType — тип поселения: число скорости читается по паре
// (тип, рецепт) (И1, §3.3).
func supplyITSetSettlementType(t *testing.T, db *sql.DB, settlementID string, typeID int64) {
	t.Helper()
	supplyITExec(t, db, `UPDATE settlements SET settlement_type_id = $1 WHERE id = $2`, typeID, settlementID)
}

// supplyITSeedBranch — ветка с буферами входа (компонент) и выхода (товар-выход).
func supplyITSeedBranch(t *testing.T, db *sql.DB, settlementID string, recipeID, outputGoodID, componentGoodID int64, input, output float64, processedAt time.Time) string {
	t.Helper()
	id := uuid.New().String()
	supplyITExec(t, db,
		`INSERT INTO settlement_branches (id, settlement_id, recipe_id, processed_at) VALUES ($1, $2, $3, $4)`,
		id, settlementID, recipeID, processedAt)
	supplyITExec(t, db,
		`INSERT INTO settlement_branch_buffers (branch_id, direction, good_id, amount) VALUES ($1, 'input', $2, $3)`,
		id, componentGoodID, input)
	supplyITExec(t, db,
		`INSERT INTO settlement_branch_buffers (branch_id, direction, good_id, amount) VALUES ($1, 'output', $2, $3)`,
		id, outputGoodID, output)
	return id
}

// supplyITSeedActiveEffect — хранимый базис нагрузки владельца (позиция —
// товар пилота supplyITPosition).
func supplyITSeedActiveEffect(t *testing.T, db *sql.DB, settlementID string, typeID int64, load float64, loadAt time.Time) {
	t.Helper()
	supplyITSeedActiveEffectPos(t, db, settlementID, typeID, supplyITPosition, load, loadAt)
}

// supplyITSeedActiveEffectPos — то же с явной позицией (для эффекта «жажда»).
func supplyITSeedActiveEffectPos(t *testing.T, db *sql.DB, settlementID string, typeID int64, position string, load float64, loadAt time.Time) {
	t.Helper()
	supplyITExec(t, db,
		`INSERT INTO active_effects (effect_type_id, owner_type, owner_id, owner_settlement_id, source_position, load, load_at)
		 VALUES ($1, 'settlement', $2, $2, $3, $4, $5)`,
		typeID, settlementID, position, load, loadAt)
}

// supplyITEffectTypeID — id типа «голод» (сид миграции 000070).
func supplyITEffectTypeID(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	return supplyITScalarInt(t, db, `SELECT id FROM effect_types WHERE name_norm = 'голод'`)
}

// supplyITOwner — вход owner-прохода: поселение с привязкой позиции-ТОВАРА
// «пища» → «голод», норма DefaultEatK; планета-комфорт (среда 0).
func supplyITOwner(settlementID, planetID string, computedAt time.Time, population int) OwnerSettlement {
	return OwnerSettlement{
		ID:                settlementID,
		PlanetID:          planetID,
		Population:        population,
		PopulationExact:   float64(population),
		ComputedAt:        computedAt,
		CreatedAt:         computedAt,
		Planet:            settlement.PlanetInput{TemperatureK: 288, GravityG: 1.0, CoreRadioactivity: 5},
		EatByPosition:     map[string]float64{supplyITPosition: settlement.DefaultEatK},
		EffectsByPosition: map[string]string{supplyITPosition: "голод"},
	}
}

// supplyITOwnerTyped — supplyITOwner с типом поселения (число скорости читается
// по паре (тип, рецепт), И1 §3.3: тип несёт OwnerSettlement).
func supplyITOwnerTyped(settlementID, planetID string, computedAt time.Time, population int, typeID int64) OwnerSettlement {
	o := supplyITOwner(settlementID, planetID, computedAt, population)
	o.SettlementTypeID = typeID
	return o
}

// supplyITSyncRun — один owner-проход и разбор единственного эффекта.
func supplyITSyncRun(t *testing.T, db *sql.DB, o OwnerSettlement, now time.Time) OwnerResult {
	t.Helper()
	out, err := NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{o})
	require.NoError(t, err)
	res, ok := out[o.ID]
	require.True(t, ok, "нет результата owner-прохода для %s", o.ID)
	return res
}

// supplyITRunPass — тот же owner-проход без записи (`runOwnerPass` напрямую):
// витрина + собранная траектория силы (`deathInput.Effects`), которую шов
// витрины не отдаёт (внутренний результат слоя потребности).
func supplyITRunPass(t *testing.T, db *sql.DB, o OwnerSettlement, now time.Time) (OwnerResult, []settlement.EffectForcePoint) {
	t.Helper()
	ctx := context.Background()
	repo := NewBranchRepository(db)
	catalog, err := repo.loadEffectTypeCatalog(ctx)
	require.NoError(t, err)
	known, err := repo.loadGoodNames(ctx)
	require.NoError(t, err)
	recs, err := loadBranches(ctx, db, []string{o.ID})
	require.NoError(t, err)
	stored, err := repo.loadActiveEffects(ctx, []string{o.ID})
	require.NoError(t, err)
	run, err := runOwnerPass(o, recs, stored[o.ID], catalog, known, ownerBatchData{}, nil, now, false)
	require.NoError(t, err)
	return run.result, run.deathInput.Effects
}

// T31/T36 + T2/T3 + T13: привязка по категории-позиции; полное покрытие → w=0;
// нет источника → coverage=0, w=1 без падения; две ветки одного типа эффекта →
// одна строка active_effects; удаление ветки-источника эффект не снимает.
func TestSupplyEffectsIntegrationBindingAndDeficitW(t *testing.T) {
	db := supplyITOpenMigrated(t)
	planetID := supplyITSeedPlanet(t, db)
	typeID := supplyITEffectTypeID(t, db)

	catID := supplyITSeedCategory(t, db, "Продовольствие", "продовольствие")
	out1 := supplyITSeedGood(t, db, "Пища", supplyITPosition, catID)
	out2 := supplyITSeedGood(t, db, "Еда", "еда", catID)
	comp := supplyITSeedGood(t, db, "Мясо", "мясо", catID)
	r1 := supplyITSeedRecipe(t, db, out1, comp, 1)
	r2 := supplyITSeedRecipe(t, db, out2, comp, 1)
	// Число скорости пары (тип × рецепт): 660 > нормы 600 — производство
	// покрывает спрос позиции (И1, §3.2). Без числа ветка была бы инертна.
	pt := supplyITSeedProducerType(t, db, "ит-тип-1")
	supplyITBindRate(t, db, pt, r1, 660)
	supplyITBindRate(t, db, pt, r2, 660)

	t0 := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	s1 := supplyITSeedSettlement(t, db, planetID, 1_000_000, t0)
	supplyITSetSettlementType(t, db, s1, pt)
	supplyITSeedBranch(t, db, s1, r1, out1, comp, 1e6, 0, t0)
	// Базис нагрузки на computed_at: иначе (новая строка) load_at = now и
	// интервал нулевой — покрытие/дефицит не наблюдаются (М4, §3.2).
	supplyITSeedActiveEffect(t, db, s1, typeID, 0, t0)

	// Шаг A: источник есть, числа скорости хватает на спрос → w=0, нагрузка не растёт.
	nowA := time.Now()
	resA := supplyITSyncRun(t, db, supplyITOwnerTyped(s1, planetID, t0, 1_000_000, pt), nowA)
	require.Len(t, resA.Effects, 1, "привязка по товару-позиции → один эффект")
	require.InDelta(t, 0.0, resA.Effects[0].W, 1e-9, "полное покрытие → w=0")
	require.InDelta(t, 0.0, resA.Effects[0].Load, 1e-9, "при w=0 нагрузка не растёт")
	require.False(t, resA.Effects[0].Enabled, "порог 24 не достигнут")
	require.Equal(t, int64(1), supplyITScalarInt(t, db, `SELECT COUNT(*) FROM active_effects WHERE owner_id = $1`, s1))

	// Шаг B: вторая ветка второго товара-позиции ТОГО ЖЕ типа эффекта («голод»):
	// оба товара привязаны к «голоду», две ветки → по-прежнему один эффект и одна
	// строка на тип (T13). Случай «две ветки ОДНОЙ позиции» интеграционно не
	// строится: позиция = goods.name_norm (UNIQUE), на товар один рецепт (UNIQUE
	// good_id) и одна ветка на (поселение, рецепт) — остаётся юнит-регресс
	// TestSyncSettlementsTwoBranchesOneEffect.
	supplyITSeedBranch(t, db, s1, r2, out2, comp, 1e6, 0, nowA)
	ownerB := supplyITOwnerTyped(s1, planetID, nowA, 1_000_000, pt)
	ownerB.EffectsByPosition["еда"] = "голод"
	ownerB.EatByPosition["еда"] = settlement.DefaultEatK
	nowB := nowA.Add(time.Hour)
	resB := supplyITSyncRun(t, db, ownerB, nowB)
	require.Len(t, resB.Effects, 1, "два источника одного типа эффекта → один эффект (T13)")
	require.InDelta(t, 0.0, resB.Effects[0].W, 1e-9, "обе позиции покрыты")
	require.Equal(t, int64(1), supplyITScalarInt(t, db, `SELECT COUNT(*) FROM active_effects WHERE owner_id = $1`, s1),
		"одна строка active_effects на тип эффекта (T13)")

	// Шаг C: удаляем ВСЕ ветки-источники → привязанная позиция без источника:
	// coverage=0, w=1, без падения (T31/T36); эффект не снимается (T13).
	supplyITExec(t, db, `DELETE FROM settlement_branches WHERE settlement_id = $1`, s1)
	nowC := nowB.Add(2 * time.Hour)
	resC := supplyITSyncRun(t, db, supplyITOwnerTyped(s1, planetID, nowB, 1_000_000, pt), nowC)
	require.Len(t, resC.Effects, 1, "удаление ветки-источника эффект не снимает (T13)")
	require.InDelta(t, 1.0, resC.Effects[0].W, 1e-9, "позиция без источника → w=1 (T36)")
	require.InDelta(t, 2.0, resC.Effects[0].Load, 1e-6, "2 часа при w=1 → load = 2 сило-часа (T4)")
	require.Equal(t, supplyITPosition, *resC.Effects[0].SourcePosition)
	require.Equal(t, int64(1), supplyITScalarInt(t, db, `SELECT COUNT(*) FROM active_effects WHERE owner_id = $1`, s1),
		"строка нагрузки остаётся после удаления ветки (T13)")
}

// T4 (точный): нагрузка `load += w·len/3600` при w=1; базис load_at = computed_at.
func TestSupplyEffectsIntegrationLoadAccumulation(t *testing.T) {
	db := supplyITOpenMigrated(t)
	planetID := supplyITSeedPlanet(t, db)
	typeID := supplyITEffectTypeID(t, db)
	catID := supplyITSeedCategory(t, db, "Продовольствие", "продовольствие")
	supplyITSeedGood(t, db, "Пища", supplyITPosition, catID)

	now := time.Now()
	computedAt := now.Add(-2 * time.Hour).Truncate(time.Microsecond)
	s1 := supplyITSeedSettlement(t, db, planetID, 1_000_000, computedAt)
	supplyITSeedActiveEffect(t, db, s1, typeID, 0, computedAt)

	res := supplyITSyncRun(t, db, supplyITOwner(s1, planetID, computedAt, 1_000_000), now)
	require.Len(t, res.Effects, 1)
	require.InDelta(t, 1.0, res.Effects[0].W, 1e-9, "нет источника → w=1")
	require.InDelta(t, 2.0, res.Effects[0].Load, 1e-9, "ровно 2 часа при w=1 → load = 2")
	require.InDelta(t, 2.0, supplyITScalarFloat(t, db,
		`SELECT load FROM active_effects WHERE owner_id = $1`, s1), 1e-9, "нагрузка записана в БД")
}

// Регресс к идее 2026-09-25 (найдено @tester): первая ПЕРСИСТЕНТНАЯ запись не
// должна обнулять накопленную нагрузку. Ветка INSERT `activeEffectUpsertSQL`
// обязана писать вычисленный `load` ($5), а не литерал 0 — иначе «счёт с
// рождения» держится только на пути «в памяти», и первый чек-поинт (~30 мин)
// начинает счёт заново. sqlmock семантику INSERT не эмулирует — кейс проверяется
// на настоящей PostgreSQL (skips без DATABASE_URL). Guard текста SQL без БД —
// TestActiveEffectUpsertInsertCarriesComputedLoad.
func TestSupplyEffectsIntegrationFirstPersistKeepsLoad(t *testing.T) {
	db := supplyITOpenMigrated(t)
	planetID := supplyITSeedPlanet(t, db)
	catID := supplyITSeedCategory(t, db, "Продовольствие", "продовольствие")
	supplyITSeedGood(t, db, "Пища", supplyITPosition, catID)

	// Новорождённое поселение (computed_at == created_at), без сохранённого
	// базиса нагрузки и без источников: за 2 ч с рождения w=1 → load = 2.
	computedAt := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	s1 := supplyITSeedSettlement(t, db, planetID, 1_000_000, computedAt)

	now := time.Now()
	res := supplyITSyncRun(t, db, supplyITOwner(s1, planetID, computedAt, 1_000_000), now)
	require.Len(t, res.Effects, 1)
	require.InDelta(t, 1.0, res.Effects[0].W, 1e-9, "нет источника → w=1")
	require.InDelta(t, 2.0, res.Effects[0].Load, 1e-6, "первый проход видит нагрузку с рождения")

	stored := supplyITScalarFloat(t, db, `SELECT load FROM active_effects WHERE owner_id = $1`, s1)
	require.InDelta(t, 2.0, stored, 1e-6, "INSERT первой строки сохранил вычисленную нагрузку (не 0)")
	require.Greater(t, stored, 0.0, "нагрузка не обнулена первой персистентной записью")
	require.Equal(t, int64(1), supplyITScalarInt(t, db,
		`SELECT COUNT(*) FROM active_effects WHERE owner_id = $1`, s1))

	// Второй проход (+1 ч) продолжает с сохранённого базиса: 2 + 1 = 3 —
	// счёт не начинается заново.
	now2 := now.Add(time.Hour)
	res2 := supplyITSyncRun(t, db, supplyITOwner(s1, planetID, now, 1_000_000), now2)
	require.InDelta(t, 3.0, res2.Effects[0].Load, 1e-6, "нагрузка продолжается, а не сбрасывается")
	require.InDelta(t, 3.0, supplyITScalarFloat(t, db,
		`SELECT load FROM active_effects WHERE owner_id = $1`, s1), 1e-6)
}

// T4/T6/М2: порог 24 (нулевой префикс) — при load < порога «снят» (сила 0);
// ровно в точке порога «включён, сила 0»; строго выше — включён, сила > 0.
// Путь «в памяти» (Δt=0) не двигает базис — нагрузка остаётся ровно заданной.
func TestSupplyEffectsIntegrationThreshold(t *testing.T) {
	db := supplyITOpenMigrated(t)
	planetID := supplyITSeedPlanet(t, db)
	typeID := supplyITEffectTypeID(t, db)
	catID := supplyITSeedCategory(t, db, "Продовольствие", "продовольствие")
	supplyITSeedGood(t, db, "Пища", supplyITPosition, catID)

	now := time.Now()
	loads := map[string]float64{"low": 23.5, "edge": 24.0, "high": 25.0}
	owners := make([]OwnerSettlement, 0, len(loads))
	ids := map[string]string{}
	for name, load := range loads {
		id := supplyITSeedSettlement(t, db, planetID, 1_000_000, now)
		supplyITSeedActiveEffect(t, db, id, typeID, load, now)
		owners = append(owners, supplyITOwner(id, planetID, now, 1_000_000))
		ids[name] = id
	}

	out, err := NewBranchRepository(db).SyncSettlements(now, owners)
	require.NoError(t, err)

	for _, c := range []struct {
		name     string
		enabled  bool
		rateZero bool
	}{
		{"low", false, true},
		{"edge", true, true},  // в точке порога «включён», но сила 0 (М2)
		{"high", true, false}, // строго выше порога — сила > 0
	} {
		res := out[ids[c.name]]
		require.Len(t, res.Effects, 1, c.name)
		e := res.Effects[0]
		require.InDelta(t, 24.0, e.Threshold, 1e-9, "%s: порог 24", c.name)
		require.InDelta(t, loads[c.name], e.Load, 1e-9, "%s: нагрузка не сдвинута (путь в памяти)", c.name)
		require.Equal(t, c.enabled, e.Enabled, "%s: состояние по порогу (load >= 24)", c.name)
		if c.rateZero {
			require.InDelta(t, 0.0, e.Rate, 1e-15, "%s: сила 0 (на/ниже порога)", c.name)
		} else {
			require.Greater(t, e.Rate, 0.0, "%s: сила строго > 0 выше порога", c.name)
		}
	}
}

// Приёмка «голод работает» + T24/T25: длительное реальное покрытие (w=0)
// уводит нагрузку РОВНО в 0 (шрама нет, решение 10), эффект снят; после
// прохода load_at == processed_at у веток == computed_at; вклад эффекта
// покрывает ровно [computed_at, now).
func TestSupplyEffectsIntegrationRecoveryToZeroAndBasis(t *testing.T) {
	db := supplyITOpenMigrated(t)
	planetID := supplyITSeedPlanet(t, db)
	typeID := supplyITEffectTypeID(t, db)

	catID := supplyITSeedCategory(t, db, "Продовольствие", "продовольствие")
	outGood := supplyITSeedGood(t, db, "Пища", supplyITPosition, catID)
	comp := supplyITSeedGood(t, db, "Мясо", "мясо", catID)
	recipe := supplyITSeedRecipe(t, db, outGood, comp, 1)

	// load = 30 сило-ч; recovery = 0.25/ч → нуль при покрытии за 120 ч.
	computedAt := time.Now().Add(-120 * time.Hour).Truncate(time.Microsecond)
	s1 := supplyITSeedSettlement(t, db, planetID, 1_000_000, computedAt)
	// Число скорости пары покрывает спрос (660 > 600): w=0 весь интервал.
	pt := supplyITSeedProducerType(t, db, "ит-тип-recovery")
	supplyITBindRate(t, db, pt, recipe, 660)
	supplyITSetSettlementType(t, db, s1, pt)
	supplyITSeedBranch(t, db, s1, recipe, outGood, comp, 1e6, 0, computedAt)
	supplyITSeedActiveEffect(t, db, s1, typeID, 30, computedAt)

	now := time.Now()
	res := supplyITSyncRun(t, db, supplyITOwnerTyped(s1, planetID, computedAt, 1_000_000, pt), now)

	require.Len(t, res.Effects, 1)
	require.InDelta(t, 0.0, res.Effects[0].W, 1e-9, "полное покрытие производством → w=0")
	require.InDelta(t, 0.0, res.Effects[0].Load, 1e-9, "120 ч покрытия при recovery 0.25 → load ровно 0 (шрама нет)")
	require.False(t, res.Effects[0].Enabled, "нагрузка 0 < порога → эффект снят")

	// T24: инвариант базиса — load_at == processed_at у ветки == computed_at.
	require.Equal(t, int64(1), supplyITScalarInt(t, db, `
		SELECT COUNT(*) FROM settlement_branches b
		JOIN settlements s ON s.id = b.settlement_id
		JOIN active_effects ae ON ae.owner_id = b.settlement_id
		WHERE b.settlement_id = $1 AND b.processed_at = s.computed_at AND s.computed_at = ae.load_at`, s1),
		"load_at == processed_at == computed_at после owner-прохода (T24)")
}

// T25: вклад эффекта покрывает ровно [computed_at, now). Траектория силы —
// внутренний результат слоя потребности (в витрине моделей её нет, production
// API ради теста не расширяется): проверяем шов owner-прохода напрямую —
// `runOwnerPass` возвращает `deathInput.Effects` (собранная траектория).
func TestSupplyEffectsIntegrationForceWindow(t *testing.T) {
	db := supplyITOpenMigrated(t)
	planetID := supplyITSeedPlanet(t, db)
	typeID := supplyITEffectTypeID(t, db)

	catID := supplyITSeedCategory(t, db, "Продовольствие", "продовольствие")
	outGood := supplyITSeedGood(t, db, "Пища", supplyITPosition, catID)
	comp := supplyITSeedGood(t, db, "Мясо", "мясо", catID)
	recipe := supplyITSeedRecipe(t, db, outGood, comp, 1)

	computedAt := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	s1 := supplyITSeedSettlement(t, db, planetID, 1_000_000, computedAt)
	supplyITSeedBranch(t, db, s1, recipe, outGood, comp, 1e6, 0, computedAt)
	supplyITSeedActiveEffect(t, db, s1, typeID, 0, computedAt)

	now := time.Now()
	_, force := supplyITRunPass(t, db, supplyITOwner(s1, planetID, computedAt, 1_000_000), now)
	require.NotEmpty(t, force, "при привязанной позиции траектория силы не пуста")
	for i, p := range force {
		require.False(t, p.Since.Before(computedAt), "сегмент %d не начинается раньше computed_at (T25)", i)
		require.False(t, p.Until.After(now), "сегмент %d не выходит за now (T25)", i)
	}
	require.WithinDuration(t, computedAt, force[0].Since, time.Millisecond, "вклад начинается с computed_at (T25)")
	require.WithinDuration(t, now, force[len(force)-1].Until, time.Millisecond, "вклад заканчивается now (T25)")
}

// T33: выходной буфер пишется ОДИН раз как `O0_b + batches_b − drawn_b` —
// двойного списания/прибавления производства нет.
func TestSupplyEffectsIntegrationOutputBufferSingleWrite(t *testing.T) {
	db := supplyITOpenMigrated(t)
	planetID := supplyITSeedPlanet(t, db)
	typeID := supplyITEffectTypeID(t, db)

	catID := supplyITSeedCategory(t, db, "Продовольствие", "продовольствие")
	outGood := supplyITSeedGood(t, db, "Пища", supplyITPosition, catID)
	comp := supplyITSeedGood(t, db, "Мясо", "мясо", catID)
	recipe := supplyITSeedRecipe(t, db, outGood, comp, 1)
	// Число скорости 333.6 < нормы 600: производство меньше спроса (p/demand =
	// 0.556), дефицит покрывается из базиса буфера (drawn > 0). Прежняя формула
	// давала тот же перекос через complexity=2 — теперь темп задаёт число пары.
	const ratePerDay = 333.6
	pt := supplyITSeedProducerType(t, db, "ит-тип-single")
	supplyITBindRate(t, db, pt, recipe, ratePerDay)

	const (
		population = 1_000_000
		baseO0     = 0.1
	)
	now := time.Now()
	computedAt := now.Add(-time.Hour).Truncate(time.Microsecond)
	s1 := supplyITSeedSettlement(t, db, planetID, population, computedAt)
	supplyITSetSettlementType(t, db, s1, pt)
	branchID := supplyITSeedBranch(t, db, s1, recipe, outGood, comp, 1e6, baseO0, computedAt)
	supplyITSeedActiveEffect(t, db, s1, typeID, 0, computedAt)

	res := supplyITSyncRun(t, db, supplyITOwnerTyped(s1, planetID, computedAt, population, pt), now)

	// Ожидания: batches = PerSecond(rate, P)·1ч; demand = PerSecond(норма, P);
	// drawn = (demand − p)·3600 — единая точка конверсии, без ·/3600.
	batches := settlement.PerSecond(ratePerDay, float64(population)) * 3600
	demand := settlement.PerSecond(settlement.DefaultEatK, float64(population))
	netPerSec := demand - batches/3600
	drawn := netPerSec * 3600
	want := baseO0 + batches - drawn

	got := supplyITScalarFloat(t, db,
		`SELECT amount FROM settlement_branch_buffers WHERE branch_id = $1 AND direction = 'output' AND good_id = $2`,
		branchID, outGood)
	require.InDelta(t, want, got, 1e-9, "выход = O0_b + batches_b − drawn_b (единственная точка записи, T33)")
	require.InDelta(t, want, res.Branches[0].Output[0].Amount, 1e-9, "display-буфер = записанный выход")
	require.InDelta(t, drawn, res.Branches[0].Eaten, 1e-9, "списано слоем потребности ровно drawn_b")

	// Контроль отсутствия двойного учёта: вариант `O0 + 2·batches − drawn` отличим.
	doubleCounted := baseO0 + 2*batches - drawn
	require.True(t, math.Abs(doubleCounted-got) > 1e-9, "batches_b вошёл ровно один раз (нет двойного прибавления)")
}

// T8: затяжной голод → гибель от голода: settlement_log.type='extinct' с
// cause='hunger', население обнулено.
func TestSupplyEffectsIntegrationExtinctHunger(t *testing.T) {
	db := supplyITOpenMigrated(t)
	planetID := supplyITSeedPlanet(t, db)
	typeID := supplyITEffectTypeID(t, db)
	catID := supplyITSeedCategory(t, db, "Продовольствие", "продовольствие")
	supplyITSeedGood(t, db, "Пища", supplyITPosition, catID)

	// Малое население, длинный интервал, высокая нагрузка (плато 1e-7/сек) —
	// население уходит ниже NDead → Recompute возвращает 0 → запись «Вымерло».
	computedAt := time.Now().Add(-300 * 24 * time.Hour).Truncate(time.Microsecond)
	const pop = 1000
	s1 := supplyITSeedSettlement(t, db, planetID, pop, computedAt)
	supplyITSeedActiveEffect(t, db, s1, typeID, 1000, computedAt)

	now := time.Now()
	res := supplyITSyncRun(t, db, supplyITOwner(s1, planetID, computedAt, pop), now)
	require.Equal(t, 0, res.Population, "население обнулено (вымерло)")
	require.InDelta(t, 0.0, supplyITScalarFloat(t, db,
		`SELECT population_exact FROM settlements WHERE id = $1`, s1), 1e-9, "population_exact = 0")

	require.Equal(t, int64(1), supplyITScalarInt(t, db, `
		SELECT COUNT(*) FROM settlement_log
		WHERE settlement_id = $1 AND type = 'extinct' AND cause = 'hunger'`, s1),
		"лог «Вымерло · Голод» (T8)")
}

// T19 (интеграция, спека 2026-09-24 §8.3): идентичность эффекта доходит от
// owner-прохода до причины гибели — «жажда» даёт settlement_log.cause='thirst',
// а не 'hunger' (общая кривая hunger у обоих типов их не различает).
func TestSupplyEffectsIntegrationExtinctThirst(t *testing.T) {
	db := supplyITOpenMigrated(t)
	planetID := supplyITSeedPlanet(t, db)
	thirstTypeID := supplyITScalarInt(t, db, `SELECT id FROM effect_types WHERE name_norm = 'жажда'`)
	catID := supplyITSeedCategory(t, db, "Продовольствие", "продовольствие")
	supplyITSeedGood(t, db, "Очищенная вода", "очищенная вода", catID)

	// Малое население, длинный интервал, высокая нагрузка — обвал ниже NDead.
	computedAt := time.Now().Add(-300 * 24 * time.Hour).Truncate(time.Microsecond)
	const pop = 1000
	s1 := supplyITSeedSettlement(t, db, planetID, pop, computedAt)
	supplyITSeedActiveEffectPos(t, db, s1, thirstTypeID, "очищенная вода", 1000, computedAt)

	o := supplyITOwner(s1, planetID, computedAt, pop)
	o.EffectsByPosition = map[string]string{"очищенная вода": "жажда"}
	o.EatByPosition = map[string]float64{"очищенная вода": settlement.DefaultEatK}

	res := supplyITSyncRun(t, db, o, time.Now())
	require.Equal(t, 0, res.Population, "население обнулено (вымерло)")

	// Привязка резолвится: эффект — «жажда», позиция — товар «очищенная вода»,
	// и та же позиция видна в витрине арифметики (спека 2026-09-24 §9.4/§10).
	require.Len(t, res.Effects, 1, "привязка по товару «очищенная вода» резолвится")
	require.Equal(t, thirstTypeID, res.Effects[0].EffectTypeID, "эффект — «жажда»")
	require.Equal(t, "Жажда", res.Effects[0].Name)
	require.NotNil(t, res.Effects[0].SourcePosition)
	require.Equal(t, "очищенная вода", *res.Effects[0].SourcePosition)
	require.Len(t, res.Arithmetic, 1, "позиция-товар в витрине арифметики")
	require.Equal(t, "очищенная вода", res.Arithmetic[0].Position)

	require.Equal(t, int64(1), supplyITScalarInt(t, db, `
		SELECT COUNT(*) FROM settlement_log
		WHERE settlement_id = $1 AND type = 'extinct' AND cause = 'thirst'`, s1),
		"лог «Вымерло · Жажда» (T19)")
}
