// Автор-поселение контракта (спека
// 2026-09-25-внутреннее-хранилище-и-рождение-заказов §1.5/§5.2/§6, подэтап 3а):
// миграция 000088 идемпотентна, CHECK author_type расширен только для автора,
// ослабленный contracts_live_has_escrow пропускает бесплатный supply; резолв
// плательщика (владелец/агент→фракция, D4) и планеты; бесплатная публикация.
// Без TEST_DATABASE_URL/DATABASE_URL тесты скипаются.
package integration

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"zorion/internal/models"
	"zorion/internal/repository"
)

// migration088SQL — содержимое миграции (для повторного прогона в тесте).
func migration088SQL(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000088_contract_author_settlement.sql"))
	if err != nil {
		t.Fatalf("миграция 000088_contract_author_settlement.sql: %v", err)
	}
	return string(src)
}

// seedSettlement — поселение с владельцем (ownerType пуст — легаси без владельца).
func seedSettlement(t *testing.T, db *sql.DB, planetID, ownerType, ownerID string) string {
	t.Helper()
	id := uuid.New().String()
	var ot, oid interface{}
	if ownerType != "" {
		ot, oid = ownerType, ownerID
	}
	if _, err := db.Exec(`INSERT INTO settlements
		(id, planet_id, population, population_exact, stability, computed_at, owner_type, owner_id)
		VALUES ($1,$2,10,10,50,NOW(),$3,$4)`, id, planetID, ot, oid); err != nil {
		t.Fatalf("insert settlement: %v", err)
	}
	return id
}

// insertContract — сырой INSERT контракта для проверки CHECK-констрейнтов.
func insertContract(t *testing.T, db *sql.DB, planetID, typ, authorType, authorID string,
	reward, escrow int64, status string, directTargetType, directTargetID interface{}) error {
	t.Helper()
	_, err := db.Exec(`INSERT INTO contracts
		(id, type, author_type, author_id, publication_planet_id, title, reward, escrow_amount,
		 status, visibility, direct_target_type, direct_target_id, expires_at)
		VALUES ($1,$2,$3,$4,$5,'T',$6,$7,$8,'public',$9,$10,NOW() + interval '1 hour')`,
		uuid.New().String(), typ, authorType, authorID, planetID, reward, escrow, status,
		directTargetType, directTargetID)
	return err
}

// TestMigration088ContractAuthorSettlement — CHECK author_type допускает
// 'settlement' только как автора; ослабленный contracts_live_has_escrow
// пропускает supply reward=0 без залога и не пропускает остальные; повторный
// прогон файла идемпотентен.
func TestMigration088ContractAuthorSettlement(t *testing.T) {
	s := openMigrated(t)
	sql088 := migration088SQL(t)

	// Идемпотентность: повторное выполнение файла не падает.
	if _, err := s.db.Exec(sql088); err != nil {
		t.Fatalf("повторный прогон 000088: %v", err)
	}

	planetID := seedWorldPlanet(t, s.db)
	settlementID := uuid.New().String()

	// author_type='settlement' разрешён (бесплатный supply — живой контракт).
	if err := insertContract(t, s.db, planetID, models.ContractTypeSupply,
		models.ContractActorSettlement, settlementID, 0, 0, models.ContractStatusOpen, nil, nil); err != nil {
		t.Fatalf("author_type='settlement' отклонён: %v", err)
	}

	// Только supply с reward=0 освобождён от залога: не-supply и supply с наградой
	// без залога — CHECK обязан упасть.
	if err := insertContract(t, s.db, planetID, models.ContractTypeSupply,
		models.ContractActorSettlement, settlementID, 5, 0, models.ContractStatusOpen, nil, nil); err == nil {
		t.Fatal("contracts_live_has_escrow не сработал: supply с наградой без залога")
	}
	if err := insertContract(t, s.db, planetID, "travel",
		models.ContractActorPlayer, uuid.New().String(), 0, 0, models.ContractStatusOpen, nil, nil); err == nil {
		t.Fatal("contracts_live_has_escrow не сработал: не-supply без залога")
	}

	// direct_target_type НЕ расширен: 'settlement' отклоняется (M6).
	if err := insertContract(t, s.db, planetID, models.ContractTypeSupply,
		models.ContractActorSettlement, settlementID, 0, 0, models.ContractStatusOpen,
		"settlement", uuid.New().String()); err == nil {
		t.Fatal("direct_target_type='settlement' не должен допускаться (M6)")
	}

	// Идемпотентность после вставок.
	if _, err := s.db.Exec(sql088); err != nil {
		t.Fatalf("повторный прогон 000088 после вставок: %v", err)
	}
}

// TestContractPublishAuthorSettlement — T3/T7/T8: планета публикации из
// поселения; платная публикация снабжения запирает залог со счёта владельца;
// бесплатная (reward=0) — без залога, money_op и лога escrow_locked.
func TestContractPublishAuthorSettlement(t *testing.T) {
	s := openMigrated(t)
	planetID := seedWorldPlanet(t, s.db)
	repo := repository.NewContractRepository(s.db)

	ownerID := uuid.New().String()
	settlementID := seedSettlement(t, s.db, planetID, models.AccountOwnerPlayer, ownerID)

	// T3: планета публикации — settlements.planet_id.
	gotPlanet, err := repo.ResolvePublicationPlanet(models.ContractActorSettlement, settlementID)
	if err != nil {
		t.Fatalf("ResolvePublicationPlanet: %v", err)
	}
	if gotPlanet != planetID {
		t.Fatalf("планета публикации %q, ожидалась %q", gotPlanet, planetID)
	}

	// T7: платная публикация — автор-поселение, залог со счёта владельца.
	paid, err := repo.Publish(repository.PublishContractParams{
		Type: models.ContractTypeSupply, AuthorType: models.ContractActorSettlement,
		AuthorID: settlementID, PublicationPlanetID: gotPlanet, Title: "T", Reward: itReward,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("Publish (платная): %v", err)
	}
	if paid.AuthorType != models.ContractActorSettlement {
		t.Fatalf("author_type=%q, ожидался %q", paid.AuthorType, models.ContractActorSettlement)
	}
	if paid.PublicationPlanetID != planetID || paid.EscrowAmount != itReward {
		t.Fatalf("платная публикация: planet=%q escrow=%d (ожидалось %q/%d)",
			paid.PublicationPlanetID, paid.EscrowAmount, planetID, itReward)
	}
	if bal, _ := accountBalance(t, s.db, models.AccountOwnerPlayer, ownerID); bal != models.PlayerBalanceSeed-itReward {
		t.Fatalf("счёт владельца после платной публикации: %d, ожидалось %d",
			bal, models.PlayerBalanceSeed-itReward)
	}

	// T8: бесплатная публикация supply (reward=0) — живёт, эскроу 0, без движения.
	free, err := repo.Publish(repository.PublishContractParams{
		Type: models.ContractTypeSupply, AuthorType: models.ContractActorSettlement,
		AuthorID: settlementID, PublicationPlanetID: gotPlanet, Title: "T", Reward: 0,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("Publish (бесплатная supply): %v", err)
	}
	if free.EscrowAmount != 0 || contractStatus(t, s.db, free.ID) != models.ContractStatusOpen {
		t.Fatalf("бесплатная публикация: escrow=%d status=%q", free.EscrowAmount, contractStatus(t, s.db, free.ID))
	}
	logs := logTypes(t, s.db, free.ID)
	if logs[models.ContractLogPublished] != 1 || logs[models.ContractLogEscrowLocked] != 0 {
		t.Fatalf("журнал бесплатной публикации: %v", logs)
	}
	// money_operations — только от платной публикации (1), бесплатная не пишет.
	if n, _, _ := moneyOp(t, s.db, models.AccountOwnerPlayer, ownerID, models.MoneyOpEscrowLock); n != 1 {
		t.Fatalf("escrow_lock записей %d, ожидалась 1 (только платная)", n)
	}
}

// TestContractPublishAuthorSettlementAgentPaysFaction — D4: владелец-агент
// платит со счёта своей фракции, а не агента.
func TestContractPublishAuthorSettlementAgentPaysFaction(t *testing.T) {
	s := openMigrated(t)
	planetID := seedWorldPlanet(t, s.db)
	worldID := mustStr(t, s.db, `SELECT world_id::text FROM planets WHERE id = $1`, planetID)
	repo := repository.NewContractRepository(s.db)

	factionID := uuid.New().String()
	if _, err := s.db.Exec(`INSERT INTO factions (id, name, type, homeworld_id)
		VALUES ($1, 'F', 'tribe', $2)`, factionID, planetID); err != nil {
		t.Fatalf("insert faction: %v", err)
	}
	agentID := uuid.New().String()
	if _, err := s.db.Exec(`INSERT INTO npc_agents (id, name, current_world_id, owner_faction_id)
		VALUES ($1, 'A', $2, $3)`, agentID, worldID, factionID); err != nil {
		t.Fatalf("insert npc_agent: %v", err)
	}
	settlementID := seedSettlement(t, s.db, planetID, models.AccountOwnerAgent, agentID)

	if _, err := repo.Publish(repository.PublishContractParams{
		Type: models.ContractTypeSupply, AuthorType: models.ContractActorSettlement,
		AuthorID: settlementID, PublicationPlanetID: planetID, Title: "T", Reward: itReward,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("Publish (владелец-агент): %v", err)
	}
	if bal, _ := accountBalance(t, s.db, models.AccountOwnerFaction, factionID); bal != models.FactionBalanceSeed-itReward {
		t.Fatalf("счёт фракции после публикации: %d, ожидалось %d",
			bal, models.FactionBalanceSeed-itReward)
	}
}

// TestContractPublishAuthorSettlementOwnerless — легаси-поселение без владельца
// (Г1) заказчиком быть не может: резолв плательщика отвергнут даже для
// бесплатного supply.
func TestContractPublishAuthorSettlementOwnerless(t *testing.T) {
	s := openMigrated(t)
	planetID := seedWorldPlanet(t, s.db)
	repo := repository.NewContractRepository(s.db)
	settlementID := seedSettlement(t, s.db, planetID, "", "")

	_, err := repo.Publish(repository.PublishContractParams{
		Type: models.ContractTypeSupply, AuthorType: models.ContractActorSettlement,
		AuthorID: settlementID, PublicationPlanetID: planetID, Title: "T", Reward: 0,
		ExpiresAt: time.Now().Add(time.Hour),
	})
	if !errors.Is(err, repository.ErrPayerUnresolved) {
		t.Fatalf("ownerless-поселение: err=%v, ожидался ErrPayerUnresolved", err)
	}
}
