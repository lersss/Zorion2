package names

import (
	"math/rand"
	"strings"
)

// GenerateStarName генерирует название для звезды
func GenerateStarName(rng *rand.Rand, usedNames map[string]bool) string {
	prefixes := []string{
		"Al", "Ar", "Bel", "Ben", "Cal", "Cel", "Dal", "Den", "El", "En",
		"Fal", "Fin", "Gal", "Gor", "Hal", "Hel", "In", "Is", "Jal", "Jen",
		"Kal", "Kel", "Lan", "Lor", "Mal", "Mar", "Nan", "Nel", "Ol", "Or",
		"Pal", "Pel", "Qu", "Quel", "Ral", "Ren", "Sal", "Sel", "Tal", "Ten",
		"Ul", "Ur", "Val", "Vel", "Wal", "Wel", "Xal", "Xen", "Yal", "Yen",
		"Zal", "Zen", "Ael", "Aer", "Bael", "Bri", "Cer", "Cor", "Dae", "Dor",
		"Astr", "Bore", "Cael", "Drac", "Elys", "Fen", "Gla", "Heli", "Ion", "Jov",
		"Kry", "Lyr", "Mira", "Neb", "Orion", "Peg", "Phae", "Qui", "Reg", "Sir",
		"Tau", "Urs", "Vega", "Xan", "Yor", "Zeph",
		"Aquil", "Ara", "Aur", "Boot", "Caelum", "Camel", "Canc", "Capr", "Carr", "Cassi",
		"Cen", "Ceph", "Cetus", "Col", "Com", "Corv", "Crat", "Cru", "Cyg", "Del",
		"Dor", "Dra", "Equ", "Eri", "For", "Gem", "Grus", "Her", "Hor", "Hyd",
		"Ind", "Lac", "Leo", "Lep", "Lib", "Lup", "Lyn", "Lyr", "Mic", "Mon",
		"Mus", "Nor", "Oct", "Oph", "Ori", "Pav", "Phe", "Pic", "Pis", "Pup",
		"Pyx", "Ret", "Sag", "Sco", "Scul", "Ser", "Sex", "Sge", "Sgr", "Tau",
		"Tel", "Tri", "Tuc", "UMa", "UMi", "Vel", "Vir", "Vol", "Vul", "Xer",
	}
	roots := []string{
		"ar", "en", "in", "on", "or", "um", "us", "is", "os", "al",
		"an", "ir", "ur", "yn", "el", "am", "ed", "id", "ul", "em",
		"ab", "ac", "ad", "ag", "ak", "ap", "as", "at", "ax", "az",
		"eb", "ec", "ef", "eg", "ek", "ep", "es", "et", "ex", "ez",
		"ib", "ic", "if", "ig", "ik", "ip", "is", "it", "ix", "iz",
		"ob", "oc", "of", "og", "ok", "op", "os", "ot", "ox", "oz",
		"ub", "uc", "uf", "ug", "uk", "up", "us", "ut", "ux", "uz",
	}
	suffixes := []string{
		"ia", "ar", "on", "is", "us", "os", "um", "a", "e", "i", "o", "y",
		"en", "or", "an", "ir", "yn", "el", "am", "ed", "id", "ul", "em",
		"ae", "ai", "ao", "au", "ei", "eu", "ie", "io", "iu", "oe",
		"oi", "ou", "ua", "ue", "ui", "uo", "ya", "ye", "yi", "yo",
	}

	for attempt := 0; attempt < 50; attempt++ {
		prefix := prefixes[rng.Intn(len(prefixes))]
		root := roots[rng.Intn(len(roots))]
		suf := suffixes[rng.Intn(len(suffixes))]
		name := prefix + root + suf
		if rng.Float64() < 0.3 {
			name = prefix + suf
		}
		if !usedNames[name] {
			usedNames[name] = true
			return name
		}
	}
	return "Star-" + randomSuffix(rng)
}

// GeneratePlanetName генерирует название для планеты.
// Имя строится из 2–3 фонетических слогов (макс. 10 символов).
// Комбинаторное пространство ~len(слогов)^3, чего достаточно на 500к+ планет,
// чтобы не выпадать в fallback.
func GeneratePlanetName(rng *rand.Rand, usedNames map[string]bool) string {
	for attempt := 0; attempt < 200; attempt++ {
		parts := 2
		if rng.Float64() < 0.75 {
			parts = 3 // 3 слога дают основной простор комбинаций
		}
		name := planetNameFromSyllables(rng, parts, 10)
		if name == "" || usedNames[name] {
			continue
		}
		usedNames[name] = true
		return capitalize(name)
	}
	return "Planet-" + randomSuffix(rng)
}

// capitalize — первая буква заглавная ('belrano' → 'Belrano').
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// planetNameFromSyllables собирает имя из parts случайных слогов.
// Возвращает "" если суммарная длина превышает maxLen (и не заглушает коллизии).
func planetNameFromSyllables(rng *rand.Rand, parts, maxLen int) string {
	sb := make([]byte, 0, maxLen)
	for i := 0; i < parts; i++ {
		syl := planetSyllables[rng.Intn(len(planetSyllables))]
		if len(sb)+len(syl) > maxLen {
			return ""
		}
		sb = append(sb, syl...)
	}
	return string(sb)
}

// planetSyllables — фонетические слоги для имён планет (2–4 символа).
var planetSyllables = []string{
	"al", "ar", "an", "as", "at", "av", "ax", "az", "am", "ab",
	"ak", "ap", "eb", "ec", "ed", "ef", "eg", "ek", "em", "en",
	"ep", "es", "et", "ev", "ex", "ez", "ib", "ic", "id", "if",
	"ig", "ik", "il", "im", "in", "ir", "ir", "is", "it", "iz",
	"ob", "oc", "od", "of", "og", "ok", "ol", "om", "on", "op",
	"or", "os", "ot", "ov", "ox", "oz", "ub", "uc", "ud", "uf",
	"ug", "uk", "ul", "um", "un", "up", "ur", "us", "ut", "ux",
	"bel", "ben", "bir", "bal", "bos", "bril", "cal", "can", "cel", "cor",
	"cry", "chal", "dan", "dar", "den", "dor", "dra", "dus", "del", "dim",
	"el", "eri", "eph", "eis", "fae", "fal", "fel", "fen", "fi", "fin",
	"gal", "gan", "gar", "gel", "gil", "gor", "gla", "gri", "hal", "har",
	"hel", "hir", "ho", "hus", "iel", "ith", "jan", "jar", "jel", "jen",
	"jin", "jor", "kal", "kar", "kel", "kir", "kol", "kra", "kry", "la",
	"lae", "lan", "lar", "le", "led", "len", "li", "lin", "lis", "loe",
	"lor", "lum", "mal", "mar", "mel", "mer", "mi", "min", "mir", "mo",
	"mor", "mus", "nal", "nan", "nar", "nae", "nel", "nem", "ni", "nid",
	"no", "nor", "nu", "pal", "par", "pel", "pen", "pha", "pis", "pra",
	"quel", "quin", "ral", "ran", "rei", "ren", "rim", "rin", "ro", "ros",
	"ru", "sal", "sar", "sel", "sem", "sen", "sha", "sil", "sim", "sol",
	"sor", "sta", "sun", "sys", "tal", "tar", "tel", "tem", "ten", "tha",
	"the", "ti", "to", "tor", "ul", "um", "un", "ur", "us", "ut",
	"ux", "val", "van", "var", "vel", "ven", "ver", "vi", "vil", "vir",
	"vol", "vor", "vul", "xal", "xan", "xen", "yal", "yan", "yar", "yel",
	"yen", "yol", "zab", "zan", "zar", "zel", "zen", "zeph", "zia", "zin",
	"zor", "zur",
}

// GenerateSatelliteName — имя спутника: «звезда-XXXX».
// Спутникам не даются собственные имена — только имя родительской звезды + суффикс.
func GenerateSatelliteName(starName string, rng *rand.Rand, usedNames map[string]bool) string {
	if starName == "" {
		starName = "Satel"
	}
	for attempt := 0; attempt < 50; attempt++ {
		name := starName + "-" + randomSuffix(rng)
		if !usedNames[name] {
			usedNames[name] = true
			return name
		}
	}
	return "Satel-" + randomSuffix(rng)
}

// GenerateFactionName генерирует название для фракции
func GenerateFactionName(rng *rand.Rand, usedNames map[string]bool) string {
	prefixes := []string{
		"Al", "Ar", "Bel", "Ben", "Cal", "Cel", "Dal", "Den", "El", "En",
		"Fal", "Fin", "Gal", "Gor", "Hal", "Hel", "In", "Is", "Jal", "Jen",
		"Kal", "Kel", "Lan", "Lor", "Mal", "Mar", "Nan", "Nel", "Ol", "Or",
		"Pal", "Pel", "Qu", "Quel", "Ral", "Ren", "Sal", "Sel", "Tal", "Ten",
		"Ul", "Ur", "Val", "Vel", "Wal", "Wel", "Xal", "Xen", "Yal", "Yen",
		"Zal", "Zen", "Ael", "Aer", "Bael", "Bri", "Cer", "Cor", "Dae", "Dor",
		"Astr", "Bore", "Cael", "Drac", "Elys", "Fen", "Gla", "Heli", "Ion", "Jov",
		"Kry", "Lyr", "Mira", "Neb", "Orion", "Peg", "Phae", "Qui", "Reg", "Sir",
		"Tau", "Urs", "Vega", "Xan", "Yor", "Zeph",
		"Aquil", "Ara", "Aur", "Boot", "Caelum", "Camel", "Canc", "Capr", "Carr", "Cassi",
		"Cen", "Ceph", "Cetus", "Col", "Com", "Corv", "Crat", "Cru", "Cyg", "Del",
		"Dor", "Dra", "Equ", "Eri", "For", "Gem", "Grus", "Her", "Hor", "Hyd",
		"Ind", "Lac", "Leo", "Lep", "Lib", "Lup", "Lyn", "Lyr", "Mic", "Mon",
		"Mus", "Nor", "Oct", "Oph", "Ori", "Pav", "Phe", "Pic", "Pis", "Pup",
		"Pyx", "Ret", "Sag", "Sco", "Scul", "Ser", "Sex", "Sge", "Sgr", "Tau",
		"Tel", "Tri", "Tuc", "UMa", "UMi", "Vel", "Vir", "Vol", "Vul", "Xer",
	}
	suffixes := []string{
		"ia", "um", "or", "is", "an", "os", "us", "a", "e", "o", "i",
		"ae", "on", "ar", "en", "ir", "yn", "el",
		"io", "eo", "ua", "ea", "oi", "ou", "ie", "ei", "au", "ai",
		"ys", "em", "id", "ul", "am", "ed", "yn", "el", "ir", "en",
	}

	for attempt := 0; attempt < 50; attempt++ {
		prefix := prefixes[rng.Intn(len(prefixes))]
		suffix := suffixes[rng.Intn(len(suffixes))]
		name := prefix + suffix
		if !usedNames[name] {
			usedNames[name] = true
			return name
		}
	}
	return "Faction-" + randomSuffix(rng)
}

// GenerateProductName генерирует абстрактное название для товара
func GenerateProductName(category string, rng *rand.Rand, usedNames map[string]bool) string {
	roots := []string{
		"Atr", "Brol", "Vex", "Gron", "Drey", "Zorn", "Kvel", "Mrain", "Nox", "Prox",
		"Svar", "Trok", "Frin", "Hard", "Zvok", "Shrak", "Erd", "Alt", "Brim", "Vald",
		"Gart", "Dreyk", "Zelt", "Korn", "Lint", "Morn", "Norn", "Orn", "Parn", "Rork",
		"Sorn", "Thorn", "Urn", "Forn", "Horn", "Tsorn", "Shorn", "Yurn", "Yarn", "Ael",
		"Bern", "Virn", "Garn", "Dern", "Zern", "Kern", "Lern", "Mern", "Nern", "Pern",
		"Xyl", "Zin", "Cry", "Flo", "Glim", "Nyx", "Onyx", "Pyro", "Quar", "Rime",
		"Scor", "Temp", "Void", "Wisp", "Zeph", "Aura", "Blaz", "Chro", "Dusk", "Echo",
		"Astr", "Bore", "Cael", "Drac", "Elys", "Fen", "Gla", "Heli", "Ion", "Jov",
		"Kry", "Lyr", "Mira", "Neb", "Orion", "Peg", "Phae", "Qui", "Reg", "Sir",
		"Tau", "Urs", "Vega", "Xan", "Yor", "Zeph", "Aquil", "Ara", "Aur", "Boot",
		"Caelum", "Camel", "Canc", "Capr", "Carr", "Cassi", "Cen", "Ceph", "Cetus", "Col",
		"Com", "Corv", "Crat", "Cru", "Cyg", "Del", "Dor", "Dra", "Equ", "Eri",
		"For", "Gem", "Grus", "Her", "Hor", "Hyd", "Ind", "Lac", "Leo", "Lep",
		"Lib", "Lup", "Lyn", "Lyr", "Mic", "Mon", "Mus", "Nor", "Oct", "Oph",
		"Ori", "Pav", "Phe", "Pic", "Pis", "Pup", "Pyx", "Ret", "Sag", "Sco",
		"Scul", "Ser", "Sex", "Sge", "Sgr", "Tau", "Tel", "Tri", "Tuc", "UMa",
		"UMi", "Vel", "Vir", "Vol", "Vul", "Xer",
	}
	suffixes := map[string][]string{
		"food":         {"in", "an", "il", "on", "or", "ite", "um", "oz", "a", "ya", "ic", "ane", "ene", "ine", "one"},
		"clothing":     {"in", "an", "il", "on", "or", "ite", "um", "oz", "a", "ya", "ic", "ane", "ene", "ine", "one"},
		"construction": {"in", "an", "il", "on", "or", "ite", "um", "oz", "a", "ya", "ic", "ane", "ene", "ine", "one"},
		"electronics":  {"in", "an", "il", "on", "or", "ite", "um", "oz", "a", "ya", "ic", "ane", "ene", "ine", "one"},
		"weapons":      {"in", "an", "il", "on", "or", "ite", "um", "oz", "a", "ya", "ic", "ane", "ene", "ine", "one"},
		"medicine":     {"in", "an", "il", "on", "or", "ite", "um", "oz", "a", "ya", "ic", "ane", "ene", "ine", "one"},
		"tools":        {"in", "an", "il", "on", "or", "ite", "um", "oz", "a", "ya", "ic", "ane", "ene", "ine", "one"},
		"transport":    {"in", "an", "il", "on", "or", "ite", "um", "oz", "a", "ya", "ic", "ane", "ene", "ine", "one"},
		"fuel":         {"in", "an", "il", "on", "or", "ite", "um", "oz", "a", "ya", "ic", "ane", "ene", "ine", "one"},
		"furniture":    {"in", "an", "il", "on", "or", "ite", "um", "oz", "a", "ya", "ic", "ane", "ene", "ine", "one"},
	}
	sufList := suffixes[category]
	if len(sufList) == 0 {
		sufList = suffixes["food"]
	}

	for attempt := 0; attempt < 50; attempt++ {
		root := roots[rng.Intn(len(roots))]
		suf := sufList[rng.Intn(len(sufList))]
		name := root + suf
		if !usedNames[name] {
			usedNames[name] = true
			return name
		}
	}
	return "Product-" + randomSuffix(rng)
}

// randomSuffix — вспомогательная функция для создания уникального суффикса
func randomSuffix(rng *rand.Rand) string {
	letters := []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]rune, 4)
	for i := range b {
		b[i] = letters[rng.Intn(len(letters))]
	}
	return string(b)
}