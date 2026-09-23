// internal/goodsstudio/aiserve/parse_test.go
// T8/T12: разбор netstat-строки и командной строки, опознание помощника с
// токеном порта (спека 2026-09-24 §3.4).
package aiserve

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseNetstatPortOwner — PID LISTENING-строки с нашим портом (метод А).
func TestParseNetstatPortOwner(t *testing.T) {
	out := "  TCP    127.0.0.1:5432    0.0.0.0:0    LISTENING    999\r\n" +
		"  TCP    127.0.0.1:3456    0.0.0.0:0    LISTENING    12345\r\n" +
		"  TCP    127.0.0.1:3457    0.0.0.0:0    LISTENING    222\r\n"
	pid, ok := parseNetstatPortOwner(out, 3456)
	require.True(t, ok)
	require.Equal(t, 12345, pid)

	// IPv6 loopback.
	pid, ok = parseNetstatPortOwner("  TCP    [::1]:3456    [::]:0    LISTENING    777\r\n", 3456)
	require.True(t, ok)
	require.Equal(t, 777, pid)

	// порт не слушается.
	_, ok = parseNetstatPortOwner(out, 8080)
	require.False(t, ok)

	// :34560 не должен матчить :3456.
	_, ok = parseNetstatPortOwner("  TCP    127.0.0.1:34560    0.0.0.0:0    LISTENING    5\r\n", 3456)
	require.False(t, ok)
}

// TestIsHelperCommandLine — канонические образцы командных строк (T8/T12).
func TestIsHelperCommandLine(t *testing.T) {
	// метод Б: opencode + serve + наш порт отдельным токеном.
	require.True(t, isHelperCommandLine(`node C:\Users\admin\AppData\opencode serve --port 3456`, 3456, true))
	require.True(t, isHelperCommandLine(`node opencode serve --port=3456`, 3456, true))
	require.True(t, isHelperCommandLine(`node opencode serve :3456`, 3456, true))
	require.True(t, isHelperCommandLine(`node opencode-ai serve --port 3456`, 3456, true))
	// посторонний node — не помощник.
	require.False(t, isHelperCommandLine(`node C:\projects\other\server.js`, 3456, true))
	// другой порт — не наш экземпляр (T12).
	require.False(t, isHelperCommandLine(`node opencode serve --port 4000`, 3456, true))
	require.False(t, isHelperCommandLine(`node opencode serve :4000`, 3456, true))
	// :34560 — не токен порта 3456.
	require.False(t, isHelperCommandLine(`node opencode serve --port 34560`, 3456, true))
	// метод А: порт уже наш — достаточно opencode + serve.
	require.True(t, isHelperCommandLine(`node opencode serve --port 4000`, 3456, false))
	require.False(t, isHelperCommandLine(`node something.js`, 3456, false))
}

// TestParseProcLines — разбор скана процессов (метод Б).
func TestParseProcLines(t *testing.T) {
	out := "1234|node|node opencode serve --port 3456\r\n" +
		"5678|node|node other.js\r\n" +
		"мусор\r\n" +
		"90|cmd.exe|cmd /c npx -y opencode-ai serve --port 3456\r\n"
	procs := parseProcLines(out)
	require.Len(t, procs, 3)
	require.Equal(t, 1234, procs[0].PID)
	require.Equal(t, "node", procs[0].Name)
	require.Contains(t, procs[0].Cmd, "opencode serve")
	require.Equal(t, 90, procs[2].PID)
}

// TestParseOneProcLine — ответ Get-CimInstance по конкретному PID (метод А).
func TestParseOneProcLine(t *testing.T) {
	p := parseOneProcLine("node.exe|node opencode serve --port 3456\r\n")
	require.Equal(t, "node.exe", p.Name)
	require.Contains(t, p.Cmd, "opencode serve")
	require.Empty(t, parseOneProcLine("").Name)
}
