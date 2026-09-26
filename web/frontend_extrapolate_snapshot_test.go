// web/frontend_extrapolate_snapshot_test.go
// Node-тест клиентской тени населения: populationAt на снимковом поселении
// возвращает замороженное число при любом nowMs (спека 2026-09-23-орбита-
// планеты-присутствие-и-снимок, тест T14, §3.1: в снимке нет population_exact/
// computed_at/r_per_sec/lambda_per_hour — иначе число «поплывёт»). Паттерн —
// frontend_contracts_test.go: модуль исполняется в Node (чистая функция, DOM не
// нужен).
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSnapshotPopulationFrozenInNode — populationAt в modal/extrapolate.js.
func TestSnapshotPopulationFrozenInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модулей")
	}
	root := "file:///" + filepath.ToSlash(repoRoot(t))
	script := strings.Replace(extrapolateSnapshotScript, "__EXTRAPOLATE__", root+"/web/static/js/modal/extrapolate.js", 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка заморозки снимка упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "EXTRAPOLATE_SNAPSHOT_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const extrapolateSnapshotScript = `
const e = await import(new URL("__EXTRAPOLATE__").href);

function eq(name, got, want) {
    if (got !== want) throw new Error(name + ': ' + got + ' != ' + want);
}

// Снимковое поселение: ровно как в §3.1 — целое population, БЕЗ population_exact,
// computed_at, r_per_sec, lambda_per_hour. Память игрока «не тикает».
const snapshot = {
    id: 's1', race_id: 'spark', race_name: 'Искры',
    population: 876000000, stability: 69,
    branches: [{ recipe_id: 73, recipe_name: 'Вода' }],
};

const now = Date.parse('2026-09-23T14:05:00Z');
const hour = 3600 * 1000;
const year = 365 * 24 * hour;

eq('snapshot now', e.populationAt(snapshot, now), 876000000);
eq('snapshot +1h', e.populationAt(snapshot, now + hour), 876000000);
eq('snapshot +1y', e.populationAt(snapshot, now + year), 876000000);
eq('snapshot far future', e.populationAt(snapshot, now + 100 * year), 876000000);

// Сумма по планетарной карточке тоже заморожена.
eq('planet snapshot frozen',
    e.planetPopulationAt({ id: 'p1', population: 876000000, settlements: [snapshot] }, now + year),
    876000000);

// Контроль: живое поселение с чек-точкой и R меняется со временем — тест не
// пустой (иначе «замороженность» ничего не проверяла бы).
const live = {
    id: 's2', race_id: 'spark', race_name: 'Искры',
    population: 1000, population_exact: 1000,
    computed_at: '2026-09-23T14:05:00Z', r_per_sec: 0.001,
};
eq('live changes over hour', e.populationAt(live, now + hour) !== e.populationAt(live, now), true);

console.log('EXTRAPOLATE_SNAPSHOT_OK');
`
