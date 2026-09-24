// Интеграционные тесты покупки в магазине модулей на НАСТОЯЩЕЙ PostgreSQL
// (спека 2026-09-24-магазин-модулей-локальный-рынок §7.2/§13): мок не знает
// про блокировки строк и арифметику UPDATE — урезание withdrawable и
// сериализацию двух параллельных покупок в один слот проверяем на живой БД.
package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"zorion/internal/auth"
	"zorion/internal/handlers"
	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

// marketOfferPrice — цена любого модуля витрины (спека §11: 4 базовых по 3000).
const marketOfferPrice = int64(3000)

// marketSeed — сценарий покупки: игрок на орбите планеты с живым поселением.
type marketSeed struct {
	userID   string
	planetID string
	offerID  int64
}

// seedMarketBuy — мир+планета, живое поселение, игрок на орбите, счёт, каталоги.
func seedMarketBuy(t *testing.T, s *scratchDB) (marketSeed, *handlers.MarketHandlers) {
	t.Helper()
	planetID := seedWorldPlanet(t, s.db)
	worldID := mustStr(t, s.db, `SELECT world_id FROM planets WHERE id = $1`, planetID)
	userID := uuid.New().String()

	if _, err := s.db.Exec(
		`INSERT INTO settlements (planet_id, population_exact) VALUES ($1, 100)`, planetID,
	); err != nil {
		t.Fatalf("insert settlement: %v", err)
	}
	pos := fmt.Sprintf(`{"status":"orbit","object_type":"planet","object_id":"%s"}`, planetID)
	if _, err := s.db.Exec(
		`INSERT INTO users (id, username, password_hash, role, current_world_id, ship_model_id, equipment, current_position)
		 VALUES ($1, $2, 'x', 'player', $3, 'starter', '{}', $4)`,
		userID, "it_market_"+userID, worldID, pos,
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if err := repository.NewAccountRepository(s.db).EnsureAccount(
		models.AccountOwnerPlayer, userID, models.PlayerBalanceSeed,
	); err != nil {
		t.Fatalf("ensure account: %v", err)
	}
	if err := ship.LoadCatalog(s.db); err != nil {
		t.Fatalf("load equipment catalog: %v", err)
	}
	if err := ship.LoadModels(s.db); err != nil {
		t.Fatalf("load ship models: %v", err)
	}

	h := handlers.NewMarketHandlers(
		s.db, repository.NewMarketRepository(s.db), repository.NewPlanetRepository(s.db),
		repository.NewUserRepository(s.db), repository.NewKnowledgeRepository(s.db),
		travel.NewManager(nil),
	)
	offerID := mustInt(t, s.db, `SELECT id FROM market_offers WHERE item_id = 'radar_1'`)
	return marketSeed{userID: userID, planetID: planetID, offerID: offerID}, h
}

// buyOnce — один вызов POST /market/buy; возвращает HTTP-код.
func buyOnce(h *handlers.MarketHandlers, seed marketSeed, slot string) int {
	body := fmt.Sprintf(`{"offer_id":%d,"slot":%q}`, seed.offerID, slot)
	req := httptest.NewRequest(http.MethodPost,
		"/api/planets/"+seed.planetID+"/market/buy", strings.NewReader(body))
	ctx := context.WithValue(req.Context(), auth.UserIDKey, seed.userID)
	ctx = context.WithValue(ctx, auth.RoleKey, string(models.RolePlayer))
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	h.BuyMarket(rec, req)
	return rec.Code
}

// «Прочая трата» (§8): withdrawable урезается до нового баланса — метка
// «заработанное» сгорает при трате; журнал kind='purchase'; модуль в слоте.
func TestMarketBuyWithdrawableTrimmed(t *testing.T) {
	s := openMigrated(t)
	seed, h := seedMarketBuy(t, s)

	// Всё «заработано»: withdrawable = balance.
	if _, err := s.db.Exec(
		`UPDATE accounts SET withdrawable = balance WHERE owner_type = 'player' AND owner_id = $1`,
		seed.userID,
	); err != nil {
		t.Fatalf("set withdrawable: %v", err)
	}

	if code := buyOnce(h, seed, "universal"); code != http.StatusOK {
		t.Fatalf("покупка: код %d", code)
	}

	balance, withdrawable := accountBalance(t, s.db, models.AccountOwnerPlayer, seed.userID)
	if balance != models.PlayerBalanceSeed-marketOfferPrice {
		t.Fatalf("balance после покупки: %d (ожидалось %d)",
			balance, models.PlayerBalanceSeed-marketOfferPrice)
	}
	if withdrawable != balance {
		t.Fatalf("withdrawable не урезан: withdrawable=%d balance=%d", withdrawable, balance)
	}

	n, delta, after := moneyOp(t, s.db, models.AccountOwnerPlayer, seed.userID, "purchase")
	if n != 1 || delta != -marketOfferPrice || after != balance {
		t.Fatalf("money_operations purchase: n=%d delta=%d balance_after=%d (balance=%d)",
			n, delta, after, balance)
	}

	if got := mustStr(t, s.db, `SELECT equipment->>'universal' FROM users WHERE id = $1`, seed.userID); got != "radar_1" {
		t.Fatalf("модуль в слоте: %q (ожидался radar_1)", got)
	}
}

// Гонка (§7.2 п.0): две ПАРАЛЛЕЛЬНЫЕ покупки в один слот сериализуются локом
// строки игрока — вторая видит установленный модуль и платит с трейд-ином
// (выкуп не теряется). Итог детерминирован: 3000 + (3000−1500) = 4500.
func TestMarketBuyRaceOneSlot(t *testing.T) {
	s := openMigrated(t)
	seed, h := seedMarketBuy(t, s)

	codes := make([]int, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = buyOnce(h, seed, "universal")
		}(i)
	}
	wg.Wait()

	for i, code := range codes {
		if code != http.StatusOK {
			t.Fatalf("покупка %d: код %d", i, code)
		}
	}

	balance, _ := accountBalance(t, s.db, models.AccountOwnerPlayer, seed.userID)
	if want := models.PlayerBalanceSeed - (marketOfferPrice + marketOfferPrice/2); balance != want {
		t.Fatalf("итог двух покупок: balance=%d (ожидалось %d: 3000 + трейд-ин 1500)",
			balance, want)
	}
	n, _, _ := moneyOp(t, s.db, models.AccountOwnerPlayer, seed.userID, "purchase")
	if n != 2 {
		t.Fatalf("записей purchase: %d (ожидалось 2)", n)
	}
}
