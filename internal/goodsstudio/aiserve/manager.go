// internal/goodsstudio/aiserve/manager.go
// Manager — состояние и управление локальным ИИ-помощником (спека
// 2026-09-24-студия-управление-локальным-ии §2/§3). Пробы (порт/health/скан
// процессов) инъектируемы — тесты не запускают реальных процессов.
package aiserve

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// DefaultLogPath — общий лог помощника (start-all.cmd и кнопка «запустить»).
const DefaultLogPath = "logs/opencode_serve.log"

// Status — общая форма ответа (спека §3, StatusView).
type Status struct {
	State         string `json:"state"`
	Managed       bool   `json:"managed"`
	Reason        string `json:"reason"`
	Detail        string `json:"detail"`
	URL           string `json:"url"`
	Port          int    `json:"port"`
	Version       string `json:"version"`
	StartingSince string `json:"starting_since"`
	LastError     string `json:"last_error"`
	Log           string `json:"log"`
}

// StartResult — результат POST /studio/api/ai/start (спека §3.2).
type StartResult struct {
	Started bool
	Status  Status
	Code    int
	Error   string
}

// StopResult — результат POST /studio/api/ai/stop (спека §3.3).
type StopResult struct {
	Stopped bool
	Status  Status
	Code    int
	Error   string
}

// Unavailable — состояние при отсутствии менеджера (тесты, выключено).
func Unavailable() Status {
	return Status{State: "stopped", Managed: false, Detail: "ИИ: недоступен"}
}

// procVerdict — вердикт по процессам (кэшируется 5 с): кандидаты методом Б и
// владелец порта методом А (спека §2/§3.4).
type procVerdict struct {
	helpers     []procInfo
	ownerPID    int
	ownerName   string
	ownerKnown  bool
	ownerHelper bool
}

// Manager — управление помощником. Все инъектируемые поля — пробы и действия
// (дефолты задаёт NewManager); in-memory состояние джоба и кэш вердикта — под
// sync.RWMutex (AGENTS.md §0).
type Manager struct {
	rawURL       string
	url          string
	host         string
	port         int
	managed      bool
	reason       string
	startTimeout time.Duration
	logPath      string
	windows      bool
	npxFound     bool

	now          func() time.Time
	portOpen     func(host string, port int) bool
	health       func(baseURL string) (string, bool)
	waitReady    func(host string, port int, timeout time.Duration) bool
	waitPortFree func(host string, port int, timeout time.Duration) bool
	scanProcs    func() []procInfo
	portOwner    func(port int) (procInfo, bool)
	kill         func(pid int) error
	spawn        func(port int, logPath string) (int, error)

	verdictTTL time.Duration

	mu            sync.RWMutex
	jobStarting   bool
	startingSince time.Time
	childPID      int
	lastError     string
	verdict       procVerdict
	verdictSet    bool
	verdictAt     time.Time
}

// NewManager — менеджер для OPENCODE_URL. Неразобранный URL — managed=false с
// причиной (порт/состояние по факту пробы недоступны).
func NewManager(rawURL string, startTimeout time.Duration, logPath string) *Manager {
	if logPath == "" {
		logPath = DefaultLogPath
	}
	m := &Manager{
		rawURL:       rawURL,
		startTimeout: startTimeout,
		logPath:      logPath,
		windows:      runtime.GOOS == "windows",
		npxFound:     lookNpx(),
		now:          time.Now,
		portOpen:     dialPort,
		health:       httpHealth,
		waitReady:    pollReady,
		waitPortFree: pollPortFree,
		scanProcs:    scanProcesses,
		portOwner:    portOwnerInfo,
		kill:         killTree,
		spawn:        spawnHelper,
		verdictTTL:   5 * time.Second,
	}
	t, err := ParseTarget(rawURL)
	if err != nil {
		m.reason = err.Error()
		m.url = rawURL
		m.host = "127.0.0.1"
		return m
	}
	m.url = t.BaseURL()
	m.host = t.Host
	m.port = t.Port
	m.managed, m.reason = t.Managed(m.windows, m.npxFound)
	return m
}

// probe — состояние по факту доступных проб (спека §2): state, version,
// вердикт по процессам, слушается ли порт.
func (m *Manager) probe() (string, string, procVerdict, bool) {
	m.mu.RLock()
	job := m.jobStarting
	m.mu.RUnlock()

	open := m.portOpen(m.host, m.port)
	version, healthy := "", false
	if open {
		version, healthy = m.health(m.url)
	}
	var v procVerdict
	switch {
	case job:
		return "starting", "", v, open
	case open && healthy:
		return "running", version, v, open
	case open:
		v = m.processVerdict(true)
		if v.ownerHelper {
			return "running", "", v, open
		}
		return "foreign", "", v, open
	default:
		if m.windows {
			v = m.processVerdict(false)
			if len(v.helpers) > 0 {
				return "starting", "", v, open
			}
		}
		return "stopped", "", v, open
	}
}

// Status — текущее состояние помощника (спека §2/§3).
func (m *Manager) Status() Status {
	state, version, v, _ := m.probe()
	m.mu.RLock()
	since := m.startingSince
	lastErr := m.lastError
	m.mu.RUnlock()
	st := Status{
		State:     state,
		Managed:   m.managed,
		Reason:    m.reason,
		URL:       m.url,
		Port:      m.port,
		Version:   version,
		LastError: lastErr,
		Log:       m.logPath,
	}
	if state == "starting" && !since.IsZero() {
		st.StartingSince = since.UTC().Format(time.RFC3339)
	}
	st.Detail = m.detail(st, since, v)
	return st
}

// detail — человеческий текст для UI (спека §4); фронт его не сочиняет.
func (m *Manager) detail(st Status, since time.Time, v procVerdict) string {
	if st.LastError != "" {
		return "ИИ: " + st.LastError
	}
	switch st.State {
	case "stopped":
		if st.Managed {
			return "ИИ: не запущен"
		}
		return "ИИ: недоступен"
	case "starting":
		if st.Managed {
			base := "ИИ: запускается… (первый запуск качает пакет, до 5 мин)"
			if !since.IsZero() {
				secs := int(m.now().Sub(since).Seconds())
				if secs < 0 {
					secs = 0
				}
				base += " · " + strconv.Itoa(secs) + " с"
			}
			return base
		}
		return "ИИ: запускается… (управление недоступно)"
	case "running":
		if st.Managed {
			if st.Version != "" {
				return "ИИ: работает (v" + st.Version + ")"
			}
			return "ИИ: работает"
		}
		return "ИИ: работает (управление недоступно)"
	case "foreign":
		// Владелец порта опознан не всегда (процессные пробы — только Windows,
		// спека §6): без PID/имени текст без них.
		if v.ownerKnown {
			return fmt.Sprintf("ИИ: порт %d занят другим процессом (PID %d, %s) — освободите порт вручную", m.port, v.ownerPID, v.ownerName)
		}
		return fmt.Sprintf("ИИ: порт %d занят другим процессом — освободите порт вручную", m.port)
	}
	return "ИИ: —"
}

// processVerdict — вердикт по процессам под кэшем 5 с (спека §2). Проба
// выполняется вне замка, запись кэша — под Lock (гонки кэша исключены).
func (m *Manager) processVerdict(open bool) procVerdict {
	m.mu.RLock()
	if m.verdictTTL > 0 && m.verdictSet && m.now().Sub(m.verdictAt) < m.verdictTTL {
		v := m.verdict
		m.mu.RUnlock()
		return v
	}
	m.mu.RUnlock()

	v := procVerdict{}
	if m.windows {
		for _, p := range m.scanProcs() {
			if isHelperCommandLine(p.Cmd, m.port, true) {
				v.helpers = append(v.helpers, p)
			}
		}
		if open {
			if o, ok := m.portOwner(m.port); ok {
				v.ownerPID, v.ownerName, v.ownerKnown = o.PID, o.Name, true
				v.ownerHelper = isHelperCommandLine(o.Cmd, m.port, false)
			}
		}
	}
	m.mu.Lock()
	m.verdict = v
	m.verdictSet = true
	m.verdictAt = m.now()
	m.mu.Unlock()
	return v
}

// invalidateVerdict — сброс кэша вердикта (после гашения/действия).
func (m *Manager) invalidateVerdict() {
	m.mu.Lock()
	m.verdictSet = false
	m.mu.Unlock()
}

// clearLastError — гашение last_error успешным действием (спека §3).
func (m *Manager) clearLastError() {
	m.mu.Lock()
	m.lastError = ""
	m.mu.Unlock()
}

// Start — POST /studio/api/ai/start (спека §3.2). Single-flight: повторный
// запуск при starting/running процесс не плодит.
func (m *Manager) Start() StartResult {
	if !m.managed {
		return StartResult{Code: 409, Error: "управление недоступно: " + m.reason, Status: m.Status()}
	}
	st := m.Status()
	switch st.State {
	case "running":
		st.Detail = "уже запущен"
		return StartResult{Code: 200, Started: false, Status: st}
	case "starting":
		return StartResult{Code: 202, Started: false, Status: st}
	case "foreign":
		v := m.processVerdict(true)
		return StartResult{
			Code:   409,
			Error:  fmt.Sprintf("порт %d занят другим процессом (PID %d, %s)", m.port, v.ownerPID, v.ownerName),
			Status: st,
		}
	}
	m.mu.Lock()
	if m.jobStarting {
		m.mu.Unlock()
		return StartResult{Code: 202, Started: false, Status: m.Status()}
	}
	m.jobStarting = true
	m.startingSince = m.now()
	m.lastError = ""
	pid, err := m.spawn(m.port, m.logPath)
	if err != nil {
		m.jobStarting = false
		m.startingSince = time.Time{}
		m.mu.Unlock()
		return StartResult{Code: 500, Error: "не удалось запустить: " + err.Error(), Status: m.Status()}
	}
	m.childPID = pid
	m.mu.Unlock()
	go m.waitReadyJob()
	return StartResult{Code: 202, Started: true, Status: m.Status()}
}

// waitReadyJob — фоновое ожидание готовности после спавна (спека §3.2):
// таймаут → last_error, процесс не гасим; состояние — по факту пробы.
func (m *Manager) waitReadyJob() {
	ok := m.waitReady(m.host, m.port, m.startTimeout)
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.jobStarting {
		return // джоб отменён (stop)
	}
	m.jobStarting = false
	m.startingSince = time.Time{}
	m.childPID = 0
	if !ok {
		m.lastError = fmt.Sprintf("не дождались готовности за %d с; смотрите %s", int(m.startTimeout.Seconds()), m.logPath)
	}
}

// Stop — POST /studio/api/ai/stop (спека §3.3): отмена нашего джоба/гашение
// своего дитя, иначе — гашение только подтверждённых кандидатов.
func (m *Manager) Stop() StopResult {
	if !m.managed {
		return StopResult{Code: 409, Error: "управление недоступно: " + m.reason, Status: m.Status()}
	}
	m.mu.Lock()
	job := m.jobStarting
	childPID := m.childPID
	if job {
		m.jobStarting = false
		m.startingSince = time.Time{}
		m.childPID = 0
	}
	m.mu.Unlock()
	if job {
		if childPID > 0 {
			if err := m.kill(childPID); err != nil {
				return StopResult{Code: 500, Error: "не удалось остановить: " + err.Error(), Status: m.Status()}
			}
		}
		m.clearLastError()
		m.invalidateVerdict()
		return StopResult{Code: 200, Stopped: true, Status: m.Status()}
	}

	open := m.portOpen(m.host, m.port)
	v := m.processVerdict(open)
	candidates := collectCandidates(v, open)
	if len(candidates) == 0 {
		if !open {
			st := m.Status()
			st.Detail = "уже остановлен"
			return StopResult{Code: 200, Stopped: false, Status: st}
		}
		if v.ownerKnown {
			return StopResult{
				Code:   409,
				Error:  fmt.Sprintf("порт %d занят процессом %s (PID %d) — это не помощник, не гашу", m.port, v.ownerName, v.ownerPID),
				Status: m.Status(),
			}
		}
		return StopResult{Code: 409, Error: "не удалось опознать процесс помощника — не гашу", Status: m.Status()}
	}
	// Критерий успеха гашения — освобождение порта (спека §3.3), а не смерть
	// каждой обёртки: метод Б даёт несколько кандидатов одного дерева, и первый
	// `taskkill /T /F` убивает дерево целиком — последующие вызовы по мёртвым PID
	// ошибку не делают фатальной. Ошибку отдельного kill запоминаем и сообщаем
	// только если порт так и не освободился.
	var killErr error
	for _, c := range candidates {
		if err := m.kill(c.PID); err != nil {
			killErr = err
		}
	}
	m.invalidateVerdict()
	if !m.waitPortFree(m.host, m.port, 10*time.Second) {
		if m.portOpen(m.host, m.port) {
			v2 := m.processVerdict(true)
			if v2.ownerHelper {
				if killErr != nil {
					return StopResult{Code: 500, Error: "не удалось остановить: " + killErr.Error(), Status: m.Status()}
				}
				return StopResult{Code: 500, Error: "не удалось остановить: процесс помощника не завершился", Status: m.Status()}
			}
			return StopResult{Code: 409, Error: fmt.Sprintf("порт %d остался занят чужим процессом — не трогаю", m.port), Status: m.Status()}
		}
	}
	m.clearLastError()
	m.invalidateVerdict()
	return StopResult{Code: 200, Stopped: true, Status: m.Status()}
}

// collectCandidates — кандидаты на гашение (спека §3.4): процессы помощника
// на нашем порту (метод Б) + владелец порта, подтверждённый помощником (метод А).
func collectCandidates(v procVerdict, open bool) []procInfo {
	out := append([]procInfo{}, v.helpers...)
	if open && v.ownerHelper {
		found := false
		for _, c := range out {
			if c.PID == v.ownerPID {
				found = true
				break
			}
		}
		if !found {
			out = append(out, procInfo{PID: v.ownerPID, Name: v.ownerName})
		}
	}
	return out
}

// --- пробы и действия: дефолты (тесты подменяют) ---

func dialPort(host string, port int) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func pollReady(host string, port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if dialPort(host, port) {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(time.Second)
	}
}

func pollPortFree(host string, port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if !dialPort(host, port) {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func httpHealth(baseURL string) (string, bool) {
	cl := &http.Client{Timeout: 2 * time.Second}
	if v, ok := healthGet(cl, baseURL+"/global/health"); ok {
		return v, true
	}
	if v, ok := healthGet(cl, baseURL+"/config"); ok {
		return v, true
	}
	return "", false
}

func healthGet(cl *http.Client, url string) (string, bool) {
	resp, err := cl.Get(url)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", false
	}
	var m map[string]interface{}
	if json.Unmarshal(body, &m) != nil {
		return "", false
	}
	v, _ := m["version"].(string)
	return v, true
}

func lookNpx() bool {
	_, err := exec.LookPath("npx")
	return err == nil
}
