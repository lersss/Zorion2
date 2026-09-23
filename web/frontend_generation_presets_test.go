// web/frontend_generation_presets_test.go
// Страж бага B16: пресеты генерации вселенной обязаны давать видимый зазор
// между кластерами. gap = max(spacing, 2*radius, minDist) − 2.5*radius > 0 и
// ratio = max(...)/radius ≥ 2.6 — это ровно те условия, при которых генератор
// (internal/generator/galaxy/poisson.go: clusterSpacing клампится к 2R;
// точки кластера принимаются до 1.25R) оставляет пустоты между скоплениями.
// Тест читает сам UNIVERSE_PRESETS из web/static/js/admin/generation.js и
// считает числа в node; тяжёлый замер недобора постоянным тестом НЕ делается.
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUniversePresetsGapInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю проверку пресетов")
	}
	root := repoRoot(t)
	genURL := "file:///" + filepath.ToSlash(filepath.Join(root, "web", "static", "js", "admin", "generation.js"))
	script := strings.Replace(generationPresetsScript, "__GEN__", genURL, 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка пресетов упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "GEN_PRESETS_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const generationPresetsScript = `
import { readFileSync } from 'node:fs';
const src = readFileSync(new URL("__GEN__"), 'utf8');

const start = src.indexOf('const UNIVERSE_PRESETS');
if (start < 0) throw new Error('UNIVERSE_PRESETS не найден');
const brace = src.indexOf('{', start);
const close = src.indexOf('\n};', brace);
if (brace < 0 || close < 0) throw new Error('не найден литерал UNIVERSE_PRESETS');
const presets = new Function('return (' + src.slice(brace, close + 2) + ')')();

const list = Object.entries(presets);
if (list.length < 14) throw new Error('пресетов меньше 14: ' + list.length);
for (const [name, p] of list) {
    for (const k of ['clusterRadius', 'clusterSpacing', 'minDist']) {
        if (typeof p[k] !== 'number') throw new Error(name + ': ' + k + ' не число');
    }
    const eff = Math.max(p.clusterSpacing, 2 * p.clusterRadius, p.minDist);
    const ratio = eff / p.clusterRadius;
    const gap = eff - 2.5 * p.clusterRadius;
    if (!(gap > 0)) throw new Error(name + ': gap=' + gap + ' (нужен > 0)');
    if (!(ratio >= 2.6)) throw new Error(name + ': ratio=' + ratio.toFixed(3) + ' < 2.6');
}
console.log('GEN_PRESETS_OK ' + list.length);
`
