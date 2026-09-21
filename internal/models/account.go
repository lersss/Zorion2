package models

import "time"

// Классы владельцев счёта (accounts.owner_type, спека
// 2026-09-22-деньги-и-эскроу §3.1). Актор, который держит деньги: игрок,
// фракция, NPC-агент. Счёта поселений/построек нет: кошелёк поселения —
// лимит, а не счёт (канон 14_money.md §14.3.0), залог платит владелец.
const (
	AccountOwnerPlayer  = "player"
	AccountOwnerFaction = "faction"
	AccountOwnerAgent   = "agent"
)

// Стартовые балансы-заглушки (спека §5, §8): калибровать позже — числа не
// согласованы между собой намеренно. FactionBalanceSeed — «практически
// бесконечная» казна фракции (решение 12), но число, не флаг и не спец-ветка.
const (
	PlayerBalanceSeed  int64 = 10000
	FactionBalanceSeed int64 = 1000000000000000
)

// Account — счёт актора (accounts, спека §3.1). Withdrawable — корзина
// «заработанное» (подмножество Balance), инвариант Withdrawable <= Balance.
type Account struct {
	OwnerType    string    `json:"owner_type"`
	OwnerID      string    `json:"owner_id"`
	Balance      int64     `json:"balance"`
	Withdrawable int64     `json:"withdrawable"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// MoneyOperation — запись журнала движений по счёту (money_operations, §3.2).
// Kind — открытый список (escrow_lock/release/return, contract_work_earn,
// admin_seed, позже mint/salary/transfer). ContractID без FK — журнал
// переживает удаление контракта.
type MoneyOperation struct {
	ID           int64     `json:"id"`
	OwnerType    string    `json:"owner_type"`
	OwnerID      string    `json:"owner_id"`
	Delta        int64     `json:"delta"`
	BalanceAfter int64     `json:"balance_after"`
	Kind         string    `json:"kind"`
	ContractID   *string   `json:"contract_id,omitempty"`
	OccurredAt   time.Time `json:"occurred_at"`
	CreatedAt    time.Time `json:"created_at"`
}
