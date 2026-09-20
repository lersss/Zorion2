package config

import (
	"fmt"
	"strings"
	"testing"
)

const racesPath = "../../../config/races.json"

// TestParseShips — валидация ships.json: ключ ∈ id races.json, поля непустые,
// blocked нормализуется (lowercase/trim/дедуп).
func TestParseShips(t *testing.T) {
	data := []byte(`{
  "humans": {
    "race_name": "Люди",
    "family": "F1",
    "texture": "paneled white-grey metal hull",
    "silhouette": "крыло-корпус в плане: нос справа, корма с дюзами слева",
    "blocked": ["Tentacle", "organic", "tentacle", " crystal "]
  }
}`)
	ships, err := ParseShips(data, "test", racesPath)
	if err != nil {
		t.Fatalf("ParseShips: %v", err)
	}
	e := ships["humans"]
	if e.RaceName != "Люди" || e.Family != "F1" {
		t.Errorf("race_name/family = %q/%q", e.RaceName, e.Family)
	}
	want := []string{"tentacle", "organic", "crystal"}
	if len(e.Blocked) != len(want) {
		t.Fatalf("blocked = %v, want %v", e.Blocked, want)
	}
	for i := range want {
		if e.Blocked[i] != want[i] {
			t.Errorf("blocked[%d] = %q, want %q", i, e.Blocked[i], want[i])
		}
	}
}

// TestParseShipsErrors — ошибки валидации (спека §4.1).
func TestParseShipsErrors(t *testing.T) {
	cases := []struct {
		name string
		data string
		want string
	}{
		{"нет в races.json", `{"nope": {"race_name":"X","family":"F1","texture":"t","silhouette":"s"}}`, "нет в config/races.json"},
		{"texture пуст", `{"humans": {"race_name":"Люди","family":"F1","texture":"  ","silhouette":"s"}}`, "texture пуст"},
		{"texture с переносом", `{"humans": {"race_name":"Люди","family":"F1","texture":"a\nb","silhouette":"s"}}`, "перенос строки"},
		{"silhouette пуст", `{"humans": {"race_name":"Люди","family":"F1","texture":"t","silhouette":""}}`, "silhouette пуст"},
		{"blocked пустой токен", `{"humans": {"race_name":"Люди","family":"F1","texture":"t","silhouette":"s","blocked":["a", " "]}}`, "пустой токен"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseShips([]byte(c.data), "test", racesPath)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want содержит %q", err, c.want)
			}
		})
	}
}

// TestLoadShips — реальный ships.json (проекция 60 рас) валиден: ключи ∈ id
// races.json, поля непустые, blocked нормализован.
func TestLoadShips(t *testing.T) {
	ships, err := LoadShipsRaces("../../../config/art/ships.json", racesPath)
	if err != nil {
		t.Fatalf("LoadShips: %v", err)
	}
	if len(ships) != 60 {
		t.Errorf("записей = %d, want 60", len(ships))
	}
	e, ok := ships["humans"]
	if !ok || e.RaceName != "Люди" || e.Family != "F1" {
		t.Errorf("humans = %+v", e)
	}
	if e.Texture == "" || e.Silhouette == "" || len(e.Blocked) == 0 {
		t.Errorf("humans: пустые поля")
	}
}

// TestParseShipDictConcepts — валидация концептов рас (ТЗ: непустой,
// ≤ 60 символов, без \n).
func TestParseShipDictConcepts(t *testing.T) {
	base := `{
  "forms": [{"key": "крыло", "concept": "wing-shaped hull", "category": "wing"}],
  "details": [{"key": "щупальца", "stem": "щупал", "phrase": "trailing tentacle antennae"}],
  "warm_markers": ["molten"],
  "textures": [{"tag": "panel lines", "warmth": "neutral", "blocked_by": []}],
  "concepts": ` + "%s" + `
}`
	// валидный концепт
	if _, err := ParseShipDict([]byte(fmt.Sprintf(base, `{"ammonia": "root-mat pod vessel"}`)), "test"); err != nil {
		t.Errorf("валидный концепт: %v", err)
	}
	// пустой концепт → ошибка
	if _, err := ParseShipDict([]byte(fmt.Sprintf(base, `{"ammonia": "  "}`)), "test"); err == nil || !strings.Contains(err.Error(), "пуст") {
		t.Errorf("пустой концепт: err = %v, want содержит «пуст»", err)
	}
	// концепт с \n → ошибка
	if _, err := ParseShipDict([]byte(fmt.Sprintf(base, `{"ammonia": "a\nb"}`)), "test"); err == nil || !strings.Contains(err.Error(), "перенос") {
		t.Errorf("концепт с \\n: err = %v, want содержит «перенос»", err)
	}
	// концепт > 60 символов → ошибка
	long := strings.Repeat("x", 61)
	if _, err := ParseShipDict([]byte(fmt.Sprintf(base, `{"ammonia": "`+long+`"}`)), "test"); err == nil || !strings.Contains(err.Error(), "60") {
		t.Errorf("длинный концепт: err = %v, want содержит «60»", err)
	}
}

// TestParseShipDict — валидация ship_dict.json: формы/детали/маркеры/теги.
func TestParseShipDict(t *testing.T) {
	data := []byte(`{
  "forms": [
    {"key": "крыло", "concept": "wing-shaped hull", "category": "wing"},
    {"key": "капсула", "concept": "capsule", "category": "capsule"}
  ],
  "details": [
    {"key": "щупальца", "stem": "щупал", "phrase": "trailing tentacle antennae"}
  ],
  "warm_markers": ["molten", "amber"],
  "textures": [
    {"tag": "panel lines", "warmth": "neutral", "blocked_by": []},
    {"tag": "ember glow beneath", "warmth": "warm", "blocked_by": ["machine"]}
  ]
}`)
	dc, err := ParseShipDict(data, "test")
	if err != nil {
		t.Fatalf("ParseShipDict: %v", err)
	}
	if len(dc.Forms) != 2 || len(dc.Details) != 1 || len(dc.WarmMarkers) != 2 || len(dc.Textures) != 2 {
		t.Errorf("неполный словарь: %+v", dc)
	}
}

// TestParseShipDictErrors — ошибки валидации (спека §4.2).
func TestParseShipDictErrors(t *testing.T) {
	cases := []struct {
		name string
		data string
		want string
	}{
		{"forms пуст", `{"forms": [], "details": [{"key":"a","stem":"a","phrase":"b"}], "warm_markers": ["x"], "textures": [{"tag":"t","warmth":"neutral","blocked_by":[]}]}`, "forms пуст"},
		{"категория вне библиотеки", `{"forms": [{"key":"к","concept":"c","category":"nope"}], "details": [{"key":"a","stem":"a","phrase":"b"}], "warm_markers": ["x"], "textures": [{"tag":"t","warmth":"neutral","blocked_by":[]}]}`, "не в библиотеке"},
		{"warmth вне набора", `{"forms": [{"key":"к","concept":"c","category":"wing"}], "details": [{"key":"a","stem":"a","phrase":"b"}], "warm_markers": ["x"], "textures": [{"tag":"t","warmth":"hot","blocked_by":[]}]}`, "warmth"},
		{"дубль warm_markers", `{"forms": [{"key":"к","concept":"c","category":"wing"}], "details": [{"key":"a","stem":"a","phrase":"b"}], "warm_markers": ["x", "x"], "textures": [{"tag":"t","warmth":"neutral","blocked_by":[]}]}`, "дубль"},
		{"дубль тега", `{"forms": [{"key":"к","concept":"c","category":"wing"}], "details": [{"key":"a","stem":"a","phrase":"b"}], "warm_markers": ["x"], "textures": [{"tag":"t","warmth":"neutral","blocked_by":[]}, {"tag":"t","warmth":"cold","blocked_by":[]}]}`, "дубль тега"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseShipDict([]byte(c.data), "test")
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want содержит %q", err, c.want)
			}
		})
	}
}

// TestLoadShipDict — реальный словарь ship_dict.json валиден и полон
// (13 форм, 11 деталей, тёплые маркеры, ~30 текстурных тегов).
func TestLoadShipDict(t *testing.T) {
	dc, err := LoadShipDict("../../../config/art/ship_dict.json")
	if err != nil {
		t.Fatalf("LoadShipDict: %v", err)
	}
	if len(dc.Forms) != 13 {
		t.Errorf("forms = %d, want 13", len(dc.Forms))
	}
	if len(dc.Details) != 11 {
		t.Errorf("details = %d, want 11", len(dc.Details))
	}
	if len(dc.WarmMarkers) < 5 {
		t.Errorf("warm_markers = %d, want >= 5", len(dc.WarmMarkers))
	}
	if len(dc.Textures) < 30 {
		t.Errorf("textures = %d, want >= 30", len(dc.Textures))
	}
	if len(dc.Concepts) != 60 {
		t.Errorf("concepts = %d, want 60 (по одной на расу)", len(dc.Concepts))
	}
	// «glow»/«heat» не маркеры: ложные срабатывания («cold engine glow»,
	// «heat shield» у людей) — решение при разработке словаря.
	for _, m := range dc.WarmMarkers {
		if m == "glow" || m == "heat" {
			t.Errorf("warm_markers содержит ложный маркер %q", m)
		}
	}
}

// TestParseShipSection — парсер раздела «Корабль (внешний вид)»: маркеры
// texture/silhouette/blocked + семейство из шапки (спека §4.1).
func TestParseShipSection(t *testing.T) {
	md := `# Люди — корабль (humans)

**Семейство:** F1 Водные
**ID:** ` + "`humans`" + `

## Корабль (внешний вид)

**Форма и силуэт.** Текст.

**Для генератора (texture):** paneled white-grey metal hull, no organic shapes

**Для генератора (silhouette):** крыло-корпус в плане: нос справа, корма слева

**Для генератора (blocked):** tentacle, organic, crystal

## Кабина (место аватара)

**Обстановка.** Не маркер генератора.
`
	texture, silhouette, blocked, family, err := ParseShipSection(md)
	if err != nil {
		t.Fatalf("ParseShipSection: %v", err)
	}
	if texture != "paneled white-grey metal hull, no organic shapes" {
		t.Errorf("texture = %q", texture)
	}
	if silhouette != "крыло-корпус в плане: нос справа, корма слева" {
		t.Errorf("silhouette = %q", silhouette)
	}
	if len(blocked) != 3 || blocked[0] != "tentacle" || blocked[2] != "crystal" {
		t.Errorf("blocked = %v", blocked)
	}
	if family != "F1" {
		t.Errorf("family = %q, want F1", family)
	}
}

// TestParseShipSectionErrors — нет раздела/маркеров → ошибка.
func TestParseShipSectionErrors(t *testing.T) {
	md := `# X

**Семейство:** F2

## Другая секция

**Для генератора (texture):** t
`
	if _, _, _, _, err := ParseShipSection(md); err == nil {
		t.Errorf("ожидалась ошибка (нет раздела)")
	}
}