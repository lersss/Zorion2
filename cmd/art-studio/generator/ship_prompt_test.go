package generator

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zorion/cmd/art-studio/config"
)

const shipDictPath = "../../../config/art/ship_dict.json"

func loadShipDict(t *testing.T) *config.ShipDictConfig {
	t.Helper()
	dc, err := config.LoadShipDict(shipDictPath)
	if err != nil {
		t.Fatalf("LoadShipDict: %v", err)
	}
	return dc
}

// TestParseSilhouetteHumans — крыло людей: форма wing, модули
// hull/wings/nozzles/cockpit, деталь стабилизаторы, светлый нос, без FIRE.
func TestParseSilhouetteHumans(t *testing.T) {
	dc := loadShipDict(t)
	sil := "крыло-корпус в плане: широкий нос (светлая кабина-стекло) справа, сужающаяся корма с дюзами слева; асимметрия по оси «нос-корма»; крылья-стабилизаторы; модули: корпус крем, крылья сталь, дюзы тёмные, кабина светлая; запас от краёв ~90 px"
	spec := ParseSilhouette(sil, dc)
	if spec.Form != "wing" || spec.FormKey != "крыло" {
		t.Errorf("form = %s/%s, want wing/крыло", spec.Form, spec.FormKey)
	}
	if spec.Fallback {
		t.Errorf("fallback = true, want false")
	}
	want := []ShipModule{
		{"hull", "cream"}, {"wings", "steel"}, {"nozzles", "dark"}, {"cockpit", "light"},
	}
	if len(spec.Modules) != len(want) {
		t.Fatalf("modules = %v, want %v", spec.Modules, want)
	}
	for i := range want {
		if spec.Modules[i] != want[i] {
			t.Errorf("modules[%d] = %v, want %v", i, spec.Modules[i], want[i])
		}
	}
	if len(spec.Details) != 1 || spec.Details[0] != "стабилизаторы" {
		t.Errorf("details = %v, want [стабилизаторы]", spec.Details)
	}
	if !spec.LightNose {
		t.Errorf("light_nose = false, want true")
	}
	if spec.WarmGlow {
		t.Errorf("warm_glow = true, want false (у людей нет тёплого свечения)")
	}
}

// TestParseSilhouetteDeepDwellers — глубинники: форма не найдена → фолбек
// капсула + warning (M1); пятна сине-зелёные → glow cold; светлый нос.
func TestParseSilhouetteDeepDwellers(t *testing.T) {
	dc := loadShipDict(t)
	sil := "округлый мягкий корпус в плане: нос — светлый полупрозрачный капюшон (почти белый, без синевы) справа, корма — пучок щупалец-антенн слева; по бокам веерные жабры-стабилизаторы; сине-зелёные светящиеся пятна рассеяны по корпусу, не сплошным носом; модули: корпус бледный, корка тёмная, пятна сине-зелёные; асимметрия: щупальца только на корме; запас от краёв ~90 px"
	spec := ParseSilhouette(sil, dc)
	if !spec.Fallback || spec.Form != "capsule" {
		t.Errorf("fallback = %v, form = %s, want true/capsule", spec.Fallback, spec.Form)
	}
	if len(spec.Warnings) == 0 {
		t.Errorf("warnings пуст, want M1")
	}
	// корпус → hull/cream; корка — неизвестный модуль (пропуск); пятна → glow/cold
	found := map[string]bool{}
	for _, m := range spec.Modules {
		found[m.Type+"/"+m.Color] = true
	}
	if !found["hull/cream"] {
		t.Errorf("нет hull/cream в %v", spec.Modules)
	}
	if !found["glow/cold"] {
		t.Errorf("нет glow/cold в %v (пятна сине-зелёные)", spec.Modules)
	}
	if !spec.LightNose {
		t.Errorf("light_nose = false, want true (капюшон)")
	}
	if spec.WarmGlow {
		t.Errorf("warm_glow = true, want false")
	}
}

// TestParseSilhouetteSaltfolk — солевые: тигель, расплав оранжевый → glow/fire,
// FIRE-модуль (warm_glow).
func TestParseSilhouetteSaltfolk(t *testing.T) {
	dc := loadShipDict(t)
	sil := "широкий низкий корпус-«тигель» в плане: округлый нос (светлая полупрозрачная соляная кромка) справа, широкая корма слева; по корпусу — каналы расплава с оранжевым свечением, пузыри, кристаллические грани; асимметрия: каналы тянутся от носа к корме; модули: кристаллы бело-розовые, расплав оранжевый; запас от краёв ~90 px"
	spec := ParseSilhouette(sil, dc)
	if spec.Form != "crucible" {
		t.Errorf("form = %s, want crucible", spec.Form)
	}
	if !spec.WarmGlow {
		t.Errorf("warm_glow = false, want true (расплав)")
	}
	found := map[string]bool{}
	for _, m := range spec.Modules {
		found[m.Type+"/"+m.Color] = true
	}
	if !found["glow/fire"] {
		t.Errorf("нет glow/fire в %v (расплав оранжевый)", spec.Modules)
	}
	// каналы и пузыри — детали этапа 1
	if !containsStr(spec.Details, "каналы") || !containsStr(spec.Details, "пузыри") {
		t.Errorf("details = %v, want каналы+пузыри", spec.Details)
	}
}

// TestParseSilhouetteCryoSwarms — крио-рои: капсула, свечение голубое → cold,
// без FIRE.
func TestParseSilhouetteCryoSwarms(t *testing.T) {
	dc := loadShipDict(t)
	sil := "вытянутая ледяная капсула-носитель в плане: нос — светлый бело-голубой ледяной капюшон (светлая кромка, не синий) справа, корма с хвостовым оперением из инея слева; сквозь полупрозрачный лёд — голубое свечение роя внутри; модули: корпус светло-голубой лёд, оперение бело-голубое, свечение роя голубое; асимметрия за счёт завихрения роя; запас от краёв ~90 px"
	spec := ParseSilhouette(sil, dc)
	if spec.Form != "capsule" {
		t.Errorf("form = %s, want capsule", spec.Form)
	}
	if spec.WarmGlow {
		t.Errorf("warm_glow = true, want false")
	}
	found := map[string]bool{}
	for _, m := range spec.Modules {
		found[m.Type+"/"+m.Color] = true
	}
	if !found["glow/cold"] {
		t.Errorf("нет glow/cold в %v (свечение голубое)", spec.Modules)
	}
	if !found["tail/bronze"] {
		t.Errorf("нет tail/bronze в %v (оперение)", spec.Modules)
	}
}

// TestParseSilhouetteMistSwarms — туман-рои: оболочка, бледно-янтарное
// свечение → fire (янтар побеждает бледн), FIRE-модуль.
func TestParseSilhouetteMistSwarms(t *testing.T) {
	dc := loadShipDict(t)
	sil := "вытянутая оболочка-«дирижабль» в плане: округлый нос (светлая полупрозрачная дымка) справа, сужающаяся корма с дымными шлейфами слева; сквозь оболочку — рой с бледно-янтарным свечением, смещён к носу; реснички-паруса по кромкам; модули: оболочка серо-белая, свечение роя бледно-янтарное; асимметрия за счёт смещения облака роя; запас от краёв ~90 px"
	spec := ParseSilhouette(sil, dc)
	if spec.Form != "envelope" {
		t.Errorf("form = %s, want envelope", spec.Form)
	}
	if !spec.WarmGlow {
		t.Errorf("warm_glow = false, want true (янтар)")
	}
	found := map[string]bool{}
	for _, m := range spec.Modules {
		found[m.Type+"/"+m.Color] = true
	}
	if !found["glow/fire"] {
		t.Errorf("нет glow/fire в %v (бледно-янтарное)", spec.Modules)
	}
	if !containsStr(spec.Details, "шлейфы") || !containsStr(spec.Details, "паруса") {
		t.Errorf("details = %v, want шлейфы+паруса", spec.Details)
	}
}

// TestParseSilhouetteAmmonia — аммиачники (диагноз визуального аудита):
// модули [hull, tentacles, patches, cockpit] — НЕ носовой прямоугольник из
// «тяжи светлые» (ложное «светл» в позиции цвета), заплаты/иллюминатор не
// выброшены как неизвестные; корпус холодный (бледно-голубой).
func TestParseSilhouetteAmmonia(t *testing.T) {
	dc := loadShipDict(t)
	sil := "корпус-«капля» в плане: скруглённый нос (светлый иней-иллюминатор) справа, корма с пучком свисающих корневых тяжей слева; на бортах — пятна гелевых заплат и иней-кромка; модули: корпус бледно-голубой, тяжи светлые, заплаты полупрозрачные, иллюминатор почти белый; асимметрия по оси «нос-корма»; запас от краёв ~90 px"
	spec := ParseSilhouette(sil, dc)
	if spec.Form != "drop" {
		t.Errorf("form = %s, want drop", spec.Form)
	}
	want := []ShipModule{
		{"hull", "cold_hull"}, {"tentacles", "steel"}, {"patches", "light"}, {"cockpit", "light"},
	}
	if len(spec.Modules) != len(want) {
		t.Fatalf("modules = %v, want %v", spec.Modules, want)
	}
	for i := range want {
		if spec.Modules[i] != want[i] {
			t.Errorf("modules[%d] = %v, want %v", i, spec.Modules[i], want[i])
		}
	}
	if len(spec.Warnings) != 0 {
		t.Errorf("warnings = %v, want пусто (все 4 модуля распознаны)", spec.Warnings)
	}
}

// TestParseSilhouetteColors — цвет корпуса по цветовым словам ТЗ (диагноз
// визуального аудита, п.3): холодные слова → cold_hull, тёплые → cream,
// фолбек — cream. «бледный» без «голубой» — НЕ холодный (deep_dwellers).
func TestParseSilhouetteColors(t *testing.T) {
	dc := loadShipDict(t)
	cases := []struct {
		color string
		want  string
	}{
		{"бледно-голубой", "cold_hull"},
		{"голубой", "cold_hull"},
		{"сине-зелёный", "cold_hull"},
		{"ледяной", "cold_hull"},
		{"иней-белый", "cold_hull"},
		{"белый", "cold_hull"},
		{"пепельно-серый", "cream"},
		{"серый", "cream"},
		{"расплавленный", "cream"},
		{"лава", "cream"},
		{"крем", "cream"},
		{"бледный", "cream"},
	}
	for _, c := range cases {
		sil := "капсула в плане: нос справа, корма слева; модули: корпус " + c.color + "; запас от краёв ~90 px"
		spec := ParseSilhouette(sil, dc)
		if len(spec.Modules) != 1 || spec.Modules[0].Type != "hull" || spec.Modules[0].Color != c.want {
			t.Errorf("корпус %q → %v, want hull/%s", c.color, spec.Modules, c.want)
		}
	}
}

// TestBuildShipPrompt1NoEngineForMachineless — раса без машин (blocked:
// machine/engine): промпт1 без «engine», с «trailing tendrils»; раса без
// блокировки машин — как раньше (диагноз визуального аудита, п.5).
func TestBuildShipPrompt1NoEngineForMachineless(t *testing.T) {
	dc := loadShipDict(t)
	rng := rand.New(rand.NewSource(1))
	machineless := config.ShipEntry{
		RaceName: "Аммиачники", Family: "F2",
		Texture:    "frosted ammonia ice hull",
		Silhouette: "капсула в плане: нос справа, корма слева; модули: корпус бледно-голубой; запас от краёв ~90 px",
		Blocked:    []string{"machine", "engine", "fire", "flame"},
	}
	spec := ParseSilhouette(machineless.Silhouette, dc)
	p := BuildShipPrompt1(rng, "ammonia", machineless, spec, dc, "")
	if strings.Contains(p, "engine at the LEFT rear") {
		t.Errorf("промпт1 содержит «engine at the LEFT rear» у расы без машин: %s", p)
	}
	if !strings.Contains(p, "trailing tendrils at the left rear") {
		t.Errorf("промпт1 не содержит «trailing tendrils at the left rear»: %s", p)
	}
	if !strings.Contains(p, "nose pointing FORWARD to the right") {
		t.Errorf("промпт1 не содержит нос-хвост: %s", p)
	}

	humans := config.ShipEntry{
		RaceName: "Люди", Family: "F1",
		Texture:    "paneled white-grey metal hull",
		Silhouette: "крыло-корпус в плане: нос справа, корма слева; модули: корпус крем; запас от краёв ~90 px",
		Blocked:    []string{"tentacle", "organic"},
	}
	spec2 := ParseSilhouette(humans.Silhouette, dc)
	p2 := BuildShipPrompt1(rng, "humans", humans, spec2, dc, "")
	if !strings.Contains(p2, "engine at the LEFT rear") {
		t.Errorf("промпт1 без блокировки машин не содержит «engine at the LEFT rear»: %s", p2)
	}
}

// TestShipDictColdTextures — плотные текстурные теги холодных рас (диагноз
// визуального аудита, п.6): теги есть в словаре (warmth=cold) и проходят
// blocked аммиачников (frost/ice/snow в их blocked нет).
func TestShipDictColdTextures(t *testing.T) {
	dc := loadShipDict(t)
	want := []string{"dense frost texture", "layered ice plates", "cracked rime", "frost crystal clusters"}
	for _, tag := range want {
		found := false
		for _, tx := range dc.Textures {
			if tx.Tag == tag {
				found = true
				if tx.Warmth != "cold" {
					t.Errorf("тег %q: warmth = %s, want cold", tag, tx.Warmth)
				}
			}
		}
		if !found {
			t.Errorf("тег %q не найден в ship_dict.textures", tag)
		}
	}
	ships, err := config.LoadShipsRaces("../../../config/art/ships.json", "../../../config/races.json")
	if err != nil {
		t.Fatalf("LoadShips: %v", err)
	}
	ammonia := ships["ammonia"]
	pool := filterShipTextures(dc, ammonia.Blocked, false)
	for _, tag := range want {
		if !containsStr(pool, tag) {
			t.Errorf("тег %q отфильтрован у аммиачников (blocked): %v", tag, pool)
		}
	}
}

// TestShipModuleInventory — инвентаризация модулей по всем 60 ТЗ (диагноз
// визуального аудита, п.4): прогон парсера по docs/gamedesign/races/ships/*.md,
// отчёт по расам с непокрытыми словами секции «модули:» (для доводки словаря).
// Не ошибка — отчёт (лог).
func TestShipModuleInventory(t *testing.T) {
	dc := loadShipDict(t)
	dir := "../../../docs/gamedesign/races/ships"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	racesWithGaps := 0
	total := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".md")
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", e.Name(), err)
		}
		lore, err := config.ParseShipSection(string(data))
		if err != nil {
			t.Logf("%s: %v", slug, err)
			continue
		}
		spec := ParseSilhouette(lore.Silhouette, dc)
		var uncovered []string
		for _, w := range spec.Warnings {
			if strings.HasPrefix(w, "неизвестный модуль:") {
				uncovered = append(uncovered, strings.TrimPrefix(w, "неизвестный модуль: "))
			}
		}
		if len(uncovered) > 0 {
			racesWithGaps++
			total += len(uncovered)
			t.Logf("%s: непокрытые модули: %v", slug, uncovered)
		}
	}
	t.Logf("рас с непокрытыми модулями: %d, всего непокрытых пар: %d (отчёт, не ошибка)", racesWithGaps, total)
}

// TestBuildShipPrompt1Concept — расовое концепт-слово в начале промпта1
// (ship_dict.concepts[slug], «корабль в образе …»); у расы без концепта —
// фолбек на форму (текущее поведение).
func TestBuildShipPrompt1Concept(t *testing.T) {
	dc := loadShipDict(t)
	rng := rand.New(rand.NewSource(1))
	ammonia := config.ShipEntry{
		RaceName: "Аммиачники", Family: "F2",
		Texture:    "frosted ammonia ice hull",
		Silhouette: "корпус-«капля» в плане: нос справа, корма слева; модули: корпус бледно-голубой; запас от краёв ~90 px",
		Blocked:    []string{"machine", "engine", "fire", "flame"},
	}
	spec := ParseSilhouette(ammonia.Silhouette, dc)
	p := BuildShipPrompt1(rng, "ammonia", ammonia, spec, dc, "")
	want := dc.Concepts["ammonia"]
	if !strings.HasPrefix(p, want+", ") {
		t.Errorf("промпт1 не начинается с концепта %q: %s", want, p)
	}
	// раса без концепта: фолбек на форму (как раньше)
	humans := config.ShipEntry{
		RaceName: "Люди", Family: "F1",
		Texture:    "paneled white-grey metal hull",
		Silhouette: "крыло-корпус в плане: нос справа, корма слева; модули: корпус крем; запас от краёв ~90 px",
	}
	spec2 := ParseSilhouette(humans.Silhouette, dc)
	p2 := BuildShipPrompt1(rng, "no_such_race", humans, spec2, dc, "")
	if !strings.HasPrefix(p2, "wing-shaped hull, ") {
		t.Errorf("промпт1 без концепта не начинается с формы: %s", p2)
	}
}

// TestShipDictConcepts — концепты рас (ТЗ п.4/п.5): покрытие всех 60 рас
// ships.json, каждый концепт непустой, ≤ 60 символов, без \n.
func TestShipDictConcepts(t *testing.T) {
	dc := loadShipDict(t)
	ships, err := config.LoadShipsRaces("../../../config/art/ships.json", "../../../config/races.json")
	if err != nil {
		t.Fatalf("LoadShips: %v", err)
	}
	for slug, c := range dc.Concepts {
		if strings.TrimSpace(c) == "" {
			t.Errorf("концепт расы %s пуст", slug)
		}
		if strings.Contains(c, "\n") {
			t.Errorf("концепт расы %s содержит перенос строки: %q", slug, c)
		}
		if len(c) > 60 {
			t.Errorf("концепт расы %s длиннее 60 символов: %q", slug, c)
		}
	}
	for slug := range ships {
		if dc.Concepts[slug] == "" {
			t.Errorf("раса %s не имеет концепта в ship_dict.concepts", slug)
		}
	}
}

// TestBuildShipPrompt1 — промпт этапа 1 людей: концепт крыла, деталь
// стабилизаторы, ориентационный хвост, без токенов blocked.
func TestBuildShipPrompt1(t *testing.T) {
	dc := loadShipDict(t)
	entry := config.ShipEntry{
		RaceName: "Люди", Family: "F1",
		Texture:    "paneled white-grey metal hull",
		Silhouette: "крыло-корпус в плане: нос справа, корма слева; крылья-стабилизаторы; модули: корпус крем, дюзы тёмные, кабина светлая; запас от краёв ~90 px",
		Blocked:    []string{"tentacle", "organic", "crystal", "pyramid", "obelisk", "bioluminescent", "alien"},
	}
	spec := ParseSilhouette(entry.Silhouette, dc)
	rng := rand.New(rand.NewSource(1))
	p := BuildShipPrompt1(rng, "humans", entry, spec, dc, "")
	for _, want := range []string{dc.Concepts["humans"], "stabilizer fins", "nose pointing FORWARD to the right", "engine at the LEFT rear", "on black background", "no text, no watermark"} {
		if !strings.Contains(p, want) {
			t.Errorf("промпт не содержит %q: %s", want, p)
		}
	}
	for _, tok := range entry.Blocked {
		if strings.Contains(p, tok) {
			t.Errorf("промпт содержит blocked-токен %q: %s", tok, p)
		}
	}
}

// TestBuildShipPrompt2 — этап 2: у людей (нет тёплого слова) нет тёплых тегов;
// у солевых (molten/orange/hot) тёплые теги есть; blocked_by фильтрует.
func TestBuildShipPrompt2(t *testing.T) {
	dc := loadShipDict(t)
	rng := rand.New(rand.NewSource(1))
	humans := config.ShipEntry{
		Texture: "paneled white-grey metal hull with ceramic heat shield tiles, riveted seams, navigation lights, subtle weathering, light blue cockpit glass, no organic shapes, no bioluminescence",
		Blocked: []string{"tentacle", "organic", "crystal", "pyramid", "obelisk", "bioluminescent", "alien"},
	}
	p := BuildShipPrompt2(rng, humans, dc, "")
	for _, warmTag := range []string{"ember glow beneath", "molten channels", "glowing orange veins", "hot shimmer", "warm amber light", "red-orange ember light", "glowing cracks", "soft warm inner glow"} {
		if strings.Contains(p, warmTag) {
			t.Errorf("у людей тёплый тег %q: %s", warmTag, p)
		}
	}
	// blocked_by: «crystalline facets» исключён (crystal в blocked)
	if strings.Contains(p, "crystalline facets") {
		t.Errorf("у людей тег crystalline facets (blocked_by crystal): %s", p)
	}
	// «dense mechanical texture» — blocked_by machine/mech, у людей нет → допустим
	if !strings.Contains(p, "paneled white-grey metal hull") {
		t.Errorf("texture расы не в промпте: %s", p)
	}

	saltfolk := config.ShipEntry{
		Texture: "white-pink crystallized salt hull with glowing orange veins, translucent molten salt channels, bubbling surface, hot shimmer, crystalline facets, no machines, no frost, no flames",
		Blocked: []string{"machine", "frost", "flame"},
	}
	p2 := BuildShipPrompt2(rng, saltfolk, dc, "")
	hasWarm := false
	for _, warmTag := range []string{"ember glow beneath", "molten channels", "glowing orange veins", "hot shimmer", "warm amber light", "red-orange ember light", "glowing cracks", "soft warm inner glow"} {
		if strings.Contains(p2, warmTag) {
			hasWarm = true
		}
	}
	if !hasWarm {
		t.Errorf("у солевых нет тёплых тегов (molten/orange/hot в texture): %s", p2)
	}
	// blocked_by: «dense mechanical texture» исключён (machine в blocked)
	if strings.Contains(p2, "dense mechanical texture") {
		t.Errorf("у солевых тег dense mechanical texture (blocked_by machine): %s", p2)
	}
}

// TestHasWarmMarker — отрицание «no warm» и ложные срабатывания.
func TestHasWarmMarker(t *testing.T) {
	dc := loadShipDict(t)
	cases := []struct {
		texture string
		want    bool
	}{
		{"cold engine glow, no bioluminescence", false}, // glow не маркер
		{"ceramic heat shield tiles", false},            // heat не маркер
		{"no warm colors, no mechanical parts", false},  // отрицание
		{"warm amber lit windows", true},
		{"faint amber inner glow", true},
		{"glowing orange veins, molten salt channels", true},
		{"dim warm glow", true},
		{"no warm colors but amber lights", true}, // amber не под отрицанием
	}
	for _, c := range cases {
		if got := hasWarmMarker(c.texture, dc.WarmMarkers); got != c.want {
			t.Errorf("hasWarmMarker(%q) = %v, want %v", c.texture, got, c.want)
		}
	}
}

// TestFilterShipTextures — гейт тёплых тегов жёсткий (фолбек не возвращает
// тёплые при warm=false).
func TestFilterShipTextures(t *testing.T) {
	dc := loadShipDict(t)
	// все теги заблокированы → фолбек на исходный пул, но тёплые не вернутся
	blocked := []string{"panel", "greebles", "rivets", "vents", "weathering", "gradients", "bubbling", "billowing", "crystalline", "translucent", "organic", "mineral", "smooth", "glassy", "bioluminescent", "porous", "woven", "layered", "metallic", "frost", "ice", "rim", "pale", "cyan", "cool", "icy", "ember", "molten", "orange", "hot", "amber", "red-orange", "cracks", "warm", "dense", "mechanical"}
	pool := filterShipTextures(dc, blocked, false)
	for _, tag := range pool {
		if textureWarmth(dc, tag) == "warm" {
			t.Errorf("warm=false, но тёплый тег %q в пуле (фолбек вернул тёплые)", tag)
		}
	}
	if len(pool) == 0 {
		t.Errorf("пул пуст после фолбека")
	}
}

// TestDeadBlockedTokens — мягкий кросс-конфиг-тест (спека §3.4/§8 критерий 4):
// мёртвые токены blocked по всем 60 расам — warning-отчёт, не ошибка.
func TestDeadBlockedTokens(t *testing.T) {
	dc := loadShipDict(t)
	ships, err := config.LoadShipsRaces("../../../config/art/ships.json", "../../../config/races.json")
	if err != nil {
		t.Fatalf("LoadShips: %v", err)
	}
	dead := DeadBlockedTokens(ships, dc)
	total := 0
	for slug, toks := range dead {
		total += len(toks)
		t.Logf("мёртвые токены %s: %v", slug, toks)
	}
	t.Logf("всего мёртвых токенов: %d (warning, не ошибка — токены писались до словарей)", total)
	// не ошибка: тест только отчитывается
}

// TestBuildShipTxt2ImgPrompt — рецепт 2026-09-21: {subject из космического
// пула}, {race.texture}, якорь ракурса, фон, якоря стиля, {tags}.
func TestBuildShipTxt2ImgPrompt(t *testing.T) {
	entry := config.ShipEntry{
		Texture: "paneled white-grey metal hull with ceramic heat shield tiles",
		Blocked: []string{"tentacle", "organic", "crystal"},
	}
	rng := rand.New(rand.NewSource(1))
	p := BuildShipTxt2ImgPrompt(rng, entry, "extra tag")
	for _, want := range []string{
		entry.Texture, ShipViewAnchor, ShipBackground, ShipStyleAnchors, "extra tag",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("промпт не содержит %q: %s", want, p)
		}
	}
	if !strings.HasPrefix(p, ShipSubjectPool[0]) && !hasAnySubject(p) {
		t.Errorf("промпт не начинается с космического субъекта: %s", p)
	}
	// морские/авиационные существительные в позитиве не используются
	for _, bad := range []string{"boat", "sailing ship", "airplane", "fighter jet"} {
		if strings.Contains(strings.ToLower(p), bad) {
			t.Errorf("промпт содержит морской/авиационный субъект %q: %s", bad, p)
		}
	}
}

// hasAnySubject — true, если промпт начинается с одного из космических
// субъектов пула.
func hasAnySubject(p string) bool {
	for _, s := range ShipSubjectPool {
		if strings.HasPrefix(p, s+", ") {
			return true
		}
	}
	return false
}

// TestBuildShipTxt2ImgPromptSubjects — субъект берётся из пула и варьируется
// от seed (не один и тот же на всех).
func TestBuildShipTxt2ImgPromptSubjects(t *testing.T) {
	entry := config.ShipEntry{Texture: "hull"}
	seen := map[string]bool{}
	for i := 0; i < 40; i++ {
		p := BuildShipTxt2ImgPrompt(rand.New(rand.NewSource(int64(i))), entry, "")
		ok := false
		for _, s := range ShipSubjectPool {
			if strings.HasPrefix(p, s+", ") {
				seen[s] = true
				ok = true
			}
		}
		if !ok {
			t.Fatalf("субъект не из пула: %s", p)
		}
	}
	if len(seen) < 3 {
		t.Errorf("разных субъектов = %d, want ≥ 3 (субъект варьируется)", len(seen))
	}
}

// TestShipNeg — негатив: станции/мусор + лодки/самолёты/вода + blocked расы.
func TestShipNeg(t *testing.T) {
	neg := ShipNeg([]string{"machine", "crystal"})
	for _, want := range []string{
		"space station", "ring", "torus", "front view", "symmetrical",
		"boat", "sailing ship", "mast", "water", "sea", "ocean",
		"airplane", "fighter jet", "propeller", "runway", "ground",
		"machine", "crystal",
	} {
		if !strings.Contains(neg, want) {
			t.Errorf("негатив не содержит %q: %s", want, neg)
		}
	}
	// без blocked — только два набора, без пустых хвостов
	if strings.HasSuffix(ShipNeg(nil), ", ") {
		t.Errorf("негатив без blocked кончается запятой: %s", ShipNeg(nil))
	}
}

// TestShipNegBackgroundCoastal — родные цвета Прибрежных (turquoise trim,
// amber windows) не запрещаются как «background»; вместо этого — сценово-
// квалифицированная форма (ground); неродной brown остаётся «brown background».
func TestShipNegBackgroundCoastal(t *testing.T) {
	texture := "salt-crusted coral and stone hull, brine-hardened organic plating, pale salt glints, warm amber lit windows, turquoise trim, wet reflective sheen, no machines, no fire"
	neg := ShipNegBackground(config.ShipEntry{Texture: texture})
	if strings.Contains(neg, "turquoise background") {
		t.Errorf("родной turquoise запрещён как background: %s", neg)
	}
	if !strings.Contains(neg, "turquoise ground") {
		t.Errorf("нет сценово-квалифицированной формы turquoise ground: %s", neg)
	}
	if !strings.Contains(neg, "brown background") {
		t.Errorf("неродной brown background отсутствует: %s", neg)
	}
}

// TestShipNegBackgroundMistfolk — родной cyan/teal Туманников не «background»,
// а сценово-квалифицированная форма; тёплые цвета (не родные) — «background».
func TestShipNegBackgroundMistfolk(t *testing.T) {
	texture := "translucent grey-blue fog envelope, dense ammonia mist inside, sail fins, soft cyan glow within the haze, faint teal core light, frost-rim ribs, no warm colors, no fire, no machines"
	neg := ShipNegBackground(config.ShipEntry{Texture: texture})
	for _, w := range []string{"cyan background", "teal background"} {
		if strings.Contains(neg, w) {
			t.Errorf("родной цвет запрещён как background (%q): %s", w, neg)
		}
	}
	if !strings.Contains(neg, "cyan ground") {
		t.Errorf("нет cyan ground: %s", neg)
	}
	if !strings.Contains(neg, "yellow background") {
		t.Errorf("нет yellow background (тёплый цвет не родной): %s", neg)
	}
}

// TestShipNegBackgroundYellowBody — раса с родным жёлтым корпусом (nether/
// sulfur_swarms): жёлтый фон идёт сценово-квалифицированно, не «yellow background».
func TestShipNegBackgroundYellowBody(t *testing.T) {
	texture := "golden sulfur rock hull with dark veins, terraced garden ledges, glowing red veins, dim ember glow, slick mucus surfaces, no frost, no water, no machines, no flames"
	neg := ShipNegBackground(config.ShipEntry{Texture: texture})
	if strings.Contains(neg, "yellow background") || strings.Contains(neg, "amber background") {
		t.Errorf("родной жёлтый запрещён как background: %s", neg)
	}
	for _, w := range []string{"yellow ground", "yellow environment", "yellow horizon"} {
		if !strings.Contains(neg, w) {
			t.Errorf("нет %q: %s", w, neg)
		}
	}
}

// TestShipNegBackgroundNoNativeYellow — у расы без родного жёлтого запрет
// «yellow background» присутствует.
func TestShipNegBackgroundNoNativeYellow(t *testing.T) {
	texture := "paneled white-grey metal hull with ceramic heat shield tiles, riveted seams, navigation lights, light blue cockpit glass"
	neg := ShipNegBackground(config.ShipEntry{Texture: texture})
	for _, w := range []string{"yellow background", "amber background", "brown background"} {
		if !strings.Contains(neg, w) {
			t.Errorf("нет %q: %s", w, neg)
		}
	}
}

// TestShipNegBackgroundWordBoundary — цвет ищется как слово: «scattered»/
// «blurred»/«armored» не делают красный родным.
func TestShipNegBackgroundWordBoundary(t *testing.T) {
	texture := "scattered armor plates, blurred weathered surface, tiered layers"
	neg := ShipNegBackground(config.ShipEntry{Texture: texture})
	if !strings.Contains(neg, "red background") {
		t.Errorf("ложный родной red (подстрочный матч): %s", neg)
	}
}

// TestShipBackgroundBlack — рецепт 2026-09-22 (возврат к чёрному фону):
// ShipBackground — плоский чёрный фон, без magenta (пунцовый заливал корпус);
// негатив фона не должен спорить с позитивным чёрным.
func TestShipBackgroundBlack(t *testing.T) {
	if !strings.Contains(ShipBackground, "pure flat black background") {
		t.Errorf("ShipBackground без чёрного фона: %s", ShipBackground)
	}
	if strings.Contains(strings.ToLower(ShipBackground), "magenta") {
		t.Errorf("ShipBackground всё ещё magenta: %s", ShipBackground)
	}
	neg := ShipNeg(nil)
	for _, bad := range []string{"black background", "dark background", "grey background", "gray background"} {
		if strings.Contains(neg, bad) {
			t.Errorf("негатив %q конфликтует с чёрным фоном: %s", bad, neg)
		}
	}
	for _, want := range []string{"gradient background", "studio backdrop", "environment", "ground", "floor", "terrain", "landscape", "horizon", "rocky ground"} {
		if !strings.Contains(neg, want) {
			t.Errorf("негатив не содержит %q: %s", want, neg)
		}
	}
}

// TestShipNegBackgroundMagenta — magenta/pink теперь безопасны как фон-негатив
// (хромакей убран): неродной magenta → «magenta background»; родной pink
// (солевые, white-pink) → сценово-квалифицированно, не «pink background».
func TestShipNegBackgroundMagenta(t *testing.T) {
	plain := ShipNegBackground(config.ShipEntry{Texture: "plain hull"})
	for _, want := range []string{"magenta background", "pink background"} {
		if !strings.Contains(plain, want) {
			t.Errorf("нет %q для неродного цвета: %s", want, plain)
		}
	}
	salt := ShipNegBackground(config.ShipEntry{Texture: "white-pink crystallized salt hull with glowing orange veins"})
	for _, bad := range []string{"magenta background", "pink background"} {
		if strings.Contains(salt, bad) {
			t.Errorf("родной pink запрещён как background (%q): %s", bad, salt)
		}
	}
	for _, want := range []string{"magenta ground", "pink ground", "pink environment", "pink horizon"} {
		if !strings.Contains(salt, want) {
			t.Errorf("нет сценово-квалифицированной формы %q: %s", want, salt)
		}
	}
}

// TestShipNegBackgroundNoDuplicateTokens — токены фона-негатива уникальны:
// pink не должен дублироваться при не-родном цвете (сгруппирован с magenta).
func TestShipNegBackgroundNoDuplicateTokens(t *testing.T) {
	neg := ShipNegBackground(config.ShipEntry{Texture: "plain hull"})
	seen := map[string]bool{}
	for _, part := range strings.Split(neg, ", ") {
		if seen[part] {
			t.Errorf("дубль токена %q в негативе: %s", part, neg)
		}
		seen[part] = true
	}
}

// TestShipNegRace — ShipNegRace = ShipNeg(blocked) + расо-зависимый негатив фона.
func TestShipNegRace(t *testing.T) {
	neg := ShipNegRace(config.ShipEntry{Texture: "plain hull", Blocked: []string{"machine"}})
	for _, want := range []string{"boat", "space station", "machine", "brown background"} {
		if !strings.Contains(neg, want) {
			t.Errorf("ShipNegRace не содержит %q: %s", want, neg)
		}
	}
}

// TestBuildShipHiResPrompt — промпт этапа Hi-Res = промпт txt2img + хвост.
func TestBuildShipHiResPrompt(t *testing.T) {
	p := BuildShipHiResPrompt("base prompt")
	if !strings.HasPrefix(p, "base prompt, ") || !strings.Contains(p, ShipHiresTail) {
		t.Errorf("промпт Hi-Res = %q", p)
	}
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}