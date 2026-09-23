// web/frontend_game_date_test.go
// Node-тест общеигрового календаря (спека 2026-09-23-орбита-планеты-присутствие-
// и-снимок §7/§12-Э1/T13): helper game_date.js — год +1000, формат UTC
// «23.09.3026, 14:05», суффикс « UTC» только у gameDateUtc, пустое/невалидное →
// «—»; одна метка даёт одинаковый вывод при разных локальных поясах (UTC —
// серверное время). Паттерн — как в frontend_deposits_test.go.
package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGameDateInNode — gameDate/gameDateShort/gameDateUtc в game_date.js.
func TestGameDateInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модулей")
	}
	root := "file:///" + filepath.ToSlash(repoRoot(t))
	script := strings.Replace(gameDateScript, "__DATE__", root+"/web/static/js/game_date.js", 1)

	run := func(tz string) string {
		cmd := exec.Command(node, "--input-type=module", "--eval", script)
		cmd.Env = append(os.Environ(), "TZ="+tz)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("проверка game_date упала (TZ=%s):\n%s\n---\n%v", tz, out, err)
		}
		return string(out)
	}

	outUTC := run("UTC")
	if !strings.Contains(outUTC, "GAME_DATE_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", outUTC)
	}
	// Вывод не зависит от локального пояса браузера: даты — в UTC (серверное
	// время), одинаково у всех игроков (решение гейта).
	outNY := run("America/New_York")
	if outUTC != outNY {
		t.Fatalf("вывод зависит от пояса:\nTZ=UTC:\n%s\nTZ=America/New_York:\n%s", outUTC, outNY)
	}
}

const gameDateScript = `
const gd = await import(new URL("__DATE__").href);

function eq(name, got, want) {
    if (got !== want) throw new Error(name + ': ' + JSON.stringify(got) + ' != ' + JSON.stringify(want));
}

// Смещение года — константа (+1000).
eq('offset', gd.GAME_YEAR_OFFSET, 1000);

// +1000 к году и UTC-формат: 2026-09-23T14:05Z → 23.09.3026, 14:05.
eq('gameDate', gd.gameDate('2026-09-23T14:05:00Z'), '23.09.3026, 14:05');
eq('gameDateShort', gd.gameDateShort('2026-09-23T14:05:00Z'), '23.09.3026');

// Суффикс « UTC» — только у gameDateUtc.
eq('gameDateUtc', gd.gameDateUtc('2026-09-23T14:05:00Z'), '23.09.3026, 14:05 UTC');
if (gd.gameDate('2026-09-23T14:05:00Z').includes('UTC')) throw new Error('gameDate не должен содержать UTC');
if (gd.gameDateShort('2026-09-23T14:05:00Z').includes('UTC')) throw new Error('gameDateShort не должен содержать UTC');

// Метка со смещением пояса → показывается UTC (серверное время), не локальный пояс.
eq('offset input -> UTC', gd.gameDate('2026-09-23T14:05:00+05:00'), '23.09.3026, 09:05');

// Двузначные день/месяц/часы/минуты.
eq('zero padding', gd.gameDate('2027-01-05T04:07:00Z'), '05.01.3027, 04:07');

// Пустое/невалидное → «—».
eq('empty string', gd.gameDate(''), '—');
eq('null', gd.gameDate(null), '—');
eq('undefined', gd.gameDate(undefined), '—');
eq('invalid', gd.gameDate('не дата'), '—');
eq('short empty', gd.gameDateShort(''), '—');
eq('utc empty', gd.gameDateUtc(''), '—');

console.log('GAME_DATE_OK');
`
