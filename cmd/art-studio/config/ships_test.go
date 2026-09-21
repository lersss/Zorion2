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
    "family": "F0",
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
	if e.RaceName != "Люди" || e.Family != "F0" {
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

// TestParseShipsTypes — формат «раса → список типов»: types читаются,
// незаполненные поля типа наследуют базовые, плоский формат = один безымянный
// тип (обратная совместимость), дубль/пустое имя типа и пустая texture — ошибка.
func TestParseShipsTypes(t *testing.T) {
	data := []byte(`{
  "humans": {
    "race_name": "Люди",
    "family": "F0",
    "texture": "base hull",
    "silhouette": "base sil",
    "blocked": ["tentacle"],
    "types": [
      {"type": "starship", "texture": "starship hull"},
      {"type": "cruiser", "texture": "cruiser hull", "blocked": ["Alien", "alien", " crystal "]}
    ]
  }
}`)
	ships, err := ParseShips(data, "test", racesPath)
	if err != nil {
		t.Fatalf("ParseShips: %v", err)
	}
	names := ships["humans"].ShipTypeNames()
	if len(names) != 2 || names[0] != "starship" || names[1] != "cruiser" {
		t.Fatalf("типы = %v, want [starship cruiser]", names)
	}
	// наследование: silhouette/blocked типа берутся из базовых, если не заданы
	st, ok := ships["humans"].ResolveShipType("starship")
	if !ok || st.Texture != "starship hull" || st.Silhouette != "base sil" {
		t.Errorf("starship = %+v, want texture starship hull / silhouette base sil", st)
	}
	if len(st.Blocked) != 1 || st.Blocked[0] != "tentacle" {
		t.Errorf("starship blocked = %v, want [tentacle] (наследие базового)", st.Blocked)
	}
	// собственный blocked типа нормализован (lowercase/trim/дедуп)
	cr, ok := ships["humans"].ResolveShipType("cruiser")
	if !ok || len(cr.Blocked) != 2 || cr.Blocked[0] != "alien" || cr.Blocked[1] != "crystal" {
		t.Errorf("cruiser blocked = %v, want [alien crystal]", cr.Blocked)
	}
	// плоская запись = один безымянный тип
	flat := ShipsConfig{"coastal": {Texture: "t", Silhouette: "s", Blocked: []string{"machine"}}}
	ft := flat["coastal"].ShipTypes()
	if len(ft) != 1 || ft[0].Type != "" || ft[0].Texture != "t" {
		t.Errorf("плоская запись = %+v, want один безымянный тип", ft)
	}
}

// TestParseShipsTypesErrors — ошибки валидации types.
func TestParseShipsTypesErrors(t *testing.T) {
	cases := []struct {
		name string
		data string
		want string
	}{
		{"пустое имя типа", `{"humans": {"race_name":"Л","family":"F0","texture":"t","silhouette":"s","types":[{"type":"","texture":"x"}]}}`, "пустое имя типа"},
		{"дубль типа", `{"humans": {"race_name":"Л","family":"F0","texture":"t","silhouette":"s","types":[{"type":"a","texture":"x"},{"type":"a","texture":"y"}]}}`, "дубль типа"},
		{"texture типа пуст", `{"humans": {"race_name":"Л","family":"F0","texture":"","silhouette":"s","types":[{"type":"a","texture":""}]}}`, "texture пуст"},
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
	if !ok || e.RaceName != "Люди" || e.Family != "F0" {
		t.Errorf("humans = %+v", e)
	}
	if e.Texture == "" || e.Silhouette == "" || len(e.Blocked) == 0 {
		t.Errorf("humans: пустые поля")
	}
	// люди — 4 типа корабля с разными texture (starship/cruiser/carrier/fighter)
	names := e.ShipTypeNames()
	if len(names) != 4 {
		t.Fatalf("humans типов = %d, want 4 (%v)", len(names), names)
	}
	seenTex := map[string]bool{}
	for _, ty := range e.ShipTypes() {
		if ty.Texture == "" {
			t.Errorf("тип %s: пустая texture", ty.Type)
		}
		if seenTex[ty.Texture] {
			t.Errorf("тип %s: texture-клон", ty.Type)
		}
		seenTex[ty.Texture] = true
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

**Семейство:** F0 Люди
**ID:** ` + "`humans`" + `

## Корабль (внешний вид)

**Форма и силуэт.** Текст.

**Для генератора (texture):** paneled white-grey metal hull, no organic shapes

**Для генератора (silhouette):** крыло-корпус в плане: нос справа, корма слева

**Для генератора (blocked):** tentacle, organic, crystal

## Кабина (место аватара)

**Обстановка.** Не маркер генератора.
`
	lore, err := ParseShipSection(md)
	if err != nil {
		t.Fatalf("ParseShipSection: %v", err)
	}
	if lore.Texture != "paneled white-grey metal hull, no organic shapes" {
		t.Errorf("texture = %q", lore.Texture)
	}
	if lore.Silhouette != "крыло-корпус в плане: нос справа, корма слева" {
		t.Errorf("silhouette = %q", lore.Silhouette)
	}
	if len(lore.Blocked) != 3 || lore.Blocked[0] != "tentacle" || lore.Blocked[2] != "crystal" {
		t.Errorf("blocked = %v", lore.Blocked)
	}
	if lore.Family != "F0" {
		t.Errorf("family = %q, want F0", lore.Family)
	}
	if len(lore.Types) != 0 {
		t.Errorf("types = %v, want пусто (легаси)", lore.Types)
	}
}

// TestParseShipSectionTypes — маркеры типов «(type <имя> texture/…)»: типы
// собираются в порядке появления, базовая texture остаётся, per-type blocked.
func TestParseShipSectionTypes(t *testing.T) {
	md := `# Люди — корабль (humans)

**Семейство:** F0 Люди

## Корабль (внешний вид)

**Для генератора (texture):** base hull

**Для генератора (silhouette):** base sil

**Для генератора (blocked):** tentacle, organic

**Для генератора (type starship texture):** starship hull

**Для генератора (type cruiser texture):** cruiser hull

**Для генератора (type cruiser blocked):** alien, crystal

## Кабина (место аватара)
`
	lore, err := ParseShipSection(md)
	if err != nil {
		t.Fatalf("ParseShipSection: %v", err)
	}
	if lore.Texture != "base hull" || lore.Silhouette != "base sil" {
		t.Errorf("базовые поля = %q / %q", lore.Texture, lore.Silhouette)
	}
	if len(lore.Types) != 2 || lore.Types[0].Type != "starship" || lore.Types[1].Type != "cruiser" {
		t.Fatalf("типы = %+v, want [starship cruiser]", lore.Types)
	}
	if lore.Types[0].Texture != "starship hull" || lore.Types[0].Blocked != nil {
		t.Errorf("starship = %+v", lore.Types[0])
	}
	if lore.Types[1].Texture != "cruiser hull" || len(lore.Types[1].Blocked) != 2 {
		t.Errorf("cruiser = %+v", lore.Types[1])
	}
}

// TestParseShipSectionErrors — нет раздела/маркеров → ошибка.
func TestParseShipSectionErrors(t *testing.T) {
	md := `# X

**Семейство:** F2

## Другая секция

**Для генератора (texture):** t
`
	if _, err := ParseShipSection(md); err == nil {
		t.Errorf("ожидалась ошибка (нет раздела)")
	}
}