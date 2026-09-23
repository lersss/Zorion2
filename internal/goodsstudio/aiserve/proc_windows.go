//go:build windows

// internal/goodsstudio/aiserve/proc_windows.go
// Процессные действия на Windows (спека §3.2/§3.4): спавн, гашение деревом,
// скан процессов, опознание владельца порта.
package aiserve

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// spawnHelper — запуск `cmd.exe /c npx -y opencode-ai serve --port <port>`
// отвязанным процессом: DETACHED_PROCESS (без окна, переживает рестарт игрового
// сервера) + CREATE_NEW_PROCESS_GROUP, stdout/stderr в logPath (append).
// Возвращает PID обёртки cmd.exe.
func spawnHelper(port int, logPath string) (int, error) {
	if dir := filepath.Dir(logPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return 0, err
		}
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
	}
	defer logFile.Close()
	cmd := exec.Command("cmd.exe", "/c", "npx", "-y", "opencode-ai", "serve", "--port", strconv.Itoa(port))
	// Помощник не должен наследовать OPENCODE_SERVER_PASSWORD/USERNAME из
	// окружения игрового сервера (спека §0: пароль считается незаданным) —
	// иначе он требует basic-auth, а клиент студии его не отправляет (401).
	cmd.Env = helperEnv(os.Environ())
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x00000008, // DETACHED_PROCESS
		HideWindow:    true,
	}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	return pid, nil
}

// killTree — `taskkill /PID <pid> /T /F` (спека §3.4): гасит процесс и потомков
// (node, держащий порт). Критерий успеха гашения — освобождение порта (§3.3);
// «процесс не найден» (уже убит предком по /T) ошибкой не считаем.
func killTree(pid int) error {
	out, err := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		low := strings.ToLower(msg)
		if strings.Contains(low, "not found") || strings.Contains(low, "не найден") {
			return nil
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// scanProcesses — скан процессов (метод Б, спека §3.4): строки
// `PID|Name|CommandLine` из Get-CimInstance Win32_Process.
func scanProcesses() []procInfo {
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		"Get-CimInstance Win32_Process | ForEach-Object { $_.ProcessId.ToString() + '|' + $_.Name + '|' + $_.CommandLine }").Output()
	if err != nil {
		return nil
	}
	return parseProcLines(string(out))
}

// portOwnerInfo — владелец порта (метод А, спека §3.4): PID LISTENING-строки
// `netstat -ano -p tcp`, затем имя и командная строка процесса.
func portOwnerInfo(port int) (procInfo, bool) {
	out, err := exec.Command("netstat", "-ano", "-p", "tcp").Output()
	if err != nil {
		return procInfo{}, false
	}
	pid, ok := parseNetstatPortOwner(string(out), port)
	if !ok {
		return procInfo{}, false
	}
	q, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		fmt.Sprintf("(Get-CimInstance Win32_Process -Filter \"ProcessId=%d\") | ForEach-Object { $_.Name + '|' + $_.CommandLine }", pid)).Output()
	if err != nil {
		return procInfo{PID: pid}, false
	}
	p := parseOneProcLine(string(q))
	p.PID = pid
	if p.Name == "" && p.Cmd == "" {
		return p, false
	}
	return p, true
}
