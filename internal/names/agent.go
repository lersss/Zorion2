// internal/names/agent.go
// Генератор имён NPC-агентов (спека 26a.1 §3): слоговый, «звучит как
// английское», формат «Имя Фамилия».
//
// Устройство (спека §3.1):
//
//	имя     = слог1_имени + слог2_имени        («Mar» + «ion» = Marion, «Cy» + «rus» = Cyrus)
//	фамилия = основа_фамилии + окончание_фамилии («Hal» + «e» = Hale; «Ven» + ∅ = Venn)
//	полное  = имя + " " + фамилия              «Marion Hale»
//
// Комбинаторика: имён = 60 × 56 = 3360; фамилий = 75 × 30 = 2250
// (окончания включают пустое — фамилия = основа); пар = 3360 × 2250 ≈ 7.5 млн.
// Цель 100к агентов → заполняемость ~1.3%, в среднем ~1.01 попытки на имя;
// запас ×75 от цели. Fallback при исчерпании — «Agent-XXXX» (как
// GeneratePlanetName), в 26a недостижим.
package names

import (
	"math/rand"
)

// agentFirstSyllable1 — сильные открывающие слоги имени (~60 шт).
var agentFirstSyllable1 = []string{
	"Mar", "Cy", "No", "Ad", "Bri", "Car", "Dan", "El", "Em", "Er",
	"Ev", "Fel", "Gab", "Hal", "Hel", "Jan", "Jen", "Jo", "Ka", "Ke",
	"Lan", "Leo", "Lu", "Ma", "Mel", "Mic", "Na", "Ni", "Ol", "Pa",
	"Pe", "Phi", "Ra", "Re", "Ri", "Ro", "Ru", "Sa", "Se", "Si",
	"So", "Ste", "Ta", "Te", "The", "Tho", "Ti", "To", "Va", "Ve",
	"Vi", "Wal", "Wil", "Za", "Ze", "Al", "Ben", "Gil", "Cor", "Dor",
}

// agentFirstSyllable2 — финали имени (~56 шт).
var agentFirstSyllable2 = []string{
	"a", "e", "i", "o", "an", "en", "in", "on", "un", "er",
	"or", "ar", "el", "al", "id", "ad", "us", "is", "os", "as",
	"ie", "ia", "io", "ew", "ow", "ay", "ey", "yn", "ot", "et",
	"it", "at", "ace", "ian", "ion", "iel", "eon", "ose", "ene", "ine",
	"one", "ard", "ert", "ick", "iff", "igh", "ille", "ish", "itt", "ock",
	"oyd", "ull", "yle", "eth", "ell", "ore",
}

// agentLastBase — основы фамилии (~75 шт).
var agentLastBase = []string{
	"Hal", "Ven", "Por", "Shaw", "Crane", "Whit", "Ash", "Bell", "Brook", "Carr",
	"Clay", "Cole", "Cross", "Dale", "Dean", "Dell", "Drake", "Fenn", "Ford", "Frost",
	"Gale", "Grant", "Gray", "Hart", "Holt", "Kent", "Kirk", "Lane", "Lark", "Leith",
	"Lind", "Lorn", "Marsh", "Moore", "Moss", "Neal", "North", "Park", "Penn", "Pike",
	"Price", "Quinn", "Reed", "Rook", "Rose", "Scott", "Sloan", "Smith", "Snow", "Stark",
	"Stone", "Storm", "Swift", "Tate", "Thorn", "Vane", "Voss", "Wade", "Ward", "Wayne",
	"Webb", "West", "Wren", "Yale", "York", "Burn", "Chase", "Doyle", "Field", "Grove",
	"Hale", "Kerr", "Linn", "Mills", "Nash",
}

// agentLastEnding — окончания фамилии (~30 шт, включая пустое: фамилия =
// основа, «Venn»). Пустое — один из элементов пула (вероятность ~1/30).
var agentLastEnding = []string{
	"e", "a", "er", "or", "et", "ell", "en", "ing", "ley", "man",
	"son", "sen", "s", "es", "ey", "ock", "ort", "ose", "eld", "burg",
	"ford", "wood", "house", "field", "shore", "", "ace", "ard", "well", "land",
}

// GenerateAgentName — «имя-фамилия» из слоговых пулов (§3.1). Гарантированно
// уникальна относительно usedNames (map растёт на каждой генерации), как
// GeneratePlanetName. Использовать только с локальным *rand.Rand вызывающего
// (AGENTS.md §0: общие *rand.Rand не потокобезопасны).
func GenerateAgentName(rng *rand.Rand, usedNames map[string]bool) string {
	for attempt := 0; attempt < 200; attempt++ {
		first := agentFirstSyllable1[rng.Intn(len(agentFirstSyllable1))] +
			agentFirstSyllable2[rng.Intn(len(agentFirstSyllable2))]
		last := agentLastBase[rng.Intn(len(agentLastBase))] +
			agentLastEnding[rng.Intn(len(agentLastEnding))]
		full := capitalize(first) + " " + capitalize(last)
		if !usedNames[full] {
			usedNames[full] = true
			return full
		}
	}
	return "Agent-" + randomSuffix(rng)
}