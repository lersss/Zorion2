// internal/goodsstudio/aiserve/manager_test.go
// T1–T6, T9, T11: разбор URL/гейт, таблица состояний, single-flight, безопасное
// гашение, таймаут готовности, managed=false, внешний starting, гонки кэша
// (спека 2026-09-24 §2/§3). Реальные процессы не запускаются — пробы и действия
// инъектируются.
package aiserve

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newTestManager — менеджер с управляемыми пробами (всё выключено по умолчанию).
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	m := NewManager("http://127.0.0.1:3456", 5*time.Second, "logs/test.log")
	m.windows = true
	m.npxFound = true
	m.managed = true
	m.reason = ""
	m.verdictTTL = 0
	m.portOpen = func(host string, port int) bool { return false }
	m.health = func(baseURL string) (string, bool) { return "", false }
	m.waitReady = func(host string, port int, timeout time.Duration) bool { return false }
	m.waitPortFree = func(host string, port int, timeout time.Duration) bool { return true }
	m.scanProcs = func() []procInfo { return nil }
	m.portOwner = func(port int) (procInfo, bool) { return procInfo{}, false }
	m.kill = func(pid int) error { return nil }
	m.spawn = func(port int, logPath string) (int, error) { return 0, nil }
	return m
}

// T1: разбор OPENCODE_URL и гейт управления.
func TestParseTargetAndManaged(t *testing.T) {
	tg, err := ParseTarget("http://127.0.0.1:3456")
	require.NoError(t, err)
	require.Equal(t, "http", tg.Scheme)
	require.Equal(t, "127.0.0.1", tg.Host)
	require.Equal(t, 3456, tg.Port)

	tg2, err := ParseTarget("https://localhost:9999")
	require.NoError(t, err)
	require.Equal(t, 9999, tg2.Port)

	_, err = ParseTarget("http://127.0.0.1")
	require.Error(t, err, "без явного порта")
	_, err = ParseTarget("ftp://127.0.0.1:3456")
	require.Error(t, err, "схема не http(s)")

	// не-loopback → managed=false
	tg3, err := ParseTarget("http://10.0.0.5:3456")
	require.NoError(t, err)
	ok, reason := tg3.Managed(true, true)
	require.False(t, ok)
	require.NotEmpty(t, reason)

	ok, reason = tg.Managed(true, true)
	require.True(t, ok)
	require.Empty(t, reason)

	ok, _ = tg.Managed(true, false)
	require.False(t, ok, "нет npx → не управляем")
	ok, _ = tg.Managed(false, true)
	require.False(t, ok, "не Windows → не управляем")
}

// T2: таблица состояний §2 на фейковой пробе.
func TestStateTable(t *testing.T) {
	// stopped: порт свободен, процесса нет.
	require.Equal(t, "stopped", newTestManager(t).Status().State)

	// starting: порт свободен, но есть живой процесс помощника (метод Б).
	m := newTestManager(t)
	m.scanProcs = func() []procInfo {
		return []procInfo{{PID: 10, Name: "node", Cmd: "node opencode serve --port 3456"}}
	}
	require.Equal(t, "starting", m.Status().State)

	// running: health ответил 2xx (version из health).
	m = newTestManager(t)
	m.portOpen = func(string, int) bool { return true }
	m.health = func(string) (string, bool) { return "1.1.20", true }
	st := m.Status()
	require.Equal(t, "running", st.State)
	require.Equal(t, "1.1.20", st.Version)

	// running по подтверждённому владельцу порта при неответившей health — НЕ starting.
	m = newTestManager(t)
	m.portOpen = func(string, int) bool { return true }
	m.health = func(string) (string, bool) { return "", false }
	m.portOwner = func(int) (procInfo, bool) {
		return procInfo{PID: 42, Name: "node", Cmd: "node opencode serve --port 3456"}, true
	}
	st = m.Status()
	require.Equal(t, "running", st.State)
	require.Empty(t, st.Version)

	// foreign: порт слушается чужим процессом.
	m = newTestManager(t)
	m.portOpen = func(string, int) bool { return true }
	m.health = func(string) (string, bool) { return "", false }
	m.portOwner = func(int) (procInfo, bool) { return procInfo{PID: 7, Name: "node", Cmd: "node other.js"}, true }
	require.Equal(t, "foreign", m.Status().State)

	// вне Windows процессной ветки нет: живой процесс не делает starting.
	m = newTestManager(t)
	m.windows = false
	m.scanProcs = func() []procInfo {
		return []procInfo{{PID: 10, Name: "node", Cmd: "node opencode serve --port 3456"}}
	}
	require.Equal(t, "stopped", m.Status().State)

	// активный джоб запуска → starting безусловно (перекрывает running).
	m = newTestManager(t)
	m.portOpen = func(string, int) bool { return true }
	m.health = func(string) (string, bool) { return "1.1.20", true }
	m.jobStarting = true
	require.Equal(t, "starting", m.Status().State)
}

// T3: single-flight — два параллельных start дают ровно один спавн.
func TestStartSingleFlight(t *testing.T) {
	m := newTestManager(t)
	release := make(chan struct{})
	m.waitReady = func(host string, port int, timeout time.Duration) bool { <-release; return false }
	var spawns int32
	m.spawn = func(port int, logPath string) (int, error) { atomic.AddInt32(&spawns, 1); return 123, nil }

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); m.Start() }()
	}
	wg.Wait()
	close(release)
	require.Eventually(t, func() bool {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return !m.jobStarting
	}, time.Second, 5*time.Millisecond)
	require.Equal(t, int32(1), atomic.LoadInt32(&spawns))
}

// T4: stop не гасит чужое — владелец без opencode → ошибка, kill не вызван.
func TestStopForeignNotKilled(t *testing.T) {
	m := newTestManager(t)
	m.portOpen = func(string, int) bool { return true }
	m.health = func(string) (string, bool) { return "", false }
	m.portOwner = func(int) (procInfo, bool) { return procInfo{PID: 7, Name: "node", Cmd: "node other.js"}, true }
	killed := false
	m.kill = func(pid int) error { killed = true; return nil }

	res := m.Stop()
	require.Equal(t, 409, res.Code)
	require.False(t, killed)
	require.Contains(t, res.Error, "не помощник")
}

// T5: таймаут готовности → состояние по факту пробы + last_error.
func TestStartTimeoutLastError(t *testing.T) {
	m := newTestManager(t)
	m.startTimeout = 5 * time.Second
	m.waitReady = func(host string, port int, timeout time.Duration) bool { return false }

	res := m.Start()
	require.Equal(t, 202, res.Code)
	require.True(t, res.Started)
	require.Eventually(t, func() bool {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return !m.jobStarting
	}, time.Second, 5*time.Millisecond)

	st := m.Status()
	require.Equal(t, "stopped", st.State)
	require.Contains(t, st.LastError, "не дождались готовности за 5 с")
	require.Contains(t, st.Detail, "не дождались готовности")
}

// T6: managed=false → start/stop 409.
func TestUnmanagedStartStop409(t *testing.T) {
	m := newTestManager(t)
	m.managed = false
	m.reason = "npx не найден в PATH (установите Node.js)"

	rs := m.Start()
	require.Equal(t, 409, rs.Code)
	require.Contains(t, rs.Error, "управление недоступно")
	require.Contains(t, rs.Error, "npx не найден")

	rp := m.Stop()
	require.Equal(t, 409, rp.Code)
	require.Contains(t, rp.Error, "управление недоступно")
}

// T9: stop внешнего starting при свободном порте → кандидат найден методом Б и
// погашен, 200.
func TestStopExternalStarting(t *testing.T) {
	m := newTestManager(t)
	m.portOpen = func(string, int) bool { return false }
	m.scanProcs = func() []procInfo {
		return []procInfo{{PID: 55, Name: "node", Cmd: "node opencode serve --port 3456"}}
	}
	killedPID := 0
	m.kill = func(pid int) error { killedPID = pid; return nil }

	res := m.Stop()
	require.Equal(t, 200, res.Code)
	require.True(t, res.Stopped)
	require.Equal(t, 55, killedPID)
}

// T9b: многокандидатный stop — метод Б даёт несколько процессов одного дерева;
// первый taskkill /T убивает дерево, последующие по мёртвым PID ошибаются, но
// критерий успеха — освобождение порта (спека §3.3), а не смерть каждой обёртки.
func TestStopMultipleCandidatesPortFreed(t *testing.T) {
	m := newTestManager(t)
	m.portOpen = func(string, int) bool { return false }
	m.scanProcs = func() []procInfo {
		return []procInfo{
			{PID: 100, Name: "cmd.exe", Cmd: "cmd /c npx -y opencode-ai serve --port 3456"},
			{PID: 200, Name: "node", Cmd: "node opencode serve --port 3456"},
			{PID: 300, Name: "node", Cmd: "node C:\\x\\opencode serve --port 3456"},
		}
	}
	var killed []int
	m.kill = func(pid int) error {
		killed = append(killed, pid)
		if pid != 100 {
			return errors.New("process not found")
		}
		return nil
	}
	m.waitPortFree = func(string, int, time.Duration) bool { return true }

	res := m.Stop()
	require.Equal(t, 200, res.Code)
	require.True(t, res.Stopped)
	require.Equal(t, []int{100, 200, 300}, killed, "гасим всех кандидатов")
}

// T9c: kill не удался и порт остался занят помощником → 500.
func TestStopKillFailsPortBusy500(t *testing.T) {
	m := newTestManager(t)
	m.portOpen = func(string, int) bool { return true }
	m.health = func(string) (string, bool) { return "", false }
	m.portOwner = func(int) (procInfo, bool) {
		return procInfo{PID: 1, Name: "node", Cmd: "node opencode serve --port 3456"}, true
	}
	m.waitPortFree = func(string, int, time.Duration) bool { return false }
	m.kill = func(pid int) error { return errors.New("access denied") }

	res := m.Stop()
	require.Equal(t, 500, res.Code)
	require.Contains(t, res.Error, "не удалось остановить")
	require.Contains(t, res.Error, "access denied")
}

// T2b: foreign без опознанного владельца (не Windows) — текст без «PID 0».
func TestForeignDetailWithoutOwner(t *testing.T) {
	m := newTestManager(t)
	m.windows = false
	m.portOpen = func(string, int) bool { return true }
	m.health = func(string) (string, bool) { return "", false }

	st := m.Status()
	require.Equal(t, "foreign", st.State)
	require.NotContains(t, st.Detail, "PID 0")
	require.Contains(t, st.Detail, "занят другим процессом")
}

// T11: параллельные Status + Stop под -race — кэш вердикта не гоняется.
func TestConcurrentStatusStop(t *testing.T) {
	m := newTestManager(t)
	m.verdictTTL = 20 * time.Millisecond
	m.portOpen = func(string, int) bool { return true }
	m.health = func(string) (string, bool) { return "", false }
	m.portOwner = func(int) (procInfo, bool) {
		return procInfo{PID: 1, Name: "node", Cmd: "node opencode serve --port 3456"}, true
	}
	m.waitPortFree = func(string, int, time.Duration) bool { return true }

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				m.Status()
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				m.Stop()
			}
		}()
	}
	wg.Wait()
}
