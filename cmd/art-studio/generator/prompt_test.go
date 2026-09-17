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
	forms := FormsFor(fc, "F2")
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

// TestRandomForm — случайная форма семейства собирается по шаблону.
func TestRandomForm(t *testing.T) {
	fc := loadForms(t)
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 20; i++ {
		f := RandomForm(rng, fc, "F4")
		if !strings.Contains(f, " of material, formed of ") {
			t.Fatalf("RandomForm не по шаблону: %q", f)
		}
	}
}

// TestBuildPrompt — промпт вариации расы (спека 67a.1 §5.2).
func TestBuildPrompt(t *testing.T) {
	fam := loadFamilies(t)
	fc := loadForms(t)
	rng := rand.New(rand.NewSource(42))
	prompt, rid, rname := BuildPrompt(rng, fam["F2"], 0, "F2", fc, 0)
	if rid != "5" || rname != "5 Аммиачники" {
		t.Errorf("rid/rname = %s/%s, want 5/5 Аммиачники", rid, rname)
	}
	for _, want := range []string{"dramatic cinematic concept art", "FRONT VIEW", "made of", "masterpiece", "no text, no watermark"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("промпт не содержит %q: %s", want, prompt)
		}
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
		prompt, _, _ := BuildPrompt(rng, fam["F2"], 0, "F2", fc, 0)
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
	prompt, _, _ := BuildPromptWide(rng, fam["F2"], -1, "F2", "", fc)
	if !strings.Contains(prompt, "ABSTRACT OBJECT") {
		t.Errorf("не-антропо: %s", prompt)
	}
	prompt2, _, _ := BuildPromptWide(rng, fam["F2"], -1, "F2", "anthro", fc)
	if !strings.Contains(prompt2, "alien humanoid race") {
		t.Errorf("антропо: %s", prompt2)
	}
	prompt3, _, _ := BuildPromptWide(rng, fam["F4"], -1, "F4", "beast", fc)
	if !strings.Contains(prompt3, "alien humanoid race") {
		t.Errorf("beast: %s", prompt3)
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