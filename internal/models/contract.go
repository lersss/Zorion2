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
// взятии) включаются по этому значению. supply — снабжение постройки
// (спека 2026-09-23-контракт-ленивая-доска-пакет-и-снабжение §4.2).
const (
	ContractTypeTravel = "travel"
	ContractTypeSupply = "supply"
)

// Виды нужды, кодируемые в package_key (contracts.package_key, §4.2):
// package_key = <kind>:<planet_id>:<author_id>:<position>. v1 — снабжение.
const (
	ContractNeedKindSupply = "supply"
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

// Причины в contract_log.data.reason (§4.3): expired/cancelled — обычные пути,
// world_deleted — принудительный возврат при удалении миров (§6.5);
// superseded — открытая доля пакета снята сверкой материализации (§3.3).
const (
	EscrowReasonExpired      = "expired"
	EscrowReasonCancelled    = "cancelled"
	EscrowReasonWorldDeleted = "world_deleted"
	EscrowReasonSuperseded   = "superseded"
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
	// Деньги/залог — внутреннее состояние контракта и счёта автора: в API не
	// сериализуются (канон 14_money.md §14.4 «деньги актора не видны»; §4.5
	// спеки 2026-09-23-контракт-ленивая-доска-пакет-и-снабжение — доска без
	// балансов). Поля остаются в Go-модели и БД — их читает логика залога.
	EscrowAmount       int64   `json:"-"`
	EscrowWithdrawable int64   `json:"-"`
	EscrowKind         string  `json:"-"`
	Status             string  `json:"status"`
	Visibility         string  `json:"visibility"`
	DirectTargetType   *string `json:"direct_target_type,omitempty"`
	DirectTargetID     *string `json:"direct_target_id,omitempty"`
	ExecutorType       *string `json:"executor_type,omitempty"`
	ExecutorID         *string `json:"executor_id,omitempty"`
	// PackageKey/ShareIndex — «пакет контрактов» (§4.2): ключ группировки долей
	// одной нужды и позиция доли в пакете; NULL — контракт вне пакета.
	PackageKey *string    `json:"package_key,omitempty"`
	ShareIndex *int       `json:"share_index,omitempty"`
	TakenAt    *time.Time `json:"taken_at,omitempty"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`

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

// TravelContractRef — лёгкая ссылка на открытый перелёт для планировщика NPC
// (спека 2026-09-22-контракт-перелёт-и-доска §1.5, B2b): маршрут from→dest
// сопоставляется с выбранным маршрутом агента. Полный контракт на тик не
// читается — payload и требования планировщику не нужны.
type TravelContractRef struct {
	ID          string
	FromWorldID string
	DestWorldID string
}

// AgentTravelArrival — прибытие агента-исполнителя (B2b): агент прибыл в
// систему WorldID — кандидат на закрытие взятого им перелёта с этим dest.
type AgentTravelArrival struct {
	AgentID string
	WorldID string
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
