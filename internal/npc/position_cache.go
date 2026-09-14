package npc

import (
	"sync/atomic"
	"time"

	"zorion/internal/models"
)

// InterpolatedPosition — позиция агента для карты (спека §8: x, y, status,
// target). Значения уже интерполированы на момент `now` — клиент не знает
// кортежей полёта (спека §7).
type InterpolatedPosition struct {
	ID             string               `json:"id"`
	Name           string               `json:"name"`
	X              float64              `json:"x"`
	Y              float64              `json:"y"`
	Status         models.NPCAgentStatus `json:"status"`
	CurrentWorldID string               `json:"current_world_id"`
	TargetWorldID  *string              `json:"target_world_id,omitempty"`
}

// PositionCache — in-memory snapshot позиций всех агентов (спека §2.2.B).
// Пересчитывается каждый тик планировщиком; чтение хендлером карты — O(1)
// без блокировок (atomic.Pointer, lock-free).
type PositionCache struct {
	ptr atomic.Pointer[[]InterpolatedPosition]
}

func (c *PositionCache) Replace(pos []InterpolatedPosition) {
	c.ptr.Store(&pos)
}

// Snapshot — текущие позиции (nil, пока не пересчитаны).
func (c *PositionCache) Snapshot() []InterpolatedPosition {
	p := c.ptr.Load()
	if p == nil {
		return nil
	}
	return *p
}

// flightProgress — прогресс полёта 0..1 с clamp (спека §3.1):
// (now - depart_at) / (arrive_at - depart_at). Нулевой/отрицательный span —
// прибыл (1).
func flightProgress(now, departAt, arriveAt time.Time) float64 {
	span := arriveAt.Sub(departAt)
	if span <= 0 {
		return 1
	}
	p := float64(now.Sub(departAt)) / float64(span)
	if p < 0 {
		return 0
	}
	if p > 1 {
		return 1
	}
	return p
}

// interpolatePosition — позиция агента на момент now (спека §3.1):
//   - flying: интерполяция from → target по прогрессу;
//   - idle/observing (и flying без кортежа полёта): позиция текущего мира.
//
// Мира нет в сетке (после перегенерации вселенной) — позиция (0,0), флаг
// ok=false; карта такие позиции может скрыть (этап 4).
func interpolatePosition(a models.NPCAgent, g *worldGrid, now time.Time) (InterpolatedPosition, bool) {
	p := InterpolatedPosition{
		ID:             a.ID,
		Name:           a.Name,
		Status:         a.Status,
		CurrentWorldID: a.CurrentWorldID,
		TargetWorldID:  a.TargetWorldID,
	}

	if a.Status == models.NPCAgentStatusFlying &&
		a.FromWorldID != nil && a.TargetWorldID != nil &&
		a.DepartAt != nil && a.ArriveAt != nil {
		fx, fy, fok := g.coordsOf(*a.FromWorldID)
		tx, ty, tok := g.coordsOf(*a.TargetWorldID)
		if fok && tok {
			pr := flightProgress(now, *a.DepartAt, *a.ArriveAt)
			p.X = fx + (tx-fx)*pr
			p.Y = fy + (ty-fy)*pr
			return p, true
		}
	}

	cx, cy, ok := g.coordsOf(a.CurrentWorldID)
	if !ok {
		return p, false
	}
	p.X, p.Y = cx, cy
	return p, true
}