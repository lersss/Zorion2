// internal/repository/settlement_stage_integration_test.go
//
// Интеграционный тест конкурентности стадий (спека 2026-09-23-стадии-поселения-
// и-скорость-производства §6.3, §15.2 T-С8): админ-создание ветки (advisory-лок
// поселения + чтение FOR UPDATE, как в AddBranch) не гоняется с переходом
// стадии и добором веток owner-проходом — дубля по UNIQUE (settlement_id,
// recipe_id) нет. Без TEST_DATABASE_URL/DATABASE_URL тест скипается (обычный
// DoD-прогон зелёный).
package repository

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// supplyITSeedStageType — подтип класса «Поселение» (parent_id = тип-родитель)
// с порогами и привязками в params (§4.3).
func supplyITSeedStageType(t *testing.T, db *sql.DB, nameNorm string, parentID int64, params string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(
		`INSERT INTO producer_types (name, name_norm, kind, parent_id, params)
		 VALUES ($1, $2, 'goods', $3, $4::jsonb) RETURNING id`,
		nameNorm, nameNorm, parentID, params,
	).Scan(&id); err != nil {
		t.Fatalf("insert stage type %q: %v", nameNorm, err)
	}
	return id
}

// supplyITSetDefaultSettlementType — базовый тип поселения в generation_config
// (rename-safe резолв ладдеры через parent_id, §4.3).
func supplyITSetDefaultSettlementType(t *testing.T, db *sql.DB, typeID int64) {
	t.Helper()
	supplyITExec(t, db,
		`INSERT INTO generation_config (key, payload) VALUES ('default_settlement_type_id', to_jsonb($1::bigint))
		 ON CONFLICT (key) DO UPDATE SET payload = EXCLUDED.payload`, typeID)
}

// T-С8: параллельно owner-проход (переход стадии + добор веток) и админ-INSERT
// той же ветки. Advisory-лок поселения сериализует их → ровно одна ветка.
func TestSettlementStageIntegrationAddBranchRace(t *testing.T) {
	db := supplyITOpenMigrated(t)
	planetID := supplyITSeedPlanet(t, db)
	catID := supplyITSeedCategory(t, db, "Продовольствие", supplyITPosition)
	out := supplyITSeedGood(t, db, "Пища", "пища", catID)
	comp := supplyITSeedGood(t, db, "Мясо", "мясо", catID)
	recipeID := supplyITSeedRecipe(t, db, out, comp, 1)

	// Класс «Поселение»: корень + две ступени (148 — пол, 151 — порог 100/50).
	var rootID int64
	require.NoError(t, db.QueryRow(
		`INSERT INTO producer_types (name, name_norm, kind) VALUES ('Поселение', 'поселение', 'goods') RETURNING id`,
	).Scan(&rootID))
	base := supplyITSeedStageType(t, db, "ит-ступень-148", rootID,
		`{"effects":{"продовольствие":"голод"},"stage":{"enter":0,"exit":0}}`)
	top := supplyITSeedStageType(t, db, "ит-ступень-151", rootID,
		`{"effects":{"продовольствие":"голод"},"stage":{"enter":100,"exit":50}}`)
	supplyITBindRate(t, db, top, recipeID, 660)
	supplyITSetDefaultSettlementType(t, db, base)

	computedAt := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	s1 := supplyITSeedSettlement(t, db, planetID, 1_000_000_000, computedAt)
	supplyITSetSettlementType(t, db, s1, base)

	o := supplyITOwner(s1, planetID, computedAt, 1_000_000_000)
	o.SettlementTypeID = base
	now := time.Now()

	var wg sync.WaitGroup
	var syncErr, addErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, syncErr = NewBranchRepository(db).SyncSettlements(now, []OwnerSettlement{o})
	}()
	go func() {
		defer wg.Done()
		addErr = itAddBranchLocked(db, s1, recipeID)
	}()
	wg.Wait()

	require.NoError(t, syncErr, "owner-проход не должен падать")
	require.NoError(t, addErr, "админ-создание ветки не должно падать (дубль по UNIQUE)")
	require.Equal(t, int64(1), supplyITScalarInt(t, db,
		`SELECT COUNT(*) FROM settlement_branches WHERE settlement_id = $1 AND recipe_id = $2`, s1, recipeID),
		"ровно одна ветка на рецепт (UNIQUE)")
	require.Equal(t, top, supplyITScalarInt(t, db, `SELECT settlement_type_id FROM settlements WHERE id = $1`, s1),
		"стадия переключилась на верхнюю ступень")
}

// itAddBranchLocked — эмуляция AddBranch (§6.3): advisory-лок поселения первым
// действием транзакции, чтение поселения FOR UPDATE, EXISTS-проверка, создание
// ветки. Та же дисциплина локов, что у админ-ручки.
func itAddBranchLocked(db *sql.DB, settlementID string, recipeID int64) error {
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	repo := NewBranchRepository(db)
	if err := repo.LockOwnerTx(ctx, tx, settlementID); err != nil {
		return err
	}
	var planetID string
	if err := tx.QueryRowContext(ctx,
		`SELECT planet_id FROM settlements WHERE id = $1 FOR UPDATE`, settlementID).Scan(&planetID); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM settlement_branches WHERE settlement_id = $1 AND recipe_id = $2)`,
		settlementID, recipeID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if err := repo.CreateBranchTx(ctx, tx, uuid.New().String(), settlementID, recipeID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// T-С1 (фильтр класса, находка M2): запись с заданным params.stage, но ВНЕ
// класса «Поселение» (parent_id ≠ корень «Поселение») в ладдеру не входит —
// и поселение такого типа не переключается. Фильтр класса живёт в SQL-предикате
// `parent_id = <корень>` (settlementStageLadderSQL), поэтому проверяется на
// живой БД; без TEST_DATABASE_URL тест скипается (как T-С8).
func TestSettlementStageIntegrationClassFilter(t *testing.T) {
	db := supplyITOpenMigrated(t)
	ctx := context.Background()
	planetID := supplyITSeedPlanet(t, db)

	var rootID int64
	require.NoError(t, db.QueryRow(
		`INSERT INTO producer_types (name, name_norm, kind) VALUES ('Поселение', 'поселение', 'goods') RETURNING id`,
	).Scan(&rootID))
	inClass := supplyITSeedStageType(t, db, "ит-класс-внутри", rootID, `{"stage":{"enter":0,"exit":0}}`)
	// Вне класса: params.stage задан, но parent_id NULL (не подтип «Поселения»).
	var outID int64
	require.NoError(t, db.QueryRow(
		`INSERT INTO producer_types (name, name_norm, kind, params)
		 VALUES ('ит-вне-класса', 'ит-вне-класса', 'goods', '{"stage":{"enter":0,"exit":0}}'::jsonb) RETURNING id`,
	).Scan(&outID))
	supplyITSetDefaultSettlementType(t, db, inClass)

	ladder, ids, err := NewBranchRepository(db).loadStageLadder(ctx)
	require.NoError(t, err)
	require.Equal(t, []int64{inClass}, ids, "в ладдеру входит только подтип класса «Поселение»")
	require.Equal(t, 1, ladder.Len())
	require.NotContains(t, ids, outID, "запись вне класса с params.stage в ладдеру не входит (M2)")

	// Поселение типа вне ладдеры при населении выше любого порога не
	// переключается: текущий тип не в ладдере → переключений нет (§4.3).
	computedAt := time.Now().Add(-2 * time.Hour).Truncate(time.Microsecond)
	s1 := supplyITSeedSettlement(t, db, planetID, 1_000_000_000, computedAt)
	supplyITSetSettlementType(t, db, s1, outID)
	o := supplyITOwner(s1, planetID, computedAt, 1_000_000_000)
	o.SettlementTypeID = outID
	_, err = NewBranchRepository(db).SyncSettlements(time.Now(), []OwnerSettlement{o})
	require.NoError(t, err)
	require.Equal(t, outID, supplyITScalarInt(t, db,
		`SELECT settlement_type_id FROM settlements WHERE id = $1`, s1),
		"тип вне ладдеры не переключается (M2)")
}
