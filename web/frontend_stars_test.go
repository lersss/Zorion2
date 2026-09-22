// web/frontend_stars_test.go
// Node-тест детерминизма звёзд (идея 2026-09-22 «внешний вид звёзд»):
// hashString/starBits дают биты в [0,1] и конечные яркость/мерцание.
// Регрессия, которую тест держит: знаковый 32-битный хеш (hash & 0xFFFFFFFF
// даёт int32) делал b1 < 0 → Math.pow(b1, 1.8) = NaN → rgba(...,NaN) и
// падение createRadialGradient в браузере. Паттерн — как в
// TestShipDrawTransformInNode (frontend_ships_test.go): node исполняет сам
// модуль, без браузера.
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestStarRenderBitsInNode — starBits/starBrightness/starTwinkle над
// «враждебными» sid (с отрицательным знаковым хешем) должны быть конечными
// и лежать в допустимых диапазонах.
func TestStarRenderBitsInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение модуля")
	}
	entryURL := "file:///" + filepath.ToSlash(filepath.Join(repoRoot(t), "web", "static", "js", "map", "star_render.js"))
	script := strings.Replace(starBitsScript, "__ENTRY__", entryURL, 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка битов звёзд упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "STAR_BITS_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const starBitsScript = `
// DOM-стаб: star_render.js тянет map/config.js, который на верхнем уровне
// берёт #mapCanvas.getContext('2d'). Верхнеуровневого падения быть не должно.
globalThis.window = globalThis;
globalThis.localStorage = { getItem: () => null, setItem: () => {}, removeItem: () => {} };
const ctxStub = {
    createRadialGradient(){ return { addColorStop(){} }; },
    createLinearGradient(){ return { addColorStop(){} }; },
    save(){}, restore(){}, beginPath(){}, arc(){}, fill(){}, stroke(){},
    moveTo(){}, lineTo(){}, closePath(){}, ellipse(){}, translate(){}, rotate(){},
    drawImage(){}, fillText(){}, strokeText(){}, measureText(){ return { width: 10 }; },
    createImageData(w, h){ return { data: new Uint8ClampedArray((w || 1) * (h || 1) * 4), width: w, height: h }; },
    putImageData(){},
    globalAlpha: 1, globalCompositeOperation: 'source-over',
};
globalThis.document = {
    getElementById(){ return { getContext(){ return ctxStub; } }; },
    createElement(){ return { width: 0, height: 0, getContext(){ return ctxStub; } }; },
    addEventListener(){}, removeEventListener(){},
};
globalThis.requestAnimationFrame = () => 0;

const mod = await import(new URL("__ENTRY__").href);

function inUnit(x){ return typeof x === 'number' && Number.isFinite(x) && x >= 0 && x <= 1; }

// Знаковый хеш — ровно как было в баге: & 0xFFFFFFFF даёт int32.
function signedHash(s){ let h = 0; for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) & 0xFFFFFFFF; return h; }

// Собираем sid, чей знаковый хеш отрицателен — без них тест не сторожит баг.
const hostile = [];
for (let i = 0; i < 5000 && hostile.length < 5; i++) {
    const s = 'sid' + i;
    if (signedHash(s) < 0) hostile.push(s);
}
if (hostile.length === 0) throw new Error('не найдено sid с отрицательным хешем — тест бесполезен');

const sids = hostile.concat(['sid0', 'sid1', 'alpha', 'Zeta-9', '']);
for (const sid of sids) {
    const bits = mod.starBits(sid);
    if (!inUnit(bits.b1) || !inUnit(bits.b2) || !inUnit(bits.b3)) {
        throw new Error('биты вне [0,1] для ' + sid + ': ' + JSON.stringify(bits));
    }
    const B = mod.starBrightness('G', bits.b1);
    if (!Number.isFinite(B)) throw new Error('starBrightness не конечна для ' + sid + ' b1=' + bits.b1);
    const tw = mod.starTwinkle(bits.b2, bits.b3, 1.5);
    if (!Number.isFinite(tw.a) || !Number.isFinite(tw.r)) throw new Error('starTwinkle не конечна для ' + sid);
}

// Видимый радиус/hit-зона принимают R снаружи и остаются конечными.
const c = { sid: hostile[0], sspec: 'O' };
const vr = mod.starVisualRadius(c, 20);
if (!Number.isFinite(vr) || vr < 20) throw new Error('starVisualRadius(c,20)=' + vr);
const hr = mod.starHitRadius(c, 20);
if (!Number.isFinite(hr) || hr > 20 * 1.6 + 1e-9) throw new Error('starHitRadius(c,20)=' + hr);

// На сильном зуме (R > HALO_MAX_PX = 240) видимый радиус не превышает потолок
// ореола: иначе якорь подписи/hover-пилюли и hit-зона уезжают от нарисованного
// ореола (решение создателя 2026-09-22 п.2, звоночек критика №2).
const bigVr = mod.starVisualRadius({ sid: 'big', sspec: 'O' }, 1000);
if (!Number.isFinite(bigVr) || bigVr > 240 + 1e-9) throw new Error('starVisualRadius(R=1000)=' + bigVr);
const bigHr = mod.starHitRadius({ sid: 'big', sspec: 'O' }, 1000);
if (!Number.isFinite(bigHr) || bigHr > 240 + 1e-9) throw new Error('starHitRadius(R=1000)=' + bigHr);

// Дефолт пресета — «Спрайт» (решение создателя 2026-09-22, гейт 2): localStorage
// в стабе пуст, значит функция обязана вернуть fallback 'sprite' (не 'eye').
const preset = mod.starVisualPreset();
if (preset !== 'sprite') throw new Error('starVisualPreset() default=' + preset);

// Ядро — одна плавная яркая точка (правка @gdesigner 2026-09-22 «мишень»):
// coreStops даёт r1/r2/a1 плюс производные a2 = a1·(1−r1), ac = a1·(1−r2)
// (альфа монотонна по радиусу — нет «ямы» и обрыва на краю диска). У O (k=1)
// центр 0.98, белая зона до 0.30; у L/T/Y (k=0) центр 0.30, белая зона 0.06.
// Числа таблицы идеи округлены до 3 знаков — сверяем с допуском округления.
const near = (a, b) => Math.abs(a - b) < 1e-3;
const oStops = mod.coreStops('O');
if (!near(oStops.r1, 0.30) || !near(oStops.r2, 0.62) || !near(oStops.a1, 0.98) ||
    !near(oStops.a2, 0.686) || !near(oStops.ac, 0.372)) {
    throw new Error('coreStops(O) не совпал с новой рецептурой: ' + JSON.stringify(oStops));
}
for (const spec of ['L', 'T', 'Y']) {
    const s = mod.coreStops(spec);
    if (!near(s.r1, 0.06) || !near(s.r2, 0.24) || !near(s.a1, 0.30) ||
        !near(s.a2, 0.282) || !near(s.ac, 0.228)) {
        throw new Error('coreStops(' + spec + ') при k=0: ' + JSON.stringify(s));
    }
}
for (const spec of ['O', 'B', 'A', 'F', 'G', 'K', 'M', 'L', 'T', 'Y', undefined]) {
    const s = mod.coreStops(spec);
    if (!(s.r1 < s.r2)) throw new Error('coreStops(' + spec + '): r1 >= r2');
    if (!(s.a1 > s.a2 && s.a2 > s.ac && s.ac > 0)) {
        throw new Error('coreStops(' + spec + ') не монотонен: ' + JSON.stringify(s));
    }
    for (const v of [s.r1, s.r2, s.a1, s.a2, s.ac]) {
        if (!Number.isFinite(v) || v < 0 || v > 1) throw new Error('coreStops(' + spec + ') вне [0,1]: ' + JSON.stringify(s));
    }
}

// Смоук отрисовки: drawStar/drawCompanion не должны падать (нет draw/
// clusterScreenRadius). Компаньоны (гейт 3) — «звёздный» вид: проверяем и
// спрайтовый путь ореола (запечённый спрайт), и фолбэк без спектра.
mod.drawStar(ctxStub, c, 100, 100, 20, { mode: 'vector', petals: true, exotic: true, ignite: true, additive: true });
// «Корона» (D): запекание кадра венца + отрисовка (вращение/кроссфейд) не
// должны падать — createImageData/putImageData в стабе.
mod.drawStar(ctxStub, { sid: hostile[0], sspec: 'O', stype: 'star' }, 100, 100, 20, { mode: 'vector', crown: true, petals: false, exotic: true, ignite: true, additive: true });
mod.drawCompanion(ctxStub, 120, 100, 9, 'K', { mode: 'sprite', petals: false, exotic: true, ignite: true, additive: true }, 'seed-c0');
mod.drawCompanion(ctxStub, 120, 100, 9, undefined, undefined, 'seed-c1');

console.log('STAR_BITS_OK hostile=' + hostile.join(','));
`
