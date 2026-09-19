// internal/races/lore.go — лор рас (спека 86a §5.1.1): config/race_lore.json,
// машиночитаемая проекция 22_races.md §3/§4. Загружается при старте тем же
// пакетом, что и каталог; валидация формата как у каталога рас.
package races

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// RaceLore — лорная запись расы: family (F1–F9 / "robotic"), character
// («характер» словами), how_live («как живут»), why («зачем»/ниша),
// coexistence («сосуществование»); origin — только у роботов (происхождение:
// люди/самозародившийся/другие био/другие ИИ, 22_races.md §4.2).
//
// Новые поля карточки «игровое восприятие» (спека 99.2.26 §3.2): kind — «Вид»
// (§4), niche — «Ниша» (§5), size_individual — «Размер особи» (§6.4),
// size_group — «Размер стаи/роя» (§6.5, строка у 17 коллективных рас, null у
// остальных 43), home_words — «Дом» словами, lore — художественный текст
// абзацами (\n\n), attributes_words — 6 слов атрибутов (дериваты чисел
// races.json, §6).
type RaceLore struct {
	ID          string `json:"id"`
	Family      string `json:"family"`
	Character   string `json:"character"`
	HowLive     string `json:"how_live"`
	Why         string `json:"why"`
	Coexistence string `json:"coexistence"`
	Origin      string `json:"origin,omitempty"`

	Kind            string            `json:"kind"`
	Niche           string            `json:"niche"`
	SizeIndividual  string            `json:"size_individual"`
	SizeGroup       *string           `json:"size_group"`
	HomeWords       string            `json:"home_words"`
	Lore            string            `json:"lore"`
	AttributesWords map[string]string `json:"attributes_words"`
}

// loreCatalog — загруженный лор (read-only после LoadLore; загрузка при
// старте до горутин — блокировка не нужна, паттерн catalog).
var loreCatalog []*RaceLore

// loreFamilies — допустимые семейства (22_races.md §2.2/§4).
var loreFamilies = map[string]bool{
	"F1": true, "F2": true, "F3": true, "F4": true, "F5": true,
	"F6": true, "F7": true, "F8": true, "F9": true, "robotic": true,
}

// kindWords — словарь «Вид» (спека 99.2.26 §4, 11 значений): природа существа
// (форма тела/тип бытия), не экологическая роль (роль — «Ниша», nicheWords).
var kindWords = map[string]bool{
	"гуманоид": true, "водный": true, "растение": true, "рой": true,
	"кристалл": true, "аморфный": true, "машина": true, "космический": true,
	"микроорганизм": true, "зверообразный": true, "гигант": true,
}

// nicheWords — словарь «Ниша» (спека 99.2.26 §5, 12 значений): экологическая
// роль («как живёт в своей среде»), не форма тела (форма — kindWords).
var nicheWords = map[string]bool{
	"хищник": true, "собиратель": true, "пастух": true, "домосед": true,
	"кочевник": true, "строитель": true, "исследователь": true, "наблюдатель": true,
	"планктон": true, "растение": true, "переработчик": true, "хранитель": true,
}

// sizeIndividualSteps — шкала «Размер особи» (спека 99.2.26 §6.4, 8 ступеней).
var sizeIndividualSteps = map[string]bool{
	"микроскопические": true, "с крупинку": true, "с ладонь": true,
	"с человека": true, "крупные": true, "гигантские": true,
	"колоссальные": true, "планетарные": true,
}

// sizeGroupSteps — шкала «Размер стаи/роя» (спека 99.2.26 §6.5, 5 ступеней).
var sizeGroupSteps = map[string]bool{
	"небольшая стая": true, "стая": true, "крупный рой": true,
	"рой-облако": true, "рой-покров": true,
}

// collectiveRaces — 17 коллективных рас (спека 99.2.26 §6.5): size_group
// обязателен, у остальных 43 — null.
var collectiveRaces = map[string]bool{
	"cryo_swarms": true, "mist_swarms": true, "sulfur_swarms": true,
	"thermo_swarms": true, "silicate_swarms": true, "spark": true,
	"scavengers": true, "echo_aliens": true, "methane_fungi": true,
	"methane_plankton": true, "ice_plankton": true, "hydrogen": true,
	"cryo_corals": true, "world_machines": true, "salt_bridge": true,
	"forges": true, "silicon_threads": true,
}

// attributeKeys — 6 осей атрибутов (порядок карточки, спека 99.2.26 §3.2).
var attributeKeys = []string{
	"aggression", "curiosity", "reproduction", "intelligence", "diplomacy", "resilience",
}

// attributeScales — линейные шкалы прилагательных (спека 99.2.26 §6.1):
// агрессия/любопытство/интеллект/дипломатия/устойчивость, 0–100 (50 = человек).
// Граница включительная: v <= limit (значение на границе — верхняя ступень).
// Ступени-резервы (пустые в текущих данных): хрупкие 0–20, переговорщики
// 66–75, завоеватели 86–100, почти неразрушимые 86–100.
var attributeScales = map[string][]struct {
	limit float64
	word  string
}{
	"aggression": {
		{20, "мирные"}, {45, "умеренные"}, {70, "агрессивные"},
		{85, "воинственные"}, {100, "завоеватели"},
	},
	"curiosity": {
		{20, "нелюбопытные"}, {44, "малолюбопытные"}, {56, "любопытные"},
		{68, "любознательные"}, {100, "очень любознательные"},
	},
	"intelligence": {
		{20, "примитивные"}, {45, "простые"}, {70, "умные"},
		{85, "очень умные"}, {100, "гении"},
	},
	"diplomacy": {
		{20, "недипломатичные"}, {45, "сдержанные"}, {65, "дипломатичные"},
		{75, "переговорщики"}, {100, "миротворцы"},
	},
	"resilience": {
		{20, "хрупкие"}, {45, "уязвимые"}, {70, "устойчивые"},
		{85, "очень устойчивые"}, {100, "почти неразрушимые"},
	},
}

// reproductionScale — лог-шкала размножения (спека 99.2.26 §6.2): m — множитель
// × человек (reproduction в races.json). Граница строгая: m < limit; последний
// шаг {0, слово} — рабочий фолбэк для m >= 100 («в сотни раз быстрее»).
var reproductionScale = []struct {
	limit float64
	word  string
}{
	{0.0001, "в сотни тысяч раз медленнее"},
	{0.001, "в тысячи раз медленнее"},
	{0.01, "в сотни раз медленнее"},
	{0.1, "в десятки раз медленнее"},
	{1, "в разы медленнее"},
	{2, "как человек"},
	{10, "в разы быстрее"},
	{100, "в десятки раз быстрее"},
	{0, "в сотни раз быстрее"}, // фолбэк: m >= 100
}

// WordForAttribute — дериват числа → слово (спека 99.2.26 §6): для
// reproduction — лог-шкала (value = множитель × человек), для остальных —
// линейные шкалы 0–100 (50 = человек). Неизвестный ключ — пустая строка.
func WordForAttribute(key string, value float64) string {
	if key == "reproduction" {
		for _, s := range reproductionScale {
			if s.limit == 0 || value < s.limit {
				return s.word
			}
		}
		return ""
	}
	steps, ok := attributeScales[key]
	if !ok {
		return ""
	}
	for _, s := range steps {
		if value <= s.limit {
			return s.word
		}
	}
	return ""
}

// attributeValue — числовое значение атрибута расы по ключу (для валидации
// attributes_words и теста согласованности §9.3).
func attributeValue(rc *Race, key string) float64 {
	switch key {
	case "aggression":
		return rc.Attributes.Aggression
	case "curiosity":
		return rc.Attributes.Curiosity
	case "reproduction":
		return rc.Attributes.Reproduction
	case "intelligence":
		return rc.Attributes.Intelligence
	case "diplomacy":
		return rc.Attributes.Diplomacy
	case "resilience":
		return rc.Attributes.Resilience
	}
	return 0
}

// LoadLore — загружает лор рас из файла и валидирует каждую запись:
// id есть в каталоге, family из набора, текстовые поля непусты; у роботов
// (family = robotic) обязателен origin, у био-рас origin пуст. При ошибке
// текущий лор не трогается.
func LoadLore(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("race lore: %w", err)
	}
	var file struct {
		Races []*RaceLore `json:"races"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("race lore %s: %w", absPath, err)
	}
	for i, l := range file.Races {
		if err := l.Validate(); err != nil {
			return fmt.Errorf("race lore %s: запись %d (%s): %w", absPath, i+1, l.ID, err)
		}
	}
	loreCatalog = file.Races
	return nil
}

// LoreCatalog — текущий лор рас (read-only; пуст, если не загружен).
func LoreCatalog() []*RaceLore {
	return loreCatalog
}

// LoreByID — лор расы по ключу (nil, если нет).
func LoreByID(id string) *RaceLore {
	for _, l := range loreCatalog {
		if l.ID == id {
			return l
		}
	}
	return nil
}

// Validate — инварианты лора (спека 86a §5.1.1): id есть в каталоге рас,
// family из набора F1–F9/robotic, character/how_live/why/coexistence непусты;
// у роботов origin обязателен, у био-рас — пуст (происхождение био-рас не
// описано, 22_races.md §3).
func (l *RaceLore) Validate() error {
	if l.ID == "" {
		return fmt.Errorf("id пуст")
	}
	if ByID(l.ID) == nil {
		return fmt.Errorf("id %q нет в каталоге рас", l.ID)
	}
	if !loreFamilies[l.Family] {
		return fmt.Errorf("family = %q, ожидается F1–F9|robotic", l.Family)
	}
	for name, v := range map[string]string{
		"character": l.Character, "how_live": l.HowLive,
		"why": l.Why, "coexistence": l.Coexistence,
	} {
		if v == "" {
			return fmt.Errorf("поле %s пусто", name)
		}
	}
	if l.Family == "robotic" && l.Origin == "" {
		return fmt.Errorf("робот: origin обязателен (происхождение, 22_races.md §4.2)")
	}
	if l.Family != "robotic" && l.Origin != "" {
		return fmt.Errorf("био-раса: origin должен быть пуст (происхождение био-рас не описано, §3)")
	}

	// Новые поля карточки «игровое восприятие» (спека 99.2.26 §9.1): словари
	// §4–§6 закрытые в коде, расширение — осознанная правка кода.
	if !kindWords[l.Kind] {
		return fmt.Errorf("kind = %q, ожидается одно из 11 значений словаря «Вид»", l.Kind)
	}
	if !nicheWords[l.Niche] {
		return fmt.Errorf("niche = %q, ожидается одно из 12 значений словаря «Ниша»", l.Niche)
	}
	if !sizeIndividualSteps[l.SizeIndividual] {
		return fmt.Errorf("size_individual = %q, ожидается одна из 8 ступеней шкалы «Размер особи»", l.SizeIndividual)
	}
	if collectiveRaces[l.ID] {
		if l.SizeGroup == nil {
			return fmt.Errorf("коллективная раса: size_group обязателен (шкала «Размер стаи/роя»)")
		}
		if !sizeGroupSteps[*l.SizeGroup] {
			return fmt.Errorf("size_group = %q, ожидается одна из 5 ступеней шкалы «Размер стаи/роя»", *l.SizeGroup)
		}
	} else if l.SizeGroup != nil {
		return fmt.Errorf("неколлективная раса: size_group должен быть null")
	}
	if l.HomeWords == "" {
		return fmt.Errorf("поле home_words пусто")
	}
	if l.Lore == "" {
		return fmt.Errorf("поле lore пусто")
	}
	if len(l.AttributesWords) != 6 {
		return fmt.Errorf("attributes_words: ожидается ровно 6 ключей (aggression, curiosity, reproduction, intelligence, diplomacy, resilience), получил %d", len(l.AttributesWords))
	}
	rc := ByID(l.ID)
	for _, key := range attributeKeys {
		word, ok := l.AttributesWords[key]
		if !ok {
			return fmt.Errorf("attributes_words: нет ключа %s", key)
		}
		value := attributeValue(rc, key)
		if expected := WordForAttribute(key, value); expected == "" || word != expected {
			return fmt.Errorf("attributes_words[%s] = %q, ожидается %q (дериват числа %v)", key, word, expected, value)
		}
	}
	return nil
}