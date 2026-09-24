// internal/handlers/market_handlers.go
// Витрина локального рынка планеты (спека
// 2026-09-24-магазин-модулей-локальный-рынок): GET /api/planets/{id}/market
// (§7.1) и POST /api/planets/{id}/market/buy (§7.2). Гейт витрины знанием —
// тем же режимом, что вкладка «Магазин» (§10): none → 404 «Нет данных»;
// settlements_count == 0 → 200 offers:[]; > 0 → каталог. Живость поселения
// на чтении не проверяется и не раскрывается (§7.2 п.2 — гейт покупки).
// can_trade — присутствие игрока (роль player) на орбите ЭТОЙ планеты
// (прецедент can_buy_report, орбитная спека §5.2/§5.5: спутник — по родителю;
// surface канал покупки не даёт — решение создателя).
// Покупка (§7.2) — ОДНА транзакция с локом строки игрока (п.0): сериализация
// покупок одного игрока, вторая покупка в тот же слот считает трейд-ин по уже
// обновлённому слоту. Деньги сгорают (печь, §8), выкуп — зачёт в оплату.
package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"zorion/internal/auth"
	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

// marketMoneyOpPurchase — вид движения журнала money_operations для покупки в
// магазине модулей (спека §8/§7.2 п.8; kind — открытый список, миграция 000061
// CHECK нет). Денежная спека §3.2 — расширение списка.
const marketMoneyOpPurchase = "purchase"

// MarketHandlers — витрина и покупка на локальном рынке планеты.
type MarketHandlers struct {
	db            *sql.DB
	marketRepo    *repository.MarketRepository
	planetRepo    *repository.PlanetRepository
	userRepo      *repository.UserRepository
	knowledgeRepo *repository.KnowledgeRepository
	travelMgr     *travel.Manager
}

func NewMarketHandlers(
	db *sql.DB,
	marketRepo *repository.MarketRepository,
	planetRepo *repository.PlanetRepository,
	userRepo *repository.UserRepository,
	knowledgeRepo *repository.KnowledgeRepository,
	travelMgr *travel.Manager,
) *MarketHandlers {
	return &MarketHandlers{
		db:            db,
		marketRepo:    marketRepo,
		planetRepo:    planetRepo,
		userRepo:      userRepo,
		knowledgeRepo: knowledgeRepo,
		travelMgr:     travelMgr,
	}
}

// GetMarket — GET /api/planets/{planetID}/market: витрина планеты (§7.1).
func (h *MarketHandlers) GetMarket(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/api/planets/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 2 || parts[1] != "market" || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	planetID := parts[0]

	planet, err := h.planetRepo.GetPlanetByID(planetID)
	if err != nil {
		writeJSONError(w, "Не удалось получить планету", http.StatusInternalServerError)
		return
	}
	if planet == nil {
		writeJSONError(w, "Планета не найдена", http.StatusNotFound)
		return
	}

	// Гейт знанием (§7.1): settlements_count — из записи знания (не живой
	// подсчёт — витрина не раскрывает бит «живое поселение сейчас»).
	k, err := h.knowledgeRepo.GetKnowledge(userID, planetID)
	if err != nil {
		writeJSONError(w, "Не удалось проверить знание планеты", http.StatusInternalServerError)
		return
	}
	if k == nil {
		writeJSONError(w, "Нет данных", http.StatusNotFound)
		return
	}
	view := buildKnowledgeView(k, time.Now())
	if view.SettlementsCount == 0 {
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"offers":    []models.MarketOffer{},
			"can_trade": false,
		})
		return
	}

	offers, err := h.marketRepo.ListOffers()
	if err != nil {
		writeJSONError(w, "Не удалось получить предложения", http.StatusInternalServerError)
		return
	}
	// Инвариант каталога (§5): item_id обязан существовать в equipment;
	// отсутствие → строка пропускается с логом, не 500.
	out := make([]models.MarketOffer, 0, len(offers))
	for _, o := range offers {
		if o.Kind == "module" {
			it := ship.EquipmentByID(o.ItemID)
			if it == nil {
				log.Printf("⚠️ market: предложение %d: модуль %s не найден в каталоге — пропуск", o.ID, o.ItemID)
				continue
			}
			o.Item = it
		}
		out = append(out, o)
	}

	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"offers":    out,
		"can_trade": h.canTrade(userID, planetID),
	})
}

// canTrade — гейт покупки (§7.1): true ⟺ ИГРОК (роль player) присутствует на
// орбите ЭТОЙ планеты. Роль не player (admin/skycomposer) → false (§7.1:113 —
// видят всё и так, флаг им не нужен). Присутствие — по прецеденту can_buy_report
// (орбитная спека §2.1/§5.5: спутник — по родителю), но ТОЛЬКО статус orbit:
// surface канал покупки не даёт (решение создателя). Вычисляется на чтении,
// ничего не пишется (инвариант 8 — позиция серверно-авторитетная).
func (h *MarketHandlers) canTrade(userID, planetID string) bool {
	user, pos, _, err := h.userRepo.GetByIDWithPosition(userID)
	if err != nil || user == nil || user.CurrentWorldID == nil {
		return false
	}
	if user.Role != models.RolePlayer {
		return false
	}
	if h.travelMgr != nil && h.travelMgr.GetFlight(userID) != nil {
		return false
	}
	if pos == nil || pos.Status != "orbit" {
		return false
	}
	return presencePlanetID(pos, *user.CurrentWorldID, h.planetRepo) == planetID
}

// buyRequest — тело POST /api/planets/{id}/market/buy (§7.2).
type buyRequest struct {
	OfferID int64  `json:"offer_id"`
	Slot    string `json:"slot"`
}

// marketSlotNumber — номер универсального слота по ключу (конвенция трюма §20.2:
// `universal` = 1, `universal2` = 2, `universal3` = 3). ok=false — ключ не
// является универсальным слотом.
func marketSlotNumber(slot string) (int, bool) {
	if slot == "universal" {
		return 1, true
	}
	if !strings.HasPrefix(slot, "universal") {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(slot, "universal"))
	if err != nil || n < 2 {
		return 0, false
	}
	return n, true
}

// BuyMarket — POST /api/planets/{planetID}/market/buy: покупка модуля (§7.2).
// Алгоритм §7.2 в одной транзакции: п.0 лок строки игрока (сериализация
// покупок), п.4 слот, п.5 сумма (трейд-ин 50 % от цены старого модуля из
// market_offers), п.6 списание одним оператором, п.7 установка (jsonb_set),
// п.8 журнал (kind='purchase'). Присутствие (п.1), живое поселение (п.2) и
// предложение (п.3) проверяются до лока (гейты; race-критично только
// слот→трейд-ин→списание, §7.2 п.0). Деньги сгорают — печь (§8).
func (h *MarketHandlers) BuyMarket(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/api/planets/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) != 3 || parts[1] != "market" || parts[2] != "buy" || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	planetID := parts[0]

	var req buyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	slotNum, ok := marketSlotNumber(req.Slot)
	if !ok {
		writeJSONError(w, "Неверный слот", http.StatusBadRequest)
		return
	}

	// п.1 Присутствие: тот же гейт, что can_trade (§7.1) — роль player, орбита
	// этой планеты (спутник — по родителю), не в полёте.
	if !h.canTrade(userID, planetID) {
		writeJSONError(w, "Покупка доступна только с орбиты планеты", http.StatusForbidden)
		return
	}

	// п.2 Живое поселение: живость — ТОЛЬКО здесь, на покупке (на чтении не
	// раскрывается, §7.1). Вымерло между витриной и покупкой → 409.
	live, err := h.marketRepo.HasLiveSettlement(planetID)
	if err != nil {
		writeJSONError(w, "Не удалось проверить рынок", http.StatusInternalServerError)
		return
	}
	if !live {
		writeJSONError(w, "Рынок закрыт", http.StatusConflict)
		return
	}

	// п.3 Предложение: для kind='module' модуль обязан существовать в каталоге.
	offer, err := h.marketRepo.GetOfferByID(req.OfferID)
	if err != nil {
		writeJSONError(w, "Не удалось получить предложение", http.StatusInternalServerError)
		return
	}
	if offer == nil || (offer.Kind == "module" && ship.EquipmentByID(offer.ItemID) == nil) {
		writeJSONError(w, "Предложение не найдено", http.StatusNotFound)
		return
	}

	// п.0/4–8: одна транзакция (паттерн belt_mining_handlers.go).
	tx, err := h.db.Begin()
	if err != nil {
		writeJSONError(w, "Не удалось выполнить покупку", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	// п.0 Сериализация: блокировка строки игрока — единственная точка
	// сериализации покупок; вторая покупка в тот же слот видит оборудование
	// ПОСЛЕ предыдущей установки и корректно считает трейд-ин (§7.2 п.0).
	var equipRaw []byte
	var modelID sql.NullString
	if err := tx.QueryRow(
		`SELECT equipment, ship_model_id FROM users WHERE id = $1 FOR UPDATE`, userID,
	).Scan(&equipRaw, &modelID); err != nil {
		log.Printf("⚠️ market: buy lock user (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось выполнить покупку", http.StatusInternalServerError)
		return
	}
	equipment := map[string]interface{}{}
	if len(equipRaw) > 0 && string(equipRaw) != "null" {
		if err := json.Unmarshal(equipRaw, &equipment); err != nil {
			log.Printf("⚠️ market: buy equipment parse (user %s): %v", userID, err)
			writeJSONError(w, "Не удалось выполнить покупку", http.StatusInternalServerError)
			return
		}
	}

	// п.4 Слот: ключ универсальный (§ выше) и номер ≤ slots.universal модели.
	// Неизвестная/отсутствующая модель — дефолт 'starter' (как трактует игра).
	mid := models.StarterShipModelID
	if modelID.Valid && modelID.String != "" {
		mid = modelID.String
	}
	model := ship.ShipModelByID(mid)
	maxSlots := 0
	if model != nil {
		if v, ok := model.Slots["universal"].(float64); ok {
			maxSlots = int(v)
		}
	}
	if slotNum > maxSlots {
		writeJSONError(w, "Неверный слот", http.StatusBadRequest)
		return
	}

	// п.5 Сумма: tradein = floor(price_старого / 2), price_старого — из
	// market_offers по item_id стоящего модуля; нет в market_offers → 0.
	// amount = max(0, price_нового − tradein) — выкуп не даёт денег сверх цены.
	tradein := int64(0)
	if oldID, ok := equipment[req.Slot].(string); ok && oldID != "" {
		var oldPrice int64
		err := tx.QueryRow(
			`SELECT price FROM market_offers WHERE kind = 'module' AND item_id = $1 ORDER BY id ASC LIMIT 1`,
			oldID,
		).Scan(&oldPrice)
		switch {
		case err == nil:
			tradein = oldPrice / 2
		case err == sql.ErrNoRows:
			tradein = 0
		default:
			log.Printf("⚠️ market: buy tradein lookup (user %s): %v", userID, err)
			writeJSONError(w, "Не удалось выполнить покупку", http.StatusInternalServerError)
			return
		}
	}
	amount := offer.Price - tradein
	if amount < 0 {
		amount = 0
	}

	// п.6 Списание (инвариант 1, §7.2 п.6): проверка достатка и списание одним
	// условным оператором; 0 строк → недостаточно средств. withdrawable урезается
	// до нового баланса (метка «заработанное» сгорает при трате, §8).
	var balanceAfter int64
	err = tx.QueryRow(
		`UPDATE accounts
		    SET balance = balance - $2,
		        withdrawable = LEAST(withdrawable, balance - $2),
		        updated_at = NOW()
		  WHERE owner_type = 'player' AND owner_id = $1 AND balance >= $2
		  RETURNING balance`,
		userID, amount,
	).Scan(&balanceAfter)
	if err == sql.ErrNoRows {
		writeJSONError(w, "Недостаточно средств", http.StatusConflict)
		return
	}
	if err != nil {
		log.Printf("⚠️ market: buy debit (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось выполнить покупку", http.StatusInternalServerError)
		return
	}

	// п.7 Установка: модуль в слот игрока (jsonb_set создаёт ключ, если его
	// нет — пустой слот пишет ключ, выкупа нет, И-М6). Выкупленный исчезает
	// (инвентаря нет) — ключ перезаписывается.
	if _, err := tx.Exec(
		`UPDATE users
		    SET equipment = jsonb_set(COALESCE(equipment, '{}'::jsonb), ARRAY[$2::text], to_jsonb($3::text), true),
		        updated_at = NOW()
		  WHERE id = $1`,
		userID, req.Slot, offer.ItemID,
	); err != nil {
		log.Printf("⚠️ market: buy install (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось выполнить покупку", http.StatusInternalServerError)
		return
	}

	// п.8 Журнал (§8): kind='purchase', delta = −amount, balance_after.
	if _, err := tx.Exec(
		`INSERT INTO money_operations (owner_type, owner_id, delta, balance_after, kind, contract_id, occurred_at)
		 VALUES ('player', $1, $2, $3, $4, NULL, NOW())`,
		userID, -amount, balanceAfter, marketMoneyOpPurchase,
	); err != nil {
		log.Printf("⚠️ market: buy journal (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось выполнить покупку", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		writeJSONError(w, "Не удалось выполнить покупку", http.StatusInternalServerError)
		return
	}

	// п.9 Ответ (§7.2).
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"ok":      true,
		"amount":  amount,
		"tradein": tradein,
		"slot":    req.Slot,
		"item_id": offer.ItemID,
	})
}