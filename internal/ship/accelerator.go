// internal/ship/accelerator.go
// Модуль-ускоритель перелёта (спека
// 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута.md §3.1): какую
// мини-игру открывает (params.game), откат (params.cooldown_min), предел бонуса
// скорости (params.bonus_max) и два порога остатка (offer/boost). Формула
// бонуса — константа модели (`bonus_min = 0.10` — пол «участие без штрафа», не
// параметр модуля); читается безопасно: битый/немонотонный/«телепорт» params →
// модуль есть, игра не предлагается (available=false, reason=unknown_game).
package ship

import (
	"encoding/json"
	"sort"

	"zorion/internal/models"
)

// AcceleratorGameRoute — единственная игра v1 («Прокладка маршрута», §6).
const AcceleratorGameRoute = "route"

// acceleratorGames — реестр мини-игр, которые СЕРВЕР умеет проводить (§6).
// ЧК3 зарегистрировал «Прокладку маршрута» (AcceleratorGameRoute) — гейт
// доступности включается сам, без правки блоков /travel и /me (чтение реестра
// из горутин-обработчиков): валидный `accel_1` даёт available по состоянию
// сегмента, а не unknown_game. Запись в каталоге с незарегистрированной игрой
// по-прежнему даёт unknown_game.
var acceleratorGames = []string{AcceleratorGameRoute}

// AcceleratorGameRegistered — сервер умеет проводить эту мини-игру (§3.4):
// незарегистрированная игра даёт `unknown_game` независимо от записи в каталоге.
func AcceleratorGameRegistered(game string) bool {
	for _, g := range acceleratorGames {
		if g == game {
			return true
		}
	}
	return false
}

// AcceleratorBonusMin — константа модели: пол «участие без штрафа» (§3.1).
// bonus = bonusMin + (bonusMax − bonusMin)·q.
const AcceleratorBonusMin = 0.10

// acceleratorSlotOrder — детерминированный порядок поиска модуля-ускорителя
// (§3.2): выделенный слот, затем универсальные. Гарантия «одно активное
// ускорение на сегмент» — серверный гейт, а не ёмкость слота.
var acceleratorSlotOrder = []string{"accelerator", "universal", "universal2", "universal3"}

// AcceleratorConfig — параметры модуля-ускорителя (§3.1).
type AcceleratorConfig struct {
	Game               string
	CooldownMin        int
	BonusMax           float64
	MinRemainingOfferS int
	MinRemainingBoostS int
}

// AcceleratorModule — первый модуль типа accelerator по детерминированному
// порядку слотов. found — модуль установлен (валидный предмет каталога нужного
// типа); valid — params читаемы и допустимы: непустой game, 0.10 ≤ bonus_max ≤
// 1.0 (М3: ниже — немонотонность, выше — «телепорт»), заданы откат и оба порога.
// found && !valid → модуль есть, игра не предлагается (unknown_game).
func AcceleratorModule(userEquipment map[string]interface{}) (id string, cfg AcceleratorConfig, found, valid bool) {
	seen := map[string]bool{}
	try := func(slot string) bool {
		if seen[slot] {
			return false
		}
		seen[slot] = true
		v, ok := userEquipment[slot]
		if !ok {
			return false
		}
		sid, ok := v.(string)
		if !ok || sid == "" {
			return false
		}
		it := EquipmentByID(sid)
		if it == nil || it.Type != models.EquipmentTypeAccelerator {
			return false
		}
		id = sid
		found = true
		cfg, valid = parseAcceleratorParams(it.Params)
		return true
	}
	for _, slot := range acceleratorSlotOrder {
		if try(slot) {
			return id, cfg, found, valid
		}
	}
	// Ускоритель в нестандартном слоте (эффект работает по типу из ЛЮБОГО слота,
	// §3.2) — остальные ключи в детерминированном (сортированном) порядке.
	keys := make([]string, 0, len(userEquipment))
	for k := range userEquipment {
		if !seen[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, slot := range keys {
		if try(slot) {
			return id, cfg, found, valid
		}
	}
	return "", AcceleratorConfig{}, false, false
}

// parseAcceleratorParams — безопасный разбор params (нечисловое/отсутствующее/
// вне границ → valid=false, без паники).
func parseAcceleratorParams(params map[string]interface{}) (AcceleratorConfig, bool) {
	var cfg AcceleratorConfig
	if params == nil {
		return cfg, false
	}
	g, ok := params["game"].(string)
	if !ok || g == "" {
		return cfg, false
	}
	cfg.Game = g
	cd, ok := numParam(params["cooldown_min"])
	if !ok || cd < 0 {
		return cfg, false
	}
	cfg.CooldownMin = int(cd)
	bm, ok := numParam(params["bonus_max"])
	if !ok || bm < AcceleratorBonusMin || bm > 1.0 {
		return cfg, false
	}
	cfg.BonusMax = bm
	offer, ok := numParam(params["min_remaining_offer_s"])
	if !ok || offer < 0 {
		return cfg, false
	}
	cfg.MinRemainingOfferS = int(offer)
	boost, ok := numParam(params["min_remaining_boost_s"])
	if !ok || boost < 0 {
		return cfg, false
	}
	cfg.MinRemainingBoostS = int(boost)
	return cfg, true
}

// numParam — число из params JSONB (float64 после json.Unmarshal; целые типы —
// страховка для дефолтов и тестов).
func numParam(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

// AcceleratorBonus — бонус скорости по качеству пути q (§3.1):
// bonus = 0.10 + (bonus_max − 0.10)·q; q зажат в [0,1]. valid=false, если
// bonus_max вне [0.10, 1.0] (немонотонность/«телепорт», М3) — нет выигрыша.
// newRem = remaining/(1+bonus) > 0 всегда (И-6): при bonus ≥ 0.10 время не растёт.
func AcceleratorBonus(q, bonusMax float64) (float64, bool) {
	if bonusMax < AcceleratorBonusMin || bonusMax > 1.0 {
		return 0, false
	}
	if q < 0 {
		q = 0
	}
	if q > 1 {
		q = 1
	}
	return AcceleratorBonusMin + (bonusMax-AcceleratorBonusMin)*q, true
}
