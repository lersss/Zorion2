package handlers

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zorion/cmd/art-studio/config"
)

// TestParseAppearanceSection — парсер раздела «Внешний вид» (98a §3):
// маркеры appearance/blocked, сплит blocked по запятой + trim каждого;
// отсутствие раздела/маркеров → ошибка.
func TestParseAppearanceSection(t *testing.T) {
	md := `# Тест (test)

## Кто это

Текст.

## Внешний вид

**Силуэт.** Описание.

**Для генератора (appearance):** sleek bodies, no face, no eyes

**Для генератора (blocked):** lava, Sulfur,  ember ,lava

## Происхождение

Текст.
`
	app, blocked, err := parseAppearanceSection(md)
	if err != nil {
		t.Fatalf("parseAppearanceSection: %v", err)
	}
	if app != "sleek bodies, no face, no eyes" {
		t.Errorf("appearance = %q", app)
	}
	want := []string{"lava", "Sulfur", "ember", "lava"}
	if len(blocked) != len(want) {
		t.Fatalf("blocked = %v, want %v", blocked, want)
	}
	for i := range want {
		if blocked[i] != want[i] {
			t.Errorf("blocked[%d] = %q, want %q", i, blocked[i], want[i])
		}
	}

	// нет раздела «Внешний вид» → ошибка
	if _, _, err := parseAppearanceSection("# Тест\n\n## Кто это\n\nТекст."); err == nil {
		t.Error("нет раздела «Внешний вид»: ошибки нет")
	}
	// маркеры вне раздела → ошибка
	mdOutside := "# Тест\n\n## Кто это\n\n**Для генератора (appearance):** x\n\n**Для генератора (blocked):** y\n"
	if _, _, err := parseAppearanceSection(mdOutside); err == nil {
		t.Error("маркеры вне раздела: ошибки нет")
	}
	// только appearance → ошибка (blocked обязателен)
	mdNoBlocked := "# Тест\n\n## Внешний вид\n\n**Для генератора (appearance):** x\n"
	if _, _, err := parseAppearanceSection(mdNoBlocked); err == nil {
		t.Error("нет blocked: ошибки нет")
	}
}

// TestUpdateFamiliesRace — запись appearance/blocked в families.json:
// LoadFamilies читает обновлённое, остальные расы и comment-поле не тронуты.
func TestUpdateFamiliesRace(t *testing.T) {
	src, err := os.ReadFile("../../../config/art/families.json")
	if err != nil {
		t.Fatalf("ReadFile families.json: %v", err)
	}
	path := filepath.Join(t.TempDir(), "families.json")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	fam, err := config.LoadFamilies(path)
	if err != nil {
		t.Fatalf("LoadFamilies: %v", err)
	}

	newApp := "test appearance phrase for rebuild"
	newBlocked := []string{"alpha", "beta", "gamma"}
	if err := updateFamiliesRace(path, "5", newApp, newBlocked); err != nil {
		t.Fatalf("updateFamiliesRace: %v", err)
	}
	fam2, err := config.LoadFamilies(path)
	if err != nil {
		t.Fatalf("LoadFamilies после записи: %v", err)
	}
	rc := fam2["F2"].Races[0] // id "5"
	if rc.Appearance != newApp {
		t.Errorf("appearance = %q, want %q", rc.Appearance, newApp)
	}
	if len(rc.Blocked) != 3 || rc.Blocked[0] != "alpha" || rc.Blocked[1] != "beta" || rc.Blocked[2] != "gamma" {
		t.Errorf("blocked = %v, want [alpha beta gamma]", rc.Blocked)
	}
	// остальные расы не тронуты
	if fam2["F2"].Races[1].Appearance != fam["F2"].Races[1].Appearance {
		t.Error("раса 6: appearance изменена")
	}
	if len(fam2["F2"].Races[1].Blocked) != len(fam["F2"].Races[1].Blocked) {
		t.Error("раса 6: blocked изменена")
	}
	// comment-поле файла сохранено
	data2, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile после записи: %v", err)
	}
	if !strings.Contains(string(data2), "Конфиг семейств рас") {
		t.Error("comment-поле файла потеряно")
	}
	// несуществующая раса → ошибка, файл не изменён
	if err := updateFamiliesRace(path, "999", newApp, newBlocked); err == nil {
		t.Error("несуществующая раса: ошибки нет")
	}

	// раса без appearance/blocked в файле → поля вставляются (защитный путь)
	noFields := `{
  "F1": {
    "name": "Тест",
    "races": [
      {
        "id": "1",
        "name": "1 Тест",
        "basis": "тест",
        "forms": ["a test form one", "a test form two"],
        "materials": ["test material one", "test material two"],
        "glows": ["test glow one", "test glow two"]
      }
    ],
    "extra": ["no face"],
    "anchor": ["anchored"],
    "scene": "on flat background",
    "neg": "text"
  }
}`
	path2 := filepath.Join(t.TempDir(), "families.json")
	if err := os.WriteFile(path2, []byte(noFields), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := updateFamiliesRace(path2, "1", newApp, newBlocked); err != nil {
		t.Fatalf("updateFamiliesRace (вставка): %v", err)
	}
	fam3, err := config.LoadFamilies(path2)
	if err != nil {
		t.Fatalf("LoadFamilies после вставки: %v", err)
	}
	rc3 := fam3["F1"].Races[0]
	if rc3.Appearance != newApp {
		t.Errorf("вставка: appearance = %q, want %q", rc3.Appearance, newApp)
	}
	if len(rc3.Blocked) != 3 || rc3.Blocked[0] != "alpha" {
		t.Errorf("вставка: blocked = %v", rc3.Blocked)
	}
}

// TestHandleRebuildPrompt — «Пересобрать промт»: лор-файл расы → запись
// appearance/blocked в families.json → перезагрузка конфига в памяти
// (Server) → /prompt использует обновлённый appearance. Нет раздела → ошибка.
func TestHandleRebuildPrompt(t *testing.T) {
	srv, _, _ := newTestStudio(t)
	// F2/5 → «Аммиачники» → slug ammonia → docs/gamedesign/races/ammonia.md
	req := httptest.NewRequest("GET", "/rebuild-prompt?fam=F2&race=5", nil)
	rr := httptest.NewRecorder()
	srv.handleRebuildPrompt(rr, req)
	var j map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &j); err != nil {
		t.Fatalf("не-JSON ответ: %v", err)
	}
	if j["ok"] != true {
		t.Fatalf("ok = %v, error = %v", j["ok"], j["error"])
	}
	app, _ := j["appearance"].(string)
	if app == "" {
		t.Fatal("appearance пуст")
	}
	bl, _ := j["blocked"].([]interface{})
	if len(bl) == 0 {
		t.Fatal("blocked пуст")
	}
	// families.json на диске обновлён
	fam, err := config.LoadFamilies(srv.familiesPath)
	if err != nil {
		t.Fatalf("LoadFamilies: %v", err)
	}
	rc := fam["F2"].Races[0]
	if rc.Appearance != app {
		t.Errorf("диск: appearance = %q, want %q", rc.Appearance, app)
	}
	// конфиг в памяти (Server) обновлён
	if f, ok := srv.family("F2"); ok {
		if f.Races[0].Appearance != app {
			t.Errorf("память: appearance = %q, want %q", f.Races[0].Appearance, app)
		}
	}
	// /prompt использует обновлённый appearance
	p := getPrompt(srv, "/prompt?fam=F2&race=5&seed=42")
	if !strings.Contains(p, app) {
		t.Errorf("/prompt не содержит новый appearance: %s", p)
	}

	// нет раздела «Внешний вид» → ошибка
	srv2, _, _ := newTestStudio(t)
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "ammonia.md"), []byte("# Тест\n\n## Кто это\n\nТекст."), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	srv2.loreDir = tmp
	req2 := httptest.NewRequest("GET", "/rebuild-prompt?fam=F2&race=5", nil)
	rr2 := httptest.NewRecorder()
	srv2.handleRebuildPrompt(rr2, req2)
	var j2 map[string]interface{}
	if err := json.Unmarshal(rr2.Body.Bytes(), &j2); err != nil {
		t.Fatalf("не-JSON ответ: %v", err)
	}
	if !strings.Contains(fmt.Sprint(j2["error"]), "нет раздела") {
		t.Errorf("нет раздела: error = %v", j2["error"])
	}
}

// TestHandleRebuildPromptAllRaces — массовый режим (race пуст): пересборка
// ВСЕХ рас семейства F2 (у всех есть лор-файлы) → ok=true, updated содержит
// все 6 рас, skipped пуст; families.json на диске и конфиг в памяти обновлены
// для каждой расы; /prompt использует новый appearance.
func TestHandleRebuildPromptAllRaces(t *testing.T) {
	srv, _, _ := newTestStudio(t)
	req := httptest.NewRequest("GET", "/rebuild-prompt?fam=F2", nil)
	rr := httptest.NewRecorder()
	srv.handleRebuildPrompt(rr, req)
	var j struct {
		OK      bool `json:"ok"`
		Updated []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Appearance string `json:"appearance"`
		} `json:"updated"`
		Skipped []struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		} `json:"skipped"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &j); err != nil {
		t.Fatalf("не-JSON ответ: %v", err)
	}
	if !j.OK {
		t.Fatalf("ok = false, skipped = %+v", j.Skipped)
	}
	wantIDs := []string{"5", "6", "7", "8", "9", "48"}
	if len(j.Updated) != len(wantIDs) {
		t.Fatalf("updated = %d рас, want %d: %+v", len(j.Updated), len(wantIDs), j.Updated)
	}
	got := map[string]string{}
	for _, u := range j.Updated {
		if u.Appearance == "" {
			t.Errorf("раса %s: appearance пуст", u.ID)
		}
		got[u.ID] = u.Appearance
	}
	for _, id := range wantIDs {
		if got[id] == "" {
			t.Errorf("раса %s не в updated", id)
		}
	}
	if len(j.Skipped) != 0 {
		t.Errorf("skipped = %+v, want пусто", j.Skipped)
	}
	// families.json на диске обновлён для каждой расы
	fam, err := config.LoadFamilies(srv.familiesPath)
	if err != nil {
		t.Fatalf("LoadFamilies: %v", err)
	}
	for _, rc := range fam["F2"].Races {
		if rc.Appearance != got[rc.ID] {
			t.Errorf("диск: раса %s appearance = %q, want %q", rc.ID, rc.Appearance, got[rc.ID])
		}
	}
	// конфиг в памяти (Server) обновлён
	if f, ok := srv.family("F2"); ok {
		for _, rc := range f.Races {
			if rc.Appearance != got[rc.ID] {
				t.Errorf("память: раса %s appearance = %q, want %q", rc.ID, rc.Appearance, got[rc.ID])
			}
		}
	}
	// /prompt использует обновлённый appearance
	p := getPrompt(srv, "/prompt?fam=F2&race=5&seed=42")
	if !strings.Contains(p, got["5"]) {
		t.Errorf("/prompt не содержит новый appearance расы 5: %s", p)
	}
}

// TestHandleRebuildPromptSkipped — массовый режим с пропуском: лор-каталог
// подменён на temp с одним валидным файлом (ammonia.md для расы 5), остальные
// расы F2 без лор-файлов → updated=[раса 5], skipped=[остальные 5 с причиной],
// ok=true; на диске раса 5 обновлена, раса 6 не тронута.
func TestHandleRebuildPromptSkipped(t *testing.T) {
	srv, _, _ := newTestStudio(t)
	famBefore, err := config.LoadFamilies(srv.familiesPath)
	if err != nil {
		t.Fatalf("LoadFamilies: %v", err)
	}
	origApp6 := famBefore["F2"].Races[1].Appearance // раса 6 «Крио-лесные»

	tmp := t.TempDir()
	validMD := "# Тест\n\n## Внешний вид\n\n**Для генератора (appearance):** test appearance\n\n**Для генератора (blocked):** alpha, beta\n"
	if err := os.WriteFile(filepath.Join(tmp, "ammonia.md"), []byte(validMD), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	srv.loreDir = tmp

	req := httptest.NewRequest("GET", "/rebuild-prompt?fam=F2", nil)
	rr := httptest.NewRecorder()
	srv.handleRebuildPrompt(rr, req)
	var j struct {
		OK      bool `json:"ok"`
		Updated []struct {
			ID         string `json:"id"`
			Appearance string `json:"appearance"`
		} `json:"updated"`
		Skipped []struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		} `json:"skipped"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &j); err != nil {
		t.Fatalf("не-JSON ответ: %v", err)
	}
	if !j.OK {
		t.Fatalf("ok = false, error = %+v", j.Skipped)
	}
	if len(j.Updated) != 1 || j.Updated[0].ID != "5" {
		t.Fatalf("updated = %+v, want [раса 5]", j.Updated)
	}
	if j.Updated[0].Appearance != "test appearance" {
		t.Errorf("updated[0].appearance = %q, want %q", j.Updated[0].Appearance, "test appearance")
	}
	if len(j.Skipped) != 5 {
		t.Fatalf("skipped = %d, want 5: %+v", len(j.Skipped), j.Skipped)
	}
	foundSkip6 := false
	for _, sk := range j.Skipped {
		if sk.ID == "6" {
			foundSkip6 = true
			if !strings.Contains(sk.Reason, "нет лор-файла") {
				t.Errorf("раса 6: reason = %q, want содержит «нет лор-файла»", sk.Reason)
			}
		}
	}
	if !foundSkip6 {
		t.Error("раса 6 не в skipped")
	}
	// на диске: раса 5 обновлена, раса 6 не тронута
	fam, err := config.LoadFamilies(srv.familiesPath)
	if err != nil {
		t.Fatalf("LoadFamilies после: %v", err)
	}
	if fam["F2"].Races[0].Appearance != "test appearance" {
		t.Errorf("диск: раса 5 appearance = %q, want %q", fam["F2"].Races[0].Appearance, "test appearance")
	}
	if fam["F2"].Races[1].Appearance != origApp6 {
		t.Errorf("диск: раса 6 appearance изменена: %q → %q", origApp6, fam["F2"].Races[1].Appearance)
	}
}

// TestHandleRebuildPromptAllSkipped — массовый режим, все расы без лор-файлов
// (пустой loreDir) → ok=false, updated пуст, skipped = все расы, error — сводка.
func TestHandleRebuildPromptAllSkipped(t *testing.T) {
	srv, _, _ := newTestStudio(t)
	srv.loreDir = t.TempDir() // пусто: ни одного лор-файла
	req := httptest.NewRequest("GET", "/rebuild-prompt?fam=F2", nil)
	rr := httptest.NewRecorder()
	srv.handleRebuildPrompt(rr, req)
	var j map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &j); err != nil {
		t.Fatalf("не-JSON ответ: %v", err)
	}
	if j["ok"] != false {
		t.Fatalf("ok = %v, want false", j["ok"])
	}
	if e, _ := j["error"].(string); e == "" {
		t.Error("error пуст")
	}
	if up, _ := j["updated"].([]interface{}); len(up) != 0 {
		t.Errorf("updated = %v, want пусто", up)
	}
	if sk, _ := j["skipped"].([]interface{}); len(sk) != 6 {
		t.Errorf("skipped = %d, want 6", len(sk))
	}
}