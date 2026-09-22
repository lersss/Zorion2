package models

import "time"

// EffectType — тип эффекта (каталог, миграция 000070, спека
// 2026-09-22-эффекты-снабжения-задержка-голод §3.1). params типа несёт только
// ссылку Curve на компоненту «Балансировки»; recovery — скаляр компоненты,
// не свойство типа (§7.1/§7.4).
type EffectType struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	NameNorm string `json:"name_norm"`
	// Impact — вид воздействия, ОТКРЫТЫЙ набор (§5.4); сейчас 'population_rate'.
	Impact string `json:"impact"`
	// Curve — params.curve: ссылка на компоненту «Балансировки» (пилот 'hunger').
	Curve     string    `json:"curve"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ActiveEffect — действующий эффект владельца (миграция 000070, §3.1): строка
// живёт всегда, состояние = f(load ≥ порог) — производное, не хранится.
// Владелец полиморфен (owner_type/owner_id); сегодня 'settlement',
// owner_settlement_id — якорь очистки (CASCADE). SourcePosition — name_norm
// позиции корзины (информационно, из params.effects), без FK.
type ActiveEffect struct {
	ID                string    `json:"id"`
	EffectTypeID      int64     `json:"effect_type_id"`
	OwnerType         string    `json:"owner_type"`
	OwnerID           string    `json:"owner_id"`
	OwnerSettlementID *string   `json:"owner_settlement_id,omitempty"`
	SourcePosition    *string   `json:"source_position,omitempty"`
	Load              float64   `json:"load"`
	LoadAt            time.Time `json:"load_at"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`

	// Витрина этапа 2 (§6), не колонки БД: ссылка на кривую, вид воздействия,
	// порог включения (нулевой префикс), текущая сила R(load), сила условия w и
	// состояние (load ≥ порог) — производное, не хранится.
	Curve  string `json:"curve,omitempty"`
	Impact string `json:"impact,omitempty"`
	// Threshold/Enabled — без omitempty: у снятого эффекта они равны 0/false,
	// и omitempty скрывал бы само состояние (порог 0 и «снят») в ответе.
	Threshold float64 `json:"threshold"`
	Rate      float64 `json:"rate,omitempty"`
	W         float64 `json:"w,omitempty"`
	Enabled   bool    `json:"enabled"`
}
