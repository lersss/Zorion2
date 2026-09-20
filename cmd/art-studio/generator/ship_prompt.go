package generator

import (
	"encoding/json"
	"math/rand"
	"strings"

	"zorion/cmd/art-studio/config"
)

// SilhouetteSpec — разобранное ТЗ силуэта (спека §3.1). Пишется в JSON для
// tools/make_ship_silhouettes.py (скрипт рисует по spec, а не парсит ТЗ —
// парсер живёт в Go, TDD-требование).
type SilhouetteSpec struct {
	Form      string       `json:"form"`      // категория формы (Python draw_<form>)
	FormKey   string       `json:"form_key"`  // русское ключевое слово ТЗ
	Fallback  bool         `json:"fallback"`  // форма не найдена → капсула
	Warnings  []string     `json:"warnings"`  // предупреждения парсера (M1/M2)
	Modules   []ShipModule `json:"modules"`   // модули поверх формы
	Details   []string     `json:"details"`   // русские ключи деталей (этап 1)
	LightNose bool         `json:"light_nose"` // светлая секция носа справа
	WarmGlow  bool         `json:"warm_glow"`  // FIRE-модуль (тёплое свечение в ТЗ)
}

// ShipModule — модуль силуэта: тип (hull/wings/tail/nozzles/cockpit/glow/
// stilts/tentacles/sails/gills/channels/bubbles/trails/patches) + цвет палитры
// art_ships §2.1 (cream/steel/bronze/dark/light/fire/cold/cold_hull).
type ShipModule struct {
	Type  string `json:"type"`
	Color string `json:"color"`
}

// shipModuleKeyword — ключевое слово модуля (спека §3.1 п.2): token — подстрока
// пары «имя цвет», typ — тип модуля, nameOnly — матчить только в позиции имени
// (первое слово пары), не в позиции цвета («светл» в «тяжи светлые» — цвет,
// не имя; диагноз визуального аудита).
type shipModuleKeyword struct {
	token    string
	typ      string
	nameOnly bool
}

// shipModuleKeywords — ключевые слова модулей (спека §3.1 п.2), длинные
// раньше коротких (стабилизаторы до крылья, накопитель до кабина);
// специфичные имена (тяжи/заплаты/иллюминатор/свечение) РАНЬШЕ общих
// («светл», «нос») — иначе «тяжи светлые» ловит «светл» как имя (диагноз
// визуального аудита).
var shipModuleKeywords = []shipModuleKeyword{
	{"стабилизаторы", "wings", false},
	{"щупальц", "tentacles", false},
	{"антенны", "tentacles", false},
	{"тяж", "tentacles", false},
	{"корень", "tentacles", false},
	{"корнев", "tentacles", false},
	{"хвост", "tentacles", false},
	{"заплат", "patches", false},
	{"нашлёпк", "patches", false},
	{"нарост", "patches", false},
	{"иллюминатор", "cockpit", false},
	{"окн", "cockpit", false},
	{"оперение", "tail", false},
	{"реснички", "sails", false},
	{"прожилки", "glow", false},
	{"накопитель", "cockpit", false},
	{"свечен", "glow", false},
	{"glow", "glow", false},
	{"огонь", "glow", false},
	{"капюшон", "cockpit", false},
	{"кромка", "cockpit", false},
	{"стекло", "cockpit", false},
	{"линза", "cockpit", false},
	{"датчик", "cockpit", false},
	{"глаз", "cockpit", false},
	{"ядро", "cockpit", false},
	{"кабина", "cockpit", false},
	{"светл", "cockpit", true},
	{"нос", "cockpit", true},
	{"крылья", "wings", false},
	{"дюзы", "nozzles", false},
	{"сопла", "nozzles", false},
	{"расплав", "glow", true},
	{"пятна", "glow", false},
	{"сваи", "stilts", false},
	{"опоры", "stilts", false},
	{"паруса", "sails", false},
	{"жабры", "gills", false},
	{"каналы", "channels", false},
	{"пузыри", "bubbles", false},
	{"шлейфы", "trails", false},
	{"корпус", "hull", false},
}

// fixedModuleColors — цвет модуля по типу (спека §3.1: точные цвета ТЗ в
// силуэт НЕ переносятся — они уходят в texture, этап 2). Корпус — фолбек
// крем; холодный корпус (cold_hull) — по цветовым словам ТЗ (resolveHullColor).
var fixedModuleColors = map[string]string{
	"hull": "cream", "wings": "steel", "tail": "bronze", "nozzles": "dark",
	"cockpit": "light", "stilts": "steel", "tentacles": "steel",
	"sails": "steel", "gills": "steel", "bubbles": "steel", "trails": "steel",
	"patches": "light",
}

// warmColorStems — тёплые корни цвета свечения (спека §3.1: оранжев*/янтарн*/
// угольн*/расплав*/жёлт*/огненн*/огонь*; падежные формы покрываются стеммингом).
var warmColorStems = []string{"оранжев", "янтарн", "угольн", "расплав", "жёлт", "огненн", "огонь"}

// coldColorStems — холодные корни цвета свечения (голуб*/сине-зелён*/бледн*/ледян*).
var coldColorStems = []string{"голуб", "сине-зелён", "бледн", "ледян"}

// fireStems — тёплые корни FIRE-модуля (спека §3.1: расплав*/уголь*/янтар*/огонь*;
// НЕ оранжев/жёлт — они только для цвета свечения).
var fireStems = []string{"расплав", "уголь", "янтар", "огонь"}

// lightNoseWords — слова светлой секции носа (спека §3.1 п.4).
var lightNoseWords = []string{"кабина", "капюшон", "кромка", "стекло", "линза", "датчик", "глаз", "накопитель", "ядро", "светл", "иллюминатор", "окн"}

// coldHullStems — холодные корни цвета корпуса (диагноз визуального аудита,
// п.3: «бледно-голубой», «голубой», «сине-», «лёд*», «ледян*», «иней*»,
// «белый»). «лёд»/«ледян» вместо «лед»: «бледный» содержит «лед» (б-л-е-д) —
// ложное срабатывание (deep_dwellers «корпус бледный» — крем).
var coldHullStems = []string{"бледно-голуб", "голуб", "сине", "лёд", "ледян", "иней", "бел"}

// warmHullStems — тёплые корни цвета корпуса («пепел*», «сера*», «расплав*»,
// «лава*»).
var warmHullStems = []string{"пепел", "сер", "расплав", "лав"}

// ParseSilhouette разбирает ТЗ силуэта (спека §3.1): форма (первое ключевое
// слово ship_dict.forms; не найдено → фолбек капсула + warning M1), модули
// (секция «модули: …» до «асимметрия»/«запас»; неизвестный модуль → пропуск +
// warning), детали (слова ship_dict.details в тексте), светлый нос, FIRE-модуль.
func ParseSilhouette(sil string, dict *config.ShipDictConfig) SilhouetteSpec {
	spec := SilhouetteSpec{}
	form, fallback := detectShipForm(sil, dict)
	spec.Form = form.Category
	spec.FormKey = form.Key
	spec.Fallback = fallback
	if fallback {
		spec.Warnings = append(spec.Warnings, "форма не найдена в ТЗ, фолбек капсула")
	}
	for _, pair := range splitModulesSection(sil) {
		typ, colorWord, ok := parseShipModulePair(pair)
		if !ok {
			spec.Warnings = append(spec.Warnings, "неизвестный модуль: "+pair)
			continue
		}
		color := fixedModuleColors[typ]
		if typ == "glow" || typ == "channels" {
			color = resolveGlowColor(colorWord)
		}
		if typ == "hull" {
			color = resolveHullColor(colorWord)
		}
		spec.Modules = append(spec.Modules, ShipModule{Type: typ, Color: color})
	}
	spec.Details = extractShipDetails(sil, dict)
	spec.LightNose = hasAnyWord(sil, lightNoseWords)
	spec.WarmGlow = hasAnyWord(sil, fireStems)
	return spec
}

// detectShipForm — первое ключевое слово формы из ship_dict.forms (порядок
// словаря). Не найдено → капсула + fallback=true.
func detectShipForm(sil string, dict *config.ShipDictConfig) (config.ShipForm, bool) {
	low := strings.ToLower(sil)
	for _, f := range dict.Forms {
		if strings.Contains(low, strings.ToLower(f.Key)) {
			return f, false
		}
	}
	for _, f := range dict.Forms {
		if f.Category == "capsule" {
			return f, true
		}
	}
	return config.ShipForm{Key: "капсула", Concept: "capsule", Category: "capsule"}, true
}

// splitModulesSection — секция «модули: …» (до «асимметрия»/«запас»), пары
// через запятую, trim « ;:». Пусто — нет секции.
func splitModulesSection(sil string) []string {
	idx := strings.Index(sil, "модули:")
	if idx < 0 {
		return nil
	}
	rest := sil[idx+len("модули:"):]
	end := len(rest)
	for _, stop := range []string{"асимметрия", "запас"} {
		if i := strings.Index(rest, stop); i >= 0 && i < end {
			end = i
		}
	}
	var out []string
	for _, pair := range strings.Split(rest[:end], ",") {
		pair = strings.Trim(pair, " \t;:")
		if pair != "" {
			out = append(out, pair)
		}
	}
	return out
}

// parseShipModulePair — тип модуля по ключевым словам + слово цвета (текст
// после ключевого слова). Неизвестный модуль → ok=false. nameOnly-токены
// («светл», «нос») матчатся только в позиции имени (первое слово пары) —
// «светлые» в «тяжи светлые» не имя, а цвет (диагноз визуального аудита).
func parseShipModulePair(pair string) (typ, colorWord string, ok bool) {
	low := strings.ToLower(pair)
	for _, kw := range shipModuleKeywords {
		if i := strings.Index(low, kw.token); i >= 0 {
			if kw.nameOnly && strings.Contains(low[:i], " ") {
				continue // токен в позиции цвета, не имени
			}
			return kw.typ, strings.TrimSpace(pair[i+len(kw.token):]), true
		}
	}
	return "", "", false
}

// resolveHullColor — цвет корпуса по слову цвета ТЗ (диагноз визуального
// аудита, п.3): холодные слова → cold_hull (холодный бледно-голубой корпус
// вместо крем), тёплые → cream (тёплый крем/пепел), непокрытый → cream.
func resolveHullColor(colorWord string) string {
	low := strings.ToLower(colorWord)
	for _, s := range coldHullStems {
		if strings.Contains(low, s) {
			return "cold_hull"
		}
	}
	for _, s := range warmHullStems {
		if strings.Contains(low, s) {
			return "cream"
		}
	}
	return "cream"
}

// resolveGlowColor — цвет свечения по слову цвета ТЗ: тёплые корни → fire,
// холодные → cold, непокрытый → fire (M2, спека §3.1).
func resolveGlowColor(colorWord string) string {
	low := strings.ToLower(colorWord)
	for _, s := range warmColorStems {
		if strings.Contains(low, s) {
			return "fire"
		}
	}
	for _, s := range coldColorStems {
		if strings.Contains(low, s) {
			return "cold"
		}
	}
	return "fire"
}

// extractShipDetails — русские ключи деталей из ship_dict.details, найденные
// в тексте ТЗ по стему (падежные формы: «шлейфами» → «шлейф»; спека §3.1 п.3),
// дедуп по порядку словаря.
func extractShipDetails(sil string, dict *config.ShipDictConfig) []string {
	low := strings.ToLower(sil)
	var out []string
	for _, d := range dict.Details {
		if strings.Contains(low, strings.ToLower(d.Stem)) {
			out = append(out, d.Key)
		}
	}
	return out
}

// hasAnyWord — true, если текст содержит любое из слов (lowercase, подстрока).
func hasAnyWord(text string, words []string) bool {
	low := strings.ToLower(text)
	for _, w := range words {
		if strings.Contains(low, w) {
			return true
		}
	}
	return false
}

// BuildShipPrompt1 — промпт этапа 1 (форма, ControlNet Canny + img2img,
// спека §3.2): {concept}, {details}, ориентационный хвост, {tags} перед
// «no text, no watermark». {concept} — расовое концепт-слово
// (ship_dict.concepts[race], «корабль в образе …»); у расы без концепта —
// фолбек на форму из словаря форм (текущее поведение). Детали — 2–3
// случайные из извлечённых, отфильтрованные по blocked; пусто → не добавляется.
func BuildShipPrompt1(rng *rand.Rand, race string, entry config.ShipEntry, spec SilhouetteSpec, dict *config.ShipDictConfig, tags string) string {
	concept := dict.Concepts[race]
	if concept == "" {
		concept = "capsule"
		for _, f := range dict.Forms {
			if f.Category == spec.Form {
				concept = f.Concept
				break
			}
		}
	}
	phrases := shipDetailsPhrases(spec.Details, dict, entry.Blocked)
	parts := []string{concept}
	if picked := pickRandom(rng, phrases, 2, 3); len(picked) > 0 {
		parts = append(parts, strings.Join(picked, ", "))
	}
	parts = append(parts, "top-down flat view, horizontal, nose pointing FORWARD to the right, "+engineTail(entry.Blocked)+", perfectly flat, no perspective, game asset, 2D sprite, centered, single ship, on black background")
	if tags != "" {
		parts = append(parts, tags)
	}
	parts = append(parts, "no text, no watermark")
	return strings.Join(parts, ", ")
}

// engineTail — ориентационный хвост этапа 1: «engine at the LEFT rear» для рас
// с машинами; для рас без машин (blocked: machine/engine/mech/robot) —
// «trailing tendrils at the left rear» (диагноз визуального аудита, п.5:
// жёсткий хвост про двигатель противоречит расе без машин).
func engineTail(blocked []string) string {
	for _, tok := range blocked {
		tl := strings.ToLower(tok)
		for _, m := range []string{"machine", "engine", "mech", "robot"} {
			if strings.Contains(tl, m) {
				return "trailing tendrils at the left rear"
			}
		}
	}
	return "engine at the LEFT rear"
}

// BuildShipPrompt2 — промпт этапа 2 (текстура, img2img без ControlNet,
// спека §3.3): {texture расы}, {texture-теги}, maximal detail, masterpiece,
// {tags}, no text, no watermark. Теги — 2–3 случайных из ship_dict.textures,
// отфильтрованных по blocked/blocked_by; тёплые — только при тёплом слове
// в texture (C2); пусто → не добавляется.
func BuildShipPrompt2(rng *rand.Rand, entry config.ShipEntry, dict *config.ShipDictConfig, tags string) string {
	warm := hasWarmMarker(entry.Texture, dict.WarmMarkers)
	pool := filterShipTextures(dict, entry.Blocked, warm)
	parts := []string{entry.Texture}
	if picked := pickRandom(rng, pool, 2, 3); len(picked) > 0 {
		parts = append(parts, strings.Join(picked, ", "))
	}
	parts = append(parts, "maximal detail, masterpiece")
	if tags != "" {
		parts = append(parts, tags)
	}
	parts = append(parts, "no text, no watermark")
	return strings.Join(parts, ", ")
}

// pickRandom — n ∈ [min, max] случайных элементов пула без повторов
// (n ограничено длиной пула; пустой пул → nil).
func pickRandom(rng *rand.Rand, pool []string, min, max int) []string {
	if len(pool) == 0 {
		return nil
	}
	n := min + rng.Intn(max-min+1)
	if n > len(pool) {
		n = len(pool)
	}
	rest := append([]string(nil), pool...)
	out := make([]string, 0, n)
	for i := 0; i < n && len(rest) > 0; i++ {
		k := rng.Intn(len(rest))
		out = append(out, rest[k])
		rest = append(rest[:k], rest[k+1:]...)
	}
	return out
}

// shipDetailsPhrases — англ. фразы извлечённых деталей (маппинг ship_dict.
// details), отфильтрованные по blocked (спека §3.2/§3.4). Пустой результат →
// детали не добавляются.
func shipDetailsPhrases(keys []string, dict *config.ShipDictConfig, blocked []string) []string {
	var phrases []string
	for _, k := range keys {
		for _, d := range dict.Details {
			if d.Key == k {
				phrases = append(phrases, d.Phrase)
				break
			}
		}
	}
	return filterPool(phrases, blocked)
}

// filterShipTextures — пул текстурных тегов этапа 2: фильтр по blocked (тег
// содержит токен) и blocked_by (токен тега в blocked расы, substring), затем
// жёсткий гейт тёплых тегов (только при тёплом слове в texture, C2). Фолбек
// пустого пула после blocked-фильтра → исходный пул (98a §5.2 п.1); гейт
// тёплых фолбека не имеет (иначе «no warm colors» вернёт тёплые теги).
func filterShipTextures(dict *config.ShipDictConfig, blocked []string, warm bool) []string {
	pool := make([]string, 0, len(dict.Textures))
	for _, t := range dict.Textures {
		if blockedMatch(t.Tag, lowerTokens(blocked)) {
			continue
		}
		if blockedByMatch(t, blocked) {
			continue
		}
		pool = append(pool, t.Tag)
	}
	if len(pool) == 0 {
		for _, t := range dict.Textures {
			pool = append(pool, t.Tag)
		}
	}
	if !warm {
		out := pool[:0]
		for _, tag := range pool {
			if textureWarmth(dict, tag) != "warm" {
				out = append(out, tag)
			}
		}
		pool = out
	}
	return pool
}

// blockedByMatch — true, если любой токен blocked_by тега присутствует в
// blocked расы (substring, регистронезависимо; спека §3.4).
func blockedByMatch(t config.ShipTexture, blocked []string) bool {
	if len(t.BlockedBy) == 0 || len(blocked) == 0 {
		return false
	}
	for _, tok := range t.BlockedBy {
		tl := strings.ToLower(tok)
		for _, b := range blocked {
			if strings.Contains(b, tl) {
				return true
			}
		}
	}
	return false
}

// textureWarmth — warmth тега по словарю (neutral, если не найден).
func textureWarmth(dict *config.ShipDictConfig, tag string) string {
	for _, t := range dict.Textures {
		if t.Tag == tag {
			return t.Warmth
		}
	}
	return "neutral"
}

// hasWarmMarker — true, если texture содержит тёплый маркер НЕ под отрицанием
// «no » (спека §3.3: «no warm colors» не включает тёплые теги; «cold engine
// glow» не матчится — «glow» не маркер).
func hasWarmMarker(texture string, markers []string) bool {
	low := strings.ToLower(texture)
	for _, m := range markers {
		ml := strings.ToLower(m)
		idx := 0
		for {
			i := strings.Index(low[idx:], ml)
			if i < 0 {
				break
			}
			i += idx
			before := ""
			if i >= 3 {
				before = low[i-3 : i]
			}
			if !strings.HasSuffix(before, "no ") {
				return true
			}
			idx = i + len(ml)
		}
	}
	return false
}

// DeadBlockedTokens — мёртвые токены blocked (спека §3.4, мягкий
// кросс-конфиг-тест): токен не встречается ни в одной англ. фразе деталей,
// ни в одном текстурном теге, ни в одном blocked_by тегов ship_dict →
// warning (не ошибка). Возвращает карту slug → мёртвые токены. Токены
// писались до словарей — мёртвые ожидаемы; после пилота решается об
// ужесточении (гейт).
func DeadBlockedTokens(ships config.ShipsConfig, dict *config.ShipDictConfig) map[string][]string {
	var haystack []string
	for _, d := range dict.Details {
		haystack = append(haystack, d.Phrase)
	}
	for _, t := range dict.Textures {
		haystack = append(haystack, t.Tag)
		haystack = append(haystack, t.BlockedBy...)
	}
	out := map[string][]string{}
	for slug, e := range ships {
		for _, tok := range e.Blocked {
			found := false
			for _, h := range haystack {
				if strings.Contains(strings.ToLower(h), tok) {
					found = true
					break
				}
			}
			if !found {
				out[slug] = append(out[slug], tok)
			}
		}
	}
	return out
}

// ShipPrompt1Template — шаблон промпта этапа 1 (для UI-панели, спека §3.2).
const ShipPrompt1Template = "{concept}, {details}, top-down flat view, horizontal, nose pointing FORWARD to the right, engine at the LEFT rear, perfectly flat, no perspective, game asset, 2D sprite, centered, single ship, on black background, {tags}, no text, no watermark"

// ShipPrompt2Template — шаблон промпта этапа 2 (для UI-панели, спека §3.3).
const ShipPrompt2Template = "{texture}, {texture-tags}, maximal detail, masterpiece, {tags}, no text, no watermark"

// SilhouetteSpecJSON — сериализация spec для скрипта силуэтов.
func SilhouetteSpecJSON(spec SilhouetteSpec) ([]byte, error) {
	return json.Marshal(spec)
}