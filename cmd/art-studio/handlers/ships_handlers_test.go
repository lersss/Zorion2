package handlers

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"zorion/cmd/art-studio/config"
	"zorion/cmd/art-studio/generator"
	"zorion/internal/models"
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
	srv.shipRegistryPath = "../../../config/ships_registry.json"
	srv.acceptedShipsDirPath = filepath.Join(pool, "final_accepted", "ships")
	return srv, shipsPath, pool
}

// newShipsTestStudioTyped — как newShipsTestStudio, но humans с типами корабля
// (starship/cruiser) — для тестов контракта «раса → список типов».
func newShipsTestStudioTyped(t *testing.T) (*Server, string, string) {
	t.Helper()
	srv, shipsPath, pool := newShipsTestStudio(t)
	shipsData := []byte(`{
  "humans": {
    "race_name": "Люди",
    "family": "F0",
    "texture": "STALE texture",
    "silhouette": "крыло-корпус в плане: широкий нос справа",
    "blocked": ["tentacle", "organic"],
    "types": [
      {"type": "starship", "texture": "STALE texture"},
      {"type": "cruiser", "texture": "cruiser material"}
    ]
  }
}`)
	if err := os.WriteFile(shipsPath, shipsData, 0o644); err != nil {
		t.Fatalf("WriteFile ships.json: %v", err)
	}
	ships, err := config.ParseShips(shipsData, shipsPath, "../../../config/races.json")
	if err != nil {
		t.Fatalf("ParseShips: %v", err)
	}
	srv.SetShips(ships, srv.shipDictRef(), shipsPath)
	srv.runner.SetShips(ships, srv.shipDictRef(), "../../../config/races.json")
	return srv, shipsPath, pool
}

// writeShipFakePy — фейковый python кораблей (frame-check + копия выреза,
// helper-процесс) для handler-тестов генерации.
func writeShipFakePy(t *testing.T) string {
	t.Helper()
	t.Setenv(fakePythonEnv, "ship")
	t.Setenv(fakePythonFrameEnv, "ok")
	return fakePythonCmd()
}

// waitShipsJob — дождаться завершения джоба пула кораблей.
func waitShipsJob(t *testing.T, poolDir string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		st := readStatusFile(poolDir)
		if !st.Running && st.Total > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestShipsInfoTypes — /ships/info?race=humans&type=cruiser: поля выбранного
// типа + список типов; неизвестный тип — ошибка.
func TestShipsInfoTypes(t *testing.T) {
	srv, _, _ := newShipsTestStudioTyped(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/info?race=humans&type=cruiser", nil))
	var resp struct {
		Type    string   `json:"type"`
		Types   []string `json:"types"`
		Texture string   `json:"texture"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp.Type != "cruiser" || resp.Texture != "cruiser material" {
		t.Errorf("info = %+v, want cruiser/cruiser material", resp)
	}
	if len(resp.Types) != 2 || resp.Types[0] != "starship" || resp.Types[1] != "cruiser" {
		t.Errorf("types = %v, want [starship cruiser]", resp.Types)
	}
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, httptest.NewRequest("GET", "/ships/info?race=humans&type=nope", nil))
	if !strings.Contains(rec2.Body.String(), "нет типа nope") {
		t.Errorf("info nope = %s", rec2.Body.String())
	}
}

// TestShipsPromptType — /ships/prompt?race=humans&type=cruiser: промпт с texture
// выбранного типа.
func TestShipsPromptType(t *testing.T) {
	srv, _, _ := newShipsTestStudioTyped(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/prompt?race=humans&type=cruiser&seed=1", nil))
	var resp struct {
		Prompt1 string `json:"prompt1"`
		Type    string `json:"type"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp.Type != "cruiser" || !strings.Contains(resp.Prompt1, "cruiser material") {
		t.Errorf("prompt = %+v, want cruiser", resp)
	}
}

// TestShipsGenType — /ships/gen?race=humans&type=cruiser: генерируется тип, в
// мете пула — Type.
func TestShipsGenType(t *testing.T) {
	srv, _, pool := newShipsTestStudioTyped(t)
	srv.cfg.PythonCmd = writeShipFakePy(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/gen?race=humans&type=cruiser&n=1", nil))
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
	waitShipsJob(t, poolDir)
	meta := readShipMetaFull(poolDir)
	if len(meta) != 1 {
		t.Fatalf("meta = %d, want 1", len(meta))
	}
	if !strings.Contains(string(mustRead(t, filepath.Join(poolDir, "meta.json"))), `"type":"cruiser"`) {
		t.Errorf("мета без type=cruiser: %s", mustRead(t, filepath.Join(poolDir, "meta.json")))
	}
}

// mustRead — прочитать файл или упасть (тесты).
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	return data
}

// TestShipsGenKeep — /ships/gen?keep=1: пул НЕ чистится, кандидат копится;
// без keep — пул чистится (обратная совместимость).
func TestShipsGenKeep(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	srv.cfg.PythonCmd = writeShipFakePy(t)
	poolDir := filepath.Join(pool, "ships_pool")
	os.MkdirAll(poolDir, 0755)
	// имитация прошлого прогона: кандидат s01.png + мета
	if err := os.WriteFile(filepath.Join(poolDir, "s01.png"), tinyPNG(t), 0o644); err != nil {
		t.Fatalf("WriteFile s01: %v", err)
	}
	meta := `[{"file":"s01.png","race":"humans","race_name":"Люди","seed":1}]`
	if err := os.WriteFile(filepath.Join(poolDir, "meta.json"), []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile meta: %v", err)
	}
	srv.Handler().ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest("GET", "/ships/gen?race=humans&n=1&keep=1", nil))
	waitShipsJob(t, poolDir)
	if _, err := os.Stat(filepath.Join(poolDir, "s01.png")); err != nil {
		t.Errorf("keep=1: предыдущий s01.png стёрт: %v", err)
	}
	if m := readShipMetaFull(poolDir); len(m) != 2 {
		t.Errorf("keep=1: meta = %d, want 2 (накопление)", len(m))
	}
}

// TestShipsAcceptTyped — приёмка расы с типом корабля: race_<slug>_<type>.png.
func TestShipsAcceptTyped(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	poolDir := filepath.Join(pool, "ships_pool")
	os.MkdirAll(poolDir, 0755)
	if err := os.WriteFile(filepath.Join(poolDir, "s01.png"), tinyPNG(t), 0o644); err != nil {
		t.Fatalf("WriteFile s01: %v", err)
	}
	meta := `[{"file":"s01.png","race":"humans","type":"starship","race_name":"Люди","seed":1}]`
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
	if !strings.Contains(resp.Msg, "Принято: race_humans_starship.png") {
		t.Fatalf("msg = %q, want race_humans_starship.png", resp.Msg)
	}
	accDir := filepath.Join(pool, "final_accepted", "ships")
	if _, err := os.Stat(filepath.Join(accDir, "race_humans_starship.png")); err != nil {
		t.Errorf("нет принятого файла: %v", err)
	}
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

// TestShipsPrompt — /ships/prompt: промпт txt2img по рецепту 2026-09-22
// (субъект + texture расы + якорь ракурса + чёрный фон) и промпт Hi-Res; без
// blocked в позитиве.
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
		"pure flat black background", "hard-surface sci-fi game asset",
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
	srv.cfg.PythonCmd = writeShipFakePy(t)

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

// writeShipPoolMeta — meta.json пула кораблей с одной записью (кандидат sNN).
// Нужна действиям ориентации: они пишут пару (A, F) в мету (спека §4.1).
func writeShipPoolMeta(t *testing.T, poolDir, file string) {
	t.Helper()
	meta := `[{"file":"` + file + `","race":"humans","race_name":"Люди","seed":1,"texture":"t","prompt1":"p1","prompt2":"p2"}]`
	if err := os.WriteFile(filepath.Join(poolDir, "meta.json"), []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile meta: %v", err)
	}
}

// TestShipsActOrientDoesNotTouchPixels — действия ориентации пишут пару (A, F)
// в мету пула и НЕ трогают пиксели файла (спека §4.1, §9 п.1): sha256 кандидата
// до == после для rotate/rot90/rot180/flipH.
func TestShipsActOrientDoesNotTouchPixels(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	poolDir := filepath.Join(pool, "ships_pool")
	p := writePoolShip(t, pool, "s01.png")
	writeShipPoolMeta(t, poolDir, "s01.png")
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, u := range []string{
		"/ships/act?file=s01.png&what=rotate&angle=37",
		"/ships/act?file=s01.png&what=rot90",
		"/ships/act?file=s01.png&what=rot180",
		"/ships/act?file=s01.png&what=flipH",
	} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", u, nil))
		if !strings.Contains(rec.Body.String(), "s01.png") {
			t.Errorf("%s: ответ без имени файла: %s", u, rec.Body.String())
		}
	}
	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("действия ориентации изменили пиксели файла")
	}
	// итоговая пара: (0,false)+37 → +90 → +180 → flipH: (53, true)
	a, f, ok := generator.ShipOrientOf(poolDir, "s01.png")
	if !ok || a != 53 || !f {
		t.Errorf("пара = (%v,%v), want (53,true)", a, f)
	}
}

// TestShipsActFlipAccumulation — накопление пары с wrap (§3.1, §9 п.2):
// (0,false) → rotate(+30) → flipH = (−30, true); продолжение rotate(+60) =
// (+30, true) — после зеркала угол накапливается в новом знаке; rotate(+200)
// из (0,false) = −160.
func TestShipsActFlipAccumulation(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	poolDir := filepath.Join(pool, "ships_pool")
	writePoolShip(t, pool, "s01.png")
	writeShipPoolMeta(t, poolDir, "s01.png")
	act := func(u string) {
		t.Helper()
		srv.Handler().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", u, nil))
	}
	act("/ships/act?file=s01.png&what=rotate&angle=30")
	act("/ships/act?file=s01.png&what=flipH")
	if a, f, _ := generator.ShipOrientOf(poolDir, "s01.png"); a != -30 || !f {
		t.Errorf("rotate(+30)+flipH = (%v,%v), want (-30,true)", a, f)
	}
	act("/ships/act?file=s01.png&what=rotate&angle=60")
	if a, f, _ := generator.ShipOrientOf(poolDir, "s01.png"); a != 30 || !f {
		t.Errorf("rotate(+60) после зеркала = (%v,%v), want (30,true)", a, f)
	}
	// wrap: отдельный кандидат с пары (0,false)
	srv2, _, pool2 := newShipsTestStudio(t)
	poolDir2 := filepath.Join(pool2, "ships_pool")
	writePoolShip(t, pool2, "s01.png")
	writeShipPoolMeta(t, poolDir2, "s01.png")
	srv2.Handler().ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest("GET", "/ships/act?file=s01.png&what=rotate&angle=200", nil))
	if a, f, _ := generator.ShipOrientOf(poolDir2, "s01.png"); a != -160 || f {
		t.Errorf("rotate(+200) = (%v,%v), want (-160,false)", a, f)
	}
}

// TestShipsActSetAngle — what=setangle абсолютный (−180 → 180 по конвенции
// (−180,180], §3.1), зеркало сохраняется.
func TestShipsActSetAngle(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	poolDir := filepath.Join(pool, "ships_pool")
	writePoolShip(t, pool, "s01.png")
	writeShipPoolMeta(t, poolDir, "s01.png")
	srv.Handler().ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest("GET", "/ships/act?file=s01.png&what=setangle&angle=-180", nil))
	if a, _, _ := generator.ShipOrientOf(poolDir, "s01.png"); a != 180 {
		t.Errorf("setangle -180 → %v, want 180", a)
	}
	srv.Handler().ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest("GET", "/ships/act?file=s01.png&what=flipH", nil))
	srv.Handler().ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest("GET", "/ships/act?file=s01.png&what=setangle&angle=25", nil))
	if a, f, _ := generator.ShipOrientOf(poolDir, "s01.png"); a != 25 || !f {
		t.Errorf("setangle 25 после flipH = (%v,%v), want (25,true)", a, f)
	}
}

// TestShipsActAutoAppliesHint — what=auto пишет пару по подсказке (§3.3),
// идемпотентен; GET /ships/auto отдаёт ту же пару (§9 п.3).
func TestShipsActAutoAppliesHint(t *testing.T) {
	for _, c := range []struct {
		name      string
		angle     float64
		mirror    bool
		ambiguous bool
		wantA     float64
		wantF     bool
	}{
		{"v_gt1_no_mirror", 12.5, false, false, -12.5, false},
		{"v_gt1_mirror", 12.5, true, false, 12.5, true},
		{"v_le1_no_mirror", 0.5, false, false, 0, false},
		{"v_le1_mirror", 0.5, true, false, 0, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv, _, pool := newShipsTestStudio(t)
			poolDir := filepath.Join(pool, "ships_pool")
			os.MkdirAll(poolDir, 0755)
			if err := os.WriteFile(filepath.Join(poolDir, "s01.png"), tinyPNG(t), 0o644); err != nil {
				t.Fatalf("WriteFile s01: %v", err)
			}
			writeShipPoolMeta(t, poolDir, "s01.png")
			srv.cfg.PythonCmd = writeOrientFake(t, c.angle, c.mirror, c.ambiguous)
			srv.Handler().ServeHTTP(httptest.NewRecorder(),
				httptest.NewRequest("GET", "/ships/act?file=s01.png&what=auto", nil))
			a, f, ok := generator.ShipOrientOf(poolDir, "s01.png")
			if !ok || a != c.wantA || f != c.wantF {
				t.Fatalf("what=auto пара = (%v,%v), want (%v,%v)", a, f, c.wantA, c.wantF)
			}
			// идемпотентность: повторный auto даёт ту же пару
			srv.Handler().ServeHTTP(httptest.NewRecorder(),
				httptest.NewRequest("GET", "/ships/act?file=s01.png&what=auto", nil))
			if a2, f2, _ := generator.ShipOrientOf(poolDir, "s01.png"); a2 != a || f2 != f {
				t.Errorf("auto не идемпотентен: (%v,%v) → (%v,%v)", a, f, a2, f2)
			}
			// GET /ships/auto отдаёт ту же пару
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/auto?file=s01.png", nil))
			var resp struct {
				Angle float64 `json:"angle"`
				Flip  bool    `json:"flip"`
				Error string  `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("json: %v (%s)", err, rec.Body.String())
			}
			if resp.Error != "" {
				t.Fatalf("error: %s", resp.Error)
			}
			if resp.Angle != a || resp.Flip != f {
				t.Errorf("/ships/auto = (%v,%v), want (%v,%v)", resp.Angle, resp.Flip, a, f)
			}
		})
	}
}

// writeOrientFake — фейковый python, отдающий отчёт profile_orientation
// (--orient-only --report <json>) с заданными angle/mirror/ambiguous
// (helper-процесс).
func writeOrientFake(t *testing.T, angle float64, mirror, ambiguous bool) string {
	t.Helper()
	inner := `{"angle": ` + strconv.FormatFloat(angle, 'g', -1, 64) +
		`, "mirror": ` + strconv.FormatBool(mirror) +
		`, "ambiguous": ` + strconv.FormatBool(ambiguous) +
		`, "reason": "sharpness"}`
	t.Setenv(fakePythonEnv, "orient")
	t.Setenv(fakePythonOrientEnv, inner)
	return fakePythonCmd()
}

// writeSlowOrientFake — «зависший» python (helper-процесс спит дольше
// shipOrientTimeout): проверка таймаута /ships/auto.
func writeSlowOrientFake(t *testing.T) string {
	t.Helper()
	t.Setenv(fakePythonEnv, "slow")
	return fakePythonCmd()
}

// TestShipsAcceptKeepsPixels — приёмка копирует файл байт-в-байт (действия
// ориентации его не тронули), пишет пару/дату и маркер orient_meta (§4.4, §9 п.4).
func TestShipsAcceptKeepsPixels(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	poolDir := filepath.Join(pool, "ships_pool")
	p := writePoolShip(t, pool, "s01.png")
	writeShipPoolMeta(t, poolDir, "s01.png")
	srv.Handler().ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest("GET", "/ships/act?file=s01.png&what=rotate&angle=37", nil))
	srv.Handler().ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest("GET", "/ships/act?file=s01.png&what=flipH", nil))
	cand, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile кандидата: %v", err)
	}
	srv.Handler().ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest("GET", "/ships/act?file=s01.png&what=accept", nil))
	accDir := filepath.Join(pool, "final_accepted", "ships")
	accepted, err := os.ReadFile(filepath.Join(accDir, "race_humans_01.png"))
	if err != nil {
		t.Fatalf("ReadFile принятого: %v", err)
	}
	if !bytes.Equal(cand, accepted) {
		t.Errorf("принятый файл != кандидат (пиксели изменились)")
	}
	sm, err := os.ReadFile(filepath.Join(accDir, "ships_meta.json"))
	if err != nil {
		t.Fatalf("ReadFile ships_meta: %v", err)
	}
	var items []struct {
		Angle      float64 `json:"angle"`
		Flip       bool    `json:"flip"`
		Date       string  `json:"date"`
		OrientMeta bool    `json:"orient_meta"`
	}
	if err := json.Unmarshal(sm, &items); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(items) != 1 || items[0].Angle != -37 || !items[0].Flip || items[0].Date == "" || !items[0].OrientMeta {
		t.Errorf("запись = %+v, want angle -37, flip true, date, orient_meta true", items)
	}
	if _, err := os.Stat(p); err == nil {
		t.Errorf("кандидат остался в пуле")
	}
}

// TestShipsAcceptRejectsSketch — эскиз (size: 100) не принимается: сообщение
// без создания файла (§4.4, §9 п.5).
func TestShipsAcceptRejectsSketch(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	poolDir := filepath.Join(pool, "ships_pool")
	os.MkdirAll(poolDir, 0755)
	if err := os.WriteFile(filepath.Join(poolDir, "s01.png"), tinyPNG(t), 0o644); err != nil {
		t.Fatalf("WriteFile s01: %v", err)
	}
	meta := `[{"file":"s01.png","race":"humans","race_name":"Люди","seed":1,"size":100}]`
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
	if !strings.Contains(resp.Msg, "эскиз") {
		t.Errorf("msg = %q, want про эскиз", resp.Msg)
	}
	if _, err := os.Stat(filepath.Join(pool, "final_accepted", "ships", "race_humans_01.png")); err == nil {
		t.Errorf("эскиз принят — файл создан")
	}
}

// TestShipsFitStillNormalizes — what=fit (единственное действие, перезаписывающее
// файл): crop по bbox + вписывание в 200×200, длинная сторона = 200 (§9 п.6).
func TestShipsFitStillNormalizes(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	p := writePoolShip(t, pool, "s01.png")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/act?file=s01.png&what=fit", nil))
	var resp struct {
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if strings.Contains(resp.Msg, "Ошибка") || resp.Msg == "?" {
		t.Errorf("msg = %q", resp.Msg)
	}
	img := loadPNG(t, p)
	if img.Bounds().Dx() != 200 || img.Bounds().Dy() != 200 {
		t.Fatalf("размер = %v, want 200×200", img.Bounds())
	}
	if bb := alphaBounds(img); bb.Dx() != 200 {
		t.Errorf("bbox ширина = %d, want 200 (длинная сторона)", bb.Dx())
	}
}

// TestShipsListCarriesOrient — GET /ships/list несёт пару (A, F) для превью
// пула (§4.2, §9 п.16).
func TestShipsListCarriesOrient(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	poolDir := filepath.Join(pool, "ships_pool")
	writePoolShip(t, pool, "s01.png")
	writeShipPoolMeta(t, poolDir, "s01.png")
	srv.Handler().ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest("GET", "/ships/act?file=s01.png&what=rotate&angle=37", nil))
	srv.Handler().ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest("GET", "/ships/act?file=s01.png&what=flipH", nil))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/list", nil))
	var items []struct {
		File  string  `json:"file"`
		Angle float64 `json:"angle"`
		Flip  bool    `json:"flip"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(items) != 1 || items[0].File != "s01.png" {
		t.Fatalf("items = %+v, want 1×s01.png", items)
	}
	if items[0].Angle != -37 || !items[0].Flip {
		t.Errorf("пара = (%v,%v), want (-37,true)", items[0].Angle, items[0].Flip)
	}
}

// TestShipsPreviewGone — роут /ships/preview удалён (предпросмотр — CSS, §4.5).
func TestShipsPreviewGone(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	writePoolShip(t, pool, "s01.png")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/preview?file=s01.png&angle=30", nil))
	if rec.Code != 404 {
		t.Errorf("code = %d, want 404 (роут удалён)", rec.Code)
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
	fake := writeSlowOrientFake(t)
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

// TestShipsGenParams — /ships/gen: tags/override/size доходят до генерации
// (мета кандидатов = override, Size = 100).
func TestShipsGenParams(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	srv.cfg.PythonCmd = writeShipFakePy(t)

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

// testRegistryShips — минимальный валидный реестр для тестов витрины/удаления:
// людской блок (default + _02), два корабля расы coastal, нейтральный последним.
func testRegistryShips() []models.ShipSprite {
	return []models.ShipSprite{
		{ID: "race_humans_starship", Name: "Люди · 1", File: "race_humans_starship.png", Race: "humans", Angle: 21.1},
		{ID: "race_humans_starship_02", Name: "Люди · 2", File: "race_humans_starship_02.png", Race: "humans"},
		{ID: "race_coastal_01", Name: "Прибрежные · 1", File: "race_coastal_01.png", Race: "coastal"},
		{ID: "race_coastal_02", Name: "Прибрежные · 2", File: "race_coastal_02.png", Race: "coastal"},
		{ID: "neutral", Name: "Нейтральный", File: "neutral.png", Race: ""},
	}
}

// writeTestShipRegistry — записать temp-реестр кораблей (валидный формат).
func writeTestShipRegistry(t *testing.T, ships []models.ShipSprite) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ships_registry.json")
	data, err := json.Marshal(struct {
		Version int                 `json:"version"`
		Ships   []models.ShipSprite `json:"ships"`
	}{Version: 1, Ships: ships})
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("WriteFile реестра: %v", err)
	}
	return p
}

// setupDeleteStudio — Server с temp-реестром, temp-папкой спрайтов игры и
// temp-папкой приёмки; возвращает Server и оба каталога.
func setupDeleteStudio(t *testing.T, ships []models.ShipSprite) (*Server, string, string) {
	t.Helper()
	srv, _, _ := newShipsTestStudio(t)
	srv.shipRegistryPath = writeTestShipRegistry(t, ships)
	srv.gameSpritesDirPath = t.TempDir()
	srv.acceptedShipsDirPath = t.TempDir()
	return srv, srv.gameSpritesDirPath, srv.acceptedShipsDirPath
}

// TestShipsIngame — GET /ships/ingame: ответ 1:1 с temp-реестром (состав/порядок
// из файла, нет жёстких чисел), race_name из config/races.json; angle/flip
// присутствуют в JSON даже при нулевых значениях (своя DTO, omitempty модели
// не просачивается). Data-agnostic (M1).
func TestShipsIngame(t *testing.T) {
	srv, _, _ := newShipsTestStudio(t)
	ships := testRegistryShips()
	srv.shipRegistryPath = writeTestShipRegistry(t, ships)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/ingame", nil))
	// указатели: отличают «поле есть со значением 0» от «поля нет» (omitempty)
	var items []struct {
		File     string   `json:"file"`
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		Race     string   `json:"race"`
		RaceName string   `json:"race_name"`
		Angle    *float64 `json:"angle"`
		Flip     *bool    `json:"flip"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(items) != len(ships) {
		t.Fatalf("items = %d, want %d", len(items), len(ships))
	}
	raceNames := loadRaceNames("../../../config/races.json")
	for i, want := range ships {
		got := items[i]
		if got.File != want.File || got.ID != want.ID || got.Name != want.Name || got.Race != want.Race {
			t.Errorf("items[%d] = %+v, want %+v", i, got, want)
		}
		if got.RaceName != raceNames[want.Race] {
			t.Errorf("items[%d].RaceName = %q, want %q", i, got.RaceName, raceNames[want.Race])
		}
		if got.Angle == nil || *got.Angle != want.Angle {
			t.Errorf("items[%d].Angle = %v, want %v (обязательное поле)", i, got.Angle, want.Angle)
		}
		if got.Flip == nil || *got.Flip != want.Flip {
			t.Errorf("items[%d].Flip = %v, want %v (обязательное поле)", i, got.Flip, want.Flip)
		}
	}
	// нулевая запись: angle/flip всё равно в JSON — иначе клиент не отличит «0°» от «поля нет»
	zero := items[1]
	if zero.ID != "race_humans_starship_02" {
		t.Fatalf("items[1].ID = %q, want race_humans_starship_02", zero.ID)
	}
	if zero.Angle == nil || *zero.Angle != 0 || zero.Flip == nil || *zero.Flip {
		t.Errorf("items[1] = %+v, want angle 0/flip false (обязательные поля)", zero)
	}
}

// TestShipsIngameDelete — POST /ships/ingame/delete: удаляются все четыре
// артефакта; мета/accepted связываются с игровым PNG по sha256 при разных
// именах (общее содержимое); ответ с пересчётом и steps.accepted; витрина
// сразу читает актуальный файл (без пересборки).
func TestShipsIngameDelete(t *testing.T) {
	srv, sprites, acc := setupDeleteStudio(t, testRegistryShips())
	png := tinyPNG(t)
	if err := os.WriteFile(filepath.Join(sprites, "race_coastal_02.png"), png, 0o644); err != nil {
		t.Fatalf("WriteFile game PNG: %v", err)
	}
	// принятые с ДРУГИМ именем, но тем же содержимым (связывание по sha256),
	// и дубль того же sha256 — удаляются все совпадения.
	for _, f := range []string{"race_coastal_2_accept.png", "race_coastal_2b_accept.png"} {
		if err := os.WriteFile(filepath.Join(acc, f), png, 0o644); err != nil {
			t.Fatalf("WriteFile accepted %s: %v", f, err)
		}
	}
	meta := `[{"file":"race_coastal_2_accept.png","race":"coastal","race_name":"Прибрежные"},` +
		`{"file":"race_coastal_2b_accept.png","race":"coastal","race_name":"Прибрежные"},` +
		`{"file":"race_other_01.png","race":"other","race_name":"Прочие"}]`
	if err := os.WriteFile(filepath.Join(acc, "ships_meta.json"), []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile ships_meta: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/ships/ingame/delete?file=race_coastal_02.png", nil))
	var resp struct {
		Ok            bool `json:"ok"`
		Race          string `json:"race"`
		ShipsLeft     int    `json:"ships_left"`
		RaceShipsLeft int    `json:"race_ships_left"`
		Steps         struct {
			Registry bool `json:"registry"`
			Meta     bool `json:"meta"`
			Accepted bool `json:"accepted"`
			Sprite   bool `json:"sprite"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v (%s)", err, rec.Body.String())
	}
	if !resp.Ok {
		t.Fatalf("ok = false: %s", rec.Body.String())
	}
	if resp.Race != "coastal" || resp.ShipsLeft != 4 || resp.RaceShipsLeft != 1 {
		t.Errorf("resp = %+v, want coastal/4/1", resp)
	}
	if !resp.Steps.Registry || !resp.Steps.Meta || !resp.Steps.Accepted || !resp.Steps.Sprite {
		t.Errorf("steps = %+v, want все true", resp.Steps)
	}
	// реестр: записи нет
	reg, err := models.ReadShipRegistry(srv.shipRegistryPath)
	if err != nil {
		t.Fatalf("ReadShipRegistry: %v", err)
	}
	for _, s := range reg {
		if s.File == "race_coastal_02.png" {
			t.Errorf("реестр всё ещё содержит race_coastal_02.png")
		}
	}
	// accepted PNG и связанные записи меты удалены; не связанная запись осталась
	for _, f := range []string{"race_coastal_2_accept.png", "race_coastal_2b_accept.png"} {
		if _, err := os.Stat(filepath.Join(acc, f)); err == nil {
			t.Errorf("accepted PNG %s не удалён", f)
		}
	}
	sm, err := os.ReadFile(filepath.Join(acc, "ships_meta.json"))
	if err != nil {
		t.Fatalf("ReadFile ships_meta: %v", err)
	}
	var items []struct {
		File string `json:"file"`
	}
	if err := json.Unmarshal(sm, &items); err != nil {
		t.Fatalf("json meta: %v", err)
	}
	if len(items) != 1 || items[0].File != "race_other_01.png" {
		t.Errorf("мета = %+v, want только race_other_01.png", items)
	}
	// игровой PNG удалён
	if _, err := os.Stat(filepath.Join(sprites, "race_coastal_02.png")); err == nil {
		t.Errorf("игровой PNG не удалён")
	}
	// витрина отдаёт актуальный состав сразу
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, httptest.NewRequest("GET", "/ships/ingame", nil))
	var got []struct {
		File string `json:"file"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(got) != 4 {
		t.Errorf("витрина после удаления = %d, want 4", len(got))
	}
}

// TestShipsIngameDeleteKeepsUnknownMetaField — removeShipsMeta сохраняет ВСЕ поля
// записи меты, включая неизвестные (будущие): удаляется только связанная запись,
// у соседней неизвестное поле и точность большого числа (seed) не теряются.
func TestShipsIngameDeleteKeepsUnknownMetaField(t *testing.T) {
	srv, sprites, acc := setupDeleteStudio(t, testRegistryShips())
	png := tinyPNG(t)
	if err := os.WriteFile(filepath.Join(sprites, "race_coastal_02.png"), png, 0o644); err != nil {
		t.Fatalf("WriteFile game PNG: %v", err)
	}
	// удаляемая запись связана по имени; соседняя несёт неизвестное поле и
	// большое число (проверка UseNumber: без него float64 потерял бы точность).
	meta := `[{"file":"race_coastal_02.png","race":"coastal","race_name":"Прибрежные","future_field":"dropped"},` +
		`{"file":"race_other_01.png","race":"other","race_name":"Прочие","future_field":"keep-me","seed":1700000000000000001}]`
	if err := os.WriteFile(filepath.Join(acc, "ships_meta.json"), []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile ships_meta: %v", err)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/ships/ingame/delete?file=race_coastal_02.png", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	sm, err := os.ReadFile(filepath.Join(acc, "ships_meta.json"))
	if err != nil {
		t.Fatalf("ReadFile ships_meta: %v", err)
	}
	var items []map[string]any
	if err := json.Unmarshal(sm, &items); err != nil {
		t.Fatalf("json meta: %v", err)
	}
	if len(items) != 1 || items[0]["file"] != "race_other_01.png" {
		t.Fatalf("мета = %+v, want только race_other_01.png", items)
	}
	if items[0]["future_field"] != "keep-me" {
		t.Errorf("неизвестное поле потеряно: %+v", items[0])
	}
	if !strings.Contains(string(sm), "1700000000000000001") {
		t.Errorf("точность большого seed потеряна: %s", sm)
	}
}

// TestShipsIngameDeleteProtected — 409 на нейтральный и людской дефолт, ничего
// не удаляется.
func TestShipsIngameDeleteProtected(t *testing.T) {
	for _, file := range []string{"neutral.png", "race_humans_starship.png"} {
		srv, sprites, _ := setupDeleteStudio(t, testRegistryShips())
		png := tinyPNG(t)
		if err := os.WriteFile(filepath.Join(sprites, file), png, 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", file, err)
		}
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/ships/ingame/delete?file="+file, nil))
		if rec.Code != http.StatusConflict {
			t.Errorf("%s: code = %d, want 409", file, rec.Code)
		}
		if _, err := os.Stat(filepath.Join(sprites, file)); err != nil {
			t.Errorf("%s: защищённый PNG удалён", file)
		}
		reg, err := models.ReadShipRegistry(srv.shipRegistryPath)
		if err != nil || len(reg) != len(testRegistryShips()) {
			t.Errorf("%s: реестр изменился (len=%d, err=%v)", file, len(reg), err)
		}
	}
}

// TestShipsIngameDeleteNothing — 404, когда удалять нечего ни на одном шаге.
func TestShipsIngameDeleteNothing(t *testing.T) {
	srv, _, _ := setupDeleteStudio(t, testRegistryShips())
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/ships/ingame/delete?file=race_ghost.png", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("code = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

// TestShipsIngameDeleteBadRequest — 400: пустое имя и обход пути (в т.ч. %5C).
func TestShipsIngameDeleteBadRequest(t *testing.T) {
	srv, _, _ := setupDeleteStudio(t, testRegistryShips())
	for _, u := range []string{
		"/ships/ingame/delete?file=",
		"/ships/ingame/delete?file=../secret.png",
		"/ships/ingame/delete?file=..%5Csecret.png",
		"/ships/ingame/delete?file=a%2Fb.png",
		"/ships/ingame/delete?file=bad%00name.png", // NUL — иначе 500 на шаге спрайта
		"/ships/ingame/delete?file=bad%0Aname.png", // перевод строки
	} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", u, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: code = %d, want 400", u, rec.Code)
		}
	}
}

// TestShipsIngameDeleteMethod — не POST → 405.
func TestShipsIngameDeleteMethod(t *testing.T) {
	srv, _, _ := setupDeleteStudio(t, testRegistryShips())
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/ingame/delete?file=race_coastal_02.png", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("code = %d, want 405", rec.Code)
	}
}

// TestShipsIngameDeleteIdempotent — запись реестра уже убрана (частичный сбой),
// но accepted PNG и мета на месте: повтор не отдаёт 404, а домешивает оставшиеся
// шаги по имени/хэшу (канал восстановления закрывается).
func TestShipsIngameDeleteIdempotent(t *testing.T) {
	var without []models.ShipSprite
	for _, s := range testRegistryShips() {
		if s.File != "race_coastal_02.png" {
			without = append(without, s)
		}
	}
	srv, sprites, acc := setupDeleteStudio(t, without)
	png := tinyPNG(t)
	if err := os.WriteFile(filepath.Join(sprites, "race_coastal_02.png"), png, 0o644); err != nil {
		t.Fatalf("WriteFile game PNG: %v", err)
	}
	if err := os.WriteFile(filepath.Join(acc, "race_coastal_2_accept.png"), png, 0o644); err != nil {
		t.Fatalf("WriteFile accepted: %v", err)
	}
	meta := `[{"file":"race_coastal_2_accept.png","race":"coastal","race_name":"Прибрежные"}]`
	if err := os.WriteFile(filepath.Join(acc, "ships_meta.json"), []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile ships_meta: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/ships/ingame/delete?file=race_coastal_02.png", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (домешивание): %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Ok    bool   `json:"ok"`
		Race  string `json:"race"`
		Steps struct {
			Registry bool `json:"registry"`
			Meta     bool `json:"meta"`
			Accepted bool `json:"accepted"`
			Sprite   bool `json:"sprite"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !resp.Ok || resp.Race != "coastal" {
		t.Errorf("resp = %+v, want ok/coastal", resp)
	}
	if resp.Steps.Registry {
		t.Errorf("steps.registry = true, want false (записи не было)")
	}
	if !resp.Steps.Meta || !resp.Steps.Accepted || !resp.Steps.Sprite {
		t.Errorf("steps = %+v, want meta/accepted/sprite true", resp.Steps)
	}
}

// TestShipsIngameDeleteLastShipAllowed — последний корабль расы удалять МОЖНО
// (решение В1): успех, race_ships_left = 0.
func TestShipsIngameDeleteLastShipAllowed(t *testing.T) {
	ships := []models.ShipSprite{
		{ID: "race_humans_starship", Name: "Люди · 1", File: "race_humans_starship.png", Race: "humans", Angle: 21.1},
		{ID: "race_humans_starship_02", Name: "Люди · 2", File: "race_humans_starship_02.png", Race: "humans"},
		{ID: "race_coastal_01", Name: "Прибрежные · 1", File: "race_coastal_01.png", Race: "coastal"},
		{ID: "neutral", Name: "Нейтральный", File: "neutral.png", Race: ""},
	}
	srv, _, _ := setupDeleteStudio(t, ships)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/ships/ingame/delete?file=race_coastal_01.png", nil))
	var resp struct {
		Ok            bool `json:"ok"`
		ShipsLeft     int  `json:"ships_left"`
		RaceShipsLeft int  `json:"race_ships_left"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v (%s)", err, rec.Body.String())
	}
	if !resp.Ok || resp.ShipsLeft != 3 || resp.RaceShipsLeft != 0 {
		t.Errorf("resp = %+v, want ok/3/0", resp)
	}
}

// TestShipsIngameImg — GET /ships/ingame/img/<file> отдаёт PNG из папки
// игровых спрайтов (в тесте — временная).
func TestShipsIngameImg(t *testing.T) {
	srv, _, _ := newShipsTestStudio(t)
	dir := t.TempDir()
	pngBytes := tinyPNG(t)
	if err := os.WriteFile(filepath.Join(dir, "neutral.png"), pngBytes, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	srv.gameSpritesDirPath = dir
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/ships/ingame/img/neutral.png", nil))
	if rec.Code != 200 {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
	if !bytes.Equal(rec.Body.Bytes(), pngBytes) {
		t.Errorf("тело != исходный PNG")
	}
}

// TestShipsIngameImgTraversal — выход из папки спрайтов не отдаёт файл снаружи
// (filepath.Base нейтрализует ../).
func TestShipsIngameImgTraversal(t *testing.T) {
	srv, _, _ := newShipsTestStudio(t)
	base := t.TempDir()
	dir := filepath.Join(base, "sprites")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	secret := []byte("SECRET-OUTSIDE")
	if err := os.WriteFile(filepath.Join(base, "secret.png"), secret, 0o644); err != nil {
		t.Fatalf("WriteFile secret: %v", err)
	}
	srv.gameSpritesDirPath = dir
	for _, u := range []string{
		"/ships/ingame/img/../secret.png",
		"/ships/ingame/img/..%5Csecret.png",
	} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", u, nil))
		if bytes.Contains(rec.Body.Bytes(), secret) {
			t.Errorf("%s: отдан файл вне папки спрайтов", u)
		}
		if rec.Code == 200 {
			t.Errorf("%s: code = 200, want не 200", u)
		}
	}
}
