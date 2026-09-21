// web/frontend_ships_test.go
// Юнит-проверка клиентского хелпера конвенции ориентации корабля
// (спека 2026-09-21-угол-корабля-в-метаданных §6.2/§6.4, §9 п.9/9a/10):
// shipDrawTransform (антипереворот по КУРСУ H, зеркало, сумма H + V·A) и
// shipOrientFor (фолбэк (0, false)). Запускается в Node — образец раннера
// TestAdminFrontendLoadsInNode.
package web

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestShipDrawTransformInNode — node импортирует ship_sprites.js и проверяет
// трансформ отрисовки для курсов 0/±90/180/−135, независимость V от A и
// фолбэк неизвестного файла.
func TestShipDrawTransformInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю проверку хелпера")
	}
	entryURL := "file:///" + filepath.ToSlash(filepath.Join(repoRoot(t), "web", "static", "js", "map", "ship_sprites.js"))
	script := strings.Replace(shipTransformScript, "__ENTRY__", entryURL, 1)
	cmd := exec.Command(node, "--input-type=module", "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("проверка shipDrawTransform упала:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "SHIP_TRANSFORM_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

const shipTransformScript = `
const mod = await import(new URL("__ENTRY__").href);
const D = Math.PI / 180;
const EPS = 1e-6;
function chk(name, got, want) {
    if (Math.abs(got - want) > EPS) throw new Error(name + ': ' + got + ' != ' + want);
}
function chkBool(name, got, want) {
    if (got !== want) throw new Error(name + ': ' + got + ' != ' + want);
}
const T = mod.shipDrawTransform;

// 9: легаси (0, false) → тождество
let r = T(0, { angle: 0, flip: false });
chk('identity rotate', r.rotate, 0);
chk('identity scaleX', r.scaleX, 1);
chk('identity scaleY', r.scaleY, 1);
// 9: angle 90 при heading 0 → π/2
r = T(0, { angle: 90, flip: false });
chk('angle90', r.rotate, Math.PI / 2);
// 9: heading 90 + angle 30 → 120 (угол складывается с курсом)
r = T(90 * D, { angle: 30, flip: false });
chk('heading90+angle30', r.rotate, 120 * D);
// 9: flip true → scaleX = −1, знак rotate не меняется
r = T(0, { angle: 0, flip: true });
chk('flip scaleX', r.scaleX, -1);
chk('flip rotate', r.rotate, 0);
chk('flip scaleY', r.scaleY, 1);

// 9a: антипереворот по КУРСУ H
r = T(0, { angle: 0, flip: false });            chk('V(0)', r.scaleY, 1);
r = T(90 * D, { angle: 0, flip: false });       chk('V(+90)', r.scaleY, 1);
r = T(-90 * D, { angle: 0, flip: false });      chk('V(-90)', r.scaleY, 1);
r = T(180 * D, { angle: 0, flip: false });      chk('V(180)', r.scaleY, -1);
r = T(-135 * D, { angle: 0, flip: false });     chk('V(-135)', r.scaleY, -1);
// A на знак V не влияет
r = T(0, { angle: 95, flip: false });           chk('V(A95,H0)', r.scaleY, 1);
r = T(180 * D, { angle: 95, flip: false });     chk('V(A95,H180)', r.scaleY, -1);
// rotate = H + V·A: heading 180, A = 20 → 160
r = T(180 * D, { angle: 20, flip: false });
chk('H180+A20', r.rotate, 160 * D);

// 10: фолбэк неизвестного файла/пустого реестра → (0, false)
let o = mod.shipOrientFor('nope.png');
chk('fallback angle', o.angle, 0);
chkBool('fallback flip', o.flip, false);
// индекс заполняется setShipOptions и отдаётся shipOrientFor
mod.setShipOptions([{ file: 'a.png', angle: 20, flip: true }]);
o = mod.shipOrientFor('a.png');
chk('index angle', o.angle, 20);
chkBool('index flip', o.flip, true);

console.log('SHIP_TRANSFORM_OK');
`
