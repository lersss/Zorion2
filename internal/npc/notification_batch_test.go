// internal/npc/notification_batch_test.go
// Тесты буфера прибытий с дросселем (спека 20a.1 §2.2.C): не чаще 1
// сообщения за интервал, ≤ maxBatch деталей + total, since — начало окна,
// формат сообщения §5.
package npc

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNotificationBatchFirstSendImmediately(t *testing.T) {
	b := NewNotificationBatch(5*time.Second, 100)
	t0 := time.Now()
	b.Add(ArrivalEvent{AgentID: "a1", ObservedAt: t0})

	msg, ok := b.FlushDue(t0)
	require.True(t, ok, "первая отправка — сразу (окно с момента старта)")
	require.NotEmpty(t, msg)
}

// Дроссель: в пределах интервала сообщение не отправляется, накопленное
// уходит следующим окном.
func TestNotificationBatchThrottle(t *testing.T) {
	b := NewNotificationBatch(5*time.Second, 100)
	t0 := time.Now()

	b.Add(ArrivalEvent{AgentID: "a1", ObservedAt: t0})
	_, ok := b.FlushDue(t0.Add(time.Second))
	require.True(t, ok, "первая отправка после первого прибытия")

	// Новое прибытие в пределах интервала — ждёт следующего окна.
	b.Add(ArrivalEvent{AgentID: "a2", ObservedAt: t0.Add(2 * time.Second)})
	_, ok = b.FlushDue(t0.Add(3 * time.Second))
	require.False(t, ok, "дроссель: раньше интервала не шлём")

	// Интервал прошёл — сообщение с накопленным a2 уходит.
	msg, ok := b.FlushDue(t0.Add(6 * time.Second))
	require.True(t, ok)
	require.Contains(t, string(msg), `"a2"`)
}

// maxBatch деталей + избыток в total (спека §2.2.C: детали сверх 100
// не хранятся, только счётчик).
func TestNotificationBatchMaxBatchAndTotal(t *testing.T) {
	b := NewNotificationBatch(time.Second, 2)
	t0 := time.Now()
	for i := 0; i < 5; i++ {
		b.Add(ArrivalEvent{AgentID: "a" + string(rune('0'+i)), ObservedAt: t0})
	}

	msg, ok := b.FlushDue(t0.Add(time.Second))
	require.True(t, ok)

	var resp struct {
		Type     string `json:"type"`
		Since    string `json:"since"`
		Arrivals []struct {
			Agent struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"agent"`
			World struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"world"`
			ObservedAt string `json:"observed_at"`
		} `json:"arrivals"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(msg, &resp))
	require.Equal(t, "npc_arrivals_batch", resp.Type)
	require.Len(t, resp.Arrivals, 2, "до maxBatch деталей")
	require.Equal(t, "a0", resp.Arrivals[0].Agent.ID, "первыми уходят самые ранние прибытия")
	require.Equal(t, 3, resp.Total, "избыток — счётчиком")
}

// since — начало окна (первое прибытие окна).
func TestNotificationBatchSince(t *testing.T) {
	b := NewNotificationBatch(time.Second, 100)
	t0 := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	b.Add(ArrivalEvent{AgentID: "a1", ObservedAt: t0})
	b.Add(ArrivalEvent{AgentID: "a2", ObservedAt: t0.Add(time.Minute)})

	msg, ok := b.FlushDue(t0.Add(time.Second))
	require.True(t, ok)
	require.Contains(t, string(msg), `"since":"2026-09-14T12:00:00Z"`)
}

// Пустое окно — ничего не отправляется.
func TestNotificationBatchFlushEmpty(t *testing.T) {
	b := NewNotificationBatch(time.Second, 100)
	_, ok := b.FlushDue(time.Now())
	require.False(t, ok)
}

// После отправки окно пусто — повторный flush ничего не даёт до новых
// прибытий.
func TestNotificationBatchWindowReset(t *testing.T) {
	b := NewNotificationBatch(time.Second, 100)
	t0 := time.Now()

	b.Add(ArrivalEvent{AgentID: "a1", ObservedAt: t0})
	_, ok := b.FlushDue(t0)
	require.True(t, ok)

	_, ok = b.FlushDue(t0.Add(2 * time.Second))
	require.False(t, ok, "окно сброшено — без новых прибытий ничего не шлём")
}