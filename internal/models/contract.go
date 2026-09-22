package models

import "time"

// Статусы контракта (contracts.status, спека 2026-09-22-контракт-модель-сущности
// §4.1/§5). Провала отдельным статусом нет: провал — это expired при непустом
// executor_id (различие живёт в contract_log, §5).
const (
	ContractStatusDraft     = "draft"
	ContractStatusOpen      = "open"
	ContractStatusTaken     = "taken"
	ContractStatusCompleted = "completed"
	ContractStatusCancelled = "cancelled"
	ContractStatusExpired   = "expired"
)

// Тип контракта (contracts.type, §4.1) — открытый список; travel — первый
// (спека перелёта §1). Тип-специфичные правила (перебазирование срока при
// взятии) включаются по этому значению.
const (
	ContractTypeTravel = "travel"
)

// Типы автора контракта (contracts.author_type, §4.1) — шире buildings.owner_type:
// добавлен building (заказчик-постройка). Полиморфно, FK нет.
const (
	ContractActorPlayer   = "player"
	ContractActorFaction  = "faction"
	ContractActorBuilding = "building"
	ContractActorAgent    = "agent"
)

// Тип исполнителя (contracts.executor_type, §4.1): игрок или NPC-агент.
const (
	ContractExecutorPlayer = "player"
	ContractExecutorAgent  = "agent"
)

// Формы видимости (contracts.visibility, §4.1).
const (
	ContractVisibilityPublic = "public"
	ContractVisibilityDirect = "direct"
)

// Признак финансирования (contracts.funding, §4.1; 15_monetization §15.4):
// regular — обычный, contract_work — подряд (оплачен реальными деньгами).
const (
	ContractFundingRegular      = "regular"
	ContractFundingContractWork = "contract_work"
)

// Форма эскроу (contracts.escrow_kind, §4.1): deposit — залог (итерация 1).
const (
	EscrowKindDeposit = "deposit"
)

// Типы записей contract_log (contract_log.type, §4.3) — открытый список.
const (
	ContractLogPublished      = "published"
	ContractLogTaken          = "taken"
	ContractLogCompleted      = "completed"
	ContractLogCancelled      = "cancelled"
	ContractLogExpired        = "expired"
	ContractLogFailed         = "failed"
	ContractLogEscrowLocked   = "escrow_locked"
	ContractLogEscrowReleased = "escrow_released"
	ContractLogEscrowReturned = "escrow_returned"
)

// Причины возврата залога (contract_log.data.reason у escrow_returned, §4.3):
// expired/cancelled — обычные пути, world_deleted — принудительный возврат при
// удалении миров (§6.5).
const (
	EscrowReasonExpired      = "expired"
	EscrowReasonCancelled    = "cancelled"
	EscrowReasonWorldDeleted = "world_deleted"
)

// Типы движения по счёту (money_operations.kind, спека денег §3.2).
const (
	MoneyOpEscrowLock    = "escrow_lock"
	MoneyOpEscrowRelease = "escrow_release"
	MoneyOpEscrowReturn  = "escrow_return"
	MoneyOpContractWork  = "contract_work_earn"
)

// Contract — состояние контракта (contracts, спека §4.1). EscrowAmount —
// запертый залог (не обнуляется при release/return); EscrowWithdrawable —
// выводимая доля залога, восстанавливается при возврате. Payload — нагрузка
// типа (открытый JSONB). AuthorType расширен значением building.
type Contract struct {
	ID                  string                 `json:"id"`
	Type                string                 `json:"type"`
	AuthorType          string                 `json:"author_type"`
	AuthorID            string                 `json:"author_id"`
	PublicationPlanetID string                 `json:"publication_planet_id"`
	Title               string                 `json:"title"`
	Description         string                 `json:"description"`
	Payload             map[string]interface{} `json:"payload"`
	Reward              int64                  `json:"reward"`
	Funding             string                 `json:"funding"`
	EscrowAmount        int64                  `json:"escrow_amount"`
	EscrowWithdrawable  int64                  `json:"escrow_withdrawable"`
	EscrowKind          string                 `json:"escrow_kind"`
	Status              string                 `json:"status"`
	Visibility          string                 `json:"visibility"`
	DirectTargetType    *string                `json:"direct_target_type,omitempty"`
	DirectTargetID      *string                `json:"direct_target_id,omitempty"`
	ExecutorType        *string                `json:"executor_type,omitempty"`
	ExecutorID          *string                `json:"executor_id,omitempty"`
	TakenAt             *time.Time             `json:"taken_at,omitempty"`
	ExpiresAt           time.Time              `json:"expires_at"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`

	// Requirements — требования контракта (доска/«мои», спека перелёта §2.1);
	// заполняются чтением (contract_requirements), не колонка contracts.
	Requirements []ContractRequirement `json:"requirements,omitempty"`
}

// ContractRequirement — строка требований контракта (contract_requirements,
// §4.2). Интервал выражается двумя односторонними строками. ThresholdNum —
// в K (в UI — °C, §7).
type ContractRequirement struct {
	ID            int64    `json:"id"`
	ContractID    string   `json:"contract_id"`
	Pos           int      `json:"pos"`
	Kind          string   `json:"kind"`
	Subject       string   `json:"subject"`
	Op            string   `json:"op"`
	ThresholdNum  *float64 `json:"threshold_num,omitempty"`
	ThresholdText *string  `json:"threshold_text,omitempty"`
	Quantity      *int64   `json:"quantity,omitempty"`
}

// ContractLogEntry — запись журнала жизни контракта (contract_log, §4.3).
// ContractID без FK — лог переживает удаление контракта. Data.reason у
// escrow_returned различает причины возврата залога.
type ContractLogEntry struct {
	ID         string                 `json:"id"`
	ContractID string                 `json:"contract_id"`
	Type       string                 `json:"type"`
	ActorType  *string                `json:"actor_type,omitempty"`
	ActorID    *string                `json:"actor_id,omitempty"`
	Data       map[string]interface{} `json:"data"`
	OccurredAt time.Time              `json:"occurred_at"`
	CreatedAt  time.Time              `json:"created_at"`
}
