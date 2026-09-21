// internal/handlers/money_handlers.go
//
// Деньги игрока (спека 2026-09-22-деньги-и-эскроу §3.2, О-д4): свой баланс и
// своя история операций. Балансы приватны (канон 14_money.md §14.4) — owner_id
// берётся из JWT, чужое не показывается.
package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"zorion/internal/auth"
	"zorion/internal/models"
	"zorion/internal/repository"
)

// moneyHistoryLimit — сколько последних операций отдавать в /me/money.
// Ограничение сверху: стоимость не растёт с числом операций (инвариант 2).
const moneyHistoryLimit = 50

type MoneyHandlers struct {
	accountRepo *repository.AccountRepository
}

func NewMoneyHandlers(accountRepo *repository.AccountRepository) *MoneyHandlers {
	return &MoneyHandlers{accountRepo: accountRepo}
}

// GetMyMoney — GET /me/money: баланс + история операций игрока. Счёт
// гарантируется лениво и идемпотентно (§3.4) — первый запрос к деньгам
// покрывает старых игроков и сбои.
func (h *MoneyHandlers) GetMyMoney(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}

	if err := h.accountRepo.EnsureAccount(models.AccountOwnerPlayer, userID, models.PlayerBalanceSeed); err != nil {
		log.Printf("/me/money: ensureAccount(%s): %v", userID, err)
	}

	acc, err := h.accountRepo.GetBalance(models.AccountOwnerPlayer, userID)
	if err != nil {
		writeJSONError(w, "Не удалось получить баланс", http.StatusInternalServerError)
		return
	}
	if acc == nil {
		writeJSONError(w, "Счёт не найден", http.StatusInternalServerError)
		return
	}

	ops, err := h.accountRepo.GetOperations(models.AccountOwnerPlayer, userID, moneyHistoryLimit)
	if err != nil {
		writeJSONError(w, "Не удалось получить историю операций", http.StatusInternalServerError)
		return
	}
	if ops == nil {
		ops = []models.MoneyOperation{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"balance":      acc.Balance,
		"withdrawable": acc.Withdrawable,
		"operations":   ops,
	})
}
