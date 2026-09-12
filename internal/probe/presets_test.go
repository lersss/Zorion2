package probe

import (
	"os"
	"path/filepath"
	"testing"
)

func writePresets(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "presets.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadPresetsDefaultsApplied(t *testing.T) {
	path := writePresets(t, `[
		{"id":"a","name":"A","params":{"alpha":2}},
		{"id":"b","name":"B","params":{}}
	]`)
	if err := LoadPresets(path); err != nil {
		t.Fatal(err)
	}
	d := DefaultCurveParams()

	a, ok := PresetByID("a")
	if !ok {
		t.Fatal("пресет a не найден")
	}
	if a.Params.Alpha != 2 {
		t.Fatalf("alpha должен быть 2, получил %v", a.Params.Alpha)
	}
	if a.Params.KBase != d.KBase {
		t.Fatalf("k_base должен взять дефолт %v, получил %v", d.KBase, a.Params.KBase)
	}
	if a.Params.PBase != d.PBase || a.Params.NDead != d.NDead {
		t.Fatal("остальные поля должны взять дефолты")
	}

	b, ok := PresetByID("b")
	if !ok {
		t.Fatal("пресет b не найден")
	}
	if b.Params != d {
		t.Fatalf("пустой пресет должен быть дефолтом: %+v vs %+v", b.Params, d)
	}
}

func TestLoadPresetsAlphaZeroPreserved(t *testing.T) {
	path := writePresets(t, `[{"id":"exp","name":"Экспонента","params":{"alpha":0}}]`)
	if err := LoadPresets(path); err != nil {
		t.Fatal(err)
	}
	p, ok := PresetByID("exp")
	if !ok {
		t.Fatal("пресет exp не найден")
	}
	if p.Params.Alpha != 0 {
		t.Fatalf("alpha=0 должна сохраниться, получил %v", p.Params.Alpha)
	}
}

func TestLoadPresetsMissingFile(t *testing.T) {
	// Сначала успешная загрузка.
	good := writePresets(t, `[{"id":"keep","name":"Keep","params":{}}]`)
	if err := LoadPresets(good); err != nil {
		t.Fatal(err)
	}
	// Ошибка не должна стирать уже загруженный список.
	path := filepath.Join(t.TempDir(), "no_such_presets.json")
	if err := LoadPresets(path); err == nil {
		t.Fatal("ожидал ошибку на отсутствующем файле")
	}
	if _, ok := PresetByID("keep"); !ok {
		t.Fatal("старый список должен сохраниться после ошибки загрузки")
	}
}

func TestLoadPresetsBadJSON(t *testing.T) {
	good := writePresets(t, `[{"id":"keep2","name":"Keep2","params":{}}]`)
	if err := LoadPresets(good); err != nil {
		t.Fatal(err)
	}
	path := writePresets(t, `{"not":"an array"`)
	if err := LoadPresets(path); err == nil {
		t.Fatal("ожидал ошибку на битом JSON")
	}
	if _, ok := PresetByID("keep2"); !ok {
		t.Fatal("старый список должен сохраниться после битого JSON")
	}
}

func TestOpenListNewPreset(t *testing.T) {
	// Новый пресет — только запись в JSON, никакой правки кода.
	path := writePresets(t, `[
		{"id":"x","name":"X","params":{}}
	]`)
	if err := LoadPresets(path); err != nil {
		t.Fatal(err)
	}
	if _, ok := PresetByID("x"); !ok {
		t.Fatal("новый пресет должен быть найден без правки кода")
	}
}

func TestParseOverrides(t *testing.T) {
	m := map[string]interface{}{
		"p_base":    float64(1000),
		"alpha":     float64(0),
		"n_crit":    float64(5),
		"atmo_penalty": []interface{}{0.9, 0.4, 0.1},
		"unknown":   "игнорируется",
	}
	ov := ParseOverrides(m)
	if ov.PBase == nil || *ov.PBase != 1000 {
		t.Fatal("p_base не распарсен")
	}
	if ov.Alpha == nil || *ov.Alpha != 0 {
		t.Fatal("alpha=0 должна распарситься")
	}
	if ov.AtmoPenalty == nil || ov.AtmoPenalty[2] != 0.1 {
		t.Fatal("atmo_penalty не распарсен")
	}

	d := DefaultCurveParams()
	merged := ApplyOverrides(d, ov)
	if merged.PBase != 1000 || merged.Alpha != 0 || merged.AtmoPenalty[2] != 0.1 {
		t.Fatalf("оверрайды не применены: %+v", merged)
	}
	if merged.KBase != d.KBase {
		t.Fatal("не тронутые поля должны остаться дефолтами")
	}
}