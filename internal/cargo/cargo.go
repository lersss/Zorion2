// internal/cargo/cargo.go
//
// Трюм игрока — грузоподъёмность корабля (спека
// 2026-09-22-трюм-грузоподъёмность-корабля §7–§9). Состояние — таблица
// player_cargo ((user_id, good_id) → quantity), ёмкость — производная:
// врождённая ёмкость модели корабля + сумма грузовых модулей (ship.CargoCapacityMass).
// Занятая масса = Σ quantity × goods.weight (И3: новых полей у груза нет).
//
// Запись — серверно-авторитетная (§9.2): пополнение только через сервис из
// игровой логики (добыча пояса, позже контракты/торговля); единственная
// player-facing запись — «груз за борт» (только уменьшает, §7.1).
//
// Конкурентность (AGENTS.md §0): TryAddCargo блокирует строку users
// (SELECT ... FOR UPDATE) — мутации трюма одного игрока сериализуются,
// поэтому два конкурентных пополнения не могут переполнить трюм (жёсткий
// предел §7-A, инвариант used ≤ total по построению).
package cargo

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"zorion/internal/ship"
)

// ErrUserNotFound / ErrGoodNotFound — игрок/товар отсутствуют; хендлер
// маппит в 404.
var (
	ErrUserNotFound = errors.New("игрок не найден")
	ErrGoodNotFound = errors.New("товар не найден")
)

// Item — строка трюма с данными товара (ответ GET /api/cargo, §9.1).
// Mass = Quantity × Weight считает сервер (И2).
type Item struct {
	GoodID   int64   `json:"good_id"`
	Name     string  `json:"name"`
	Kind     string  `json:"kind"`
	Quantity float64 `json:"quantity"`
	Weight   float64 `json:"weight"`
	Mass     float64 `json:"mass"`
}

// MassLimit — лимит по оси массы (§4: лимит по осям).
type MassLimit struct {
	Used  float64 `json:"used"`
	Total float64 `json:"total"`
}

// Limits — набор осей лимита (сейчас одна — mass; задел под объём, §4).
type Limits struct {
	Mass MassLimit `json:"mass"`
}

// View — трюм для ответа API (§9.1): лимиты по осям + строки груза.
type View struct {
	Limits Limits `json:"limits"`
	Items  []Item `json:"items"`
}

// Service — трюм игрока: чтение/запись player_cargo + расчёт ёмкости.
type Service struct {
	db *sql.DB
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// queryRower — общее для *sql.DB и *sql.Tx (чтение одного значения).
type queryRower interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}

const (
	userShipSQL          = `SELECT ship_model_id, equipment FROM users WHERE id = $1`
	userShipForUpdateSQL = userShipSQL + ` FOR UPDATE`

	cargoItemsSQL = `
		SELECT pc.good_id, g.name, g.kind, pc.quantity, g.weight
		FROM player_cargo pc
		JOIN goods g ON g.id = pc.good_id
		WHERE pc.user_id = $1
		ORDER BY g.name, pc.good_id
	`

	usedMassSQL = `
		SELECT COALESCE(SUM(pc.quantity * g.weight), 0)
		FROM player_cargo pc
		JOIN goods g ON g.id = pc.good_id
		WHERE pc.user_id = $1
	`

	goodWeightSQL = `SELECT weight FROM goods WHERE id = $1`

	upsertCargoSQL = `
		INSERT INTO player_cargo (user_id, good_id, quantity, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (user_id, good_id)
		DO UPDATE SET quantity = player_cargo.quantity + EXCLUDED.quantity, updated_at = NOW()
	`
)

// parseEquipment — equipment игрока (JSONB) → map. NULL/битый JSON → nil
// (безопасный отказ: ёмкость = только врождённая, §11).
func parseEquipment(raw []byte) map[string]interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// capacity читает модель корабля и оборудование игрока и считает ёмкость.
// forUpdate=true — блокировка строки users для мутаций (сериализация, §0).
func capacity(q queryRower, userID string, forUpdate bool) (float64, error) {
	sqlStr := userShipSQL
	if forUpdate {
		sqlStr = userShipForUpdateSQL
	}
	var modelID sql.NullString
	var equipRaw []byte
	err := q.QueryRow(sqlStr, userID).Scan(&modelID, &equipRaw)
	if err == sql.ErrNoRows {
		return 0, ErrUserNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("cargo: чтение модели корабля: %w", err)
	}
	return ship.CargoCapacityMass(modelID.String, parseEquipment(equipRaw)), nil
}

// GetCargo — строки груза игрока с данными товара (имя/kind/weight).
func (s *Service) GetCargo(userID string) ([]Item, error) {
	rows, err := s.db.Query(cargoItemsSQL, userID)
	if err != nil {
		return nil, fmt.Errorf("cargo: чтение груза: %w", err)
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.GoodID, &it.Name, &it.Kind, &it.Quantity, &it.Weight); err != nil {
			return nil, fmt.Errorf("cargo: скан строки груза: %w", err)
		}
		it.Mass = it.Quantity * it.Weight
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cargo: перебор строк груза: %w", err)
	}
	return items, nil
}

// Capacity — ёмкость трюма игрока (тонны).
func (s *Service) Capacity(userID string) (float64, error) {
	return capacity(s.db, userID, false)
}

// CargoMass — занятая масса игрока = Σ quantity × goods.weight.
func (s *Service) CargoMass(userID string) (float64, error) {
	var used float64
	if err := s.db.QueryRow(usedMassSQL, userID).Scan(&used); err != nil {
		return 0, fmt.Errorf("cargo: чтение занятой массы: %w", err)
	}
	return used, nil
}

// View — трюм для ответа API: лимит по массе (used/total) + строки. used
// считает сервер как Σ mass строк (И2), поэтому used ≤ total держится.
func (s *Service) View(userID string) (*View, error) {
	items, err := s.GetCargo(userID)
	if err != nil {
		return nil, err
	}
	total, err := s.Capacity(userID)
	if err != nil {
		return nil, err
	}
	used := 0.0
	for _, it := range items {
		used += it.Mass
	}
	if items == nil {
		items = []Item{}
	}
	return &View{
		Limits: Limits{Mass: MassLimit{Used: used, Total: total}},
		Items:  items,
	}, nil
}

// TryAddCargo — положить qty единиц товара с клампом по свободной ёмкости
// (§7-A, жёсткий предел): принимает min(qty, свободно в единицах), остаток
// не списывается у источника. qty ≤ 0 — no-op. Возвращает принятое количество.
// Атомарно: блокировка строки users (FOR UPDATE) сериализует пополнения
// одного игрока — конкурентные вызовы не переполнят трюм.
func (s *Service) TryAddCargo(userID string, goodID int64, qty float64) (float64, error) {
	if qty <= 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("cargo: begin: %w", err)
	}
	defer tx.Rollback()

	total, err := capacity(tx, userID, true)
	if err != nil {
		return 0, err
	}

	var weight float64
	err = tx.QueryRow(goodWeightSQL, goodID).Scan(&weight)
	if err == sql.ErrNoRows {
		return 0, ErrGoodNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("cargo: вес товара: %w", err)
	}

	// Вес — тонны (спека §6): 0/отрицательный weight = битые данные каталога
	// (CHECK weight > 0 у goods нет, §6). Отказываем явно, а не молча
	// отключаем кламп («бесконечный трюм»): пополнение без ограничения массы.
	if weight <= 0 {
		return 0, nil
	}

	var used float64
	if err := tx.QueryRow(usedMassSQL, userID).Scan(&used); err != nil {
		return 0, fmt.Errorf("cargo: занятая масса: %w", err)
	}

	free := total - used
	if free <= 0 {
		return 0, nil
	}
	accepted := qty
	if maxUnits := free / weight; accepted > maxUnits {
		accepted = maxUnits
	}
	// Инвариант И1 строго (used ≤ total): free/weight × weight может дать
	// лишний ULP сверх free (проверено: 100/0.3×0.3 = 100.00000000000001) —
	// сдвигаем принятое вниз, пока accepted×weight строго не уложится в free.
	for i := 0; i < 8 && accepted > 0 && accepted*weight > free; i++ {
		accepted = math.Nextafter(accepted, 0)
	}
	if accepted*weight > free {
		return 0, nil
	}
	if accepted <= 0 {
		return 0, nil
	}

	if _, err := tx.Exec(upsertCargoSQL, userID, goodID, accepted); err != nil {
		return 0, fmt.Errorf("cargo: запись груза: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("cargo: commit: %w", err)
	}
	return accepted, nil
}

// takeTx — общее снятие qty единиц в транзакции (TakeCargo и JettisonCargo):
// не больше, чем есть; строка с нулём удаляется (§8.2). all=true — снять
// весь товар строки. Возвращает фактически снятое количество.
func takeTx(tx *sql.Tx, userID string, goodID int64, qty float64, all bool) (float64, error) {
	var have float64
	err := tx.QueryRow(
		`SELECT quantity FROM player_cargo WHERE user_id = $1 AND good_id = $2 FOR UPDATE`,
		userID, goodID,
	).Scan(&have)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("cargo: чтение строки груза: %w", err)
	}
	taken := qty
	if all || taken > have {
		taken = have
	}
	if taken <= 0 {
		return 0, nil
	}
	if have-taken <= 0 {
		if _, err := tx.Exec(`DELETE FROM player_cargo WHERE user_id = $1 AND good_id = $2`, userID, goodID); err != nil {
			return 0, fmt.Errorf("cargo: удаление строки груза: %w", err)
		}
	} else {
		if _, err := tx.Exec(
			`UPDATE player_cargo SET quantity = $1, updated_at = NOW() WHERE user_id = $2 AND good_id = $3`,
			have-taken, userID, goodID,
		); err != nil {
			return 0, fmt.Errorf("cargo: снятие груза: %w", err)
		}
	}
	return taken, nil
}

// TakeCargo — снять не больше, чем есть (строка с нулём удаляется). qty ≤ 0 —
// no-op. Возвращает фактически снятое количество.
func (s *Service) TakeCargo(userID string, goodID int64, qty float64) (float64, error) {
	if qty <= 0 {
		return 0, nil
	}
	return s.mutate(userID, goodID, qty, false)
}

// JettisonCargo — «груз за борт» (§7.1): сброс qty единиц своего товара
// (all=true — весь товар строки). Только уменьшает: списывается не больше,
// чем есть; сброшенное исчезает (объект в космосе не создаётся).
func (s *Service) JettisonCargo(userID string, goodID int64, qty float64, all bool) (float64, error) {
	if goodID <= 0 {
		return 0, ErrGoodNotFound
	}
	if err := s.goodExists(goodID); err != nil {
		return 0, err
	}
	if !all && qty <= 0 {
		return 0, nil
	}
	return s.mutate(userID, goodID, qty, all)
}

// JettisonAll — сброс всего трюма (§7.1: «выбросить всё»). Возвращает
// суммарно сброшенное количество (в единицах).
func (s *Service) JettisonAll(userID string) (float64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("cargo: begin: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.Query(`DELETE FROM player_cargo WHERE user_id = $1 RETURNING quantity`, userID)
	if err != nil {
		return 0, fmt.Errorf("cargo: сброс трюма: %w", err)
	}
	total := 0.0
	for rows.Next() {
		var q float64
		if err := rows.Scan(&q); err != nil {
			rows.Close()
			return 0, fmt.Errorf("cargo: скан сброса: %w", err)
		}
		total += q
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("cargo: перебор сброса: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("cargo: commit: %w", err)
	}
	return total, nil
}

// goodExists — товар есть в каталоге (валидация §9.2).
func (s *Service) goodExists(goodID int64) error {
	var one int
	err := s.db.QueryRow(`SELECT 1 FROM goods WHERE id = $1`, goodID).Scan(&one)
	if err == sql.ErrNoRows {
		return ErrGoodNotFound
	}
	if err != nil {
		return fmt.Errorf("cargo: проверка товара: %w", err)
	}
	return nil
}

// mutate — транзакция снятия (общая для TakeCargo/JettisonCargo).
func (s *Service) mutate(userID string, goodID int64, qty float64, all bool) (float64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("cargo: begin: %w", err)
	}
	defer tx.Rollback()

	taken, err := takeTx(tx, userID, goodID, qty, all)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("cargo: commit: %w", err)
	}
	return taken, nil
}
