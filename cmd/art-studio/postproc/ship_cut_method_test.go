package postproc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFakeArgDump — фейковый python: пишет все полученные аргументы в файл.
// Позволяет проверить argv вызова ShipSpriteCut без реального rembg.
func writeFakeArgDump(t *testing.T, dumpPath string) string {
	t.Helper()
	fp := filepath.Join(t.TempDir(), "fake_python.cmd")
	script := "@echo off\r\n" +
		"echo %* > \"" + dumpPath + "\"\r\n" +
		"exit /b 0\r\n"
	if err := os.WriteFile(fp, []byte(script), 0o644); err != nil {
		t.Fatalf("WriteFile fake python: %v", err)
	}
	return fp
}

// TestShipSpriteCutUsesRembg — конвейер выреза идёт через `--method rembg`
// (рецепт 2026-09-23): пороговый `edge` больше не вызывается, но сохранены
// --fill-holes/--no-orient/--report и --canvas.
func TestShipSpriteCutUsesRembg(t *testing.T) {
	dir := t.TempDir()
	dump := filepath.Join(dir, "args.txt")
	fake := writeFakeArgDump(t, dump)
	if _, err := ShipSpriteCut(fake, filepath.Join(dir, "raw.png"),
		filepath.Join(dir, "s01.png"), 100); err != nil {
		t.Fatalf("ShipSpriteCut: %v", err)
	}
	b, err := os.ReadFile(dump)
	if err != nil {
		t.Fatalf("ReadFile dump: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, "--method rembg") {
		t.Errorf("argv = %q, want --method rembg", got)
	}
	if strings.Contains(got, "--method edge") || strings.Contains(got, "--tol") {
		t.Errorf("argv = %q, пороговый edge/--tol не должен вызываться", got)
	}
	for _, want := range []string{"--fill-holes", "--no-orient", "--report", "--canvas 100"} {
		if !strings.Contains(got, want) {
			t.Errorf("argv = %q, потерян %q", got, want)
		}
	}
}
