// internal/goodsstudio/aiserve/parse.go
// Разбор OPENCODE_URL и опознание процесса помощника (спека
// 2026-09-24-студия-управление-локальным-ии §2/§3.4). Чистые функции —
// без вызова процессов, тестируются на канонических образцах (T8/T12).
package aiserve

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// procInfo — процесс-кандидат: PID, имя и командная строка (имя — для текста
// ошибки, спека §3.4).
type procInfo struct {
	PID  int
	Name string
	Cmd  string
}

var (
	errInvalidURL = errors.New("OPENCODE_URL не разобран")
	errScheme     = errors.New("OPENCODE_URL: схема должна быть http/https")
	errNoPort     = errors.New("OPENCODE_URL без явного порта")
)

// Target — разобранный OPENCODE_URL (спека §2 п.1): схема/host/порт.
type Target struct {
	Scheme string
	Host   string
	Port   int
}

// BaseURL — канонический адрес пробы (без хвостового слэша).
func (t Target) BaseURL() string {
	return t.Scheme + "://" + net.JoinHostPort(t.Host, strconv.Itoa(t.Port))
}

// ParseTarget — разбор OPENCODE_URL: схема http/https и явный порт обязательны
// (спека §2 п.1). Иначе — ошибка с человеческой причиной.
func ParseTarget(raw string) (Target, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return Target{}, errInvalidURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return Target{}, errScheme
	}
	host := u.Hostname()
	if host == "" {
		return Target{}, errInvalidURL
	}
	portStr := u.Port()
	if portStr == "" {
		return Target{}, errNoPort
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return Target{}, errNoPort
	}
	return Target{Scheme: u.Scheme, Host: host, Port: port}, nil
}

// isLoopbackHost — host указывает на локальную машину (127.0.0.0/8, ::1,
// localhost).
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Managed — гейт управления (спека §2 п.2): loopback + Windows + npx.
func (t Target) Managed(windows, npxFound bool) (bool, string) {
	if !isLoopbackHost(t.Host) {
		return false, "управление помощником — только на локальной машине"
	}
	if !windows {
		return false, "управление помощником доступно только на Windows"
	}
	if !npxFound {
		return false, "npx не найден в PATH (установите Node.js)"
	}
	return true, ""
}

// isHelperCommandLine — подтверждение процесса помощника по командной строке
// (спека §3.4): регистронезависимо содержит `opencode` И `serve`, а при
// requirePort — ещё и порт как отдельный токен (`--port <port>` / `:<port>`).
// Без токена порта — другой экземпляр opencode: не наш (T12).
func isHelperCommandLine(cmd string, port int, requirePort bool) bool {
	c := strings.ToLower(cmd)
	if !strings.Contains(c, "opencode") || !strings.Contains(c, "serve") {
		return false
	}
	if !requirePort {
		return true
	}
	return hasPortToken(c, port)
}

// hasPortToken — есть ли порт отдельным токеном: `:<port>`, `--port <port>`,
// `--port=<port>` (за токеном не идёт цифра, иначе :34560 != :3456).
func hasPortToken(cmd string, port int) bool {
	p := strconv.Itoa(port)
	for _, sep := range []string{":" + p, "--port " + p, "--port=" + p} {
		idx := 0
		for {
			i := strings.Index(cmd[idx:], sep)
			if i < 0 {
				break
			}
			end := idx + i + len(sep)
			if end >= len(cmd) || !isASCIIDigit(cmd[end]) {
				return true
			}
			idx = end
		}
	}
	return false
}

func isASCIIDigit(b byte) bool { return b >= '0' && b <= '9' }

// parseNetstatPortOwner — PID владельца порта из вывода `netstat -ano -p tcp`
// (спека §3.4, метод А): строка LISTENING с `:<port>` в локальном адресе,
// PID — последняя колонка.
func parseNetstatPortOwner(out string, port int) (int, bool) {
	suffix := ":" + strconv.Itoa(port)
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(strings.TrimSpace(line))
		if len(f) < 4 {
			continue
		}
		if strings.ToUpper(f[len(f)-2]) != "LISTENING" {
			continue
		}
		if !strings.HasSuffix(f[1], suffix) {
			continue
		}
		pid, err := strconv.Atoi(f[len(f)-1])
		if err != nil {
			continue
		}
		return pid, true
	}
	return 0, false
}

// parseProcLines — разбор вывода скана процессов: строки `PID|Name|CommandLine`
// (спека §3.4, метод Б).
func parseProcLines(out string) []procInfo {
	var res []procInfo
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 2 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			continue
		}
		p := procInfo{PID: pid, Name: strings.TrimSpace(parts[1])}
		if len(parts) == 3 {
			p.Cmd = strings.TrimSpace(parts[2])
		}
		res = append(res, p)
	}
	return res
}

// parseOneProcLine — разбор одной строки `Name|CommandLine` (метод А: ответ
// Get-CimInstance по конкретному PID).
func parseOneProcLine(out string) procInfo {
	line := strings.TrimSpace(out)
	if line == "" {
		return procInfo{}
	}
	parts := strings.SplitN(line, "|", 2)
	p := procInfo{Name: strings.TrimSpace(parts[0])}
	if len(parts) == 2 {
		p.Cmd = strings.TrimSpace(parts[1])
	}
	return p
}
