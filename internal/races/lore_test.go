// internal/races/lore_test.go
// Тесты лора рас (спека 86a §5.1.1): config/race_lore.json — машиночитаемая
// проекция 22_races.md §3/§4; валидация формата как у каталога рас.
package races

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Лор загружается: 60 записей, все проходят валидацию (id в каталоге,
// family из набора, текстовые поля непусты).
func TestLoadLoreValidatesAllRaces(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	require.NoError(t, LoadLore("../../config/race_lore.json"))
	require.Len(t, LoreCatalog(), 60, "лор — 60 записей (по одной на расу)")
	for _, l := range LoreCatalog() {
		require.NoError(t, l.Validate(), "лор %s", l.ID)
	}
}

// Каждая раса каталога имеет запись лора (и наоборот — ровно 60).
func TestLoreCoversEveryRace(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	require.NoError(t, LoadLore("../../config/race_lore.json"))
	byID := map[string]*RaceLore{}
	for _, l := range LoreCatalog() {
		byID[l.ID] = l
	}
	for _, r := range Catalog() {
		assert.NotNil(t, byID[r.ID], "раса %s имеет запись лора", r.ID)
	}
}

// Family — из набора F0–F9/robotic (22_races.md §2.2/§4, канон F0–F9
// с 2026-09-21: люди вынесены в F0).
func TestLoreFamiliesValid(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	require.NoError(t, LoadLore("../../config/race_lore.json"))
	for _, l := range LoreCatalog() {
		assert.True(t, loreFamilies[l.Family], "лор %s: family %q из набора", l.ID, l.Family)
	}
}

// Роботы (family = robotic) имеют origin; био-расы — нет (22_races.md §3/§4).
func TestLoreRobotsHaveOriginBioDont(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	require.NoError(t, LoadLore("../../config/race_lore.json"))
	for _, l := range LoreCatalog() {
		if l.Family == "robotic" {
			assert.NotEmpty(t, l.Origin, "робот %s: origin задан", l.ID)
		} else {
			assert.Empty(t, l.Origin, "био-раса %s: origin пуст", l.ID)
		}
	}
}

// Валидация ловит id вне каталога.
func TestLoreRejectsUnknownID(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := &RaceLore{ID: "nope", Family: "F1", Character: "x", HowLive: "x", Why: "x", Coexistence: "x"}
	err := l.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "нет в каталоге")
}

// Валидация ловит family вне набора.
func TestLoreRejectsBadFamily(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := &RaceLore{ID: "humans", Family: "F10", Character: "x", HowLive: "x", Why: "x", Coexistence: "x"}
	err := l.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "family")
}

// F0 — допустимое семейство (люди вынесены в отдельное семейство, 2026-09-21).
func TestLoreAcceptsF0Family(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := validLore()
	l.Family = "F0"
	require.NoError(t, l.Validate())
}

// Инвариант 2026-09-21: люди — единственная био-раса F0; F1 «Водные» —
// ровно расы 2–4 (oceanids, deep_dwellers, coastal).
func TestLoreF0IsHumansOnly(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	require.NoError(t, LoadLore("../../config/race_lore.json"))
	var f0, f1 []string
	for _, l := range LoreCatalog() {
		switch l.Family {
		case "F0":
			f0 = append(f0, l.ID)
		case "F1":
			f1 = append(f1, l.ID)
		}
	}
	assert.Equal(t, []string{"humans"}, f0, "F0 — только люди")
	assert.ElementsMatch(t, []string{"oceanids", "deep_dwellers", "coastal"}, f1, "F1 — чисто водные (2–4)")
}

// Валидация ловит пустое текстовое поле.
func TestLoreRejectsEmptyField(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := &RaceLore{ID: "humans", Family: "F1", Character: "", HowLive: "x", Why: "x", Coexistence: "x"}
	err := l.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "character")
}

// Валидация ловит робота без origin.
func TestLoreRejectsRobotWithoutOrigin(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := &RaceLore{ID: "archivists", Family: "robotic", Character: "x", HowLive: "x", Why: "x", Coexistence: "x"}
	err := l.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "origin")
}

// Валидация ловит био-расу с origin.
func TestLoreRejectsBioWithOrigin(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := &RaceLore{ID: "humans", Family: "F1", Character: "x", HowLive: "x", Why: "x", Coexistence: "x", Origin: "люди"}
	err := l.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "origin")
}

// ==================== НОВЫЕ ПОЛЯ (99.2.26 §9.1) ====================

// validLore — валидная запись лора (humans, неколлективная): все новые поля
// по данным расы, слова атрибутов — дериваты чисел races.json.
func validLore() *RaceLore {
	return &RaceLore{
		ID: "humans", Family: "F1",
		Character: "x", HowLive: "x", Why: "x", Coexistence: "x",
		Kind: "гуманоид", Niche: "строитель", SizeIndividual: "с человека",
		HomeWords: "умеренные миры", Lore: "Люди живут на берегах.",
		AttributesWords: map[string]string{
			"aggression": "агрессивные", "curiosity": "любопытные",
			"reproduction": "как человек", "intelligence": "умные",
			"diplomacy": "дипломатичные", "resilience": "устойчивые",
		},
	}
}

// validCollectiveLore — валидная запись лора коллективной расы (sulfur_swarms).
func validCollectiveLore() *RaceLore {
	return &RaceLore{
		ID: "sulfur_swarms", Family: "F4",
		Character: "x", HowLive: "x", Why: "x", Coexistence: "x",
		Kind: "рой", Niche: "хищник", SizeIndividual: "с ладонь",
		SizeGroup: strPtr("рой-облако"),
		HomeWords: "вулканические миры", Lore: "Серные рои охотятся.",
		AttributesWords: map[string]string{
			"aggression": "агрессивные", "curiosity": "любопытные",
			"reproduction": "в десятки раз быстрее", "intelligence": "простые",
			"diplomacy": "сдержанные", "resilience": "устойчивые",
		},
	}
}

// strPtr — указатель на строку (для size_group).
func strPtr(s string) *string { return &s }

// Валидация ловит kind вне словаря «Вид» (§4).
func TestLoreRejectsBadKind(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := validLore()
	l.Kind = "дракон"
	err := l.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kind")
}

// Валидация ловит niche вне словаря «Ниша» (§5).
func TestLoreRejectsBadNiche(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := validLore()
	l.Niche = "король"
	err := l.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "niche")
}

// Валидация ловит size_individual вне шкалы «Размер особи» (§6.4).
func TestLoreRejectsBadSizeIndividual(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := validLore()
	l.SizeIndividual = "огромные"
	err := l.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "size_individual")
}

// Валидация ловит size_group вне шкалы «Размер стаи/роя» (§6.5).
func TestLoreRejectsBadSizeGroup(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := validCollectiveLore()
	l.SizeGroup = strPtr("огромный рой")
	err := l.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "size_group")
}

// Коллективная раса без size_group — ошибка (§6.5).
func TestLoreRejectsCollectiveWithoutSizeGroup(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := validCollectiveLore()
	l.SizeGroup = nil
	err := l.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "size_group")
}

// Неколлективная раса с size_group — ошибка (§6.5).
func TestLoreRejectsNonCollectiveWithSizeGroup(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := validLore()
	l.SizeGroup = strPtr("стая")
	err := l.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "size_group")
}

// Пустые home_words/lore — ошибка (§9.1).
func TestLoreRejectsEmptyHomeWords(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := validLore()
	l.HomeWords = ""
	err := l.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "home_words")
}

func TestLoreRejectsEmptyLore(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := validLore()
	l.Lore = ""
	err := l.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lore")
}

// Не 6 ключей attributes_words — ошибка (§9.1).
func TestLoreRejectsWrongAttributeKeysCount(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := validLore()
	delete(l.AttributesWords, "resilience")
	err := l.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "attributes_words")
}

// Слово вне шкалы своей оси — ошибка (§9.1).
func TestLoreRejectsWordOutsideScale(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	l := validLore()
	l.AttributesWords["aggression"] = "несуществующее"
	err := l.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "attributes_words")
}

// ==================== ШКАЛЫ (99.2.26 §9.2) ====================

// TestScaleBoundaries — линейные шкалы: значения на границах дают ожидаемые
// слова (граница включительная, v <= limit); монотонность (рост числа →
// не-убывающая ступень).
func TestScaleBoundaries(t *testing.T) {
	cases := []struct {
		key   string
		value float64
		want  string
	}{
		// Агрессия (§6.1): 0–20 мирные, 21–45 умеренные, 46–70 агрессивные,
		// 71–85 воинственные, 86–100 завоеватели (резерв).
		{"aggression", 0, "мирные"},
		{"aggression", 20, "мирные"},
		{"aggression", 21, "умеренные"},
		{"aggression", 45, "умеренные"},
		{"aggression", 46, "агрессивные"},
		{"aggression", 70, "агрессивные"},
		{"aggression", 71, "воинственные"},
		{"aggression", 85, "воинственные"},
		{"aggression", 86, "завоеватели"},
		{"aggression", 100, "завоеватели"},
		// Любопытство (§6.1): 0–20, 21–44, 45–56, 57–68, 69–100.
		{"curiosity", 0, "нелюбопытные"},
		{"curiosity", 20, "нелюбопытные"},
		{"curiosity", 21, "малолюбопытные"},
		{"curiosity", 44, "малолюбопытные"},
		{"curiosity", 45, "любопытные"},
		{"curiosity", 56, "любопытные"},
		{"curiosity", 57, "любознательные"},
		{"curiosity", 68, "любознательные"},
		{"curiosity", 69, "очень любознательные"},
		{"curiosity", 100, "очень любознательные"},
		// Интеллект (§6.1): 0–20, 21–45, 46–70, 71–85, 86–100.
		{"intelligence", 0, "примитивные"},
		{"intelligence", 20, "примитивные"},
		{"intelligence", 21, "простые"},
		{"intelligence", 45, "простые"},
		{"intelligence", 46, "умные"},
		{"intelligence", 70, "умные"},
		{"intelligence", 71, "очень умные"},
		{"intelligence", 85, "очень умные"},
		{"intelligence", 86, "гении"},
		{"intelligence", 100, "гении"},
		// Дипломатия (§6.1): 0–20, 21–45, 46–65, 66–75 (резерв), 76–100.
		{"diplomacy", 0, "недипломатичные"},
		{"diplomacy", 20, "недипломатичные"},
		{"diplomacy", 21, "сдержанные"},
		{"diplomacy", 45, "сдержанные"},
		{"diplomacy", 46, "дипломатичные"},
		{"diplomacy", 65, "дипломатичные"},
		{"diplomacy", 66, "переговорщики"},
		{"diplomacy", 75, "переговорщики"},
		{"diplomacy", 76, "миротворцы"},
		{"diplomacy", 100, "миротворцы"},
		// Устойчивость (§6.1): 0–20 (резерв), 21–45, 46–70, 71–85, 86–100 (резерв).
		{"resilience", 0, "хрупкие"},
		{"resilience", 20, "хрупкие"},
		{"resilience", 21, "уязвимые"},
		{"resilience", 45, "уязвимые"},
		{"resilience", 46, "устойчивые"},
		{"resilience", 70, "устойчивые"},
		{"resilience", 71, "очень устойчивые"},
		{"resilience", 85, "очень устойчивые"},
		{"resilience", 86, "почти неразрушимые"},
		{"resilience", 100, "почти неразрушимые"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, WordForAttribute(c.key, c.value), "%s = %v", c.key, c.value)
	}

	// Монотонность: рост числа → не-убывающая ступень шкалы.
	for key, scale := range attributeScales {
		prev := 0
		for v := 0.0; v <= 100; v += 0.5 {
			idx := scaleIndex(scale, v)
			assert.GreaterOrEqual(t, idx, prev, "%s: монотонность при %v", key, v)
			prev = idx
		}
	}
}

// scaleIndex — номер ступени линейной шкалы для значения (0-based).
func scaleIndex(scale []struct {
	limit float64
	word  string
}, v float64) int {
	for i, s := range scale {
		if v <= s.limit {
			return i
		}
	}
	return len(scale) - 1
}

// TestReproductionScaleBoundaries — лог-шкала размножения (§9.2): граница
// строгая (m < limit), последний шаг — фолбэк для m >= 100.
func TestReproductionScaleBoundaries(t *testing.T) {
	cases := []struct {
		value float64
		want  string
	}{
		{0.00001, "в сотни тысяч раз медленнее"},
		{0.0001, "в тысячи раз медленнее"}, // 0.0001 ≤ m < 0.001
		{0.0005, "в тысячи раз медленнее"},
		{0.001, "в сотни раз медленнее"}, // 0.001 ≤ m < 0.01
		{0.01, "в десятки раз медленнее"}, // 0.01 ≤ m < 0.1
		{0.1, "в разы медленнее"},         // 0.1 ≤ m < 1
		{0.5, "в разы медленнее"},
		{1, "как человек"}, // 1 ≤ m < 2
		{1.5, "как человек"},
		{2, "в разы быстрее"}, // 2 ≤ m < 10
		{9, "в разы быстрее"},
		{10, "в десятки раз быстрее"}, // 10 ≤ m < 100
		{99, "в десятки раз быстрее"},
		{100, "в сотни раз быстрее"}, // фолбэк: m >= 100
		{200, "в сотни раз быстрее"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, WordForAttribute("reproduction", c.value), "reproduction = %v", c.value)
	}
}

// ==================== СОГЛАСОВАННОСТЬ 60×6 (99.2.26 §9.3, DoD) ====================

// TestAttributeWordsMatchScales — для каждой из 60 рас и каждого из 6
// атрибутов: WordForAttribute(key, число из config/races.json) ==
// attributes_words[key]. 360 проверок. Слова — дериваты чисел: правка чисел
// без правки слов роняет тест сознательно.
func TestAttributeWordsMatchScales(t *testing.T) {
	require.NoError(t, LoadCatalog("../../config/races.json"))
	require.NoError(t, LoadLore("../../config/race_lore.json"))
	require.Len(t, LoreCatalog(), 60)
	for _, l := range LoreCatalog() {
		rc := ByID(l.ID)
		require.NotNil(t, rc, "раса %s в каталоге", l.ID)
		for _, key := range attributeKeys {
			word, ok := l.AttributesWords[key]
			require.True(t, ok, "лор %s: ключ %s в attributes_words", l.ID, key)
			assert.Equal(t, WordForAttribute(key, attributeValue(rc, key)), word,
				"раса %s: %s", l.ID, key)
		}
	}
}