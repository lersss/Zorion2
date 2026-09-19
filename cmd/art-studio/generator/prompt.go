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
// blocked (98a §5.2 п.4) фильтрует shapes/struct/character/parts перед
// комбинаторным циклом; пустой пул после фильтрации → исходный.
func FormsFor(fc *config.FormsConfig, famID string, blocked []string) []string {
	keys := fc.CategoryKeys[famID]
	segs := parsePhraseTemplate(fc.PhraseTemplate)
	shapes := filterShapes(fc.Shapes, blocked)
	if len(shapes) == 0 {
		shapes = fc.Shapes
	}
	structs := filterPool(fc.Struct, blocked)
	if len(structs) == 0 {
		structs = fc.Struct
	}
	chars := filterPool(fc.Character, blocked)
	if len(chars) == 0 {
		chars = fc.Character
	}
	parts := filterPool(fc.Parts, blocked)
	if len(parts) == 0 {
		parts = fc.Parts
	}
	var out []string
	for _, sh := range shapes {
		if len(keys) > 0 && !intersects(sh.Categories, keys) {
			continue
		}
		for _, st := range structs {
			for _, ch := range chars {
				for _, pt := range parts {
					out = append(out, buildPhrase(segs, st, ch, sh.Shape, pt))
				}
			}
		}
	}
	return out
}

// filterPool возвращает копию пула без элементов, содержащих любой из токенов
// blocked (подстрока, регистронезависимо: lowercase обеих сторон). Пустой
// blocked → пул без изменений (не-регрессия для рас без фильтрации). Фолбек
// при пустом результате — на вызывающей стороне (98a §5.2: акценты не
// фолбечат, морф-пулы фолбечат на глобальные формы, character/parts — на
// исходный пул).
func filterPool(pool, blocked []string) []string {
	if len(blocked) == 0 {
		return pool
	}
	low := lowerTokens(blocked)
	out := make([]string, 0, len(pool))
	for _, item := range pool {
		if !blockedMatch(item, low) {
			out = append(out, item)
		}
	}
	return out
}

// filterShapes — фильтр []Shape по полю Shape (98a §5.2 п.4).
func filterShapes(shapes []config.Shape, blocked []string) []config.Shape {
	if len(blocked) == 0 {
		return shapes
	}
	low := lowerTokens(blocked)
	out := make([]config.Shape, 0, len(shapes))
	for _, sh := range shapes {
		if !blockedMatch(sh.Shape, low) {
			out = append(out, sh)
		}
	}
	return out
}

// lowerTokens — lowercase-копия списка токенов (регистронезависимый матчинг).
func lowerTokens(blocked []string) []string {
	low := make([]string, len(blocked))
	for i, tok := range blocked {
		low[i] = strings.ToLower(tok)
	}
	return low
}

// blockedMatch — true, если элемент содержит любой токен blocked
// (подстрока, регистронезависимо; blocked ожидается в lowercase).
func blockedMatch(item string, blocked []string) bool {
	low := strings.ToLower(item)
	for _, tok := range blocked {
		if strings.Contains(low, tok) {
			return true
		}
	}
	return false
}

// phraseSeg — сегмент шаблона фразы: обычный текст или плейсхолдер {key}.
type phraseSeg struct {
	text string // текст сегмента; для плейсхолдера — "{key}" как в шаблоне
	key  string // ключ плейсхолдера ("" — обычный текст)
}

// parsePhraseTemplate разбивает шаблон на сегменты один раз на вызов FormsFor
// (кеш по шаблону): фраза собирается конкатенацией, без NewReplacer на каждую
// комбинацию (на полном словаре ~69M строк NewReplacer давал ~98 с).
func parsePhraseTemplate(tpl string) []phraseSeg {
	var segs []phraseSeg
	for {
		i := strings.IndexByte(tpl, '{')
		if i < 0 {
			if tpl != "" {
				segs = append(segs, phraseSeg{text: tpl})
			}
			return segs
		}
		if i > 0 {
			segs = append(segs, phraseSeg{text: tpl[:i]})
		}
		j := strings.IndexByte(tpl[i:], '}')
		if j < 0 {
			segs = append(segs, phraseSeg{text: tpl[i:]})
			return segs
		}
		raw := tpl[i : i+j+1]
		segs = append(segs, phraseSeg{text: raw, key: tpl[i+1 : i+j]})
		tpl = tpl[i+j+1:]
	}
}

func buildPhrase(segs []phraseSeg, struct_, char, shape, part string) string {
	var b strings.Builder
	b.Grow(64)
	for _, s := range segs {
		if s.key == "" {
			b.WriteString(s.text)
			continue
		}
		switch s.key {
		case "struct":
			b.WriteString(struct_)
		case "character":
			b.WriteString(char)
		case "shape":
			b.WriteString(shape)
		case "part":
			b.WriteString(part)
		default:
			b.WriteString(s.text) // неизвестный плейсхолдер — как в шаблоне
		}
	}
	return b.String()
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
// personage — эксперимент 82a: при true субъект «a personage» вместо
// «an abstract structure» (остальной шаблон и негативы не меняются).
// tags — дополнительные теги с фронта (98b): непустые вставляются в конец
// промпта перед «masterpiece, game avatar, no text, no watermark» (высокий
// вес в SDXL); пустые — поведение как без параметра.
func BuildPrompt(rng *rand.Rand, fam config.Family, raceIdx int, famID string, fc *config.FormsConfig, palette int, personage bool, tags string) (prompt, raceID, raceName string) {
	race := fam.Races[raceIdx]
	mat := race.Materials[rng.Intn(len(race.Materials))]
	glow := race.Glows[rng.Intn(len(race.Glows))]
	form := race.Forms[rng.Intn(len(race.Forms))]
	// character/parts — из отфильтрованных пулов (98a §5.2 п.1); пустой пул
	// после фильтрации → исходный (фолбек, промпт не битый).
	charPool := filterPool(fc.Character, race.Blocked)
	if len(charPool) == 0 {
		charPool = fc.Character
	}
	partsPool := filterPool(fc.Parts, race.Blocked)
	if len(partsPool) == 0 {
		partsPool = fc.Parts
	}
	character := charPool[rng.Intn(len(charPool))]
	parts := partsPool[rng.Intn(len(partsPool))]
	extra := fam.Extra[rng.Intn(len(fam.Extra))]
	anchor := fam.Anchor[rng.Intn(len(fam.Anchor))]
	scene := pickScene(rng, fam.Scene)
	accent := ""
	if palette > 0 && len(fc.PaletteAccents) > 0 {
		// акценты — из отфильтрованного пула (98a §5.2 п.5, С1); если все
		// отфильтрованы — акцент не добавляется (не фолбек на исходный).
		accPool := filterPool(fc.PaletteAccents, race.Blocked)
		if len(accPool) > 0 {
			// акцент добавляется всегда при palette>0, сила — числом акцентов (0..3)
			n := 1 + (palette-1)*3/100
			if n > 3 {
				n = 3
			}
			acc := append([]string(nil), accPool...)
			for i := 0; i < n && len(acc) > 0; i++ {
				k := rng.Intn(len(acc))
				accent += ", " + acc[k]
				acc = append(acc[:k], acc[k+1:]...)
			}
		}
	}
	subject := "an abstract structure"
	if personage {
		subject = "a personage"
	}
	// appearance вставляется после формы (98a §5.4): только если задана.
	formPart := form
	if race.Appearance != "" {
		formPart = form + ", " + race.Appearance
	}
	tagsPart := ""
	if tags != "" {
		tagsPart = ", " + tags
	}
	prompt = fmt.Sprintf("dramatic cinematic concept art of %s, FRONT VIEW, made of %s, %s, %s%s, with %s %s, %s, %s, centered, %s%s, masterpiece, game avatar, no text, no watermark",
		subject, mat, formPart, glow, accent, character, parts, extra, anchor, scene, tagsPart)
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
// Мехоморфный: кадр до торса — жёсткие запреты полного тела/ног (создатель
// 2026-09-17: «мехи в полный рост, а нужно максимум до торса»).
func NegFor(fam config.Family, morph string) string {
	neg := fam.Neg
	if morph != "" && fam.AnthroNeg != "" {
		neg = fam.AnthroNeg
	}
	if morph == "mech" {
		neg += ", full body, legs, standing pose, whole figure, hips, lower body"
	}
	return neg
}

// BuildPromptWide — кандидат эталона (широкий поиск по всему семейству,
// спека 67a.1 §5.2). raceIdx < 0 — раса выбирается случайно (материал/свечение
// от неё). morph — морф генерации: "" = не-антропо (абстрактный объект),
// "anthro"/"beast"/"xeno"/"amorph"/"crystal"/"mech"/"titan" — гуманоидные
// морфы из материала расы (select морфа на вкладке «Эталон»).
// personage — эксперимент 82a: при true в не-антропо ветке субъект «a personage»
// вместо «an abstract object» (морф-ветка и негативы не меняются).
// tags — дополнительные теги с фронта (98b): непустые вставляются в конец
// промпта — в морф-ветках сразу после scene, перед «game avatar»; в не-антропо
// перед «masterpiece, game avatar, no text, no watermark». Пустые — как без
// параметра.
func BuildPromptWide(rng *rand.Rand, fam config.Family, raceIdx int, famID string, morph string, fc *config.FormsConfig, personage bool, tags string) (prompt, raceID, raceName string) {
	if raceIdx < 0 {
		raceIdx = rng.Intn(len(fam.Races))
	}
	race := fam.Races[raceIdx]
	mat := race.Materials[rng.Intn(len(race.Materials))]
	glow := race.Glows[rng.Intn(len(race.Glows))]
	scene := pickScene(rng, fam.Scene)
	tagsPart := ""
	if tags != "" {
		tagsPart = ", " + tags
	}
	if morph != "" {
		// Структурное правило «раса-не-существо» (98a §5.3): если после
		// фильтрации по blocked пуст хотя бы один из базовых органических
		// пулов — anthro_forms (глобальный) ИЛИ beast_forms (семейный,
		// фолбек на глобальный) — раса не-существо, все 7 морфов недоступны,
		// генерация переходит в не-антропо ветку.
		anthroBase := filterPool(fc.AnthroForms, race.Blocked)
		beastBase := fam.BeastForms
		if len(beastBase) == 0 {
			beastBase = fc.BeastForms
		}
		beastBase = filterPool(beastBase, race.Blocked)
		if len(anthroBase) == 0 || len(beastBase) == 0 {
			morph = ""
		} else {
			// свои формы семейства (F4/F5 — звериные), иначе глобальный список
			// морфа; пустой список выбранного морфа — фолбек на глобальные
			// формы морфа (98a §5.2 п.3)
			var forms []string
			switch morph {
			case "anthro":
				forms = filterPool(fam.AnthroForms, race.Blocked)
				if len(forms) == 0 {
					forms = filterPool(fc.AnthroForms, race.Blocked)
				}
			case "beast":
				forms = filterPool(fam.BeastForms, race.Blocked)
				if len(forms) == 0 {
					forms = filterPool(fc.BeastForms, race.Blocked)
				}
			case "xeno":
				forms = filterPool(fc.XenoForms, race.Blocked)
			case "amorph":
				forms = filterPool(fc.AmorphousForms, race.Blocked)
			case "crystal":
				forms = filterPool(fc.CrystalForms, race.Blocked)
			case "mech":
				forms = filterPool(fc.MechForms, race.Blocked)
			case "titan":
				forms = filterPool(fc.TitanForms, race.Blocked)
			default:
				forms = filterPool(fc.AnthroForms, race.Blocked)
			}
			if len(forms) == 0 {
				forms = filterPool(fc.AnthroForms, race.Blocked)
			}
			form := forms[rng.Intn(len(forms))]
			clothes := ""
			if len(fam.AnthroClothes) > 0 {
				// одежда фильтруется по blocked; пустой пул → без одежды
				clothesPool := filterPool(fam.AnthroClothes, race.Blocked)
				if len(clothesPool) > 0 {
					clothes = " wearing " + clothesPool[rng.Intn(len(clothesPool))]
				}
			}
			if morph == "anthro" {
				// антропо: человеческое тело, лицо, взгляд на зрителя
				// (без «alien» — общий тег делал все морфы одинаковыми «серыми
				// пришельцами», решение создателя 2026-09-18)
				prompt = fmt.Sprintf("realistic portrait of a humanoid race, FRONT VIEW, looking directly at viewer, %s made of %s, %s, head and shoulders,%s torso extending down below the frame, anchored, centered, %s%s, game avatar, no text, no watermark",
					form, mat, glow, clothes, scene, tagsPart)
			} else if morph == "mech" {
				// мехоморфный: гуманоидная машина. БЕЗ «realistic portrait» и
				// «looking directly at viewer» — иначе SDXL рисует лицо/голову
				// (головы у мехов). Кадр — максимум до торса: жёсткие сигналы
				// «chest-up framing, upper body only, no legs» (решение создателя
				// 2026-09-17).
				prompt = fmt.Sprintf("close-up concept art of a humanoid mechanical creature, FRONT VIEW, chest-up framing, upper body only, %s made of %s, %s,%s torso extending down below the frame, no legs, no full body, anchored, centered, %s%s, game avatar, no text, no watermark",
					form, mat, glow, clothes, scene, tagsPart)
			} else {
				// морфы (зверо/ксено/аморф/кристалл/титан): существо смотрит на
				// зрителя, но тело/форму задаёт форма морфа (без «alien» — общий
				// тег делал аморфов «существами», решение создателя 2026-09-18)
				prompt = fmt.Sprintf("realistic portrait of a creature, FRONT VIEW, looking directly at viewer, %s made of %s, %s,%s anchored, centered, %s%s, game avatar, no text, no watermark",
					form, mat, glow, clothes, scene, tagsPart)
			}
			return prompt, race.ID, race.Name
		}
	}
	subject := "an ABSTRACT OBJECT"
	if personage {
		subject = "a personage"
	}
	form := race.Forms[rng.Intn(len(race.Forms))]
	// appearance вставляется после формы в не-антропо ветке (98a §5.4)
	formPart := form
	if race.Appearance != "" {
		formPart = form + ", " + race.Appearance
	}
	prompt = fmt.Sprintf("dramatic cinematic concept art of %s, FRONT VIEW, made of %s, %s, %s, no face, no eyes, no mouth, no human features, asymmetric, anchored by a solid base standing on the bottom edge of the frame, centered, %s%s, masterpiece, game avatar, no text, no watermark",
		subject, mat, formPart, glow, scene, tagsPart)
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