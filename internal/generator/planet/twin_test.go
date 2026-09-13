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