// web/frontend_flight_status_test.go
// Строка «Статус полёта» в шапке игрока показывала внутренние коды миров
// («В полёте: <uuid> → <uuid>»); хвост 87 реестра docs/TAILS.md. Решающая
// проверка вынесена в чистую функцию flightStatusText (index.html) и
// исполняется в Node — страница монолитный HTML с модульным инлайн-скриптом,
// целиком её в Node без DOM не исполнить (паттерн frontend_studio_subtype_test.go
// для монолита, frontend_panel_autorefresh_test.go — исполнение чистой функции).
// Название есть — печатается название; названия нет (старый сервер, мир
// удалён) — откат на id, как до хвоста 87.
package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// indexHTML — исходник монолитной страницы игрока.
func indexHTML(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "web", "index.html"))
	if err != nil {
		t.Fatalf("чтение index.html: %v", err)
	}
	return string(b)
}

// indexFunc — текст функции верхнего уровня из index.html: от «function name(»
// до её закрывающей скобки (считаем баланс скобок; подстановка ${…} внутри
// шаблонной строки баланс сохраняет). Отступы в монолите не важны.
func indexFunc(t *testing.T, src, name string) string {
	t.Helper()
	i := strings.Index(src, "function "+name+"(")
	if i < 0 {
		t.Fatalf("в index.html нет функции %s", name)
	}
	open := strings.Index(src[i:], "{")
	if open < 0 {
		t.Fatalf("%s: не найдено тело функции", name)
	}
	depth := 0
	for j := i + open; j < len(src); j++ {
		switch src[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[i : j+1]
			}
		}
	}
	t.Fatalf("%s: тело функции не закрыто", name)
	return ""
}

// TestFlightStatusTextInNode — подсказка полёта печатает НАЗВАНИЯ миров из
// /me.flight.from_name/to_name, а мир не найден (удалён) — «—», не код.
func TestFlightStatusTextInNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node не найден в PATH — пропускаю исполнение функции страницы")
	}
	fn := indexFunc(t, indexHTML(t), "flightStatusText")
	script := "const flightStatusText = " + fn + ";\n" + flightStatusAssertions

	cmd := exec.Command(node, "--eval", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("flightStatusText упала в node:\n%s\n---\n%v", out, err)
	}
	if !strings.Contains(string(out), "FLIGHT_STATUS_OK") {
		t.Fatalf("node не дошёл до конца проверки; вывод:\n%s", out)
	}
}

// TestFlightStatusUsesNamesFromMe — шапка зовёт flightStatusText, а старый
// шаблон с внутренними id из index.html вычищен (иначе правка есть, а игрок
// по-прежнему видит коды).
func TestFlightStatusUsesNamesFromMe(t *testing.T) {
	src := indexHTML(t)
	if !strings.Contains(src, "flightStatusText(data.flight)") {
		t.Fatal("loadUser не печатает статус полёта через flightStatusText")
	}
	if strings.Contains(src, "flight.from} →") {
		t.Fatal("в index.html остался шаблон статуса полёта с внутренними id миров")
	}
}

const flightStatusAssertions = `
function eq(name, got, want) {
    if (got !== want) throw new Error(name + ': "' + got + '" != "' + want + '"');
}

// Названия вместо id.
const named = flightStatusText({
    from: '11111111-1111-1111-1111-111111111111',
    to: '22222222-2222-2222-2222-222222222222',
    from_name: 'Земля',
    to_name: 'Кеплер-22',
});
eq('names', named, '🚀 В полёте: Земля → Кеплер-22');
if (named.indexOf('11111111') >= 0 || named.indexOf('22222222') >= 0) {
    throw new Error('в строке остались id миров: ' + named);
}

// Поля названий нет (сервер без хвоста 87) — откат на id, как было раньше.
eq('no name fields', flightStatusText({ from: 'w1', to: 'w2' }),
    '🚀 В полёте: w1 → w2');
// Название пустое (мир удалён/перегенерирован) — тоже откат на id, а не «—».
eq('empty names', flightStatusText({ from: 'w-gone', to: 'w-to', from_name: '', to_name: '' }),
    '🚀 В полёте: w-gone → w-to');
// Отката некуда (нет ни названия, ни id) — «—», строка не рвётся.
eq('no name and no id', flightStatusText({ from_name: '', to_name: '' }),
    '🚀 В полёте: — → —');

// Полёта нет — «На месте».
eq('no flight', flightStatusText(null), 'На месте');

console.log('FLIGHT_STATUS_OK');
`
