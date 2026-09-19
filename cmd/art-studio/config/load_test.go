package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	formsPath    = "../../../config/art/forms.json"
	familiesPath = "../../../config/art/families.json"
	humansPath   = "../../../config/art/humans.json"
)

// TestLoadForms — словарь форм: 55×50×40×50 компонентов (расширен газом/туманом,
// решение создателя 2026-09-17) + 43 антропо-формы + palette_accents
// + category_keys F1–F10 (инвариант 67a.1 §11.6, 76a.1 §6.6, 88a.1 §6.5).
func TestLoadForms(t *testing.T) {
	fc, err := LoadForms(formsPath)
	if err != nil {
		t.Fatalf("LoadForms: %v", err)
	}
	if len(fc.Shapes) < 55 {
		t.Errorf("shapes = %d, want >= 55", len(fc.Shapes))
	}
	if len(fc.Struct) < 50 {
		t.Errorf("struct = %d, want >= 50", len(fc.Struct))
	}
	if len(fc.Character) < 40 {
		t.Errorf("character = %d, want >= 40", len(fc.Character))
	}
	if len(fc.Parts) < 50 {
		t.Errorf("parts = %d, want >= 50", len(fc.Parts))
	}
	if len(fc.AnthroForms) < 43 {
		t.Errorf("anthro_forms = %d, want >= 43", len(fc.AnthroForms))
	}
	for _, m := range []struct {
		name string
		n    int
		got  []string
	}{
		{"beast_forms", 30, fc.BeastForms},
		{"xeno_forms", 30, fc.XenoForms},
		{"amorphous_forms", 25, fc.AmorphousForms},
		{"crystal_forms", 25, fc.CrystalForms},
		{"mech_forms", 25, fc.MechForms},
		{"titan_forms", 25, fc.TitanForms},
	} {
		if len(m.got) < m.n {
			t.Errorf("%s = %d, want >= %d", m.name, len(m.got), m.n)
		}
	}
	if len(fc.PaletteAccents) == 0 {
		t.Errorf("palette_accents пуст")
	}
	for _, fam := range []string{"F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10"} {
		if len(fc.CategoryKeys[fam]) == 0 {
			t.Errorf("category_keys[%s] пуст", fam)
		}
	}
}

// TestLoadFamilies — 59 рас F1–F10, id из 99.2.21 §5 и 99.2.24 (F10),
// непустые поля, морф-наборы у всех семейств (инварианты 67a.1 §11.6, 76a.1 §6, 88a.1 §6).
func TestLoadFamilies(t *testing.T) {
	fam, err := LoadFamilies(familiesPath)
	if err != nil {
		t.Fatalf("LoadFamilies: %v", err)
	}
	total := 0
	for _, f := range fam {
		total += len(f.Races)
	}
	if total != 59 {
		t.Errorf("всего рас = %d, want 59", total)
	}
	want := map[string][]string{
		"F1":  {"2", "3", "4"},
		"F2":  {"5", "6", "7", "8", "9", "48"},
		"F3":  {"11", "12", "13", "14"},
		"F4":  {"15", "16", "17", "18", "19", "20", "21", "22", "43"},
		"F5":  {"23", "24", "25", "26", "27", "28", "29", "30", "31", "49"},
		"F6":  {"32", "33", "34", "35", "36", "37"},
		"F7":  {"40", "41", "42"},
		"F8":  {"10", "39", "50"},
		"F9":  {"38", "44", "45", "46", "47"},
		"F10": {"51", "52", "53", "54", "55", "56", "57", "58", "59", "60"},
	}
	for fid, ids := range want {
		f, ok := fam[fid]
		if !ok {
			t.Errorf("нет семейства %s", fid)
			continue
		}
		if len(f.Races) != len(ids) {
			t.Errorf("%s: рас %d, want %d", fid, len(f.Races), len(ids))
		}
		for i, id := range ids {
			if f.Races[i].ID != id {
				t.Errorf("%s[%d].id = %s, want %s", fid, i, f.Races[i].ID, id)
			}
		}
	}
	for fid, f := range fam {
		if f.Name == "" {
			t.Errorf("%s: name пуст", fid)
		}
		if len(f.Extra) == 0 || len(f.Anchor) == 0 || f.Scene == "" || f.Neg == "" {
			t.Errorf("%s: extra/anchor/scene/neg неполны", fid)
		}
		for _, rc := range f.Races {
			if rc.Name == "" || rc.Basis == "" {
				t.Errorf("%s/%s: name/basis пусты", fid, rc.ID)
			}
			if len(rc.Forms) < 2 || len(rc.Forms) > 60 {
				t.Errorf("%s/%s: forms %d (нужно 2–60)", fid, rc.ID, len(rc.Forms))
			}
			if len(rc.Materials) < 2 || len(rc.Materials) > 3 {
				t.Errorf("%s/%s: materials %d (нужно 2–3)", fid, rc.ID, len(rc.Materials))
			}
			if len(rc.Glows) < 2 || len(rc.Glows) > 3 {
				t.Errorf("%s/%s: glows %d (нужно 2–3)", fid, rc.ID, len(rc.Glows))
			}
		}
	}
	for _, fid := range []string{"F1", "F2", "F3", "F4", "F5", "F6", "F7", "F8", "F9", "F10"} {
		if len(fam[fid].BeastForms) == 0 {
			t.Errorf("%s: beast_forms пуст", fid)
		}
		if len(fam[fid].AnthroClothes) == 0 {
			t.Errorf("%s: anthro_clothes пуст", fid)
		}
		if fam[fid].AnthroNeg == "" {
			t.Errorf("%s: anthro_neg пуст", fid)
		}
	}
}

// TestLoadHumans — оси рандома непустые, neg/prompt_template/params валидны.
func TestLoadHumans(t *testing.T) {
	hc, err := LoadHumans(humansPath)
	if err != nil {
		t.Fatalf("LoadHumans: %v", err)
	}
	for _, k := range []string{"GENDER", "SKIN", "AGE", "HAIR", "BEARD", "CLOTHES", "ARMOR", "HELMET", "EXPR", "FACIAL", "SCENE"} {
		if len(hc.Axes[k]) == 0 {
			t.Errorf("ось %s пуста", k)
		}
	}
	if hc.Neg == "" {
		t.Error("neg пуст")
	}
	if hc.PromptTemplate == "" {
		t.Error("prompt_template пуст")
	}
	if hc.Params.MaxCount == 0 {
		t.Error("params.max_count = 0")
	}
}

// TestLoadStudio — дефолты studio.json (спека 67a.1 §4.4).
func TestLoadStudio(t *testing.T) {
	cfg, err := LoadStudio("../../../config/art/studio.json")
	if err != nil {
		t.Fatalf("LoadStudio: %v", err)
	}
	if cfg.Port != 8798 {
		t.Errorf("port = %d, want 8798", cfg.Port)
	}
	if cfg.ComfyURL == "" || cfg.PythonCmd == "" || cfg.RembgCLI == "" {
		t.Error("comfy_url/python_cmd/rembg_cli пусты")
	}
	if cfg.Workers != 2 {
		t.Errorf("workers = %d, want 2", cfg.Workers)
	}
	if cfg.DenoiseRef != 0.35 {
		t.Errorf("denoise_ref = %v, want 0.35", cfg.DenoiseRef)
	}
}

// writeFamilies пишет минимальный families.json во временный файл.
func writeFamilies(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "families.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

const minimalFamily = `{
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

// TestLoadFamiliesAppearanceBlocked — валидация appearance/blocked (98a §4.3):
// длина appearance ≤ 200, без \n; blocked — непустые токены без пробелов,
// нормализация lowercase+trim, дедуп (тихо, порядок первого вхождения).
func TestLoadFamiliesAppearanceBlocked(t *testing.T) {
	// валидные appearance + blocked: нормализация (lowercase/trim/дедуп)
	path := writeFamilies(t, strings.Replace(minimalFamily,
		`"glows": ["test glow one", "test glow two"]`,
		`"glows": ["test glow one", "test glow two"],
        "appearance": "test appearance phrase",
        "blocked": ["Lava", " lava ", "lava", "Sulfur"]`, 1))
	fam, err := LoadFamilies(path)
	if err != nil {
		t.Fatalf("LoadFamilies: %v", err)
	}
	rc := fam["F1"].Races[0]
	if rc.Appearance != "test appearance phrase" {
		t.Errorf("appearance = %q", rc.Appearance)
	}
	want := []string{"lava", "sulfur"}
	if len(rc.Blocked) != 2 || rc.Blocked[0] != want[0] || rc.Blocked[1] != want[1] {
		t.Errorf("blocked = %v, want %v", rc.Blocked, want)
	}

	// appearance > 200 символов → ошибка
	path = writeFamilies(t, strings.Replace(minimalFamily,
		`"glows": ["test glow one", "test glow two"]`,
		`"glows": ["test glow one", "test glow two"],
        "appearance": "`+strings.Repeat("x", 201)+`"`, 1))
	if _, err := LoadFamilies(path); err == nil {
		t.Error("appearance > 200 символов: ошибки нет")
	}

	// appearance с \n → ошибка
	path = writeFamilies(t, strings.Replace(minimalFamily,
		`"glows": ["test glow one", "test glow two"]`,
		`"glows": ["test glow one", "test glow two"],
        "appearance": "line one\nline two"`, 1))
	if _, err := LoadFamilies(path); err == nil {
		t.Error("appearance с \\n: ошибки нет")
	}

	// blocked с пробелом → ошибка
	path = writeFamilies(t, strings.Replace(minimalFamily,
		`"glows": ["test glow one", "test glow two"]`,
		`"glows": ["test glow one", "test glow two"],
        "blocked": ["bad token"]`, 1))
	if _, err := LoadFamilies(path); err == nil {
		t.Error("blocked с пробелом: ошибки нет")
	}

	// blocked с пустым токеном → ошибка
	path = writeFamilies(t, strings.Replace(minimalFamily,
		`"glows": ["test glow one", "test glow two"]`,
		`"glows": ["test glow one", "test glow two"],
        "blocked": ["lava", ""]`, 1))
	if _, err := LoadFamilies(path); err == nil {
		t.Error("blocked с пустым токеном: ошибки нет")
	}
}