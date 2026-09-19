package generator

import (
	"math/rand"
	"strings"
	"testing"

	"zorion/cmd/art-studio/config"
)

func loadForms(t *testing.T) *config.FormsConfig {
	t.Helper()
	fc, err := config.LoadForms("../../../config/art/forms.json")
	if err != nil {
		t.Fatalf("LoadForms: %v", err)
	}
	return fc
}

func loadFamilies(t *testing.T) config.FamiliesConfig {
	t.Helper()
	fam, err := config.LoadFamilies("../../../config/art/families.json")
	if err != nil {
		t.Fatalf("LoadFamilies: %v", err)
	}
	return fam
}

// TestFormsFor — сборка формы по phrase_template + category_keys
// (DoD 67a.1 §10): фраза по шаблону, только формы категорий семейства.
func TestFormsFor(t *testing.T) {
	fc := loadForms(t)
	// Полное произведение F2 — ~69M строк (69 shapes × 107 struct × 89 character
	// × 105 parts): материализовать его в юнит-тесте нельзя (~98 с + ~4 ГБ).
	// Фильтр категорий и шаблон проверяем на реальных shapes/template, урезав
	// ортогональные оси struct/character/parts до 3 элементов — логика та же.
	small := *fc
	small.Struct = fc.Struct[:3]
	small.Character = fc.Character[:3]
	small.Parts = fc.Parts[:3]
	forms := FormsFor(&small, "F2", nil)
	if len(forms) == 0 {
		t.Fatal("FormsFor(F2) пуст")
	}
	if !strings.Contains(forms[0], " of material, formed of ") {
		t.Errorf("форма не по шаблону: %q", forms[0])
	}
	keys := map[string]bool{}
	for _, k := range fc.CategoryKeys["F2"] {
		keys[k] = true
	}
	for _, f := range forms {
		words := strings.Fields(f)
		if len(words) < 4 {
			t.Fatalf("короткая форма: %q", f)
		}
		shape := words[3] // "a {struct} {character} {shape} of material..."
		ok := false
		for _, sh := range fc.Shapes {
			if sh.Shape == shape {
				for _, c := range sh.Categories {
					if keys[c] {
						ok = true
					}
				}
			}
		}
		if !ok {
			t.Errorf("форма %q имеет shape %q без категорий F2", f, shape)
		}
	}
}

// TestBuildPrompt — промпт вариации расы (спека 67a.1 §5.2).
func TestBuildPrompt(t *testing.T) {
	fam := loadFamilies(t)
	fc := loadForms(t)
	rng := rand.New(rand.NewSource(42))
	prompt, rid, rname := BuildPrompt(rng, fam["F2"], 0, "F2", fc, 0, false, "")
	if rid != "5" || rname != "5 Аммиачники" {
		t.Errorf("rid/rname = %s/%s, want 5/5 Аммиачники", rid, rname)
	}
	for _, want := range []string{"dramatic cinematic concept art", "FRONT VIEW", "made of", "masterpiece", "no text, no watermark"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("промпт не содержит %q: %s", want, prompt)
		}
	}
}

// TestBuildPromptTags — 98b: непустые tags вставляются в конец промпта перед
// «masterpiece, game avatar, no text, no watermark»; пустой tags — промпт
// ровно как без параметра (тот же сид → тот же промпт).
func TestBuildPromptTags(t *testing.T) {
	fam := loadFamilies(t)
	fc := loadForms(t)
	base, _, _ := BuildPrompt(rand.New(rand.NewSource(42)), fam["F2"], 0, "F2", fc, 0, false, "")
	tagged, _, _ := BuildPrompt(rand.New(rand.NewSource(42)), fam["F2"], 0, "F2", fc, 0, false, "intricate details, volumetric lighting")
	if !strings.Contains(tagged, "intricate details, volumetric lighting, masterpiece, game avatar, no text, no watermark") {
		t.Errorf("tags не перед финальной частью: %s", tagged)
	}
	// пустой tags — тот же промпт, что и без параметра (тот же сид)
	base2, _, _ := BuildPrompt(rand.New(rand.NewSource(42)), fam["F2"], 0, "F2", fc, 0, false, "")
	if base != base2 {
		t.Errorf("пустой tags меняет промпт: %q vs %q", base, base2)
	}
}

// TestBuildPromptFormFromRace — форма вариации берётся из узкого списка расы
// (race.Forms, идентичность), а не из общего словаря: промпт содержит одну из
// форм расы и деталь-ось «лица» ({character} {parts} из forms.json).
func TestBuildPromptFormFromRace(t *testing.T) {
	fam := loadFamilies(t)
	fc := loadForms(t)
	rng := rand.New(rand.NewSource(42))
	race := fam["F2"].Races[0]
	for i := 0; i < 20; i++ {
		prompt, _, _ := BuildPrompt(rng, fam["F2"], 0, "F2", fc, 0, false, "")
		formOK := false
		for _, f := range race.Forms {
			if strings.Contains(prompt, f) {
				formOK = true
				break
			}
		}
		if !formOK {
			t.Errorf("форма не из узкого списка расы (нет race.Forms): %s", prompt)
		}
		charOK, partsOK := false, false
		for _, c := range fc.Character {
			if strings.Contains(prompt, c) {
				charOK = true
				break
			}
		}
		for _, p := range fc.Parts {
			if strings.Contains(prompt, p) {
				partsOK = true
				break
			}
		}
		if !charOK || !partsOK {
			t.Errorf("нет деталь-оси «лица» ({character} {parts}): %s", prompt)
		}
	}
}

// TestBuildPromptWide — кандидаты эталона: не-антропо, антропо и морфы
// (спека §5.2): morph="" — абстрактный объект, morph != "" — гуманоидная ветка
// со списком форм морфа (beast — свои звериные формы семейства F4).
func TestBuildPromptWide(t *testing.T) {
	fam := loadFamilies(t)
	fc := loadForms(t)
	rng := rand.New(rand.NewSource(7))
	prompt, _, _ := BuildPromptWide(rng, fam["F2"], -1, "F2", "", fc, false, "")
	if !strings.Contains(prompt, "ABSTRACT OBJECT") {
		t.Errorf("не-антропо: %s", prompt)
	}
	prompt2, _, _ := BuildPromptWide(rng, fam["F2"], -1, "F2", "anthro", fc, false, "")
	if !strings.Contains(prompt2, "humanoid race") {
		t.Errorf("антропо: %s", prompt2)
	}
	// beast: фиксированная раса-существо F4 (Серные гнёзда, idx 0) — у не-существ
	// морфы недоступны структурно (покрыто TestBuildPromptWideMorphBlocked)
	prompt3, _, _ := BuildPromptWide(rng, fam["F4"], 0, "F4", "beast", fc, false, "")
	if !strings.Contains(prompt3, "realistic portrait of a creature") {
		t.Errorf("beast: %s", prompt3)
	}
	if strings.Contains(prompt3, "humanoid race") {
		t.Errorf("beast: не должен содержать humanoid race: %s", prompt3)
	}
	formOK := false
	for _, f := range fam["F4"].BeastForms {
		if strings.Contains(prompt3, f) {
			formOK = true
			break
		}
	}
	if !formOK {
		t.Errorf("beast: форма не из fam.BeastForms: %s", prompt3)
	}
}

// TestBuildPromptWideTags — 98b: tags вставляются во все ветки BuildPromptWide:
// не-антропо — перед «masterpiece, game avatar, no text, no watermark»;
// морфы (anthro/beast) — сразу после scene, перед «game avatar». Пустой tags —
// формат без изменений (нет двойных запятых).
func TestBuildPromptWideTags(t *testing.T) {
	fam := loadFamilies(t)
	fc := loadForms(t)
	tags := "intricate details, volumetric lighting"
	cases := []struct{ fid, morph string }{
		{"F2", ""}, // не-антропо
		{"F2", "anthro"},
		{"F4", "beast"},
	}
	for _, c := range cases {
		rng := rand.New(rand.NewSource(7))
		p, _, _ := BuildPromptWide(rng, fam[c.fid], -1, c.fid, c.morph, fc, false, tags)
		if !strings.Contains(p, tags) {
			t.Errorf("%s/%s: tags отсутствуют: %s", c.fid, c.morph, p)
		}
		if c.morph == "" {
			if !strings.Contains(p, tags+", masterpiece, game avatar, no text, no watermark") {
				t.Errorf("%s/%s: tags не перед masterpiece: %s", c.fid, c.morph, p)
			}
		} else {
			if !strings.Contains(p, tags+", game avatar, no text, no watermark") {
				t.Errorf("%s/%s: tags не перед game avatar: %s", c.fid, c.morph, p)
			}
		}
	}
	// пустой tags — формат без изменений
	rng := rand.New(rand.NewSource(7))
	p, _, _ := BuildPromptWide(rng, fam["F2"], -1, "F2", "", fc, false, "")
	if strings.Contains(p, ", ,") {
		t.Errorf("пустой tags даёт двойную запятую: %s", p)
	}
}

// TestBuildPromptWideFormFromRace — кандидат эталона (morph="") берёт форму из
// race.Forms выбранной расы (78a решение 4, вариант «б»), а не из общего
// словаря форм; серия вызовов даёт разброс форм и промптов (решение 5).
func TestBuildPromptWideFormFromRace(t *testing.T) {
	fam := loadFamilies(t)
	fc := loadForms(t)
	raceIdx := 0
	race := fam["F2"].Races[raceIdx]
	rng := rand.New(rand.NewSource(1))
	formsSeen := map[string]bool{}
	promptsSeen := map[string]bool{}
	for i := 0; i < 30; i++ {
		prompt, rid, _ := BuildPromptWide(rng, fam["F2"], raceIdx, "F2", "", fc, false, "")
		if rid != race.ID {
			t.Fatalf("rid = %s, want %s", rid, race.ID)
		}
		if strings.Contains(prompt, " of material, formed of ") {
			t.Errorf("форма из общего словаря форм, а не race.Forms: %s", prompt)
		}
		formOK := false
		for _, f := range race.Forms {
			if strings.Contains(prompt, f) {
				formOK = true
				formsSeen[f] = true
				break
			}
		}
		if !formOK {
			t.Errorf("форма не из race.Forms расы %s: %s", race.ID, prompt)
		}
		promptsSeen[prompt] = true
	}
	if len(formsSeen) < 2 {
		t.Errorf("разброс форм: за 30 вызовов %d уникальных, want >= 2", len(formsSeen))
	}
	if len(promptsSeen) < 2 {
		t.Errorf("разброс промптов: за 30 вызовов %d уникальных, want >= 2", len(promptsSeen))
	}
}

// TestPersonage — эксперимент 82a: при personage=true субъект «a personage»
// вместо «an abstract object/structure»; при false — прежнее поведение.
func TestPersonage(t *testing.T) {
	fam := loadFamilies(t)
	fc := loadForms(t)
	rng := rand.New(rand.NewSource(1))
	p, _, _ := BuildPromptWide(rng, fam["F2"], 0, "F2", "", fc, true, "")
	if !strings.Contains(p, "a personage") {
		t.Errorf("BuildPromptWide personage=true: нет «a personage»: %s", p)
	}
	if strings.Contains(p, "ABSTRACT OBJECT") {
		t.Errorf("BuildPromptWide personage=true: остался «ABSTRACT OBJECT»: %s", p)
	}
	p2, _, _ := BuildPromptWide(rng, fam["F2"], 0, "F2", "", fc, false, "")
	if !strings.Contains(p2, "ABSTRACT OBJECT") {
		t.Errorf("BuildPromptWide personage=false: нет «ABSTRACT OBJECT»: %s", p2)
	}
	p3, _, _ := BuildPrompt(rng, fam["F2"], 0, "F2", fc, 0, true, "")
	if !strings.Contains(p3, "a personage") {
		t.Errorf("BuildPrompt personage=true: нет «a personage»: %s", p3)
	}
	if strings.Contains(p3, "an abstract structure") {
		t.Errorf("BuildPrompt personage=true: остался «an abstract structure»: %s", p3)
	}
	p4, _, _ := BuildPrompt(rng, fam["F2"], 0, "F2", fc, 0, false, "")
	if !strings.Contains(p4, "an abstract structure") {
		t.Errorf("BuildPrompt personage=false: нет «an abstract structure»: %s", p4)
	}
}

// TestBuildHumanPrompt — оси рандома людей (спека 67a.1 §4.3.1).
func TestBuildHumanPrompt(t *testing.T) {
	hc, err := config.LoadHumans("../../../config/art/humans.json")
	if err != nil {
		t.Fatalf("LoadHumans: %v", err)
	}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 50; i++ {
		prompt, gender := BuildHumanPrompt(rng, hc)
		if gender != "man" && gender != "woman" {
			t.Fatalf("gender = %q", gender)
		}
		if !strings.Contains(prompt, "realistic portrait of a human") {
			t.Errorf("промпт: %s", prompt)
		}
		if !strings.Contains(prompt, "FRONT VIEW") {
			t.Errorf("промпт: %s", prompt)
		}
	}
}

// TestFilterPool — фильтрация пула по blocked (98a §5.1): substring-матчинг,
// регистронезависимо; пустой blocked → пул без изменений; пустой пул → пусто.
func TestFilterPool(t *testing.T) {
	pool := []string{"lava-fingers", "basalt-pillars", "crystal-spires", "shards"}
	// substring + регистронезависимость
	got := filterPool(pool, []string{"LAVA"})
	if len(got) != 3 || got[0] != "basalt-pillars" {
		t.Errorf("filterPool(LAVA) = %v", got)
	}
	got = filterPool(pool, []string{"pillar"})
	if len(got) != 3 {
		t.Errorf("filterPool(pillar) = %v", got)
	}
	// пустой blocked → пул как есть
	got = filterPool(pool, nil)
	if len(got) != 4 {
		t.Errorf("filterPool(nil) = %v", got)
	}
	// пустой пул → пусто
	got = filterPool(nil, []string{"lava"})
	if len(got) != 0 {
		t.Errorf("filterPool(пустой пул) = %v", got)
	}
	// все отфильтрованы → пусто (фолбек — на вызывающей стороне)
	got = filterPool(pool, []string{"lava", "basalt", "crystal", "shards"})
	if len(got) != 0 {
		t.Errorf("filterPool(все) = %v", got)
	}
}

// extractCharParts извлекает фрагмент "{character} {parts}" из промпта
// BuildPrompt (текст после последнего "with " до следующей запятой).
// Последний "with " — шаблонный (форма/материал/appearance могут содержать
// "with" раньше; extra/anchor/scene его не содержат).
func extractCharParts(prompt string) string {
	i := strings.LastIndex(prompt, "with ")
	if i < 0 {
		return ""
	}
	rest := prompt[i+len("with "):]
	j := strings.Index(rest, ",")
	if j < 0 {
		return rest
	}
	return rest[:j]
}

// TestBuildPromptBlocked — 1000 сидов: character/parts в промпте не содержат
// ни одного токена blocked расы (98a §5.6).
func TestBuildPromptBlocked(t *testing.T) {
	fam := loadFamilies(t)
	fc := loadForms(t)
	for fid, f := range fam {
		for ri, race := range f.Races {
			if len(race.Blocked) == 0 {
				continue
			}
			rng := rand.New(rand.NewSource(42))
			for i := 0; i < 1000; i++ {
				prompt, _, _ := BuildPrompt(rng, f, ri, fid, fc, 0, false, "")
				cp := extractCharParts(prompt)
				for _, tok := range race.Blocked {
					if strings.Contains(strings.ToLower(cp), tok) {
						t.Errorf("%s/%s: character/parts содержат blocked %q: %s", fid, race.ID, tok, prompt)
					}
				}
			}
		}
	}
}

// TestBuildPromptWideMorphBlocked — морф-ветка BuildPromptWide (98a §5.6):
// (а) структурное правило «раса-не-существо»: базовые пулы anthro/beast пусты
//     после фильтрации → любой из 7 морфов → не-антропо ветка (нет морф-
//     маркеров alien/portrait/creature/humanoid);
// (б) морф с допустимыми формами работает (раса-существо);
// (в) anthro_clothes фильтруются по blocked.
func TestBuildPromptWideMorphBlocked(t *testing.T) {
	fam := loadFamilies(t)
	fc := loadForms(t)

	// (а) Метан-планктон (F3, id 12) — раса-не-существо: blocked содержит
	// humanoid (опустошает anthro_forms) и creature (опустошает beast_forms).
	ri := -1
	for i, r := range fam["F3"].Races {
		if r.ID == "12" {
			ri = i
			break
		}
	}
	if ri < 0 {
		t.Fatal("нет расы 12 в F3")
	}
	for _, morph := range []string{"anthro", "beast", "xeno", "amorph", "crystal", "mech", "titan"} {
		rng := rand.New(rand.NewSource(int64(len(morph))))
		prompt, _, _ := BuildPromptWide(rng, fam["F3"], ri, "F3", morph, fc, false, "")
		if !strings.Contains(prompt, "ABSTRACT OBJECT") {
			t.Errorf("морф %s: раса-не-существо должна дать не-антропо ветку: %s", morph, prompt)
		}
		// Морф-маркеры отсутствуют (морф-ветка не достигнута). «no face, no
		// eyes» в шаблоне и «not a creature» в appearance — отрицания, не
		// морф-контент (98a §5.4 М4: дублирование отрицаний допустимо).
		for _, bad := range []string{"alien humanoid race", "alien creature", "realistic portrait", "humanoid mechanical creature"} {
			if strings.Contains(prompt, bad) {
				t.Errorf("морф %s: промпт содержит морф-маркер %q: %s", morph, bad, prompt)
			}
		}
	}

	// (б) Прибрежные (F1, id 4) — раса-существо: морф anthro работает.
	ri4 := -1
	for i, r := range fam["F1"].Races {
		if r.ID == "4" {
			ri4 = i
			break
		}
	}
	if ri4 < 0 {
		t.Fatal("нет расы 4 в F1")
	}
	rng := rand.New(rand.NewSource(1))
	prompt, _, _ := BuildPromptWide(rng, fam["F1"], ri4, "F1", "anthro", fc, false, "")
	if !strings.Contains(prompt, "humanoid race") {
		t.Errorf("anthro для Прибрежных: %s", prompt)
	}

	// (в) anthro_clothes фильтруются: синтетическая раса F1 с blocked ["kelp"] —
	// F1 anthro_clothes содержит kelp-элементы, они не должны попасть в промпт.
	synth := fam["F1"]
	synth.Races[ri4].Blocked = []string{"kelp"}
	rng2 := rand.New(rand.NewSource(2))
	seenClothes := 0
	for i := 0; i < 100; i++ {
		p, _, _ := BuildPromptWide(rng2, synth, ri4, "F1", "anthro", fc, false, "")
		if !strings.Contains(p, "humanoid race") {
			t.Fatalf("anthro: %s", p)
		}
		if strings.Contains(p, "kelp") {
			t.Errorf("clothes содержат kelp (не отфильтрован): %s", p)
		}
		if strings.Contains(p, "wearing ") {
			seenClothes++
		}
	}
	if seenClothes == 0 {
		t.Error("clothes не выбраны ни разу (пул пуст?)")
	}
}

// TestBuildPromptPaletteAccents — акценты палитры фильтруются по blocked (С1,
// 98a §5.2 п.5): при palette>0 акценты не содержат токенов blocked; все
// отфильтрованы → акцент не добавляется (0 акцентов, не ошибка).
func TestBuildPromptPaletteAccents(t *testing.T) {
	fam := loadFamilies(t)
	fc := loadForms(t)

	// Крио-рои (F2, id 9): blocked блокирует тёплые акценты
	// (amber/gold/peach/crimson/copper/golden/yellow) — из 16 остаются 11.
	ri := -1
	for i, r := range fam["F2"].Races {
		if r.ID == "9" {
			ri = i
			break
		}
	}
	if ri < 0 {
		t.Fatal("нет расы 9 в F2")
	}
	rng := rand.New(rand.NewSource(3))
	for i := 0; i < 50; i++ {
		prompt, _, _ := BuildPrompt(rng, fam["F2"], ri, "F2", fc, 100, false, "")
		for _, warm := range []string{"amber", "gold", "peach", "crimson", "copper", "golden", "yellow"} {
			if strings.Contains(prompt, warm) {
				t.Errorf("тёплый акцент %q у Крио-роёв: %s", warm, prompt)
			}
		}
	}

	// Все акценты отфильтрованы → акцент не добавляется: синтетическая раса
	// с blocked, покрывающим все 16 акцентов palette_accents.
	allAccents := []string{"amber", "violet", "teal", "rose", "emerald", "gold", "magenta",
		"cyan", "turquoise", "lavender", "peach", "indigo", "crimson", "aqua", "copper", "ivory"}
	synth := fam["F2"]
	synth.Races[ri].Blocked = allAccents
	rng2 := rand.New(rand.NewSource(4))
	for i := 0; i < 20; i++ {
		prompt, _, _ := BuildPrompt(rng2, synth, ri, "F2", fc, 100, false, "")
		for _, acc := range fc.PaletteAccents {
			if strings.Contains(prompt, acc) {
				t.Errorf("акцент %q не отфильтрован: %s", acc, prompt)
			}
		}
	}
}

// TestBuildPromptAppearance — appearance вставляется в промпт после формы
// (98a §5.4); при пустой appearance формат без изменений.
func TestBuildPromptAppearance(t *testing.T) {
	fam := loadFamilies(t)
	fc := loadForms(t)

	// Прибрежные (F1, id 4) — appearance задана.
	ri := -1
	for i, r := range fam["F1"].Races {
		if r.ID == "4" {
			ri = i
			break
		}
	}
	if ri < 0 {
		t.Fatal("нет расы 4 в F1")
	}
	app := fam["F1"].Races[ri].Appearance
	if app == "" {
		t.Fatal("у Прибрежных нет appearance")
	}
	rng := rand.New(rand.NewSource(5))
	prompt, _, _ := BuildPrompt(rng, fam["F1"], ri, "F1", fc, 0, false, "")
	if !strings.Contains(prompt, app) {
		t.Errorf("appearance не в промпте: %s", prompt)
	}
	// appearance после формы и до " with " (деталь-ось)
	form := ""
	for _, f := range fam["F1"].Races[ri].Forms {
		if strings.Contains(prompt, f) {
			form = f
			break
		}
	}
	if form == "" {
		t.Fatalf("форма не из race.Forms: %s", prompt)
	}
	iForm := strings.Index(prompt, form)
	iApp := strings.Index(prompt, app)
	iWith := strings.LastIndex(prompt, " with ")
	if iForm < 0 || iApp < 0 || iWith < 0 || !(iForm < iApp && iApp < iWith) {
		t.Errorf("appearance не после формы/до детали: %s", prompt)
	}

	// Раса без appearance (Аммиачники F2 id 5) — формат без изменений.
	prompt2, _, _ := BuildPrompt(rng, fam["F2"], 0, "F2", fc, 0, false, "")
	if strings.Contains(prompt2, ", ,") {
		t.Errorf("двойная запятая без appearance: %s", prompt2)
	}
}

// poolContains — true, если хоть один элемент пула содержит токен (substring,
// регистронезависимо).
func poolContains(pool []string, tok string) bool {
	for _, item := range pool {
		if strings.Contains(strings.ToLower(item), tok) {
			return true
		}
	}
	return false
}

// TestBlockedMatchesDictionaries — кросс-конфиг (98a §5.6): каждый токен
// blocked всех рас имеет ≥ 1 совпадение (substring) в словарях forms.json
// (character/parts/shapes/struct/морф-списки) или в семейных морф-пулах
// families.json (beast_forms/anthro_clothes) или в palette_accents —
// мёртвых токенов нет.
func TestBlockedMatchesDictionaries(t *testing.T) {
	fam := loadFamilies(t)
	fc := loadForms(t)
	global := [][]string{
		fc.Character, fc.Parts, fc.Struct,
		fc.AnthroForms, fc.BeastForms, fc.XenoForms,
		fc.AmorphousForms, fc.CrystalForms, fc.MechForms, fc.TitanForms,
		fc.PaletteAccents,
	}
	for _, sh := range fc.Shapes {
		global = append(global, []string{sh.Shape})
	}
	for fid, f := range fam {
		family := [][]string{f.BeastForms, f.AnthroClothes}
		for _, race := range f.Races {
			for _, tok := range race.Blocked {
				ok := false
				for _, pool := range global {
					if poolContains(pool, tok) {
						ok = true
						break
					}
				}
				if !ok {
					for _, pool := range family {
						if poolContains(pool, tok) {
							ok = true
							break
						}
					}
				}
				if !ok {
					t.Errorf("%s/%s: blocked токен %q не имеет совпадений в словарях", fid, race.ID, tok)
				}
			}
		}
	}
}

// TestPilotPrompts — критерии приёмки пилота (98a §7.1/§7.4/§7.5): 20 промптов
// на каждую пилотную расу — ни один не содержит blocked-токен в character/parts;
// вариативность ≥ 10 форм, ≥ 6 character, ≥ 6 parts; Кшарры: parts 100% не
// тронуты, character/struct ≥ 98%.
func TestPilotPrompts(t *testing.T) {
	fam := loadFamilies(t)
	fc := loadForms(t)
	pilots := []struct{ fid, id string }{
		{"F1", "4"}, {"F3", "12"}, {"F1", "3"}, {"F2", "9"}, {"F3", "14"}, {"F5", "23"},
	}
	for _, p := range pilots {
		f := fam[p.fid]
		ri := -1
		for i, r := range f.Races {
			if r.ID == p.id {
				ri = i
				break
			}
		}
		if ri < 0 {
			t.Fatalf("нет расы %s в %s", p.id, p.fid)
		}
		race := f.Races[ri]
		rng := rand.New(rand.NewSource(100))
		forms := map[string]bool{}
		chars := map[string]bool{}
		parts := map[string]bool{}
		for i := 0; i < 20; i++ {
			prompt, _, _ := BuildPrompt(rng, f, ri, p.fid, fc, 0, false, "")
			cp := extractCharParts(prompt)
			for _, tok := range race.Blocked {
				if strings.Contains(strings.ToLower(cp), tok) {
					t.Errorf("%s/%s: blocked %q в character/parts: %s", p.fid, race.ID, tok, prompt)
				}
			}
			formOK := false
			for _, frm := range race.Forms {
				if strings.Contains(prompt, frm) {
					forms[frm] = true
					formOK = true
					break
				}
			}
			if !formOK {
				t.Errorf("%s/%s: форма не из race.Forms: %s", p.fid, race.ID, prompt)
			}
			cpWords := strings.Split(cp, " ")
			if len(cpWords) != 2 {
				t.Fatalf("%s/%s: character/parts фрагмент %q", p.fid, race.ID, cp)
			}
			chars[cpWords[0]] = true
			parts[cpWords[1]] = true
		}
		if len(forms) < 10 {
			t.Errorf("%s/%s: форм %d, want >= 10", p.fid, race.ID, len(forms))
		}
		if len(chars) < 6 {
			t.Errorf("%s/%s: character %d, want >= 6", p.fid, race.ID, len(chars))
		}
		if len(parts) < 6 {
			t.Errorf("%s/%s: parts %d, want >= 6", p.fid, race.ID, len(parts))
		}
	}

	// Кшарры (F5, id 23): parts 100% не тронуты, character/struct >= 98%.
	ksh := fam["F5"].Races[0]
	if ksh.ID != "23" {
		t.Fatalf("F5[0].id = %s, want 23", ksh.ID)
	}
	if got := len(filterPool(fc.Parts, ksh.Blocked)); got != len(fc.Parts) {
		t.Errorf("Кшарры: parts %d/%d тронуты, want 100%%", len(fc.Parts)-got, len(fc.Parts))
	}
	if got := len(filterPool(fc.Character, ksh.Blocked)); float64(got)/float64(len(fc.Character)) < 0.98 {
		t.Errorf("Кшарры: character %d/%d, want >= 98%%", got, len(fc.Character))
	}
	if got := len(filterPool(fc.Struct, ksh.Blocked)); float64(got)/float64(len(fc.Struct)) < 0.98 {
		t.Errorf("Кшарры: struct %d/%d, want >= 98%%", got, len(fc.Struct))
	}
}