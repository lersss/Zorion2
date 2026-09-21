// web/frontend_graphics_test.go
// Node-тест настроек графики (идея 2026-09-22 «внешний вид звёзд», вкладка
// дашборда «⚙️ Графика»): starVisualOptions() уважает тумблер starVisualTwinkle
// и режим starVisualLite («Для слабых ПК» = спрайт + все эффекты off), не
// затирая сохранённые ключи игрока; starNeedsAnim() не поднимает непрерывный
// кадр без мерцания и в lite-режиме; dashboard/graphics.js импортируется в
// Node без верхнеуровневого обращения к document/localStorage (Node-граф
// фронта). Паттерн — как в TestStarRenderBitsInNode (frontend_stars_test.go).
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGraphicsOptionsInNode — облегчённый режим и тумблер мерцания в
// star_render.js + Node-безопасность dashboard/graphics.js.
func TestGraphicsOptionsInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модулей")
	}
	root := "file:///" + filepath.ToSlash(repoRoot(t))
	script := graphicsOptionsScript
	script = strings.Replace(script, "__STAR__", root+"/web/static/js/map/star_render.js", 1)
	script = strings.Replace(script, "__CONFIG__", root+"/web/static/js/map/config.js", 1)
	script = strings.Replace(script, "__GFX__", root+"/web/static/js/dashboard/graphics.js", 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка настроек графики упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "GRAPHICS_OPTIONS_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const graphicsOptionsScript = `
// DOM/localStorage-стаб: модуль карты тянет config.js, который на верхнем
// уровне берёт #mapCanvas.getContext('2d').
globalThis.window = globalThis;
const store = new Map();
globalThis.localStorage = {
    getItem: (k) => store.has(k) ? store.get(k) : null,
    setItem: (k, v) => { store.set(k, String(v)); },
    removeItem: (k) => { store.delete(k); },
};
const ctxStub = {
    createRadialGradient(){ return { addColorStop(){} }; },
    createLinearGradient(){ return { addColorStop(){} }; },
    save(){}, restore(){}, beginPath(){}, arc(){}, fill(){}, stroke(){},
    moveTo(){}, lineTo(){}, closePath(){}, ellipse(){}, translate(){}, rotate(){},
    drawImage(){}, fillText(){}, strokeText(){}, measureText(){ return { width: 10 }; },
};
globalThis.document = {
    getElementById(){ return { getContext(){ return ctxStub; } }; },
    createElement(){ return { width: 0, height: 0, getContext(){ return ctxStub; } }; },
    addEventListener(){}, removeEventListener(){}, hidden: false,
};
globalThis.requestAnimationFrame = () => 0;

const sr = await import(new URL("__STAR__").href);
const cfg = await import(new URL("__CONFIG__").href);

function eq(name, got, want) {
    if (got !== want) throw new Error(name + ': ' + JSON.stringify(got) + ' != ' + JSON.stringify(want));
}

// 1. Дефолты при пустом localStorage: спрайт, мерцание вкл, лучи выкл.
store.clear();
let o = sr.starVisualOptions();
eq('default mode', o.mode, 'sprite');
eq('default twinkle', o.twinkle, true);
eq('default petals', o.petals, false);
eq('default exotic', o.exotic, true);
eq('default ignite', o.ignite, true);
eq('default additive', o.additive, true);

// 2. starVisualTwinkle='0' — мерцание выключено, остальное как у вида звёзд.
store.set('starVisualTwinkle', '0');
o = sr.starVisualOptions();
eq('twinkle off', o.twinkle, false);
eq('twinkle off: mode', o.mode, 'sprite');
eq('twinkle off: petals', o.petals, false);
store.delete('starVisualTwinkle');

// 3. Вид «Фото» — лучи включены (набор пресета как fallback ключа).
store.set('starVisualPreset', 'photo');
o = sr.starVisualOptions();
eq('photo mode', o.mode, 'vector');
eq('photo petals', o.petals, true);

// 4. Режим «Для слабых ПК»: спрайт + все эффекты off; сохранённые ключи целы.
store.set('starVisualPetals', '1');
store.set('starVisualExotic', '1');
store.set('starVisualIgnite', '1');
store.set('starVisualAdditive', '1');
store.set('starVisualTwinkle', '1');
store.set('starVisualLite', '1');
o = sr.starVisualOptions();
eq('lite mode', o.mode, 'sprite');
eq('lite twinkle', o.twinkle, false);
eq('lite ignite', o.ignite, false);
eq('lite additive', o.additive, false);
eq('lite exotic', o.exotic, false);
eq('lite petals', o.petals, false);
eq('lite kept petals key', localStorage.getItem('starVisualPetals'), '1');
eq('lite kept preset key', localStorage.getItem('starVisualPreset'), 'photo');

// 5. Выключили режим — вернулись прежние значения игрока (photo, лучи вкл).
store.set('starVisualLite', '0');
o = sr.starVisualOptions();
eq('lite off mode', o.mode, 'vector');
eq('lite off petals', o.petals, true);
eq('lite off twinkle', o.twinkle, true);

// 6. starNeedsAnim: без мерцания и зажигания непрерывный кадр не поднимается;
// lite-режим гасит его тоже.
cfg.state.starSingles = 10;
sr.resetIgnite();
store.set('starVisualTwinkle', '0');
eq('needsAnim twinkle off', sr.starNeedsAnim(), false);
store.set('starVisualTwinkle', '1');
eq('needsAnim twinkle on', sr.starNeedsAnim(), true);
store.set('starVisualLite', '1');
eq('needsAnim lite', sr.starNeedsAnim(), false);
store.set('starVisualLite', '0');
store.delete('starVisualTwinkle');

// 7. «Корона» (D): читается как crown; венец анимируется -> непрерывный кадр
// поднимается даже при выключенном мерцании; lite-режим гасит и её.
store.clear();
cfg.state.starSingles = 10;
sr.resetIgnite();
store.set('starVisualPreset', 'crown');
o = sr.starVisualOptions();
eq('crown mode', o.mode, 'vector');
eq('crown flag', o.crown, true);
store.set('starVisualTwinkle', '0');
eq('crown needsAnim (twinkle off)', sr.starNeedsAnim(), true);
store.set('starVisualLite', '1');
eq('crown lite mode', sr.starVisualOptions().mode, 'sprite');
eq('crown lite flag', sr.starVisualOptions().crown, false);
eq('crown needsAnim lite', sr.starNeedsAnim(), false);

// 8. dashboard/graphics.js импортируется без верхнеуровневого падения
// (document/localStorage трогаются только внутри initGraphicsSettings).
const gfx = await import(new URL("__GFX__").href);
eq('graphics init exported', typeof gfx.initGraphicsSettings, 'function');

console.log('GRAPHICS_OPTIONS_OK');
`
