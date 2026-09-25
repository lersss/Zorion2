// internal/integration/property_repository_test.go
// Собственность игрока на НАСТОЯЩЕЙ PostgreSQL (спека
// 2026-09-26-собственность-игрока-в-дашборде §5/§13): sqlmock не воспроизводит
// кодирование параметров драйвером (lib/pq) и работу SQL-джойнов — здесь
// проверяются живой выборкой: только своё (И-1), сироты чужого владельца (И-5),
// адрес и режимы знания (И-3), порядок (§5.3). Без TEST_DATABASE_URL/
// DATABASE_URL тест скипается.
package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"zorion/internal/repository"
)

// TestPropertyRepositoryAgainstDB — пакетное чтение собственности на живой БД.
func TestPropertyRepositoryAgainstDB(t *testing.T) {
	s := openMigrated(t)

	worldID := uuid.New().String()
	if _, err := s.db.Exec(
		`INSERT INTO worlds (id, name, coord_x, coord_y) VALUES ($1, 'Кеплер-3', 0, 0)`, worldID); err != nil {
		t.Fatalf("world: %v", err)
	}

	insertPlanet := func(name string, orbit int) string {
		id := uuid.New().String()
		if _, err := s.db.Exec(
			`INSERT INTO planets (id, world_id, name, orbit_index, data) VALUES ($1, $2, $3, $4, '{}')`,
			id, worldID, name, orbit); err != nil {
			t.Fatalf("planet %s: %v", name, err)
		}
		return id
	}
	pA := insertPlanet("Кеплер-3 b", 1)
	pB := insertPlanet("Кеплер-3 c", 2)
	pC := insertPlanet("Кеплер-3 d", 3)
	pD := insertPlanet("Кеплер-3 e", 4)

	typeSettle := mustInt(t, s.db,
		`INSERT INTO producer_types (name, name_norm, kind) VALUES ('Поселение', 'it-property-поселение', 'goods') RETURNING id`)
	typeBuilding := mustInt(t, s.db,
		`INSERT INTO producer_types (name, name_norm, kind) VALUES ('Аутпост', 'it-property-аутпост', 'goods') RETURNING id`)

	playerID := uuid.New().String()
	otherID := uuid.New().String()
	if _, err := s.db.Exec(
		`INSERT INTO users (id, username, password_hash, role) VALUES
		 ($1, 'it_prop_player', 'x', 'player'), ($2, 'it_prop_other', 'x', 'player')`,
		playerID, otherID); err != nil {
		t.Fatalf("users: %v", err)
	}

	// Своё: поселение на A, поселение на C, строение на B.
	settleA := uuid.New().String()
	settleC := uuid.New().String()
	buildB := uuid.New().String()
	execSeed(t, s, `INSERT INTO settlements
		(id, planet_id, population, population_exact, stability, computed_at, race_id, settlement_type_id, owner_type, owner_id)
		VALUES ($1, $2, 10, 10, 50, NOW(), NULL, $3, 'player', $4)`, settleA, pA, typeSettle, playerID)
	execSeed(t, s, `INSERT INTO settlements
		(id, planet_id, population, population_exact, stability, computed_at, race_id, settlement_type_id, owner_type, owner_id)
		VALUES ($1, $2, 10, 10, 50, NOW(), NULL, $3, 'player', $4)`, settleC, pC, typeSettle, playerID)
	execSeed(t, s, `INSERT INTO buildings (id, planet_id, building_type, owner_type, owner_id, producer_type_id)
		VALUES ($1, $2, 'producer', 'player', $3, $4)`, buildB, pB, playerID, typeBuilding)

	// Сироты чужого владельца (И-5): не должны попасть в выдачу.
	foreignBuilding := uuid.New().String()
	execSeed(t, s, `INSERT INTO buildings (id, planet_id, building_type, owner_type, owner_id, producer_type_id)
		VALUES ($1, $2, 'producer', 'player', $3, $4)`, foreignBuilding, pD, otherID, typeBuilding)
	execSeed(t, s, `INSERT INTO buildings (id, planet_id, building_type, owner_type, owner_id)
		VALUES ($1, $2, 'producer', 'faction', $3)`, uuid.New().String(), pD, otherID)

	// Знание: снимок на B (свежий), скан на C (старый), на A/D — нет.
	snapAt := time.Now().Add(-time.Hour).UTC()
	execSeed(t, s, `INSERT INTO player_planet_knowledge (user_id, planet_id, data, scanned_at, source)
		VALUES ($1, $2, $3::jsonb, $4, 'presence')`,
		playerID, pB, fmt.Sprintf(`{"snapshot":{"at":%q}}`, snapAt.Format(time.RFC3339)), snapAt)
	execSeed(t, s, `INSERT INTO player_planet_knowledge (user_id, planet_id, data, scanned_at, source)
		VALUES ($1, $2, '{"settlements_count":1}'::jsonb, $3, 'scanner')`,
		playerID, pC, time.Now().Add(-30*24*time.Hour))

	repo := repository.NewPropertyRepository(s.db)

	// presence — игрок физически на планете A.
	items, err := repo.GetPlayerProperty(playerID, pA)
	require.NoError(t, err)
	require.Len(t, items, 3, "только своё: 2 поселения + 1 строение (И-1/И-5)")

	// Порядок (§5.3): поселения (A, C по имени планеты), затем строение (B).
	require.Equal(t, []string{settleA, settleC, buildB},
		[]string{items[0].ID, items[1].ID, items[2].ID})

	assertItem := func(it repository.PropertyItem, kind, name, subtitle, planetName string) {
		require.Equal(t, kind, it.Kind)
		require.Equal(t, name, it.Name)
		require.Equal(t, subtitle, it.Subtitle)
		require.Equal(t, worldID, it.WorldID)
		require.Equal(t, "Кеплер-3", it.WorldName)
		require.Equal(t, planetName, it.PlanetName)
	}
	assertItem(items[0], "settlement", "Люди", "Поселение", "Кеплер-3 b")
	assertItem(items[1], "settlement", "Люди", "Поселение", "Кеплер-3 d")
	assertItem(items[2], "building", "Аутпост", "Строение", "Кеплер-3 c")

	// Режимы знания (§4.3): presence без даты; scan протух; snapshot свеж.
	require.Equal(t, "presence", items[0].Knowledge.Mode)
	require.Nil(t, items[0].Knowledge.At)
	require.False(t, items[0].Knowledge.Fresh)

	require.Equal(t, "scan", items[1].Knowledge.Mode)
	require.NotNil(t, items[1].Knowledge.At)
	require.False(t, items[1].Knowledge.Fresh, "скан старше KnowledgeTTL")

	require.Equal(t, "snapshot", items[2].Knowledge.Mode)
	require.NotNil(t, items[2].Knowledge.At)
	require.True(t, items[2].Knowledge.Fresh)

	for _, it := range items {
		require.NotEqual(t, foreignBuilding, it.ID, "сирота чужого владельца не попадает")
	}

	// Пустой список: игрок без собственности → пустой срез (И-4).
	emptyUser := uuid.New().String()
	if _, err := s.db.Exec(
		`INSERT INTO users (id, username, password_hash, role) VALUES ($1, 'it_prop_empty', 'x', 'player')`,
		emptyUser); err != nil {
		t.Fatalf("empty user: %v", err)
	}
	empty, err := repo.GetPlayerProperty(emptyUser, "")
	require.NoError(t, err)
	require.Empty(t, empty)
}

// execSeed — INSERT сценарной фикстуры (ошибка фатальна).
func execSeed(t *testing.T, s *scratchDB, query string, args ...interface{}) {
	t.Helper()
	if _, err := s.db.Exec(query, args...); err != nil {
		t.Fatalf("seed %q: %v", query, err)
	}
}
