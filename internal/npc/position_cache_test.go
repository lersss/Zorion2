// internal/npc/position_cache_test.go
// Тесты интерполяции позиции и PositionCache (спека 20a.1 §3.1, §2.2.B).
package npc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zorion/internal/mapcache"
	"zorion/internal/models"
)

func ptrStr(s string) *string { return &s }

func ptrTime(t time.Time) *time.Time { return &t }

func TestFlightProgressClamp(t *testing.T) {
	depart := time.Now()
	arrive := depart.Add(100 * time.Second)

	require.Equal(t, 0.0, flightProgress(depart.Add(-time.Hour), depart, arrive), "до вылета — 0")
	require.Equal(t, 0.5, flightProgress(depart.Add(50*time.Second), depart, arrive))
	require.Equal(t, 1.0, flightProgress(arrive, depart, arrive), "на момент прибытия — 1")
	require.Equal(t, 1.0, flightProgress(arrive.Add(time.Hour), depart, arrive), "после прибытия — 1 (clamp)")
	require.Equal(t, 1.0, flightProgress(depart, depart, depart), "span ≤ 0 (битый кортеж) — прибыл")
}

func TestInterpolatePositionFlying(t *testing.T) {
	depart := time.Now()
	arrive := depart.Add(100 * time.Second)
	grid := buildGrid([]mapcache.World{
		{ID: "w1", Name: "Alpha", X: 0, Y: 0},
		{ID: "w2", Name: "Beta", X: 100, Y: 0},
	})

	agent := models.NPCAgent{
		ID: "a1", Status: models.NPCAgentStatusFlying,
		CurrentWorldID: "w1",
		FromWorldID:    ptrStr("w1"), TargetWorldID: ptrStr("w2"),
		DepartAt: ptrTime(depart), ArriveAt: ptrTime(arrive),
	}

	// Середина пути: (0+100)/2, 0.
	p, ok := interpolatePosition(agent, grid, depart.Add(50*time.Second))
	require.True(t, ok)
	require.InDelta(t, 50.0, p.X, 0.001)
	require.InDelta(t, 0.0, p.Y, 0.001)
	require.NotNil(t, p.TargetWorldID)
	require.Equal(t, "w2", *p.TargetWorldID)
	// Имена миров для тултипа агента (идея 2026-09-18): названия, не айди.
	require.Equal(t, "Alpha", p.CurrentWorldName)
	require.NotNil(t, p.TargetWorldName)
	require.Equal(t, "Beta", *p.TargetWorldName)

	// После прибытия (progress=1) — позиция цели.
	p, _ = interpolatePosition(agent, grid, arrive.Add(time.Hour))
	require.InDelta(t, 100.0, p.X, 0.001)
}

func TestInterpolatePositionIdle(t *testing.T) {
	grid := buildGrid([]mapcache.World{{ID: "w1", X: 7, Y: -3}})
	agent := models.NPCAgent{
		ID: "a1", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w1",
	}

	p, ok := interpolatePosition(agent, grid, time.Now())
	require.True(t, ok)
	require.Equal(t, 7.0, p.X)
	require.Equal(t, -3.0, p.Y)
}

func TestInterpolatePositionNoWorld(t *testing.T) {
	// Мира нет в сетке (после перегенерации вселенной) — ok=false.
	grid := buildGrid(nil)
	agent := models.NPCAgent{ID: "a1", Status: models.NPCAgentStatusIdle, CurrentWorldID: "w1"}
	_, ok := interpolatePosition(agent, grid, time.Now())
	require.False(t, ok)
}

func TestPositionCacheReplaceSnapshot(t *testing.T) {
	var c PositionCache
	require.Nil(t, c.Snapshot(), "до первого тика позиций нет")

	c.Replace([]InterpolatedPosition{{ID: "a1", X: 1, Y: 2}})
	snap := c.Snapshot()
	require.Len(t, snap, 1)
	require.Equal(t, "a1", snap[0].ID)

	// Replace целиком заменяет снимок.
	c.Replace([]InterpolatedPosition{{ID: "a2", X: 3, Y: 4}})
	require.Len(t, c.Snapshot(), 1)
	require.Equal(t, "a2", c.Snapshot()[0].ID)
}