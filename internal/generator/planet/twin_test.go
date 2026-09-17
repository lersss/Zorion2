// Тесты генератора «близнецов» (twin.go): детерминированные клоны шаблона
// с оверрайдами группы и раскладкой по соседствующим квадратам.
package planet

import (
	"encoding/json"
	"testing"

	"zorion/internal/generator/settlement"
)

// mustData — JSON планеты как map.
func mustData(t *testing.T, p *PlanetData) map[string]interface{} {
	t.Helper()
	var data map[string]interface{}
	if err := json.Unmarshal(p.Data, &data); err != nil {
		t.Fatalf("unmarshal %s: %v", p.Name, err)
	}
	return data
}

// Близнецы в группе идентичны по физике, отличаются только оверрайдом между
// группами; тег _experiment проставлен; звезды двух групп стоят рядом.
func TestGenerateTwins(t *testing.T) {
	spec := TwinSpec{
		ID: "young_worlds",
		Base: map[string]interface{}{
			"temperature": 288.0, "water_percent": 70.0, "mass": 1.0,
			"type": "землеподобная", "surface_dominant": "океаны",
			"atmosphere": "азотно-кислородная", "hydrosphere": "океаны",
			"system_age": 1.0,
		},
		Groups: []TwinGroup{
			{ID: "young", Name: "Молодые", PlanetsPerWorld: 3,
				Overrides: map[string]interface{}{"system_age": 1.0}},
			{ID: "old", Name: "Старые", PlanetsPerWorld: 3,
				Overrides: map[string]interface{}{"system_age": 10.0}},
		},
	}

	g := NewGenerator(nil, 5)
	worlds, planets, err := g.GenerateTwins(spec, nil)
	if err != nil {
		t.Fatalf("GenerateTwins: %v", err)
	}

	// Ровно одна звезда на группу, рядом: young (0,0), old (600,0).
	if len(worlds["young"]) != 1 || len(worlds["old"]) != 1 {
		t.Fatalf("звёзд: young=%d old=%d, ожидалось по 1", len(worlds["young"]), len(worlds["old"]))
	}
	young := worlds["young"][0]
	old := worlds["old"][0]
	if young.CoordX != 0 || young.CoordY != 0 {
		t.Errorf("young звезда должна быть в (0,0), получил (%v, %v)", young.CoordX, young.CoordY)
	}
	if old.CoordX != twinStarOffset || old.CoordY != 0 {
		t.Errorf("old звезда должна быть в (%.0f,0), получил (%v, %v)", twinStarOffset, old.CoordX, old.CoordY)
	}

	if len(planets["young"]) != 3 || len(planets["old"]) != 3 {
		t.Fatalf("планет: young=%d old=%d, ожидалось по 3", len(planets["young"]), len(planets["old"]))
	}

	// Внутри группы планеты физически идентичны (только system_age групп).
	checkGroup := func(group string, wantAgeVal float64) {
		first := mustData(t, planets[group][0])
		for i, p := range planets[group] {
			d := mustData(t, p)
			if d["temperature"] != first["temperature"] || d["water_percent"] != first["water_percent"] {
				t.Errorf("%s[%d]: физика отличается внутри группы", group, i)
			}
			if age, ok := d["system_age"].(float64); !ok || age != wantAgeVal {
				t.Errorf("%s[%d]: system_age = %v, ожидалось %v", group, i, d["system_age"], wantAgeVal)
			}
			exp, ok := d["_experiment"].(map[string]interface{})
			if !ok || exp["id"] != "young_worlds" || exp["group"] != group {
				t.Errorf("%s[%d]: тег _experiment = %v", group, i, d["_experiment"])
			}
		}
	}
	checkGroup("young", 1.0)
	checkGroup("old", 10.0)

	// Между группами отличается ровно одна ось.
	y := mustData(t, planets["young"][0])
	o := mustData(t, planets["old"][0])
	for k, v := range y {
		if k == "system_age" || k == "_experiment" {
			continue
		}
		if ov, ok := o[k]; !ok || ov != v {
			t.Errorf("ось %q отличается между группами: young=%v old=%v", k, v, ov)
		}
	}
}

// Dot-override (core.radioactivity) сливается вложенно: остальные поля core
// наследуются из base и не затираются.
func TestCloneTwinDataDotOverride(t *testing.T) {
	base := map[string]interface{}{
		"temperature": 288.0,
		"core": map[string]interface{}{
			"type":         "металлическое",
			"mass_percent": 30.0,
			"activity":     60.0,
			"radioactivity": 20.0,
			"age":          1.0,
		},
	}
	overrides := map[string]interface{}{
		"core.radioactivity": 90.0,
	}

	result := cloneTwinData(base, overrides)

	// base не затёрт.
	if result["temperature"] != 288.0 {
		t.Errorf("temperature потерян: %v", result["temperature"])
	}

	// core — вложенный map с обновлённым radioactivity.
	core, ok := result["core"].(map[string]interface{})
	if !ok {
		t.Fatalf("core потерял вложенность: %T %v", result["core"], result["core"])
	}
	if core["radioactivity"] != 90.0 {
		t.Errorf("core.radioactivity = %v, ожидалось 90", core["radioactivity"])
	}
	// Остальные поля core не затёрты.
	if core["type"] != "металлическое" {
		t.Errorf("core.type потерян: %v", core["type"])
	}
	if core["mass_percent"] != 30.0 {
		t.Errorf("core.mass_percent потерян: %v", core["mass_percent"])
	}

	// base не мутирован.
	baseCore, _ := base["core"].(map[string]interface{})
	if baseCore["radioactivity"] != 20.0 {
		t.Errorf("base мутирован: core.radioactivity = %v", baseCore["radioactivity"])
	}
}

// Невалидный spec отклоняется до генерации.
func TestTwinSpecValidate(t *testing.T) {
	valid := TwinSpec{ID: "x", Base: map[string]interface{}{"temperature": 288.0},
		Groups: []TwinGroup{{ID: "g", PlanetsPerWorld: 1}}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("валидный spec отклонён: %v", err)
	}

	invalid := []TwinSpec{
		{ID: "", Base: map[string]interface{}{"temperature": 288.0},
			Groups: []TwinGroup{{ID: "g", PlanetsPerWorld: 1}}},
		{ID: "x", Base: map[string]interface{}{}, Groups: []TwinGroup{{ID: "g", PlanetsPerWorld: 1}}},
		{ID: "x", Base: map[string]interface{}{"temperature": 288.0}, Groups: nil},
		{ID: "x", Base: map[string]interface{}{"temperature": 288.0},
			Groups: []TwinGroup{{ID: "g", PlanetsPerWorld: 0}}},
		{ID: "x", Base: map[string]interface{}{"temperature": 288.0},
			Groups: []TwinGroup{{ID: "g", PlanetsPerWorld: 1,
				Settlement: SettlementSpec{Chance: 1.0,
					Population: settlement.Population{Kind: "random", Min: 10, Max: 1}}}}},
		{ID: "x", Base: map[string]interface{}{"temperature": 288.0},
			Groups: []TwinGroup{{ID: "g", PlanetsPerWorld: 1,
				Settlement: SettlementSpec{Chance: 1.0,
					Population: settlement.Population{Kind: "random", Min: 1, Max: 3_000_000_000}}}}},
		{ID: "x", Base: map[string]interface{}{"temperature": 288.0},
			Groups: []TwinGroup{{ID: "g", PlanetsPerWorld: 1,
				Settlement: SettlementSpec{Chance: 1.0, SettlementsPerPlanet: -1}}}},
	}
	for _, s := range invalid {
		if err := s.Validate(); err == nil {
			t.Errorf("spec %+v должен быть невалидным", s)
		}
	}
}

// Раса группы (99.2.23 §5.1): race_id из каталога, пусто — ок; неизвестная
// раса — ошибка валидации.
func TestTwinSpecValidateRaceID(t *testing.T) {
	valid := TwinSpec{ID: "x", Base: map[string]interface{}{"temperature": 288.0},
		Groups: []TwinGroup{{ID: "g", PlanetsPerWorld: 1, RaceID: "ammonia"}}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("валидный race_id отклонён: %v", err)
	}

	// Пусто = люди/легаси — ок.
	empty := TwinSpec{ID: "x", Base: map[string]interface{}{"temperature": 288.0},
		Groups: []TwinGroup{{ID: "g", PlanetsPerWorld: 1}}}
	if err := empty.Validate(); err != nil {
		t.Fatalf("пустой race_id отклонён: %v", err)
	}

	// Неизвестная раса — ошибка.
	bad := TwinSpec{ID: "x", Base: map[string]interface{}{"temperature": 288.0},
		Groups: []TwinGroup{{ID: "g", PlanetsPerWorld: 1, RaceID: "no_such_race"}}}
	if err := bad.Validate(); err == nil {
		t.Fatal("неизвестная раса должна быть отклонена")
	}
}

// Числовые поля base/overrides сверяются с рамками реестра полей: вне рамок —
// ошибка (защита от «непроизводимого генератором» объекта, например массы
// коричневого карлика). Температура в данных — K, сверяется как (K−273) °C.
func TestTwinSpecValidateFieldRanges(t *testing.T) {
	valid := TwinSpec{ID: "x", Base: map[string]interface{}{"temperature": 288.0, "mass": 1.0},
		Groups: []TwinGroup{{ID: "g", PlanetsPerWorld: 1,
			Overrides: map[string]interface{}{"temperature": 500.0, "mass": 8.0, "gravity": 2.0}}}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("валидный spec отклонён: %v", err)
	}

	// Границы включительно: 20 K = −253 °C, 2500 K = 2227 °C.
	edge := TwinSpec{ID: "x", Base: map[string]interface{}{"temperature": 20.0},
		Groups: []TwinGroup{{ID: "g", PlanetsPerWorld: 1,
			Overrides: map[string]interface{}{"temperature": 2500.0}}}}
	if err := edge.Validate(); err != nil {
		t.Fatalf("граничные значения отклонены: %v", err)
	}

	cases := []struct {
		name string
		base map[string]interface{}
		ov   map[string]interface{}
	}{
		{"масса 10000 — коричневый карлик", map[string]interface{}{"mass": 10000.0}, nil},
		{"температура 19 K ниже минимума", map[string]interface{}{"temperature": 19.0}, nil},
		{"температура 2501 K выше максимума", map[string]interface{}{"temperature": 2501.0}, nil},
		{"плотность 0.05 ниже минимума", nil, map[string]interface{}{"density": 0.05}},
		{"гравитация 40 выше максимума", nil, map[string]interface{}{"gravity": 40.0}},
		{"спутников 30 выше максимума", nil, map[string]interface{}{"moons": 30}},
		{"радиоактивность ядра 150 выше максимума",
			map[string]interface{}{"core": map[string]interface{}{"radioactivity": 150.0}}, nil},
		{"размер 12.6 выше плато", nil, map[string]interface{}{"size": 12.6}},
	}
	for _, tc := range cases {
		spec := TwinSpec{ID: "x", Base: map[string]interface{}{"temperature": 288.0},
			Groups: []TwinGroup{{ID: "g", PlanetsPerWorld: 1}}}
		for k, v := range tc.base {
			spec.Base[k] = v
		}
		if tc.ov != nil {
			spec.Groups[0].Overrides = tc.ov
		}
		if err := spec.Validate(); err == nil {
			t.Errorf("%s: должен быть отклонён", tc.name)
		}
	}
}