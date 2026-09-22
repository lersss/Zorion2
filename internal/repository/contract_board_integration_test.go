// internal/repository/contract_board_integration_test.go
//
// Интеграционные тесты материализации доски на НАСТОЯЩЕЙ PostgreSQL (спека
// 2026-09-23-контракт-ленивая-доска-пакет-и-снабжение §14: T2/T4/T8/T15/T16).
// Мок не проверяет ни гонки, ни порядок захвата блокировок — репродукция T16
// (материализация × ExpireDue без взаимоблокировки) возможна только на живой БД.
//
// Тест живёт в package repository, потому что шов нужд (boardNeeds) —
// unexported: производственный API ради тестов не расширяется. Scratch-харнесс
// компактно повторён здесь (образец — internal/integration/helpers_test.go):
// своя случайная схема zorion_it_<rand>, migrations.Apply, DROP SCHEMA CASCADE.
// Без TEST_DATABASE_URL/DATABASE_URL тесты скипаются.
package repository

import (
	"database/sql"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"zorion/internal/models"
	"zorion/migrations"
)

// itBoardReward — цена тестового контракта (> 0: CHECK contracts_live_has_escrow).
const itBoardReward = 300

// boardITScratch — изолированная схема для одного интеграционного теста.
type boardITScratch struct {
	db *sql.DB // пул с search_path=<schema>
}

// boardITOpenMigrated — случайная схема с накатанными миграциями; скип без
// TEST_DATABASE_URL/DATABASE_URL (обычный DoD-прогон остаётся зелёным).
func boardITOpenMigrated(t *testing.T) *boardITScratch {
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

	dsn, err := boardITDSN(base, schema)
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
	return &boardITScratch{db: db}
}

// boardITDSN — search_path через options DSN (на каждое соединение пула).
func boardITDSN(base, schema string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("options", "-csearch_path="+schema)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// boardITSeedPlanet — минимальные world+planet (FK publication_planet_id).
func boardITSeedPlanet(t *testing.T, db *sql.DB) string {
	t.Helper()
	worldID := uuid.New().String()
	planetID := uuid.New().String()
	if _, err := db.Exec(
		`INSERT INTO worlds (id, name, coord_x, coord_y) VALUES ($1, 'IT', 0, 0)`, worldID,
	); err != nil {
		t.Fatalf("insert world: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO planets (id, world_id, name, orbit_index) VALUES ($1, $2, 'IT', 1)`,
		planetID, worldID,
	); err != nil {
		t.Fatalf("insert planet: %v", err)
	}
	return planetID
}

// boardITSeedBuilding — постройка-заказчик (author_type='building'); владелец —
// игрок (его счёт и есть плательщик залога, resolvePayerAccountQ).
func boardITSeedBuilding(t *testing.T, db *sql.DB, planetID, ownerID string) string {
	t.Helper()
	buildingID := uuid.New().String()
	if _, err := db.Exec(
		`INSERT INTO buildings (id, planet_id, building_type, owner_type, owner_id)
		 VALUES ($1, $2, 'factory', 'player', $3)`,
		buildingID, planetID, ownerID,
	); err != nil {
		t.Fatalf("insert building: %v", err)
	}
	return buildingID
}

// boardITNeed — нужда одного пакета.
func boardITNeed(buildingID, planetID, goodID string, target, actual float64, caps []float64) BoardNeed {
	return BoardNeed{
		AuthorID:   buildingID,
		PlanetID:   planetID,
		GoodID:     goodID,
		Target:     target,
		Actual:     actual,
		Capacities: caps,
		BaseReward: 10,
	}
}

func boardITCount(t *testing.T, db *sql.DB, query string, args ...interface{}) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return n
}

func boardITStr(t *testing.T, db *sql.DB, query string, args ...interface{}) string {
	t.Helper()
	var s string
	if err := db.QueryRow(query, args...).Scan(&s); err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return s
}

// boardITShare — снимок полей доли, которые материализация трогать не должна.
type boardITShare struct {
	status   string
	reward   int64
	quantity int64
	expires  time.Time
}

func boardITShareSnapshot(t *testing.T, db *sql.DB, contractID string) boardITShare {
	t.Helper()
	var s boardITShare
	err := db.QueryRow(`SELECT c.status, c.reward, c.expires_at,
			COALESCE((SELECT r.quantity FROM contract_requirements r
			           WHERE r.contract_id = c.id AND r.kind = 'goods'
			           ORDER BY r.pos LIMIT 1), 0)
		FROM contracts c WHERE c.id = $1`, contractID).
		Scan(&s.status, &s.reward, &s.expires, &s.quantity)
	if err != nil {
		t.Fatalf("share snapshot %s: %v", contractID, err)
	}
	return s
}

// T2 (живая гонка): две параллельные материализации одной планеты → ровно один
// комплект долей (частичный уникальный индекс держит (package_key, share_index)
// при status='open') и ровно одна строка чек-точки; ошибок нет.
func TestMaterializeBoardRaceIT(t *testing.T) {
	s := boardITOpenMigrated(t)
	planetID := boardITSeedPlanet(t, s.db)
	playerID := uuid.New().String()
	buildingID := boardITSeedBuilding(t, s.db, planetID, playerID)

	repo := NewContractRepository(s.db)
	need := boardITNeed(buildingID, planetID, "good-1", 100, 0, []float64{100})
	repo.boardNeeds = func(string) ([]BoardNeed, error) { return []BoardNeed{need}, nil }

	now := time.Now()
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = repo.MaterializeBoard(planetID, now)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("MaterializeBoard[%d]: %v", i, err)
		}
	}

	pkg := supplyPackageKey(planetID, buildingID, "good-1")
	if n := boardITCount(t, s.db,
		`SELECT COUNT(*) FROM contracts WHERE package_key = $1 AND status = 'open'`, pkg); n != 1 {
		t.Fatalf("открытых долей пакета %d, ожидалась ровно 1", n)
	}
	if sum := boardITCount(t, s.db,
		`SELECT COALESCE(SUM(r.quantity), 0) FROM contracts c
		   JOIN contract_requirements r ON r.contract_id = c.id AND r.kind = 'goods'
		 WHERE c.package_key = $1 AND c.status = 'open'`, pkg); sum != 100 {
		t.Fatalf("суммарный объём открытых долей %d, ожидался 100", sum)
	}
	if n := boardITCount(t, s.db,
		`SELECT COUNT(*) FROM contract_board_state WHERE planet_id = $1`, planetID); n != 1 {
		t.Fatalf("строк чек-точки %d, ожидалась ровно 1", n)
	}
}

// T16 (живая, §14): материализация (публикация/отмена долей) и ExpireDue на
// одной планете параллельно, несколько циклов — обе операции завершаются без
// взаимоблокировки (40P01) и без «залипания».
func TestMaterializeBoardExpireDueNoDeadlockIT(t *testing.T) {
	s := boardITOpenMigrated(t)
	planetID := boardITSeedPlanet(t, s.db)
	// Один и тот же игрок — автор истёкших контрактов и владелец постройки:
	// обе транзакции спорят за ОДНУ строку accounts (сильнейший случай).
	playerID := uuid.New().String()
	buildingID := boardITSeedBuilding(t, s.db, planetID, playerID)

	repo := NewContractRepository(s.db)
	target := 100.0
	repo.boardNeeds = func(string) ([]BoardNeed, error) {
		return []BoardNeed{boardITNeed(buildingID, planetID, "good-1", target, 0, []float64{100})}, nil
	}

	for i := 0; i < 6; i++ {
		// Живой истёкший контракт этой планеты — работа для ExpireDue.
		if _, err := repo.Publish(PublishContractParams{
			Type:                "delivery",
			AuthorType:          models.ContractActorPlayer,
			AuthorID:            playerID,
			PublicationPlanetID: planetID,
			Title:               "IT expired",
			Reward:              itBoardReward,
			ExpiresAt:           time.Now().Add(-time.Minute),
		}); err != nil {
			t.Fatalf("цикл %d: Publish истёкшего: %v", i, err)
		}
		// Чередуем дефицит: 200 (публикация 2 долей) / 100 (отмена лишней) —
		// материализация реально трогает contracts+accounts каждый цикл.
		if i%2 == 0 {
			target = 200
		} else {
			target = 100
		}
		// Сброс чек-точки: материализация идёт безусловно (иначе кадэнс пропустит).
		if _, err := s.db.Exec(`DELETE FROM contract_board_state WHERE planet_id = $1`, planetID); err != nil {
			t.Fatalf("цикл %d: reset board_state: %v", i, err)
		}

		var wg sync.WaitGroup
		var mErr, eErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, mErr = repo.MaterializeBoard(planetID, time.Now())
		}()
		go func() {
			defer wg.Done()
			_, eErr = repo.ExpireDue(ContractScope{PlanetID: planetID})
		}()
		wg.Wait()

		if mErr != nil {
			t.Fatalf("цикл %d: MaterializeBoard: %v", i, mErr)
		}
		if eErr != nil {
			t.Fatalf("цикл %d: ExpireDue: %v", i, eErr)
		}
	}

	// Материализация отработала: открытые доли пакета есть; истёкших живых нет.
	pkg := supplyPackageKey(planetID, buildingID, "good-1")
	if n := boardITCount(t, s.db,
		`SELECT COUNT(*) FROM contracts WHERE package_key = $1 AND status = 'open'`, pkg); n == 0 {
		t.Fatal("после циклов нет открытых долей пакета — материализация не отработала")
	}
	if n := boardITCount(t, s.db,
		`SELECT COUNT(*) FROM contracts
		 WHERE publication_planet_id = $1 AND status IN ('open','taken') AND expires_at <= NOW()`,
		planetID); n != 0 {
		t.Fatalf("остались живые просроченные контракты (%d) — ExpireDue не отработал", n)
	}
}

// T4 (живая): взятая доля при материализации не меняется — статус, награда,
// объём требования и expires_at те же; пакет при нулевом дефиците схлопывается.
func TestMaterializeBoardTakenShareUntouchedIT(t *testing.T) {
	s := boardITOpenMigrated(t)
	planetID := boardITSeedPlanet(t, s.db)
	playerID := uuid.New().String()
	buildingID := boardITSeedBuilding(t, s.db, planetID, playerID)

	repo := NewContractRepository(s.db)
	repo.boardNeeds = func(string) ([]BoardNeed, error) {
		return []BoardNeed{boardITNeed(buildingID, planetID, "good-1", 100, 0, []float64{100})}, nil
	}
	if ok, err := repo.MaterializeBoard(planetID, time.Now()); err != nil || !ok {
		t.Fatalf("первая материализация: ok=%v err=%v", ok, err)
	}

	pkg := supplyPackageKey(planetID, buildingID, "good-1")
	shareID := boardITStr(t, s.db,
		`SELECT id FROM contracts WHERE package_key = $1 AND status = 'open'`, pkg)
	executorID := uuid.New().String()
	if ok, err := repo.Take(shareID, models.ContractExecutorPlayer, executorID, nil); err != nil || !ok {
		t.Fatalf("Take: ok=%v err=%v", ok, err)
	}
	before := boardITShareSnapshot(t, s.db, shareID)

	// Дефицит упал до нуля → пакет схлопывается; сброс чек-точки — материализация идёт.
	repo.boardNeeds = func(string) ([]BoardNeed, error) {
		return []BoardNeed{boardITNeed(buildingID, planetID, "good-1", 0, 0, []float64{100})}, nil
	}
	if _, err := s.db.Exec(`DELETE FROM contract_board_state WHERE planet_id = $1`, planetID); err != nil {
		t.Fatalf("reset board_state: %v", err)
	}
	if ok, err := repo.MaterializeBoard(planetID, time.Now()); err != nil || !ok {
		t.Fatalf("повторная материализация: ok=%v err=%v", ok, err)
	}

	after := boardITShareSnapshot(t, s.db, shareID)
	if after.status != before.status || after.reward != before.reward ||
		after.quantity != before.quantity || !after.expires.Equal(before.expires) {
		t.Fatalf("взятая доля изменилась материализацией: до %+v, после %+v", before, after)
	}
	if after.status != models.ContractStatusTaken {
		t.Fatalf("статус взятой доли %q, ожидался %q", after.status, models.ContractStatusTaken)
	}
	if n := boardITCount(t, s.db,
		`SELECT COUNT(*) FROM contracts WHERE package_key = $1 AND status = 'open'`, pkg); n != 0 {
		t.Fatalf("открытых долей после схлопывания %d, ожидалось 0", n)
	}
}

// T8/T15 (живая): планета без нужд — ни строки чек-точки, ни контрактов;
// с нуждами — ровно одна строка чек-точки, повторный прогон её не множит.
func TestMaterializeBoardSparseIT(t *testing.T) {
	s := boardITOpenMigrated(t)
	planetID := boardITSeedPlanet(t, s.db)
	playerID := uuid.New().String()
	buildingID := boardITSeedBuilding(t, s.db, planetID, playerID)

	repo := NewContractRepository(s.db)

	// Нужд нет (шов пуст): no-op, ничего не пишется.
	ok, err := repo.MaterializeBoard(planetID, time.Now())
	if err != nil || ok {
		t.Fatalf("без нужд: ok=%v err=%v", ok, err)
	}
	if n := boardITCount(t, s.db,
		`SELECT COUNT(*) FROM contract_board_state WHERE planet_id = $1`, planetID); n != 0 {
		t.Fatalf("без нужд создано строк чек-точки %d, ожидалось 0", n)
	}
	if n := boardITCount(t, s.db,
		`SELECT COUNT(*) FROM contracts WHERE publication_planet_id = $1`, planetID); n != 0 {
		t.Fatalf("без нужд создано контрактов %d, ожидалось 0", n)
	}

	// Нужды есть: строка чек-точки ровно одна, комплект долей один.
	repo.boardNeeds = func(string) ([]BoardNeed, error) {
		return []BoardNeed{boardITNeed(buildingID, planetID, "good-1", 100, 0, []float64{100})}, nil
	}
	if ok, err := repo.MaterializeBoard(planetID, time.Now()); err != nil || !ok {
		t.Fatalf("с нуждами: ok=%v err=%v", ok, err)
	}
	if n := boardITCount(t, s.db,
		`SELECT COUNT(*) FROM contract_board_state WHERE planet_id = $1`, planetID); n != 1 {
		t.Fatalf("строк чек-точки %d, ожидалась ровно 1", n)
	}

	// Повторный прогон (сброс чек-точки — безусловный) не множит ни строку, ни доли.
	if _, err := s.db.Exec(`DELETE FROM contract_board_state WHERE planet_id = $1`, planetID); err != nil {
		t.Fatalf("reset board_state: %v", err)
	}
	if ok, err := repo.MaterializeBoard(planetID, time.Now()); err != nil || !ok {
		t.Fatalf("повтор с нуждами: ok=%v err=%v", ok, err)
	}
	if n := boardITCount(t, s.db,
		`SELECT COUNT(*) FROM contract_board_state WHERE planet_id = $1`, planetID); n != 1 {
		t.Fatalf("после повтора строк чек-точки %d, ожидалась ровно 1", n)
	}
	pkg := supplyPackageKey(planetID, buildingID, "good-1")
	if n := boardITCount(t, s.db,
		`SELECT COUNT(*) FROM contracts WHERE package_key = $1 AND status = 'open'`, pkg); n != 1 {
		t.Fatalf("открытых долей пакета %d, ожидалась ровно 1", n)
	}
}

// F1 (живая): пакет выпал из источника нужд → его открытые доли снимаются
// (залог возвращён автору, лог superseded), а «ручной» открытый контракт без
// package_key на той же планете остаётся open (в сверку не входит).
func TestMaterializeBoardCancelsDroppedPackageIT(t *testing.T) {
	s := boardITOpenMigrated(t)
	planetID := boardITSeedPlanet(t, s.db)
	playerID := uuid.New().String()
	buildingID := boardITSeedBuilding(t, s.db, planetID, playerID)

	repo := NewContractRepository(s.db)
	repo.boardNeeds = func(string) ([]BoardNeed, error) {
		return []BoardNeed{boardITNeed(buildingID, planetID, "good-1", 100, 0, []float64{100})}, nil
	}
	if ok, err := repo.MaterializeBoard(planetID, time.Now()); err != nil || !ok {
		t.Fatalf("первая материализация: ok=%v err=%v", ok, err)
	}

	pkg := supplyPackageKey(planetID, buildingID, "good-1")
	shareID := boardITStr(t, s.db,
		`SELECT id FROM contracts WHERE package_key = $1 AND status = 'open'`, pkg)

	// «Ручной» открытый контракт без package_key на той же планете.
	manual, err := repo.Publish(PublishContractParams{
		Type:                "delivery",
		AuthorType:          models.ContractActorPlayer,
		AuthorID:            playerID,
		PublicationPlanetID: planetID,
		Title:               "IT manual",
		Reward:              itBoardReward,
		ExpiresAt:           time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("Publish ручного контракта: %v", err)
	}

	// Пакет выпал из нужд: источник отдаёт нужду ДРУГОЙ позиции.
	repo.boardNeeds = func(string) ([]BoardNeed, error) {
		return []BoardNeed{boardITNeed(buildingID, planetID, "good-2", 100, 0, []float64{100})}, nil
	}
	if _, err := s.db.Exec(`DELETE FROM contract_board_state WHERE planet_id = $1`, planetID); err != nil {
		t.Fatalf("reset board_state: %v", err)
	}
	if ok, err := repo.MaterializeBoard(planetID, time.Now()); err != nil || !ok {
		t.Fatalf("материализация после выпадения пакета: ok=%v err=%v", ok, err)
	}

	// Доля выпавшего пакета снята, залог возвращён автору.
	if got := boardITStr(t, s.db, `SELECT status FROM contracts WHERE id = $1`, shareID); got != models.ContractStatusCancelled {
		t.Fatalf("статус доли выпавшего пакета %q, ожидался %q", got, models.ContractStatusCancelled)
	}
	// Возврат залога — по журналу движения счёта (delta), а не по текущему
	// балансу: параллельно публикуется доля нового пакета (good-2), её залог
	// списывается в той же транзакции.
	var returnDelta int64
	if err := s.db.QueryRow(`SELECT delta FROM money_operations
		WHERE contract_id = $1 AND kind = 'escrow_return'`, shareID).
		Scan(&returnDelta); err != nil {
		t.Fatalf("money_operations escrow_return: %v", err)
	}
	if returnDelta <= 0 {
		t.Fatalf("возврат залога: delta=%d, ожидалось положительное", returnDelta)
	}
	if n := boardITCount(t, s.db,
		`SELECT COUNT(*) FROM contract_log WHERE contract_id = $1 AND type = 'cancelled'`, shareID); n != 1 {
		t.Fatalf("лог cancelled у снятой доли %d, ожидался 1", n)
	}
	if n := boardITCount(t, s.db,
		`SELECT COUNT(*) FROM contract_log WHERE contract_id = $1 AND type = 'escrow_returned'
		   AND data->>'reason' = 'superseded'`, shareID); n != 1 {
		t.Fatalf("лог escrow_returned(superseded) у снятой доли %d, ожидался 1", n)
	}
	if n := boardITCount(t, s.db,
		`SELECT COUNT(*) FROM money_operations
		 WHERE contract_id = $1 AND kind = 'escrow_return'`, shareID); n != 1 {
		t.Fatalf("money_operations escrow_return у снятой доли %d, ожидался 1", n)
	}

	// «Ручной» контракт без package_key не тронут.
	if got := boardITStr(t, s.db, `SELECT status FROM contracts WHERE id = $1`, manual.ID); got != models.ContractStatusOpen {
		t.Fatalf("статус ручного контракта %q, ожидался %q (вне пакета — сверка не трогает)",
			got, models.ContractStatusOpen)
	}
}
