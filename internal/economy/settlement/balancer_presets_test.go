// internal/economy/settlement/balancer_presets_test.go
// Тесты слоя пресетов кривых балансировщика (спека 99.2.17 §5/§6/§9,
// итерация 7): save/apply/delete, перезапись, default не удаляется,
// reset-default, файл создаётся при старте, атомарность записи, валидация
// имён 422, персистентность через рестарт, битый JSON → fallback, 404.
// Используют os.MkdirTemp (не трогают реальный config/balancer_presets.json).
package settlement

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newPresetEnv — временный каталог + путь + Load при старте + cleanup
// (кривые → дефолты, чтобы не влиять на соседние тесты пакета).
func newPresetEnv(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "presets.json")
	if err := LoadBalancerPresets(path); err != nil {
		t.Fatalf("LoadBalancerPresets(%s): %v", path, err)
	}
	t.Cleanup(func() {
		for _, c := range balancerComponentOrder {
			if err := ResetCurve(c); err != nil {
				t.Errorf("ResetCurve(%q): %v", c, err)
			}
		}
	})
	return path
}

// customHeat — кастомная кривая жары для проверки чувствительности.
func customHeat() ComponentCurve {
	return ComponentCurve{
		Nodes: []SegmentNode{{X: 30, Y: 0}, {X: 100, Y: 0.5}, {X: 4000, Y: 0.9}},
		Bends: []float64{0, 0},
	}
}

// TestPresetSaveApplyDelete — AddPreset → ApplyPreset (SetCurve отдаёт узлы
// пресета; evaluateCurve чувствителен) → DeletePreset → ApplyPreset: «не найден».
func TestPresetSaveApplyDelete(t *testing.T) {
	newPresetEnv(t)

	// Кривая в store — кастомная; «Сохранить как» берёт её.
	if err := SetCurve("heat", customHeat()); err != nil {
		t.Fatalf("SetCurve: %v", err)
	}
	if _, err := SavePreset("heat", "жесткая-жара"); err != nil {
		t.Fatalf("SavePreset: %v", err)
	}
	// Сбросим кривую на дефолт, чтобы применение было заметно.
	if err := ResetCurve("heat"); err != nil {
		t.Fatalf("ResetCurve: %v", err)
	}
	if got := HeatTemperatureChangeRate(373.15); !(got < 0.3) {
		t.Fatalf("дефолт: R(100 °C) = %v, хочу < 0.3 (до применения)", got)
	}
	nodes, bends, err := ApplyPreset("heat", "жесткая-жара")
	if err != nil {
		t.Fatalf("ApplyPreset: %v", err)
	}
	if len(nodes) != 3 || nodes[1].Y != 0.5 {
		t.Fatalf("ApplyPreset вернул узлы %+v, хочу кастомную кривую (y(100)=0.5)", nodes)
	}
	got := HeatTemperatureChangeRate(373.15)
	if got < 0.4 || got > 0.6 {
		t.Errorf("после ApplyPreset: R(100 °C) = %v, хочу ≈ 0.5 (кривая из пресета)", got)
	}
	_ = bends

	if err := DeletePreset("heat", "жесткая-жара"); err != nil {
		t.Fatalf("DeletePreset: %v", err)
	}
	if _, _, err := ApplyPreset("heat", "жесткая-жара"); err != ErrPresetNotFound {
		t.Errorf("ApplyPreset после удаления: %v, хочу ErrPresetNotFound", err)
	}
}

// TestPresetOverwrite — AddPreset с тем же (component, name) — перезапись
// без ошибки, узлы новые, updated_at не раньше прежнего.
func TestPresetOverwrite(t *testing.T) {
	newPresetEnv(t)

	if err := SetCurve("heat", customHeat()); err != nil {
		t.Fatalf("SetCurve: %v", err)
	}
	p1, err := SavePreset("heat", "x")
	if err != nil {
		t.Fatalf("SavePreset #1: %v", err)
	}
	other := ComponentCurve{
		Nodes: []SegmentNode{{X: 30, Y: 0}, {X: 200, Y: 0.7}, {X: 4000, Y: 0.98}},
		Bends: []float64{1, -1},
	}
	if err := SetCurve("heat", other); err != nil {
		t.Fatalf("SetCurve: %v", err)
	}
	p2, err := SavePreset("heat", "x")
	if err != nil {
		t.Fatalf("SavePreset #2 (перезапись): %v", err)
	}
	if p2.UpdatedAt.Before(p1.UpdatedAt) {
		t.Errorf("updated_at не обновился: %v → %v", p1.UpdatedAt, p2.UpdatedAt)
	}
	nodes, bends, err := ApplyPreset("heat", "x")
	if err != nil {
		t.Fatalf("ApplyPreset: %v", err)
	}
	if len(nodes) != 3 || nodes[1].Y != 0.7 {
		t.Errorf("перезапись не применилась: узлы %+v, хочу y(200)=0.7", nodes)
	}
	if bends[0] != 1 || bends[1] != -1 {
		t.Errorf("перезапись не применилась: bends %v, хочу [1 -1]", bends)
	}
}

// TestDefaultNotDeletable — DeletePreset("default") → ошибка (422 на уровне
// хендлера); восстановление — reset-default.
func TestDefaultNotDeletable(t *testing.T) {
	newPresetEnv(t)

	if err := DeletePreset("heat", "default"); err == nil {
		t.Error("удаление default — не ошибка")
	}
	if _, _, err := ApplyPreset("heat", "default"); err != nil {
		t.Errorf("default должен быть применим: %v", err)
	}
}

// TestResetDefaultPreset — испортили кривую SetCurve + AddPreset →
// ResetDefaultPreset: кривая = кодовый дефолт, пресет default в файле
// перезаписан кодовыми узлами, active = default.
func TestResetDefaultPreset(t *testing.T) {
	path := newPresetEnv(t)

	if err := SetCurve("heat", customHeat()); err != nil {
		t.Fatalf("SetCurve: %v", err)
	}
	if _, err := SavePreset("heat", "default"); err != nil {
		t.Fatalf("SavePreset(default): %v", err)
	}
	nodes, _, err := ResetDefaultPreset("heat")
	if err != nil {
		t.Fatalf("ResetDefaultPreset: %v", err)
	}
	def := defaultHeatCurve()
	if len(nodes) != len(def.Nodes) {
		t.Fatalf("ResetDefaultPreset: %d узлов, хочу %d", len(nodes), len(def.Nodes))
	}
	for i := range def.Nodes {
		if nodes[i] != def.Nodes[i] {
			t.Errorf("узел %d после reset-default = %+v, хочу %+v", i, nodes[i], def.Nodes[i])
		}
	}
	// Пресет default в файле перезаписан кодовыми узлами; active = default.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("чтение файла: %v", err)
	}
	var f balancerPresetsFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("файл невалиден: %v", err)
	}
	if f.Active["heat"] != "default" {
		t.Errorf("active[heat] = %q, хочу default", f.Active["heat"])
	}
	found := false
	for _, p := range f.Presets {
		if p.Component == "heat" && p.Name == "default" {
			found = true
			if len(p.Nodes) != len(def.Nodes) || p.Nodes[0].X != def.Nodes[0].X {
				t.Errorf("пресет default в файле не кодовый: %d узлов", len(p.Nodes))
			}
		}
	}
	if !found {
		t.Error("пресет default/heat отсутствует в файле")
	}
}

// TestPresetFileCreatedOnStart — путь не существует → Load создаёт файл с
// 4 default-пресетами, active — все default; повторный Load — идемпотентен.
func TestPresetFileCreatedOnStart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.json")
	t.Cleanup(func() {
		for _, c := range balancerComponentOrder {
			_ = ResetCurve(c)
		}
	})
	if err := LoadBalancerPresets(path); err != nil {
		t.Fatalf("LoadBalancerPresets: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("файл не создан: %v", err)
	}
	var f balancerPresetsFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("созданный файл невалиден: %v", err)
	}
	if len(f.Presets) != len(balancerComponentOrder) {
		t.Errorf("пресетов %d, хочу %d (по одному default на компоненту)", len(f.Presets), len(balancerComponentOrder))
	}
	for _, comp := range balancerComponentOrder {
		if f.Active[comp] != "default" {
			t.Errorf("active[%s] = %q, хочу default", comp, f.Active[comp])
		}
		ok := false
		for _, p := range f.Presets {
			if p.Component == comp && p.Name == "default" {
				ok = true
			}
		}
		if !ok {
			t.Errorf("пресет default/%s отсутствует", comp)
		}
	}
	// Идемпотентность: повторный Load не меняет файл.
	data2, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("чтение: %v", err)
	}
	if err := LoadBalancerPresets(path); err != nil {
		t.Fatalf("повторный Load: %v", err)
	}
	data3, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("чтение после повторного Load: %v", err)
	}
	if !bytes.Equal(data2, data3) {
		t.Error("повторный Load изменил файл (не идемпотентен)")
	}
}

// TestPresetAtomicWrite — после операций рядом с файлом нет *.tmp;
// содержимое валидно; отдельный SavePresetsFile — tmp не остаётся.
func TestPresetAtomicWrite(t *testing.T) {
	path := newPresetEnv(t)

	if err := SetCurve("heat", customHeat()); err != nil {
		t.Fatalf("SetCurve: %v", err)
	}
	if _, err := SavePreset("heat", "x"); err != nil {
		t.Fatalf("SavePreset: %v", err)
	}
	if _, _, err := ApplyPreset("heat", "x"); err != nil {
		t.Fatalf("ApplyPreset: %v", err)
	}
	if err := DeletePreset("heat", "x"); err != nil {
		t.Fatalf("DeletePreset: %v", err)
	}
	assertNoTmp(t, filepath.Dir(path))

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("чтение: %v", err)
	}
	if err := SavePresetsFile(); err != nil {
		t.Fatalf("SavePresetsFile: %v", err)
	}
	assertNoTmp(t, filepath.Dir(path))
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("чтение после SavePresetsFile: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Error("SavePresetsFile изменил файл без изменений состояния")
	}
	var f balancerPresetsFile
	if err := json.Unmarshal(after, &f); err != nil {
		t.Errorf("файл невалиден после записи: %v", err)
	}
}

func assertNoTmp(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("остался tmp-файл: %s", e.Name())
		}
	}
}

// TestPresetValidation422 — невалидные: component не из 4; name пустой /
// > 32 символов / недопустимые символы; регистр различается.
func TestPresetValidation422(t *testing.T) {
	newPresetEnv(t)

	if _, err := SavePreset("acid", "x"); err == nil {
		t.Error("неизвестная компонента — не ошибка")
	}
	if _, err := SavePreset("heat", ""); err == nil {
		t.Error("пустое имя — не ошибка")
	}
	if _, err := SavePreset("heat", strings.Repeat("а", 33)); err == nil {
		t.Error("имя 33 символа — не ошибка")
	}
	for _, bad := range []string{"а/б", `а\б`, `а"б`, "а\nб"} {
		if _, err := SavePreset("heat", bad); err == nil {
			t.Errorf("имя %q — не ошибка (недопустимые символы)", bad)
		}
	}
	// Допустимые: дефис, пробел, кириллица; регистр различается.
	if _, err := SavePreset("heat", "жесткая-жара"); err != nil {
		t.Errorf("допустимое имя отклонено: %v", err)
	}
	if _, err := SavePreset("heat", "Жесткая жара"); err != nil {
		t.Errorf("допустимое имя (заглавная/пробел) отклонено: %v", err)
	}
	if _, err := SavePreset("heat", "жара"); err != nil {
		t.Errorf("допустимое имя (другой регистр) отклонено: %v", err)
	}
	// «Жара» и «жара» — разные пресеты (чувствительность к регистру).
	if _, _, err := ApplyPreset("heat", "Жара"); err != ErrPresetNotFound {
		t.Errorf("«Жара» существует после сохранения «жара»: %v", err)
	}
	if _, _, err := ApplyPreset("heat", "жара"); err != nil {
		t.Errorf("«жара» не найдена: %v", err)
	}
}

// TestPresetPersistenceAcrossRestart — AddPreset + ApplyPreset (имитация
// рестарта: новый LoadBalancerPresets на том же файле) → активные кривые
// восстановлены из active-пресетов.
func TestPresetPersistenceAcrossRestart(t *testing.T) {
	path := newPresetEnv(t)

	if err := SetCurve("heat", customHeat()); err != nil {
		t.Fatalf("SetCurve: %v", err)
	}
	if _, err := SavePreset("heat", "x"); err != nil {
		t.Fatalf("SavePreset: %v", err)
	}
	// «Рестарт»: перезагрузка из того же файла.
	if err := LoadBalancerPresets(path); err != nil {
		t.Fatalf("повторный Load: %v", err)
	}
	nodes, _, _ := GetCurve("heat")
	if len(nodes) != 3 || nodes[1].Y != 0.5 {
		t.Errorf("активная кривая не восстановлена: %+v, хочу y(100)=0.5", nodes)
	}
	_, active, _ := ListPresets("heat")
	if active != "x" {
		t.Errorf("active[heat] = %q, хочу x", active)
	}
}

// TestPresetFileWithoutRecoveryBackCompat — файл пресетов СТАРОГО формата
// (без поля `recovery`, в т.ч. без компоненты hunger) читается: прочие
// компоненты не меняются (heat загружается ровно из файла), hunger получает
// заводское значение скаляра 0.25 (обратная совместимость, §7.1; регресс
// ревью этапа 3).
func TestPresetFileWithoutRecoveryBackCompat(t *testing.T) {
	t.Cleanup(func() {
		for _, c := range balancerComponentOrder {
			_ = ResetCurve(c)
		}
		_ = ResetComponentScalar(HungerCurveKey)
	})

	heat := customHeat()
	hunger := validHungerCurve()

	cases := []struct {
		name    string
		presets []BalancerPreset
		active  map[string]string
	}{
		{
			// Файл до появления hunger: 4 среды, recovery нигде нет.
			name: "old4NoHunger",
			presets: []BalancerPreset{
				{Name: "default", Component: "heat", Nodes: heat.Nodes, Bends: heat.Bends},
				{Name: "default", Component: "cold", Nodes: defaultColdCurve().Nodes, Bends: defaultColdCurve().Bends},
				{Name: "default", Component: "gravity", Nodes: defaultGravityCurve().Nodes, Bends: defaultGravityCurve().Bends},
				{Name: "default", Component: "radiation", Nodes: defaultRadiationCurve().Nodes, Bends: defaultRadiationCurve().Bends},
			},
			active: map[string]string{"heat": "default", "cold": "default", "gravity": "default", "radiation": "default"},
		},
		{
			// hunger есть, но без поля recovery (null).
			name: "hungerWithoutRecovery",
			presets: []BalancerPreset{
				{Name: "default", Component: "heat", Nodes: heat.Nodes, Bends: heat.Bends},
				{Name: "default", Component: HungerCurveKey, Nodes: hunger.Nodes, Bends: hunger.Bends},
			},
			active: map[string]string{"heat": "default", HungerCurveKey: "default"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "old.json")
			// Скаляр испорчен: Load обязан вернуть hunger к заводскому 0.25.
			if err := SetComponentScalar(HungerCurveKey, 0.9); err != nil {
				t.Fatalf("SetComponentScalar: %v", err)
			}
			raw, err := json.Marshal(balancerPresetsFile{Presets: c.presets, Active: c.active})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if err := os.WriteFile(path, raw, 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			if err := LoadBalancerPresets(path); err != nil {
				t.Fatalf("LoadBalancerPresets старого формата: %v", err)
			}

			// Прочие компоненты не изменились — heat ровно из файла.
			nodes, bends, ok := GetCurve("heat")
			if !ok || len(nodes) != len(heat.Nodes) || nodes[1].Y != heat.Nodes[1].Y || bends[0] != heat.Bends[0] {
				t.Errorf("heat из старого файла не применён: %+v / %v", nodes, bends)
			}
			if _, ok := ComponentScalar("heat"); ok {
				t.Error("ComponentScalar(heat) — скаляр у не-эффект-компоненты")
			}
			// hunger получает заводское 0.25 (в файле recovery отсутствовал).
			if v, ok := ComponentScalar(HungerCurveKey); !ok || v != 0.25 {
				t.Errorf("hunger recovery = %v ok=%v, хочу заводское 0.25", v, ok)
			}
		})
	}
}

// TestPresetCorruptFileFallback — файл с битым JSON → Load возвращает
// ошибку-предупреждение, кривые = дефолты, нет panic; первый SavePreset
// пересоздаёт файл.
func TestPresetCorruptFileFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.json")
	t.Cleanup(func() {
		for _, c := range balancerComponentOrder {
			_ = ResetCurve(c)
		}
	})
	if err := os.WriteFile(path, []byte("{broken json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := LoadBalancerPresets(path); err == nil {
		t.Error("битый JSON — не ошибка")
	}
	// Кривые = дефолты (сервер не падает, store в начальном состоянии).
	nodes, _, _ := GetCurve("heat")
	if len(nodes) != len(defaultHeatCurve().Nodes) {
		t.Errorf("кривая после битого файла: %d узлов, хочу дефолт %d", len(nodes), len(defaultHeatCurve().Nodes))
	}
	// Первый SavePreset пересоздаёт файл.
	if _, err := SavePreset("heat", "ok"); err != nil {
		t.Fatalf("SavePreset после битого файла: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("файл не пересоздан: %v", err)
	}
	var f balancerPresetsFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Errorf("пересозданный файл невалиден: %v", err)
	}
}

// TestPresetUnknownComponentAndApply404 — ApplyPreset с отсутствующим
// (component, name) → ErrPresetNotFound (404 на уровне хендлера);
// на несуществующей компоненте — ошибка валидации.
func TestPresetUnknownComponentAndApply404(t *testing.T) {
	newPresetEnv(t)

	if _, _, err := ApplyPreset("heat", "nope"); err != ErrPresetNotFound {
		t.Errorf("ApplyPreset(heat/nope): %v, хочу ErrPresetNotFound", err)
	}
	if _, _, err := ApplyPreset("acid", "x"); err == nil || err == ErrPresetNotFound {
		t.Errorf("ApplyPreset(acid/x): %v, хочу ошибку валидации (не 404)", err)
	}
	if _, _, ok := ListPresets("acid"); ok {
		t.Error("ListPresets(acid) — ok, хочу false")
	}
	if _, _, ok := ListPresets("heat"); !ok {
		t.Error("ListPresets(heat) — !ok, хочу true")
	}
}
