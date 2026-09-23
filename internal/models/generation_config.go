// internal/models/generation_config.go
package models

import (
	"encoding/json"
	"time"
)

// DefaultSettlementTypeIDKey — ключ generation_config с id базового типа
// поселения (payload — JSON-число). Решение создателя 2026-09-23: связь
// «поселение → тип» резолвится по id, а не по имени (переименование контента
// не должно ронять типы новых поселений). Пишется сидом каталога
// (goodsstudio.SeedProducers) на свежей БД и миграцией 000075 на существующих;
// читается repository.ResolveDefaultSettlementTypeID.
const DefaultSettlementTypeIDKey = "default_settlement_type_id"

// SettlementArithmeticVisibleKey — ключ generation_config с настройкой
// видимости блока арифметики поселения игроку (payload — JSON-bool, дефолт
// true при отсутствии ключа). Решение создателя Г5 (спека 2026-09-23-стадии-
// поселения §8.3/§10): persistent — настройку обещано «позже выключить»,
// in-memory сбрасывалась бы рестартом. Пишется/читается
// repository.SettlementArithmeticVisibleToPlayer.
const SettlementArithmeticVisibleKey = "settlement_arithmetic_visible_to_player"

// GenerationConfig — строка реестра конфигов генерации (таблица generation_config,
// спека 99.2.3 §3). Паттерн «JSON-дефолты в коде + override в БД» — как матрица
// совместимости. Payload — произвольный JSON по ключу.
type GenerationConfig struct {
	Key       string          `json:"key"`
	Payload   json.RawMessage `json:"payload"`
	UpdatedAt time.Time       `json:"updated_at"`
}
