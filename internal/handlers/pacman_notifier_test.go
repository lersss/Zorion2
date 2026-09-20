// internal/handlers/pacman_notifier_test.go
// Тесты дросселя нотификатора пакмана (спека 2026-09-20 §5.2): позиция ≤10 Гц,
// поедание ≤1 событие/батч ≤200 миров, start/end по одному, персональные
// уведомления. Паттерн notification_batch_test.go: дроссель проверяется на
// уровне логики (flush), без реальных WS-клиентов (PITFALLS.md, gorilla/websocket).
package handlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zorion/internal/mapcache"
)

// Дроссель позиции: ≤10 Гц (спека §5.2). Первая позиция уходит сразу,
// повторная в пределах интервала — нет (posPending остаётся), после
// интервала — уходит.
func TestPacmanNotifierPositionThrottle(t *testing.T) {
	n := NewPacmanNotifier(NewWebSocketHub())
	n.StartEvent(100, 1700)

	n.Position(1, 2)
	n.flush()
	require.False(t, n.posPending, "первая позиция отправлена")

	n.Position(3, 4)
	n.flush()
	require.True(t, n.posPending, "в пределах 100 мс позиция не отправляется (дроссель)")

	time.Sleep(pacmanPositionIntervalMs*time.Millisecond + 10*time.Millisecond)
	n.flush()
	require.False(t, n.posPending, "после интервала позиция отправлена")
}

// Поедание: ≤1 событие/батч, ≤200 миров в деталях (спека §5.2); счётчик
// eaten_total несёт остаток.
func TestPacmanNotifierEatenLimit(t *testing.T) {
	n := NewPacmanNotifier(NewWebSocketHub())
	n.StartEvent(1000, 1700)

	batch := make([]mapcache.World, 250)
	for i := range batch {
		batch[i] = mapcache.World{ID: "w" + string(rune('a'+i%26)) + string(rune('0'+i/26))}
	}
	n.Eaten(batch, 250, 1000, 5, 6)
	require.Len(t, n.eatenWorlds, maxWorldsPerEatenEvent, "детали обрезаны до 200")
	require.Equal(t, 250, n.eatenTotal, "счётчик несёт остаток")
	require.True(t, n.eatenPending)
	n.flush()
	require.False(t, n.eatenPending, "поедание отправлено (1 событие на батч)")
}

// start/end — по одному; StartEvent сбрасывает состояние прошлого прогона.
func TestPacmanNotifierStartEndOnce(t *testing.T) {
	n := NewPacmanNotifier(NewWebSocketHub())

	n.StartEvent(100, 1700)
	require.True(t, n.startPending)
	n.flush()
	require.False(t, n.startPending, "start отправлен один раз")

	n.End("done", 100, 100, 5000)
	require.True(t, n.endPending)
	n.flush()
	require.False(t, n.endPending, "end отправлен один раз")

	// Новый прогон: StartEvent сбрасывает состояние.
	n.StartEvent(50, 1700)
	require.True(t, n.startPending)
	require.False(t, n.endPending, "новый прогон — end прошлого не висит")
	require.Equal(t, 0, n.eatenTotal, "счётчик сброшен")
}

// Персональные уведомления: очередь уходит SendToUser (нет клиента — no-op).
func TestPacmanNotifierNotices(t *testing.T) {
	n := NewPacmanNotifier(NewWebSocketHub())
	n.Notice("user-1", "Ваш мир съеден пакманом")
	n.Notice("user-2", "Полёт прерван — цель съедена")
	require.Len(t, n.notices, 2)
	n.flush()
	require.Empty(t, n.notices, "уведомления отправлены")
}