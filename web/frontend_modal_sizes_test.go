// web/frontend_modal_sizes_test.go
// Node-тест лестницы размеров модалки системы (спека 2026-09-22 «Визуальная
// система размеров», этап 1). Проверяет единую функцию радиуса планеты
// planetRadius(size), масштаб звёзд ×1.5 и снятие клампа под кадр в
// computeLayout (finalStarRadius без maxStarRadius + пол звезды).
// Паттерн — как в TestStarRenderBitsInNode (frontend_stars_test.go): node
// исполняет сам модуль, без браузера (layout.js тянет только state/utils —
// без DOM на верхнем уровне).
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestModalSizeLadderInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модуля")
	}
	root := repoRoot(t)
	layoutURL := "file:///" + filepath.ToSlash(filepath.Join(root, "web", "static", "js", "modal", "layout.js"))
	utilsURL := "file:///" + filepath.ToSlash(filepath.Join(root, "web", "static", "js", "modal", "utils.js"))
	stateURL := "file:///" + filepath.ToSlash(filepath.Join(root, "web", "static", "js", "modal", "state.js"))
	script := strings.Replace(modalSizesScript, "__LAYOUT__", layoutURL, 1)
	script = strings.Replace(script, "__UTILS__", utilsURL, 1)
	script = strings.Replace(script, "__STATE__", stateURL, 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка лестницы размеров упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "MODAL_SIZES_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const modalSizesScript = `
globalThis.window = globalThis;
globalThis.localStorage = { getItem: () => null, setItem(){}, removeItem(){} };

const layout = await import(new URL("__LAYOUT__").href);
const utils = await import(new URL("__UTILS__").href);
const state = await import(new URL("__STATE__").href);

const near = (a, b, eps) => Math.abs(a - b) < (eps === undefined ? 1e-6 : eps);

// --- planetRadius: clamp(10·size^0.6, 5, 43) (спека §4.2) ---
if (typeof layout.planetRadius !== 'function') throw new Error('planetRadius не экспортирована');
const pr = layout.planetRadius;
if (pr(0.3) !== 5) throw new Error('planetRadius(0.3)=' + pr(0.3) + ' (ожидался пол 5)');
if (pr(1.0) !== 10) throw new Error('planetRadius(1.0)=' + pr(1.0) + ' (Земля = 10)');
if (!near(pr(0.53), 6.8, 0.05)) throw new Error('planetRadius(0.53)=' + pr(0.53));
if (!near(pr(3.0), 19.3, 0.05)) throw new Error('planetRadius(3.0)=' + pr(3.0));
if (!near(pr(5.9), 29.0, 0.05)) throw new Error('planetRadius(5.9)=' + pr(5.9));
if (!near(pr(11.2), 42.6, 0.05)) throw new Error('planetRadius(11.2)=' + pr(11.2));
if (pr(11.2) > 43 + 1e-9) throw new Error('planetRadius превысил потолок 43');
// Гигант/Земля ≈ ×4.26, гигант заметно больше камня.
if (!(pr(11.2) / pr(1.0) > 4.0)) throw new Error('гигант/Земля слишком мал: ' + (pr(11.2) / pr(1.0)));
if (!(pr(11.2) > pr(0.53))) throw new Error('гигант не больше камня');
// Неизвестный/битый size → Земля (10), не NaN и не 0.
if (pr(undefined) !== 10 || pr(0) !== 10 || pr(NaN) !== 10) throw new Error('planetRadius fallback != 10');

// --- getStarSize: масштаб ×1.5, экзотика без изменений (спека §4.1) ---
const starWant = { O: 180, B: 158, A: 135, F: 113, G: 90, K: 72, M: 54, L: 45, T: 36, Y: 27 };
for (const k of Object.keys(starWant)) {
    if (utils.getStarSize(k, 'star') !== starWant[k]) {
        throw new Error('getStarSize(' + k + ')=' + utils.getStarSize(k, 'star') + ', want ' + starWant[k]);
    }
}
if (utils.getStarSize('', 'white_dwarf') !== 11) throw new Error('WD не 11');
if (utils.getStarSize('', 'black_hole') !== 7) throw new Error('BH не 7');
if (utils.getStarSize('', 'neutron') !== 7) throw new Error('NS не 7');
if (utils.getStarSize('', 'protostar') !== 30) throw new Error('protostar не 30');
// O/M сохраняют пропорцию ×3.33.
if (!near(utils.getStarSize('O', 'star') / utils.getStarSize('M', 'star'), 180 / 54, 1e-6)) throw new Error('O/M нарушен');

// --- computeLayout: кламп снят + пол звезды (спека §4.1/§5.2) ---
const planets = [
    { orbit_index: 0, size: 0.3, type: 'ледяная' },
    { orbit_index: 1, size: 11.2, type: 'газовый гигант' },
];
// Кадр 800×600: старая maxStarRadius = 240·0.25 = 60 — звезда O (180) не должна
// больше клампиться к 60; пол (1.15·42.6 ≈ 49) ниже O — не срабатывает.
const Lo = layout.computeLayout(planets, 180, 800, 600);
if (Lo.finalStarRadius !== 180) throw new Error('O клампится под кадр: finalStarRadius=' + Lo.finalStarRadius);
if (Lo.sizeMultiplier !== undefined) throw new Error('sizeMultiplier не удалён из layout');
if (typeof layout.getPlanetSize === 'function') throw new Error('getPlanetSize не заменён на planetRadius');
// M (54) > пол (49) — порядок классов сохраняется.
const Lm = layout.computeLayout(planets, 54, 800, 600);
if (Lm.finalStarRadius !== 54) throw new Error('M затронут полом: ' + Lm.finalStarRadius);
// Поздняя T (36) в системе с гигантом поднимается полом до ≈49.
const Lt = layout.computeLayout(planets, 36, 800, 600);
if (!near(Lt.finalStarRadius, 49.0, 0.2)) throw new Error('пол T = ' + Lt.finalStarRadius + ' (ожидалось ≈49)');
// Система без планет: пол не срабатывает, звезда — класс.
const Ln = layout.computeLayout([], 27, 800, 600);
if (Ln.finalStarRadius !== 27) throw new Error('звезда без планет: ' + Ln.finalStarRadius);
// Крупный планеты нет: пол ниже класса G (90).
const Lg = layout.computeLayout([{ orbit_index: 0, size: 1.0, type: 'землеподобная' }], 90, 800, 600);
if (Lg.finalStarRadius !== 90) throw new Error('G с Землёй: ' + Lg.finalStarRadius + ' (пол 11.5 не должен поднимать)');

// --- Пол применяется к компаньонам (спека §4.3): поздний Y (27) в системе с
// гигантом 11.2 (42.6 px) обязан подняться полом до ≈49, а не остаться меньше
// планеты. ---
state.modalState.systemType = 'binary';
state.modalState.binaryType = '';
state.modalState.companion = 'Y';
state.modalState.companionColor = '#9e4b2c';
state.modalState.companionMass = 1;
state.modalState.stellarMass = 1;
state.modalState.companionSepAU = 0.5;
const Lc = layout.computeLayout(planets, 27, 800, 600);
const compStar = (Lc.stars || []).find(s => s.kind === 'companion');
if (!compStar) throw new Error('компаньон не найден в stars');
if (!near(compStar.radius, 49.0, 0.2)) throw new Error('компаньон без пола: ' + compStar.radius + ' (ожидалось ≈49)');
if (compStar.radius < layout.planetRadius(11.2)) throw new Error('компаньон меньше планеты: ' + compStar.radius);

// --- Экзотика пол НЕ получает (спека §4.1): WD 11 и ЧД 7 в системе с тем же
// гигантом остаются компактными (пол поднял бы их до ≈49). ---
state.modalState.systemType = 'single';
state.modalState.companion = '';
state.modalState.companionColor = '';
state.modalState.companionSepAU = null;
state.modalState.starType = 'white_dwarf';
const Lwd = layout.computeLayout(planets, 11, 800, 600);
if (Lwd.finalStarRadius !== 11) throw new Error('WD получил пол: ' + Lwd.finalStarRadius + ' (ожидалось 11)');
state.modalState.starType = 'black_hole';
const Lbh = layout.computeLayout(planets, 7, 800, 600);
if (Lbh.finalStarRadius !== 7) throw new Error('ЧД получил пол: ' + Lbh.finalStarRadius + ' (ожидалось 7)');
state.modalState.starType = 'star';

console.log('MODAL_SIZES_OK');
`
