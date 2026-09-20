package handlers

import (
	"bytes"
	"encoding/json"
	"image"
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

// TestShipsPrompt — /ships/prompt: промпты этапов 1/2 без токенов blocked
// (детали этапа 1 фильтруются).
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
	dc, err := config.LoadShipDict("../../../config/art/ship_dict.json")
	if err != nil {
		t.Fatalf("LoadShipDict: %v", err)
	}
	if !strings.Contains(resp.Prompt1, dc.Concepts["humans"]) {
		t.Errorf("prompt1 не содержит концепт людей %q: %s", dc.Concepts["humans"], resp.Prompt1)
	}
	for _, tok := range []string{"tentacle", "crystal", "pyramid", "obelisk", "alien"} {
		if strings.Contains(resp.Prompt1, tok) {
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
	script := "@echo off\r\nif \"%2\"==\"--spec\" goto spec\r\ncopy %2 %3 >nul 2>&1\r\nexit /b 0\r\n:spec\r\ncopy %3 %5 >nul 2>&1\r\nexit /b 0\r\n"
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

// TestShipsGenParams — /ships/gen: tags/override/size доходят до генерации
// (мета кандидатов = override, Size = 100).
func TestShipsGenParams(t *testing.T) {
	srv, _, pool := newShipsTestStudio(t)
	fakePy := filepath.Join(t.TempDir(), "fake_ship_python.cmd")
	script := "@echo off\r\nif \"%2\"==\"--spec\" goto spec\r\ncopy %2 %3 >nul 2>&1\r\nexit /b 0\r\n:spec\r\ncopy %3 %5 >nul 2>&1\r\nexit /b 0\r\n"
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