// Package generator — генерация аватаров: промпты (перенос race_gen.py /
// human_gen.py) и фоновые джобы (2 параллельных воркера, мягкий СТОП).
package generator

import (
	"fmt"
	"math/rand"
	"strings"

	"zorion/cmd/art-studio/config"
)

// FormsFor возвращает формы семейства по категориям (словарь форм,
// спека 67a.1 §4.1/§5.2). Если категорий нет — все комбинации.
func FormsFor(fc *config.FormsConfig, famID string) []string {
	keys := fc.CategoryKeys[famID]
	var out []string
	for _, sh := range fc.Shapes {
		if len(keys) > 0 && !intersects(sh.Categories, keys) {
			continue
		}
		for _, st := range fc.Struct {
			for _, ch := range fc.Character {
				for _, pt := range fc.Parts {
					out = append(out, buildPhrase(fc.PhraseTemplate, st, ch, sh.Shape, pt))
				}
			}
		}
	}
	return out
}

// RandomForm — случайная форма семейства без построения полного списка
// (эквивалент rng.choice(forms_for(fam)) из прототипа).
func RandomForm(rng *rand.Rand, fc *config.FormsConfig, famID string) string {
	keys := fc.CategoryKeys[famID]
	var shapes []config.Shape
	for _, sh := range fc.Shapes {
		if len(keys) == 0 || intersects(sh.Categories, keys) {
			shapes = append(shapes, sh)
		}
	}
	if len(shapes) == 0 {
		shapes = fc.Shapes
	}
	sh := shapes[rng.Intn(len(shapes))]
	st := fc.Struct[rng.Intn(len(fc.Struct))]
	ch := fc.Character[rng.Intn(len(fc.Character))]
	pt := fc.Parts[rng.Intn(len(fc.Parts))]
	return buildPhrase(fc.PhraseTemplate, st, ch, sh.Shape, pt)
}

func buildPhrase(tpl, struct_, char, shape, part string) string {
	r := strings.NewReplacer("{struct}", struct_, "{character}", char, "{shape}", shape, "{part}", part)
	return r.Replace(tpl)
}

func intersects(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

// BuildPrompt — промпт вариации расы (всегда от эталона, арт-ТЗ 67a):
// "dramatic cinematic concept art of an abstract structure, FRONT VIEW,
// made of {mat}, {form}, {glow}, with {character} {parts}, {extra}, {anchor},
// centered, {scene}, masterpiece, game avatar, no text, no watermark".
// Форма — из узкого списка расы (race.Forms, идентичность), деталь-ось
// «лица» — {character} {parts} из forms.json; материал/свечение — из расы
// (палитра); palette 0–100 — отклонение палитры: при >0 добавляется цветовой
// акцент из PaletteAccents (больше = сильнее акцент, не ломая основу расы);
// композицию эталона держит ControlNet Canny от маски (art_principles.md §3).
func BuildPrompt(rng *rand.Rand, fam config.Family, raceIdx int, famID string, fc *config.FormsConfig, palette int) (prompt, raceID, raceName string) {
	race := fam.Races[raceIdx]
	mat := race.Materials[rng.Intn(len(race.Materials))]
	glow := race.Glows[rng.Intn(len(race.Glows))]
	form := race.Forms[rng.Intn(len(race.Forms))]
	character := fc.Character[rng.Intn(len(fc.Character))]
	parts := fc.Parts[rng.Intn(len(fc.Parts))]
	extra := fam.Extra[rng.Intn(len(fam.Extra))]
	anchor := fam.Anchor[rng.Intn(len(fam.Anchor))]
	scene := pickScene(rng, fam.Scene)
	accent := ""
	if palette > 0 && len(fc.PaletteAccents) > 0 {
		// акцент добавляется всегда при palette>0, сила — числом акцентов (0..3)
		n := 1 + (palette-1)*3/100
		if n > 3 {
			n = 3
		}
		acc := append([]string(nil), fc.PaletteAccents...)
		for i := 0; i < n && len(acc) > 0; i++ {
			k := rng.Intn(len(acc))
			accent += ", " + acc[k]
			acc = append(acc[:k], acc[k+1:]...)
		}
	}
	prompt = fmt.Sprintf("dramatic cinematic concept art of an abstract structure, FRONT VIEW, made of %s, %s, %s%s, with %s %s, %s, %s, centered, %s, masterpiece, game avatar, no text, no watermark",
		mat, form, glow, accent, character, parts, extra, anchor, scene)
	return prompt, race.ID, race.Name
}

// LightNeg — лёгкий негатив для вариаций (арт-ТЗ 67a): без запретов
// материала/цвета/creature расы (общий fam.Neg самоконфликтен для вариаций).
func LightNeg() string {
	return "text, watermark, blurry, low quality, deformed, ugly, duplicate, 3D render, cartoon, anime, human, person, face, eyes, nose, mouth, ears, chin, head, portrait, human anatomy, limbs, hands, body, flesh, meat, organ, naked, nude, cropped, cut off, floating"
}

// NegFor — негатив для генерации кандидатов: морфам (любой morph != "") нужен
// СВОЙ негатив (без запрета human/face/head — иначе конфликт с промптом,
// однообразие), если семейство задало anthro_neg. Не-антропо — обычный fam.Neg.
func NegFor(fam config.Family, morph string) string {
	if morph != "" && fam.AnthroNeg != "" {
		return fam.AnthroNeg
	}
	return fam.Neg
}

// BuildPromptWide — кандидат эталона (широкий поиск по всему семейству,
// спека 67a.1 §5.2). raceIdx < 0 — раса выбирается случайно (материал/свечение
// от неё). morph — морф генерации: "" = не-антропо (абстрактный объект),
// "anthro"/"beast"/"xeno"/"amorph"/"crystal"/"mech"/"titan" — гуманоидные
// морфы из материала расы (select морфа на вкладке «Эталон»).
func BuildPromptWide(rng *rand.Rand, fam config.Family, raceIdx int, famID string, morph string, fc *config.FormsConfig) (prompt, raceID, raceName string) {
	if raceIdx < 0 {
		raceIdx = rng.Intn(len(fam.Races))
	}
	race := fam.Races[raceIdx]
	mat := race.Materials[rng.Intn(len(race.Materials))]
	glow := race.Glows[rng.Intn(len(race.Glows))]
	scene := pickScene(rng, fam.Scene)
	if morph != "" {
		// свои формы семейства (F4/F5 — звериные), иначе глобальный список морфа;
		// пустой список выбранного морфа — фолбек на глобальные антропо-формы
		var forms []string
		switch morph {
		case "anthro":
			forms = fam.AnthroForms
			if len(forms) == 0 {
				forms = fc.AnthroForms
			}
		case "beast":
			forms = fam.BeastForms
			if len(forms) == 0 {
				forms = fc.BeastForms
			}
		case "xeno":
			forms = fc.XenoForms
		case "amorph":
			forms = fc.AmorphousForms
		case "crystal":
			forms = fc.CrystalForms
		case "mech":
			forms = fc.MechForms
		case "titan":
			forms = fc.TitanForms
		default:
			forms = fc.AnthroForms
		}
		if len(forms) == 0 {
			forms = fc.AnthroForms
		}
		form := forms[rng.Intn(len(forms))]
		clothes := ""
		if len(fam.AnthroClothes) > 0 {
			clothes = " wearing " + fam.AnthroClothes[rng.Intn(len(fam.AnthroClothes))]
		}
		// «natural skin texture» убрано: тянет к человеческой коже; «face» убрано:
		// тянет к человеческому лицу и мешает звериным/ксено-морфам; остаётся
		// только «looking directly at viewer» — взгляд на зрителя
		prompt = fmt.Sprintf("realistic portrait of an alien humanoid race, FRONT VIEW, looking directly at viewer, %s made of %s, %s, head and shoulders,%s torso extending down below the frame, anchored, centered, %s, game avatar, no text, no watermark",
			form, mat, glow, clothes, scene)
		return prompt, race.ID, race.Name
	}
	form := RandomForm(rng, fc, famID)
	prompt = fmt.Sprintf("dramatic cinematic concept art of an ABSTRACT OBJECT, FRONT VIEW, made of %s, %s, %s, no face, no eyes, no mouth, no human features, asymmetric, anchored by a solid base extending to the bottom edge of the frame, centered, %s, masterpiece, game avatar, no text, no watermark",
		mat, form, glow, scene)
	return prompt, race.ID, race.Name
}

// BuildHumanPrompt — промпт портрета человека по осям humans.json
// (перенос human_gen.py build_prompt, спека 67a.1 §4.3.1).
// Возвращает промпт и пол ("man"/"woman").
func BuildHumanPrompt(rng *rand.Rand, hc *config.HumansConfig) (prompt, gender string) {
	axes := hc.Axes
	gender = pick(rng, axes["GENDER"])
	skin := pick(rng, axes["SKIN"])
	age := pick(rng, axes["AGE"])
	hair := pick(rng, axes["HAIR"])
	beard := ""
	if gender == "man" && rng.Float64() < hc.Params.BeardChance {
		beard = ", " + pick(rng, axes["BEARD"])
	}
	clothes := pick(rng, axes["CLOTHES"])
	armor := pick(rng, axes["ARMOR"])
	helmet := pick(rng, axes["HELMET"])
	expr := pick(rng, axes["EXPR"])
	facial := pick(rng, axes["FACIAL"])
	if rng.Float64() < hc.Params.SecondFacialChance {
		facial += ", " + pick(rng, axes["FACIAL"])
	}
	scene := pick(rng, axes["SCENE"])
	repl := strings.NewReplacer(
		"{gender}", gender,
		"{skin}", skin,
		"{age}", age,
		"{hair}", hair,
		"{beard}", beard,
		"{facial}", facial,
		"{expr}", expr,
		"{clothes}", clothes,
		"{armor}", armor,
		"{helmet}", helmet,
		"{scene}", scene,
	)
	return repl.Replace(hc.PromptTemplate), gender
}

func pick(rng *rand.Rand, items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[rng.Intn(len(items))]
}

func pickScene(rng *rand.Rand, scene string) string {
	parts := strings.Split(scene, ", ")
	return parts[rng.Intn(len(parts))]
}