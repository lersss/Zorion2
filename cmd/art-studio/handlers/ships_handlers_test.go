package handlers

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zorion/cmd/art-studio/config"
)

// newShipsTestStudio — Server с подключёнными конфигами кораблей.
// ships.json копируется во временный файл: /ships/rebuild пишет в него,
// не трогая реальный конфиг. loreDir/raceSlug — реальные.
func newShipsTestStudio(t *testing.T) (*Server, string, string) {
	t.Helper()
	srv, _, pool := newTestStudio(t)
	dc, err := config.LoadShipDict("../../../config/art/ship_dict.json")
	if err != nil {
		t.Fatalf("LoadShipDict: %v", err)
	}
	shipsPath := filepath.Join(t.TempDir(), "ships.json")
	// texture — «устаревшая» (не совпадает с лор-файлом): пересборка должна её
	// заменить; silhouette — реальный (для /ships/prompt)
	shipsData := []byte(`{
  "humans": {
    "race_name": "Люди",
    "family": "F1",
    "texture": "STALE texture",
    "silhouette": "крыло-корпус в плане: широкий нос (светлая кабина-стекло) справа, сужающаяся корма с дюзами слева; асимметрия по оси «нос-корма»; крылья-стабилизаторы; модули: корпус крем, крылья сталь, дюзы тёмные, кабина светлая; запас от краёв ~90 px",
    "blocked": ["tentacle", "organic", "crystal", "pyramid", "obelisk", "bioluminescent", "alien"]
  }
}`)
	if err := os.WriteFile(shipsPath, shipsData, 0o644); err != nil {
		t.Fatalf("WriteFile ships.json: %v", err)
	}
	ships, err := config.ParseShips(shipsData, shipsPath, "../../../config/races.json")
	if err != nil {
		t.Fatalf("ParseShips: %v", err)
	}
	srv.SetShips(ships, dc, shipsPath)
	srv.runner.SetShips(ships, dc, "../../../config/races.json")
	srv.shipsDirPath = "../../../docs/gamedesign/races/ships"
	srv.racesPath = "../../../config/races.json"
	srv.raceNameBySlug = loadRaceNames("../../../config/races.json")
	return srv, shipsPath, pool
}

// TestShipsRaces — /ships/races: 60 рас из каталога races/ships/.
func TestShipsRaces(t *testing.T) {
	srv, _, _ := newShipsTestStudio(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/races", nil))
	var resp struct {
		Races []struct {
			Slug string `json:"slug"`
			Name string `json:"name"`
		} `json:"races"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(resp.Races) != 60 {
		t.Errorf("races = %d, want 60", len(resp.Races))
	}
	found := false
	for _, rc := range resp.Races {
		if rc.Slug == "humans" && rc.Name == "Люди" {
			found = true
		}
	}
	if !found {
		t.Errorf("нет humans/Люди в списке")
	}
}

// TestShipsInfo — /ships/info: запись из ships.json; фолбек из лор-файла
// для расы без записи.
func TestShipsInfo(t *testing.T) {
	srv, _, _ := newShipsTestStudio(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/info?race=humans", nil))
	var resp struct {
		Race       string   `json:"race"`
		RaceName   string   `json:"race_name"`
		Family     string   `json:"family"`
		Texture    string   `json:"texture"`
		Silhouette string   `json:"silhouette"`
		Blocked    []string `json:"blocked"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp.Texture != "STALE texture" {
		t.Errorf("texture = %q, want STALE (из ships.json)", resp.Texture)
	}
	// фолбек: расы нет в ships.json → из лор-файла
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, httptest.NewRequest("GET", "/ships/info?race=coastal", nil))
	var resp2 struct {
		Texture string `json:"texture"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !strings.Contains(resp2.Texture, "coral") {
		t.Errorf("фолбек texture = %q, want из лор-файла coastal", resp2.Texture)
	}
}

// TestShipsPrompt — /ships/prompt: промпт txt2img по рецепту 2026-09-21
// (субъект + texture расы + якорь ракурса) и промпт Hi-Res; без blocked в
// позитиве.
func TestShipsPrompt(t *testing.T) {
	srv, _, _ := newShipsTestStudio(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/prompt?race=humans&seed=1", nil))
	var resp struct {
		Prompt1 string `json:"prompt1"`
		Prompt2 string `json:"prompt2"`
		Race    string `json:"race"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp.Prompt1 == "" || resp.Prompt2 == "" {
		t.Fatalf("пустые промпты: %+v", resp)
	}
	for _, want := range []string{
		"STALE texture", "dorsal three-quarter view of a single flying starship, nose pointing right",
		"plain black background", "hard-surface sci-fi game asset",
	} {
		if !strings.Contains(resp.Prompt1, want) {
			t.Errorf("prompt1 не содержит %q: %s", want, resp.Prompt1)
		}
	}
	if !strings.Contains(resp.Prompt2, "maximal detail, dense surface detail, masterpiece") {
		t.Errorf("prompt2 без хвоста Hi-Res: %s", resp.Prompt2)
	}
	for _, tok := range []string{"tentacle", "crystal", "pyramid", "obelisk", "alien"} {
		if strings.Contains(strings.ToLower(resp.Prompt1), tok) {
			t.Errorf("prompt1 содержит blocked-токен %q: %s", tok, resp.Prompt1)
		}
	}
}

// TestShipsRebuild — /ships/rebuild?race=humans: патч ships.json из лор-файла
// + reload (Server и Runner видят новую запись).
func TestShipsRebuild(t *testing.T) {
	srv, shipsPath, _ := newShipsTestStudio(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/rebuild?race=humans", nil))
	var resp struct {
		Ok      bool     `json:"ok"`
		Updated []string `json:"updated"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !resp.Ok || len(resp.Updated) != 1 {
		t.Fatalf("resp = %+v", resp)
	}
	// файл на диске обновлён (текстура из лор-файла)
	data, err := os.ReadFile(shipsPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "paneled white-grey metal hull") {
		t.Errorf("ships.json не обновлён: %s", data)
	}
	// reload: Server видит новую запись
	entry, ok := srv.shipsEntry("humans")
	if !ok || !strings.Contains(entry.Texture, "paneled white-grey metal hull") {
		t.Errorf("Server не перезагружен: %+v", entry)
	}
}

// TestShipsRebuildAll — /ships/rebuild (массово): все 60 рас пересобраны.
func TestShipsRebuildAll(t *testing.T) {
	srv, _, _ := newShipsTestStudio(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/rebuild", nil))
	var resp struct {
		Ok      bool `json:"ok"`
		Updated []struct {
			Slug string `json:"slug"`
		} `json:"updated"`
		Skipped []struct {
			Slug string `json:"slug"`
		} `json:"skipped"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("ok = false: %s", rec.Body.String())
	}
	if len(resp.Updated) != 60 {
		t.Errorf("updated = %d, want 60 (skipped %d)", len(resp.Updated), len(resp.Skipped))
	}
}

// TestShipsGen — /ships/gen: джоб стартует, кандидаты появляются в пуле.
func TestShipsGen(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	// фейковый python для кораблей (--spec split-форма cmd.exe)
	fakePy := filepath.Join(t.TempDir(), "fake_ship_python.cmd")
	script := "@echo off\r\nif \"%3\"==\"--frame-check\" goto frame\r\ncopy %2 %3 >nul 2>&1\r\nexit /b 0\r\n:frame\r\necho {\"touch\":[],\"elong\":2.0,\"ok\":true} > %5\r\nexit /b 0\r\n"
	if err := os.WriteFile(fakePy, []byte(script), 0o644); err != nil {
		t.Fatalf("WriteFile fake python: %v", err)
	}
	srv.cfg.PythonCmd = fakePy

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/gen?race=humans&n=2", nil))
	var resp struct {
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !strings.Contains(resp.Msg, "Корабли расы humans") {
		t.Fatalf("msg = %q", resp.Msg)
	}
	// ждём завершения джоба
	poolDir := filepath.Join(pool, "ships_pool")
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		st := readStatusFile(poolDir)
		if !st.Running && st.Total > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	// кандидаты + мета
	meta := readShipMetaFile(poolDir)
	if len(meta) != 2 {
		t.Errorf("meta = %d, want 2", len(meta))
	}
}

// TestShipsAct — /ships/act?what=accept: race_<slug>_NN.png + ships_meta.json.
func TestShipsAct(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	poolDir := filepath.Join(pool, "ships_pool")
	os.MkdirAll(poolDir, 0755)
	// кандидат s01.png + мета
	img := tinyPNG(t)
	if err := os.WriteFile(filepath.Join(poolDir, "s01.png"), img, 0o644); err != nil {
		t.Fatalf("WriteFile s01: %v", err)
	}
	meta := `[{"file":"s01.png","race":"humans","race_name":"Люди","seed":1,"texture":"t","prompt1":"p1","prompt2":"p2"}]`
	if err := os.WriteFile(filepath.Join(poolDir, "meta.json"), []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile meta: %v", err)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/act?file=s01.png&what=accept", nil))
	var resp struct {
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !strings.Contains(resp.Msg, "Принято: race_humans_01.png") {
		t.Fatalf("msg = %q", resp.Msg)
	}
	accDir := filepath.Join(pool, "final_accepted", "ships")
	if _, err := os.Stat(filepath.Join(accDir, "race_humans_01.png")); err != nil {
		t.Errorf("нет принятого файла: %v", err)
	}
	// ships_meta.json рядом с файлами
	sm, err := os.ReadFile(filepath.Join(accDir, "ships_meta.json"))
	if err != nil || !strings.Contains(string(sm), "race_humans_01.png") {
		t.Errorf("ships_meta.json не записан: %v", err)
	}
	// кандидат убран из пула
	if _, err := os.Stat(filepath.Join(poolDir, "s01.png")); err == nil {
		t.Errorf("кандидат остался в пуле")
	}
}

// TestRotateDegClockwise — rotateDeg(+90) = по часовой: метка «верх» уходит
// вправо (конвенция «⟳ = плюс»), слева — вниз.
func TestRotateDegClockwise(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 3, 3))
	src.Set(1, 0, color.NRGBA{R: 255, A: 255}) // верх-центр — красный
	src.Set(1, 1, color.NRGBA{A: 255})         // центр
	out := rotateDeg(src, 90)
	// холст-диагональ ceil(hypot(3,3)) = 5, центр (2,2); верх → право
	checkRGB(t, out, 3, 2, 255, 0, 0)
}

// TestRefitShipFillsLongSide — «вписать в кадр»: bbox обрезан и вписан
// длинной стороной в 200 (как у эталонных спрайтов), прозрачный фон,
// центрирование.
func TestRefitShipFillsLongSide(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 10, 10))
	for y := 2; y < 6; y++ {
		for x := 0; x < 10; x++ {
			src.Set(x, y, color.NRGBA{G: 255, A: 255})
		}
	}
	out := refitShip(src, 200)
	if out.Bounds().Dx() != 200 || out.Bounds().Dy() != 200 {
		t.Fatalf("размер = %v, want 200×200", out.Bounds())
	}
	bb := alphaBounds(out)
	if bb.Dx() != 200 {
		t.Errorf("bbox ширина = %d, want 200 (длинная сторона)", bb.Dx())
	}
	if bb.Dy() < 78 || bb.Dy() > 82 {
		t.Errorf("bbox высота = %d, want ~80 (10×4 → 200×80)", bb.Dy())
	}
	if bb.Min.Y < 55 || bb.Min.Y > 65 {
		t.Errorf("bbox Y = %d, want ~60 (центрирование)", bb.Min.Y)
	}
}

// writePoolShip — кандидат 4×2 в пуле (левая половина красная, правая синяя);
// возвращает путь файла (тесты ре-нормализации 200×200).
func writePoolShip(t *testing.T, pool, name string) string {
	t.Helper()
	poolDir := filepath.Join(pool, "ships_pool")
	if err := os.MkdirAll(poolDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	src := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			if x < 2 {
				src.Set(x, y, color.NRGBA{R: 255, A: 255})
			} else {
				src.Set(x, y, color.NRGBA{B: 255, A: 255})
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	p := filepath.Join(poolDir, name)
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return p
}

// TestShipsActFlipH — /ships/act?what=flipH: зеркало + ре-нормализация
// (красный уходит вправо, синий — влево; кадр 200×200, полный).
func TestShipsActFlipH(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	p := writePoolShip(t, pool, "s01.png")
	srv.Handler().ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest("GET", "/ships/act?file=s01.png&what=flipH", nil))
	img := loadPNG(t, p)
	if img.Bounds().Dx() != 200 || img.Bounds().Dy() != 200 {
		t.Fatalf("flipH размер = %v, want 200×200", img.Bounds())
	}
	checkRGB(t, img, 5, 100, 0, 0, 255)   // левый — синий
	checkRGB(t, img, 194, 100, 255, 0, 0) // правый — красный
}

// TestShipsActRotateOps — /ships/act?what=rot90|rot180|rotate&angle=|fit:
// файл перезаписывается на месте, после ре-нормализации кадр 200×200.
func TestShipsActRotateOps(t *testing.T) {
	for _, c := range []struct{ what, q string }{
		{"rot90", ""}, {"rot180", ""}, {"rotate", "&angle=30"}, {"fit", ""},
	} {
		srv, _, pool := newShipsTestStudio(t)
		p := writePoolShip(t, pool, "s01.png")
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/act?file=s01.png&what="+c.what+c.q, nil))
		var resp struct {
			Msg string `json:"msg"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("%s json: %v", c.what, err)
		}
		if strings.Contains(resp.Msg, "Ошибка") || resp.Msg == "?" {
			t.Errorf("%s msg = %q", c.what, resp.Msg)
		}
		img := loadPNG(t, p)
		if img.Bounds().Dx() != 200 || img.Bounds().Dy() != 200 {
			t.Errorf("%s размер = %v, want 200×200", c.what, img.Bounds())
		}
	}
	// отсутствующий файл — сообщение без правки
	srv, _, _ := newShipsTestStudio(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/act?file=nope.png&what=rotate&angle=45", nil))
	if !strings.Contains(rec.Body.String(), "?") {
		t.Errorf("msg для отсутствующего файла = %s", rec.Body.String())
	}
}

// TestShipsPreview — /ships/preview: PNG 200×200, файл пула не меняется
// (живой предпросмотр слайдера — только чтение).
func TestShipsPreview(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	p := writePoolShip(t, pool, "s01.png")
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/preview?file=s01.png&angle=30", nil))
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q", ct)
	}
	img, err := png.Decode(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if img.Bounds().Dx() != 200 || img.Bounds().Dy() != 200 {
		t.Errorf("размер = %v, want 200×200", img.Bounds())
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(before, after) {
		t.Errorf("превью изменило файл пула")
	}
}

// TestShipsAuto — /ships/auto: подсказка носа из Python-отчёта; угол
// инвертируется в конвенцию «по часовой» (−12.5), зеркало пробрасывается.
func TestShipsAuto(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	poolDir := filepath.Join(pool, "ships_pool")
	os.MkdirAll(poolDir, 0755)
	if err := os.WriteFile(filepath.Join(poolDir, "s01.png"), tinyPNG(t), 0o644); err != nil {
		t.Fatalf("WriteFile s01: %v", err)
	}
	fake := filepath.Join(t.TempDir(), "fake_orient.cmd")
	body := "@echo off\r\n" +
		"echo {\"orient\": {\"angle\": 12.5, \"mirror\": true, \"ambiguous\": false, \"reason\": \"sharpness\"}}> \"%5\"\r\n" +
		"exit /b 0\r\n"
	if err := os.WriteFile(fake, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile fake: %v", err)
	}
	srv.cfg.PythonCmd = fake
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/auto?file=s01.png", nil))
	var resp struct {
		Angle     float64 `json:"angle"`
		Mirror    bool    `json:"mirror"`
		Ambiguous bool    `json:"ambiguous"`
		Reason    string  `json:"reason"`
		Error     string  `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v (%s)", err, rec.Body.String())
	}
	if resp.Error != "" {
		t.Fatalf("error: %s", resp.Error)
	}
	if resp.Angle != -12.5 {
		t.Errorf("angle = %v, want -12.5 (по часовой)", resp.Angle)
	}
	if !resp.Mirror {
		t.Errorf("mirror = false, want true")
	}
	if resp.Reason != "sharpness" {
		t.Errorf("reason = %q", resp.Reason)
	}
}

// TestShipsAutoTimeout — зависший python не подвешивает /ships/auto: по
// таймауту возвращается внятная ошибка, а не бесконечное ожидание.
func TestShipsAutoTimeout(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	poolDir := filepath.Join(pool, "ships_pool")
	os.MkdirAll(poolDir, 0755)
	if err := os.WriteFile(filepath.Join(poolDir, "s01.png"), tinyPNG(t), 0o644); err != nil {
		t.Fatalf("WriteFile s01: %v", err)
	}
	fake := filepath.Join(t.TempDir(), "slow_orient.cmd")
	// >nul 2>&1 — чтобы внук (ping) не держал пайп CombinedOutput открытым
	// после убийства cmd.exe по таймауту.
	body := "@echo off\r\nping -n 4 127.0.0.1 >nul 2>&1\r\nexit /b 0\r\n"
	if err := os.WriteFile(fake, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile fake: %v", err)
	}
	old := shipOrientTimeout
	shipOrientTimeout = 300 * time.Millisecond
	defer func() { shipOrientTimeout = old }()
	srv.cfg.PythonCmd = fake
	rec := httptest.NewRecorder()
	start := time.Now()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/auto?file=s01.png", nil))
	var resp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v (%s)", err, rec.Body.String())
	}
	if resp.Error == "" {
		t.Fatalf("error пусто, want таймаут")
	}
	if !strings.Contains(resp.Error, "таймаут") {
		t.Errorf("error = %q, want таймаут", resp.Error)
	}
	if d := time.Since(start); d > 4*time.Second {
		t.Errorf("ответ вернулся через %s — таймаут не сработал", d)
	}
}

// TestShipsAcceptPicksOwnMeta — приёмка берёт мету ИМЕННО принятого файла,
// а не последнего в пуле (go 1.21: `&m` в `for _, m := range` — общая
// переменная цикла; при нескольких кандидатах писался чужой race/seed).
func TestShipsAcceptPicksOwnMeta(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	poolDir := filepath.Join(pool, "ships_pool")
	os.MkdirAll(poolDir, 0755)
	for _, f := range []string{"s01.png", "s02.png"} {
		if err := os.WriteFile(filepath.Join(poolDir, f), tinyPNG(t), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", f, err)
		}
	}
	meta := `[` +
		`{"file":"s01.png","race":"humans","race_name":"Люди","seed":11,"prompt1":"p1","prompt2":"p2"},` +
		`{"file":"s02.png","race":"coastal","race_name":"Прибрежные","seed":22,"prompt1":"p3","prompt2":"p4"}` +
		`]`
	if err := os.WriteFile(filepath.Join(poolDir, "meta.json"), []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile meta: %v", err)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/act?file=s01.png&what=accept", nil))
	var resp struct {
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !strings.Contains(resp.Msg, "Принято: race_humans_01.png") {
		t.Fatalf("msg = %q, want race_humans_01.png (мета своего файла, не последнего)", resp.Msg)
	}
	sm, err := os.ReadFile(filepath.Join(pool, "final_accepted", "ships", "ships_meta.json"))
	if err != nil {
		t.Fatalf("ReadFile ships_meta: %v", err)
	}
	var items []struct {
		File string `json:"file"`
		Race string `json:"race"`
		Seed int64  `json:"seed"`
	}
	if err := json.Unmarshal(sm, &items); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(items) != 1 || items[0].Race != "humans" || items[0].Seed != 11 {
		t.Errorf("запись = %+v, want humans/11", items)
	}
}

// TestShipsAcceptTransform — приёмка пишет в ships_meta.json накопленный
// угол/зеркало (зеркало меняет знак угла) и дату.
func TestShipsAcceptTransform(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	poolDir := filepath.Join(pool, "ships_pool")
	os.MkdirAll(poolDir, 0755)
	if err := os.WriteFile(filepath.Join(poolDir, "s01.png"), tinyPNG(t), 0o644); err != nil {
		t.Fatalf("WriteFile s01: %v", err)
	}
	meta := `[{"file":"s01.png","race":"humans","race_name":"Люди","seed":1,"prompt1":"p1","prompt2":"p2"}]`
	if err := os.WriteFile(filepath.Join(poolDir, "meta.json"), []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile meta: %v", err)
	}
	for _, u := range []string{
		"/ships/act?file=s01.png&what=rotate&angle=30",
		"/ships/act?file=s01.png&what=flipH",
		"/ships/act?file=s01.png&what=accept",
	} {
		srv.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", u, nil))
	}
	sm, err := os.ReadFile(filepath.Join(pool, "final_accepted", "ships", "ships_meta.json"))
	if err != nil {
		t.Fatalf("ReadFile ships_meta: %v", err)
	}
	var items []struct {
		File  string  `json:"file"`
		Angle float64 `json:"angle"`
		Flip  bool    `json:"flip"`
		Date  string  `json:"date"`
	}
	if err := json.Unmarshal(sm, &items); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].Angle != -30 {
		t.Errorf("angle = %v, want -30 (зеркало меняет знак)", items[0].Angle)
	}
	if !items[0].Flip {
		t.Errorf("flip = false, want true")
	}
	if items[0].Date == "" {
		t.Errorf("date пусто")
	}
}

// TestShipsAccepted — /ships/accepted?race=: счётчик принятых по расе.
func TestShipsAccepted(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	accDir := filepath.Join(pool, "final_accepted", "ships")
	os.MkdirAll(accDir, 0755)
	reg := `[{"file":"race_humans_01.png","race":"humans"},{"file":"race_coastal_01.png","race":"coastal"}]`
	if err := os.WriteFile(filepath.Join(accDir, "ships_meta.json"), []byte(reg), 0o644); err != nil {
		t.Fatalf("WriteFile ships_meta: %v", err)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/accepted?race=humans", nil))
	var resp struct {
		Race     string `json:"race"`
		Accepted int    `json:"accepted"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp.Race != "humans" || resp.Accepted != 1 {
		t.Errorf("resp = %+v, want humans/1", resp)
	}
}

// loadPNG — декодировать PNG-файл (тесты правки ориентации).
func loadPNG(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open %s: %v", path, err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("Decode %s: %v", path, err)
	}
	return img
}

// checkRGB — пиксель (x,y) равен (r,g,b) с допуском 2 (конверсия цветов).
func checkRGB(t *testing.T, img image.Image, x, y int, r, g, b uint8) {
	t.Helper()
	cr, cg, cb, _ := img.At(x, y).RGBA()
	got := [3]uint8{uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8)}
	want := [3]uint8{r, g, b}
	for i := range got {
		d := int(got[i]) - int(want[i])
		if d < 0 {
			d = -d
		}
		if d > 2 {
			t.Errorf("пиксель(%d,%d) = %v, want %v", x, y, got, want)
			return
		}
	}
}

// TestShipsGenParams — /ships/gen: tags/override/size доходят до генерации
// (мета кандидатов = override, Size = 100).
func TestShipsGenParams(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	fakePy := filepath.Join(t.TempDir(), "fake_ship_python.cmd")
	script := "@echo off\r\nif \"%3\"==\"--frame-check\" goto frame\r\ncopy %2 %3 >nul 2>&1\r\nexit /b 0\r\n:frame\r\necho {\"touch\":[],\"elong\":2.0,\"ok\":true} > %5\r\nexit /b 0\r\n"
	if err := os.WriteFile(fakePy, []byte(script), 0o644); err != nil {
		t.Fatalf("WriteFile fake python: %v", err)
	}
	srv.cfg.PythonCmd = fakePy

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/gen?race=humans&n=1&tags=extra+tag&prompt1_override=MANUAL1&prompt2_override=MANUAL2&size=100", nil))
	var resp struct {
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !strings.Contains(resp.Msg, "Корабли расы humans") {
		t.Fatalf("msg = %q", resp.Msg)
	}
	poolDir := filepath.Join(pool, "ships_pool")
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		st := readStatusFile(poolDir)
		if !st.Running && st.Total > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	meta := readShipMetaFull(poolDir)
	if len(meta) != 1 {
		t.Fatalf("meta = %d, want 1", len(meta))
	}
	if meta[0].Prompt1 != "MANUAL1" || meta[0].Prompt2 != "MANUAL2" {
		t.Errorf("override не дошёл: %q / %q", meta[0].Prompt1, meta[0].Prompt2)
	}
	if meta[0].Size != 100 {
		t.Errorf("Size = %d, want 100", meta[0].Size)
	}
}

// TestShipsVote — /ships/vote: like/dislike/clear пишут вердикт в meta.json
// пула (файл кандидата не перемещается, 98c).
func TestShipsVote(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	poolDir := filepath.Join(pool, "ships_pool")
	os.MkdirAll(poolDir, 0755)
	img := tinyPNG(t)
	if err := os.WriteFile(filepath.Join(poolDir, "s01.png"), img, 0o644); err != nil {
		t.Fatalf("WriteFile s01: %v", err)
	}
	meta := `[{"file":"s01.png","race":"humans","race_name":"Люди","seed":1,"texture":"t","prompt1":"p1","prompt2":"p2"}]`
	if err := os.WriteFile(filepath.Join(poolDir, "meta.json"), []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile meta: %v", err)
	}

	// like
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/vote?file=s01.png&vote=like", nil))
	var resp struct {
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !strings.Contains(resp.Msg, "Нравится") {
		t.Fatalf("msg = %q", resp.Msg)
	}
	m := readShipMetaFull(poolDir)
	if len(m) != 1 || m[0].Vote != "like" {
		t.Errorf("vote = %+v, want like", m)
	}
	// файл не перемещён
	if _, err := os.Stat(filepath.Join(poolDir, "s01.png")); err != nil {
		t.Errorf("кандидат перемещён: %v", err)
	}

	// dislike
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, httptest.NewRequest("GET", "/ships/vote?file=s01.png&vote=dislike", nil))
	m2 := readShipMetaFull(poolDir)
	if len(m2) != 1 || m2[0].Vote != "dislike" {
		t.Errorf("vote = %+v, want dislike", m2)
	}

	// clear
	rec3 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec3, httptest.NewRequest("GET", "/ships/vote?file=s01.png&vote=clear", nil))
	m3 := readShipMetaFull(poolDir)
	if len(m3) != 1 || m3[0].Vote != "" {
		t.Errorf("vote = %+v, want пусто", m3)
	}

	// неизвестный файл — ошибка
	rec4 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec4, httptest.NewRequest("GET", "/ships/vote?file=nope.png&vote=like", nil))
	var resp4 struct {
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(rec4.Body.Bytes(), &resp4); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !strings.Contains(resp4.Msg, "нет кандидата") {
		t.Errorf("msg = %q, want нет кандидата", resp4.Msg)
	}
}

// readShipMetaFull — полная мета пула кораблей (для тестов).
func readShipMetaFull(poolDir string) []struct {
	File    string `json:"file"`
	Prompt1 string `json:"prompt1"`
	Prompt2 string `json:"prompt2"`
	Size    int    `json:"size"`
	Vote    string `json:"vote"`
} {
	var out []struct {
		File    string `json:"file"`
		Prompt1 string `json:"prompt1"`
		Prompt2 string `json:"prompt2"`
		Size    int    `json:"size"`
		Vote    string `json:"vote"`
	}
	data, err := os.ReadFile(filepath.Join(poolDir, "meta.json"))
	if err != nil {
		return out
	}
	json.Unmarshal(data, &out)
	return out
}

// readStatusFile — статус пула (для тестов).
func readStatusFile(poolDir string) struct {
	Running bool `json:"running"`
	Total   int  `json:"total"`
} {
	var st struct {
		Running bool `json:"running"`
		Total   int  `json:"total"`
	}
	data, err := os.ReadFile(filepath.Join(poolDir, "status.json"))
	if err != nil {
		return st
	}
	json.Unmarshal(data, &st)
	return st
}

// readShipMetaFile — мета пула кораблей (для тестов).
func readShipMetaFile(poolDir string) []struct {
	File string `json:"file"`
} {
	var out []struct {
		File string `json:"file"`
	}
	data, err := os.ReadFile(filepath.Join(poolDir, "meta.json"))
	if err != nil {
		return out
	}
	json.Unmarshal(data, &out)
	return out
}

// tinyPNG — байты валидного PNG 1×1 (для тестов приёмки).
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	var buf strings.Builder
	_ = buf
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return b.Bytes()
}