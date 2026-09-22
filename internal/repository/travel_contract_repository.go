// internal/repository/travel_contract_repository.go
//
// Перелёт как контракт: интеграция с исполнителем-NPC-агентом (спека
// 2026-09-22-контракт-перелёт-и-доска §1.5, B2b). Отдельного поиска контрактов
// у агента нет (О-п8): планировщик NPCManager выбирает маршрут сам, агент
// берёт лишь совпадающий открытый перелёт с целью-системой (dest_planet_id
// IS NULL) и без проверки двигателя. Закрытие — батчем по прибытиям (одна tx);
// тела истечения/завершения переиспользуются (expireDueTx/completeTx), SQL
// переходов не дублируется.
package repository

import (
	"fmt"
	"time"

	"zorion/internal/models"
)

// ListOpenSystemTravels — открытые перелёты с целью-системой (payload
// dest_planet_id IS NULL), нужные планировщику NPC для сопоставления маршрута
// (§1.5, B2b). Одна выборка на тик (инвариант 2: не по агенту). Контракты с
// целью-планетой не возвращаются: у агентов нет внутрисистемного движения,
// достичь планеты они не могут — иначе закрыли бы контракт на звезде
// преждевременно.
func (r *ContractRepository) ListOpenSystemTravels() ([]models.TravelContractRef, error) {
	rows, err := r.db.Query(`SELECT id, payload->>'from_world_id', payload->>'dest_world_id'
		FROM contracts
		WHERE type = 'travel' AND status = 'open' AND expires_at > NOW()
		  AND payload->>'dest_planet_id' IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("list open system travels: %w", err)
	}
	defer rows.Close()
	var out []models.TravelContractRef
	for rows.Next() {
		var ref models.TravelContractRef
		if err := rows.Scan(&ref.ID, &ref.FromWorldID, &ref.DestWorldID); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

// TakeTravel — агент берёт открытый перелёт: атомарный flip open→taken через
// Take (0 строк = гонка/истёк, §1.4). expiresAt != nil перебазирует срок при
// взятии — правило типа travel (§4.3): срок исполнения отсчитывается от
// взятия, а не от публикации (иначе контракт, взятый в конце offer_window,
// истечёт в полёте — исполнитель теряет награду; капкан «взял перед
// истечением», для игрока закрыт, теперь закрыт и для агента). Требование к
// двигателю к агенту не применяется (§1.5, у агентов своя npcSpeedFactor).
func (r *ContractRepository) TakeTravel(contractID, agentID string, expiresAt *time.Time) (bool, error) {
	return r.Take(contractID, models.ContractExecutorAgent, agentID, expiresAt)
}

// CloseArrivedTravels — закрытие перелётов агентов по прибытии (§1.5, B2b):
// для каждого агента, прибывшего в целевую систему, — взятый им перелёт с
// этим dest_world_id и целью-системой. Порядок и механику переиспользует у
// точки игрока (§1.4): сперва ленивое истечение (просроченный — провал, залог
// автору), затем завершение (залог исполнителю-агенту). Всё — ОДНОЙ
// транзакцией (до 2000 прибытий на тик: сбой на любом откатывает проход).
// Возвращает число фактически выполненных контрактов.
func (r *ContractRepository) CloseArrivedTravels(arrivals []models.AgentTravelArrival) (int, error) {
	if len(arrivals) == 0 {
		return 0, nil
	}
	// Прибытия — пара (агент, мир): контракт закрывается, только если
	// executor_id агента и dest перелёта совпали с прибытием.
	arrived := make(map[string]struct{}, len(arrivals))
	for _, a := range arrivals {
		arrived[a.AgentID+"\x00"+a.WorldID] = struct{}{}
	}

	tx, err := r.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Кандидаты — все currently-taken перелёты агентов (их число ограничено
	// активными исполнителями); пара (executor_id, dest) сверяется в Go.
	rows, err := tx.Query(`SELECT id, executor_id, payload->>'dest_world_id' FROM contracts
		WHERE type = 'travel' AND status = 'taken' AND executor_type = 'agent'
		  AND payload->>'dest_planet_id' IS NULL`)
	if err != nil {
		return 0, fmt.Errorf("close arrived travels: %w", err)
	}
	type candidate struct{ id, agentID string }
	var candidates []candidate
	for rows.Next() {
		var c candidate
		var dest string
		if err := rows.Scan(&c.id, &c.agentID, &dest); err != nil {
			rows.Close()
			return 0, err
		}
		if _, ok := arrived[c.agentID+"\x00"+dest]; !ok {
			continue
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	now := time.Now()
	completed := 0
	for _, c := range candidates {
		if _, err := expireDueTx(tx, ContractScope{ContractID: c.id}); err != nil {
			return 0, err
		}
		ok, err := completeTx(tx, c.id, c.agentID, now)
		if err != nil {
			return 0, err
		}
		if ok {
			completed++
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return completed, nil
}
