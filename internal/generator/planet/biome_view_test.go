// internal/generator/planet/biome_view_test.go
//
// Тесты рецепта вида биома (спека 2026-09-23 «Мир прогулки»): синхронизация
// заводского сида и рабочего файла (§10 п.17), правила слияния «семейство ⊕
// дельта» (§2.3), нефатальная валидация и диагностика (§2.8), реестр
// примитивов (§2.1), рецепты трёх образцов (§4) и фолбэк без рецепта (§2.3).
package planet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==================== СИНХРОНИЗАЦИЯ СИДА (§10 п.17) ====================

// TestBiomeViewSeedSynced — view-часть заводского сида (go:embed) и рабочего
// файла config/biome_catalog.json совпадает (§2.7): рецепт — часть справочника,
// расхождение копий недопустимо. Сравнение семантическое (формат не важен).
func TestBiomeViewSeedSynced(t *testing.T) {
	seed := GetBiomeCatalog()
	require.NotNil(t, seed)

	path := filepath.Join("..", "..", "..", "config", "biome_catalog.json")
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "рабочий файл справочника должен существовать")
	var work BiomeCatalog
	require.NoError(t, json.Unmarshal(raw, &work))

	assert.True(t, reflect.DeepEqual(seed.ViewFamilies, work.ViewFamilies),
		"view_families: сид и рабочий файл обязаны совпадать (§2.7)")
	assert.True(t, reflect.DeepEqual(seed.ViewPrimitives, work.ViewPrimitives),
		"view_primitives: сид и рабочий файл обязаны совпадать (§2.7)")

	workByID := map[string]BiomeDef{}
	for _, b := range work.Biomes {
		workByID[b.ID] = b
	}
	withView := 0
	for _, b := range seed.Biomes {
		wb, ok := workByID[b.ID]
		require.True(t, ok, "биом %q есть в сиде, но не в рабочем файле", b.ID)
		assert.True(t, reflect.DeepEqual(b.View, wb.View), "биом %q: view расходится сид↔файл", b.ID)
		if len(b.View) > 0 {
			withView++
		}
	}
	assert.GreaterOrEqual(t, withView, 4, "три образца + лес — рецепты обязаны быть в обеих копиях")
}

// ==================== ПРАВИЛА СЛИЯНИЯ (§2.3) ====================

func TestBiomeViewMergeRules(t *testing.T) {
	cat := &BiomeCatalog{
		ViewFamilies: []map[string]any{
			{
				"id":   "семейство",
				"name": "Семейство",
				"relief": map[string]any{
					"ridge":  1.0,
					"layers": []any{map[string]any{"prim": "wave"}},
				},
				"palette":      map[string]any{"base": "#111111", "dark": "#222222"},
				"placement":    map[string]any{"mode": "clustered", "tile": float64(200)},
				"life_density": 0.2,
			},
		},
		ViewPrimitives: []ViewPrimitive{
			{ID: "wave", Kind: "relief1d"},
			{ID: "dome", Kind: "relief1d"},
			{ID: "crest", Kind: "relief1d"},
			{ID: "clustered", Kind: "placement"},
		},
	}
	b := &BiomeDef{
		ID: "биом",
		View: map[string]any{
			"family": "семейство",
			// скаляр — перекрывает пресет.
			"life_density": 0.9,
			// карта — слияние по ключам, отсутствующий ключ из пресета.
			"palette": map[string]any{"base": "#aaaaaa"},
			// список — замена целиком.
			"relief": map[string]any{
				"layers": []any{map[string]any{"prim": "dome"}, map[string]any{"prim": "crest"}},
			},
		},
	}
	view, errs, warns := cat.resolveBiomeView(b)
	require.Empty(t, errs)
	require.Empty(t, warns)

	assert.InDelta(t, 0.9, view["life_density"], 1e-9, "скаляр: дельта перекрывает пресет")
	assert.Equal(t, "#aaaaaa", view["palette"].(map[string]any)["base"], "карта: дельта поверх")
	assert.Equal(t, "#222222", view["palette"].(map[string]any)["dark"], "карта: отсутствующий ключ из пресета")
	assert.Equal(t, "clustered", view["placement"].(map[string]any)["mode"], "карта: нетронутый ключ пресета")
	layers := view["relief"].(map[string]any)["layers"].([]any)
	require.Len(t, layers, 2, "список: замена целиком, а не склейка")
	assert.Equal(t, "dome", layers[0].(map[string]any)["prim"])
	assert.InDelta(t, 1.0, view["relief"].(map[string]any)["ridge"], 1e-9, "скаляр карты из пресета сохранён")
}

// ==================== НЕФАТАЛЬНАЯ ВАЛИДАЦИЯ И ДИАГНОСТИКА (§2.8) ====================

func TestBiomeViewValidateNonFatal(t *testing.T) {
	cat := *GetBiomeCatalog()
	cat.Biomes = append([]BiomeDef{}, cat.Biomes...)

	// Битый рецепт: неизвестное семейство + битый примитив + не-hex цвет +
	// перевёрнутый диапазон.
	cat.Biomes[0].ID = "битый_вид"
	cat.Biomes[0].View = map[string]any{
		"family":  "нет_такого",
		"palette": map[string]any{"base": "не-цвет"},
		"relief": map[string]any{
			"layers": []any{
				map[string]any{"prim": "нет_такого_примитива"},
				map[string]any{"prim": "wave", "lambda": []any{float64(100), float64(10)}},
			},
		},
	}

	// Каталог структурно валиден: вид НЕ входит в Validate() (§2.8 п.1).
	require.NoError(t, cat.Validate(), "ошибка вида не должна ронять каталог")

	view, source := cat.ResolveBiomeView("битый_вид")
	assert.Nil(t, view)
	assert.Equal(t, "fallback", source, "битый рецепт → фолбэк одного биома (§2.8 п.3)")

	d := cat.ViewDiagnostics()
	require.NotEmpty(t, d.Errors)
	fields := map[string]bool{}
	for _, e := range d.Errors {
		assert.Equal(t, "битый_вид", e.Biome, "диагностика называет биом")
		fields[e.Field] = true
	}
	assert.True(t, fields["family"], "ошибка неизвестного семейства названа полем family")
	assert.True(t, fields["palette.base"], "ошибка цвета названа полем palette.base")
}

func TestBiomeViewBadPrimKind(t *testing.T) {
	cat := *GetBiomeCatalog()
	cat.Biomes = append([]BiomeDef{}, cat.Biomes...)
	cat.Biomes[0].ID = "вид_с_видом"
	// decor ссылается на relief-примитив — вид блока не совпадает.
	cat.Biomes[0].View = map[string]any{
		"decor": []any{map[string]any{"prim": "wave"}},
	}
	_, source := cat.ResolveBiomeView("вид_с_видом")
	assert.Equal(t, "fallback", source)
	d := cat.ViewDiagnostics()
	require.NotEmpty(t, d.Errors)
	assert.Contains(t, d.Errors[0].Reason, "ожидался")
}

// ==================== РЕЕСТР ПРИМИТИВОВ (§2.1) ====================

func TestBiomeViewPrimitivesRegistry(t *testing.T) {
	cat := GetBiomeCatalog()
	kinds := cat.primKindIndex()
	require.Len(t, kinds, 33, "8 рельефных + 16 декора + лиана (hang) + 3 размещения + 5 поздних 2D")
	assert.Equal(t, "relief1d", kinds["wave"])
	assert.Equal(t, "decor", kinds["cactus"])
	assert.Equal(t, "placement", kinds["clustered"])
	assert.Equal(t, "relief2d", kinds["overhang"])
	assert.Equal(t, "relief2d", kinds["crater"], "crater зарегистрирован в Э5.2 (§3.1)")

	// Все примитивы в пресетах семейств ссылаются на известный id нужного вида.
	for _, f := range cat.ViewFamilies {
		id, _ := f["id"].(string)
		if relief, ok := f["relief"].(map[string]any); ok {
			for _, l := range asMapList(relief["layers"]) {
				prim, _ := l["prim"].(string)
				assert.Equal(t, "relief1d", kinds[prim], "семейство %q: слой %q", id, prim)
			}
		}
		for _, d := range asMapList(f["decor"]) {
			prim, _ := d["prim"].(string)
			assert.Equal(t, "decor", kinds[prim], "семейство %q: декор %q", id, prim)
		}
	}
}

// ==================== РЕЦЕПТЫ ОБРАЗЦОВ (§4) ====================

func TestBiomeViewSamplesResolve(t *testing.T) {
	cat := GetBiomeCatalog()
	for _, id := range []string{"пески_пустыни", "джунгли", "леса", "горы"} {
		view, source := cat.ResolveBiomeView(id)
		require.Equal(t, "catalog", source, "образец %q должен резолвиться из справочника", id)
		require.NotNil(t, view)
		def := cat.BiomeByID(id)
		require.NotNil(t, def)
		base, ok := nestedString(view, "palette", "base")
		require.True(t, ok, "%q: палитра обязана нести base", id)
		assert.Equal(t, def.Color, base, "%q: palette.base == color (источник истины — color, §3.4)", id)
		assert.NotEmpty(t, def.Color, "%q: color заполнен (§1 п.8)", id)
	}

	// Пески пустыни: ни одного дерева в декорe (§4.1, чинит А5).
	desert, _ := cat.ResolveBiomeView("пески_пустыни")
	for _, d := range asMapList(desert["decor"]) {
		assert.NotEqual(t, "tree", d["prim"], "в пустыне не бывает деревьев")
	}

	// Джунгли: дерево и лианы (§4.2).
	jungle, _ := cat.ResolveBiomeView("джунгли")
	prims := map[string]bool{}
	for _, d := range asMapList(jungle["decor"]) {
		prims[d["prim"].(string)] = true
	}
	assert.True(t, prims["tree"], "джунгли: крона — доминанта")
	assert.NotEmpty(t, asMapList(jungle["hang"]), "джунгли: лианы (hang)")

	// Горы: снеговая линия и профильные слои (§4.3).
	mountains, _ := cat.ResolveBiomeView("горы")
	relief := mountains["relief"].(map[string]any)
	require.NotNil(t, mountains["snowLine"], "горы: снеговая линия (§4.3)")
	require.Len(t, asMapList(relief["layers"]), 4, "горы: crest+spike+step+fan")
}

// ==================== БЮДЖЕТ ВЕРТИКАЛИ (§6 п.6, §2.8 п.2) ====================

// Объявленный профиль не должен выходить за верх растра чанка (иначе земля
// твёрдая, но не нарисована). Образцы бюджет проходят, заведомо огромный
// рельеф — ошибка вида (фолбэк биома).
func TestBiomeViewVerticalBudget(t *testing.T) {
	real := GetBiomeCatalog()
	for _, id := range []string{"горы", "пески_пустыни", "джунгли", "леса"} {
		v, source := real.ResolveBiomeView(id)
		require.Equal(t, "catalog", source, "образец %q обязан резолвиться", id)
		top, ok := declaredReliefTop(v)
		require.True(t, ok)
		assert.GreaterOrEqual(t, top, viewBaseY-viewChunkTopMargin+viewTopSafety, "%s: объявленный профиль в бюджете", id)
	}

	cat := *real
	cat.Biomes = append([]BiomeDef{}, real.Biomes...)
	cat.Biomes[0].ID = "гигантский_рельеф"
	cat.Biomes[0].View = map[string]any{
		"relief": map[string]any{
			"ridge": 1.0,
			"layers": []any{
				map[string]any{"prim": "crest", "amp": []any{float64(5000), float64(9000)}},
			},
		},
	}
	_, source := cat.ResolveBiomeView("гигантский_рельеф")
	assert.Equal(t, "fallback", source, "превышение бюджета → фолбэк биома")

	d := cat.ViewDiagnostics()
	found := false
	for _, e := range d.Errors {
		if e.Field == "relief" {
			found = true
			assert.Equal(t, "гигантский_рельеф", e.Biome)
		}
	}
	assert.True(t, found, "превышение бюджета названо полем relief")
}

// ==================== 2D-ФОРМЫ Э5.2 (§3.1/§3.5) ====================

// TestBiomeViewFormsResolve — дельты `горы`/`каменные_пустоши` резолвятся из
// справочника, `relief.forms[].prim` — вид `relief2d`, бюджет с формами проходит
// (сумма аддитивных несущих, `relief.scale` к формам не применяется), а битый
// `forms` уводит РОВНО один биом в фолбэк (нефатально, §3.3/§B.1.2).
func TestBiomeViewFormsResolve(t *testing.T) {
	cat := GetBiomeCatalog()
	kinds := cat.primKindIndex()
	for _, id := range []string{"горы", "каменные_пустоши"} {
		view, source := cat.ResolveBiomeView(id)
		require.Equal(t, "catalog", source, "биом %q: рецепт с формами обязан резолвиться", id)
		relief, ok := view["relief"].(map[string]any)
		require.True(t, ok, "%q: relief", id)
		forms := asMapList(relief["forms"])
		require.NotEmpty(t, forms, "%q: relief.forms (Э5.2, §3.5)", id)
		for i, f := range forms {
			prim, _ := f["prim"].(string)
			assert.Equal(t, "relief2d", kinds[prim], "%q: forms[%d].prim=%q — 2D-форма (§3.1)", id, i, prim)
		}
		top, ok := declaredReliefTop(view)
		require.True(t, ok)
		assert.GreaterOrEqual(t, top, viewBaseY-viewChunkTopMargin+viewTopSafety,
			"%q: объявленный профиль с формами в бюджете (top=%.1f)", id, top)
	}
	// Целевые операторы Э5.2: природная арка arch=1 (горы) и трещина crack2d.
	mountains, _ := cat.ResolveBiomeView("горы")
	assert.NotNil(t, formByPrim(mountains, "overhang", 1), "горы: природная арка overhang arch=1")
	badlands, _ := cat.ResolveBiomeView("каменные_пустоши")
	assert.NotNil(t, formByPrim(badlands, "crack2d", 0), "каменные_пустоши: сквозная трещина crack2d")

	// Битый forms (relief1d вместо relief2d) — ровно один биом в фолбэк.
	mut := *cat
	mut.Biomes = append([]BiomeDef{}, cat.Biomes...)
	bi := -1
	for i := range mut.Biomes {
		if mut.Biomes[i].ID == "горы" {
			bi = i
			break
		}
	}
	require.GreaterOrEqual(t, bi, 0)
	mut.Biomes[bi].View = map[string]any{
		"family": "горные",
		"relief": map[string]any{"forms": []any{map[string]any{"prim": "wave"}}},
	}
	_, source := mut.ResolveBiomeView("горы")
	assert.Equal(t, "fallback", source, "битый forms уводит биом в фолбэк (§3.3)")
	other, sOther := mut.ResolveBiomeView("каменные_пустоши")
	require.Equal(t, "catalog", sOther, "битый forms не каскадит на соседний биом")
	require.NotNil(t, other)
}

// TestBiomeViewFormsOpeningWarning — `opening` ≤ роста игрока + запас даёт
// ПРЕДУПРЕЖДЕНИЕ (рецепт применяется, §3.3/M8), а не ошибку; у `arch=0` нет
// `opening` — предупреждения нет. Бюджет форм — СУММА аддитивных несущих
// (`arch=1` → opening + h, `arch=0` → h; `crater` → rim), `relief.scale` к
// формам НЕ применяется (§3.3/§5 п.6), `crack2d` вверх не поднимает.
func TestBiomeViewFormsOpeningWarning(t *testing.T) {
	cat := *GetBiomeCatalog()
	cat.Biomes = append([]BiomeDef{}, cat.Biomes...)
	cat.Biomes[0].ID = "форма_просвет"
	cat.Biomes[0].View = map[string]any{
		"relief": map[string]any{"forms": []any{
			map[string]any{"prim": "overhang", "arch": float64(1), "h": float64(30), "opening": float64(10)},
		}},
	}
	view, errs, warns := cat.resolveBiomeView(&cat.Biomes[0])
	require.Empty(t, errs)
	require.Len(t, warns, 1, "opening ниже роста игрока + запас — предупреждение (§3.3)")
	assert.Contains(t, warns[0].Field, "opening")
	assert.NotNil(t, view, "предупреждение нефатально — рецепт применяется")

	// arch=0 без `opening` — предупреждений нет (просвет производен от рельефа).
	cat.Biomes[0].View = map[string]any{
		"relief": map[string]any{"forms": []any{
			map[string]any{"prim": "overhang", "arch": float64(0), "h": float64(30), "reach": float64(50)},
		}},
	}
	_, errs2, warns2 := cat.resolveBiomeView(&cat.Biomes[0])
	require.Empty(t, errs2)
	require.Empty(t, warns2)

	// Бюджет: arch=1 → opening + h, crater → rim, crack2d → 0; сумма без scale.
	view3 := map[string]any{"relief": map[string]any{
		"scale": float64(0.5),
		"forms": []any{
			map[string]any{"prim": "overhang", "arch": float64(1), "h": []any{float64(20), float64(50)}, "opening": []any{float64(80), float64(150)}},
			map[string]any{"prim": "crater", "rim": float64(25)},
			map[string]any{"prim": "crack2d", "w": float64(30), "depth": float64(200)},
		},
	}}
	top, ok := declaredReliefTop(view3)
	require.True(t, ok)
	// rise 1D = 115·0 + 40·(1−0.7·0) = 40; ·scale 0.5 = 20; формы = (150+50)+25 = 225.
	assert.InDelta(t, viewBaseY-20-225, top, 1e-9,
		"бюджет форм — сумма аддитивных, scale к формам не применяется")
}

// TestBiomeViewFormsDefaultAmp — форма без явных `h`/`opening` берёт ДЕФОЛТЫ
// клиента (`_collectFormIntervals`, surface_world.js: h → 20, opening → 60),
// иначе серверный бюджет посчитал бы такую форму за 0, а клиент нарисовал бы её
// с ненулевой амплитудой (forward-compat, §3.3/§5 п.6).
func TestBiomeViewFormsDefaultAmp(t *testing.T) {
	arch1 := map[string]any{"prim": "overhang", "arch": float64(1)}
	assert.InDelta(t, 60+20, formUpAmp(arch1), 1e-9, "arch=1 без h/opening: opening 60 + h 20")

	arch0 := map[string]any{"prim": "overhang", "arch": float64(0)}
	assert.InDelta(t, 20, formUpAmp(arch0), 1e-9, "arch=0 без h: h 20")

	zero := map[string]any{"prim": "overhang", "arch": float64(0), "h": float64(0)}
	assert.InDelta(t, 0, formUpAmp(zero), 1e-9, "явный h=0 остаётся нулём (не дефолт)")
}

// formByPrim — первая запись relief.forms с данным prim; при arch != 0 — с этим
// arch (0 = arch не важен: crack2d/шляпа/любой arch=0).
func formByPrim(view map[string]any, prim string, arch float64) map[string]any {
	relief, _ := view["relief"].(map[string]any)
	for _, f := range asMapList(relief["forms"]) {
		if p, _ := f["prim"].(string); p != prim {
			continue
		}
		if arch == 0 || viewNum(f["arch"], 0) == arch {
			return f
		}
	}
	return nil
}

// ==================== ФОЛБЭК БЕЗ РЕЦЕПТА (§2.3) ====================

func TestBiomeViewFallbackNoRecipe(t *testing.T) {
	cat := GetBiomeCatalog()
	// ЧК4 (крио) раскатан — рецепта по-прежнему нет у водных биомов;
	// берём `океаны` как стабильный пример.
	view, source := cat.ResolveBiomeView("океаны")
	assert.Nil(t, view, "биом без рецепта — вид не отдаётся (клиент по FORMATIONS)")
	assert.Equal(t, "fallback", source)

	d := cat.ViewDiagnostics()
	assert.Contains(t, d.Missing, "океаны", "биомы без рецепта видны в диагностике (§2.7)")
	assert.Empty(t, d.Errors, "в заводском справочнике ошибок вида нет")
}

// ==================== РЕЦЕПТЫ ЧК3 — ВУЛКАНИЗМ (§4.7) ====================

// TestBiomeViewVolcanicResolve — 7 дельт вулканизма (§4.7.4) резолвятся из
// справочника, палитра совпадает с color (0 предупреждений, §4.7.9 п.4), а
// новые нормативные параметры движка (§4.7.12) присутствуют в данных.
func TestBiomeViewVolcanicResolve(t *testing.T) {
	cat := GetBiomeCatalog()
	d := cat.ViewDiagnostics()
	require.Empty(t, d.Errors, "ЧК3: ошибок вида нет")
	require.Empty(t, d.Warnings, "ЧК3: предупреждений вида нет (palette.base == color)")

	for _, id := range []string{
		"лавовые_поля", "вулканические_поля", "обсидиановые_поля", "серные_поля",
		"магмовый_океан", "венерианские_плоскогорья", "криовулканические_поля",
	} {
		view, source := cat.ResolveBiomeView(id)
		require.Equal(t, "catalog", source, "биом %q: рецепт обязан резолвиться", id)
		require.NotNil(t, view)
		def := cat.BiomeByID(id)
		require.NotNil(t, def)
		base, ok := nestedString(view, "palette", "base")
		require.True(t, ok, "%q: палитра обязана нести base", id)
		assert.Equal(t, def.Color, base, "%q: palette.base == color (§4.7.2)", id)
	}

	// В1: flow читает lobes/slope/levees (канатные потоки, §4.7.12).
	lav, _ := cat.ResolveBiomeView("лавовые_поля")
	flow := reliefLayer(lav, "flow")
	require.NotNil(t, flow, "лавовые_поля: flow-язык")
	assert.NotNil(t, flow["lobes"], "flow.lobes")
	assert.NotNil(t, flow["slope"], "flow.slope")
	assert.Equal(t, 0.25, flow["levees"], "flow.levees")

	// В3: обсидиан — матовые кристаллы (cluster + glow:false).
	obs, _ := cat.ResolveBiomeView("обсидиановые_поля")
	spl := reliefLayer(obs, "spike")
	require.NotNil(t, spl, "обсидиановые_поля: spike")
	assert.Equal(t, true, spl["cluster"], "spike.cluster (§4.7.12)")
	assert.Equal(t, false, crystalGlow(obs), "обсидиан: crystal.glow false (матовый)")

	// В2/В6: fan читает w (переключатель детального конуса, §4.7.13).
	for _, id := range []string{"вулканические_поля", "венерианские_плоскогорья"} {
		v, _ := cat.ResolveBiomeView(id)
		fan := reliefLayer(v, "fan")
		require.NotNil(t, fan, "%q: fan", id)
		assert.NotNil(t, fan["w"], "%q: fan.w задан", id)
		assert.NotNil(t, fan["roughness"], "%q: fan.roughness при w", id)
	}

	// В6: снег отключён, горизонт — свой (2 пояса).
	ven, _ := cat.ResolveBiomeView("венерианские_плоскогорья")
	assert.Equal(t, 0.0, ven["snowLine"], "венерианские: snowLine 0 (§4.7.4)")
	hz, ok := ven["horizon"].(map[string]any)
	require.True(t, ok, "венерианские: горизонт в дельте")
	assert.Len(t, asMapList(hz["layers"]), 2, "венерианские: 2 пояса горизонта")

	// Пресет `лавовые` получил горизонт (вставка B, §4.7.1) — наследуют все 6.
	cryo, _ := cat.ResolveBiomeView("криовулканические_поля")
	hz2, ok := cryo["horizon"].(map[string]any)
	require.True(t, ok, "лавовые: горизонт пресета")
	assert.Len(t, asMapList(hz2["layers"]), 2, "лавовые: 2 пояса")
}

// ==================== РЕЦЕПТЫ ЧК4 — КРИО (§4.8) ====================

// TestBiomeViewCryoResolve — 5 дельт крио (§4.8.4) резолвятся из справочника,
// палитра совпадает с color (0 предупреждений, §4.8.9 п.4), горизонт `ледяные`
// (вставка B, §4.8.1) наследуют 4 ледяных биома, а отложенные параметры
// движка (§4.7.12) присутствуют в данных.
func TestBiomeViewCryoResolve(t *testing.T) {
	cat := GetBiomeCatalog()
	d := cat.ViewDiagnostics()
	require.Empty(t, d.Errors, "ЧК4: ошибок вида нет")
	require.Empty(t, d.Warnings, "ЧК4: предупреждений вида нет (palette.base == color)")

	for _, id := range []string{
		"ледники", "мёрзлые_газы", "сухой_лёд", "инеевые_рощи", "азотно-ледяная_тундра",
	} {
		view, source := cat.ResolveBiomeView(id)
		require.Equal(t, "catalog", source, "биом %q: рецепт обязан резолвиться", id)
		require.NotNil(t, view)
		def := cat.BiomeByID(id)
		require.NotNil(t, def)
		base, ok := nestedString(view, "palette", "base")
		require.True(t, ok, "%q: палитра обязана нести base", id)
		assert.Equal(t, def.Color, base, "%q: palette.base == color (§4.8.2)", id)
	}

	// Горизонт пресета `ледяные` (вставка B, §4.8.1): 2 пояса у 4 ледяных биомов.
	for _, id := range []string{"ледники", "мёрзлые_газы", "сухой_лёд", "инеевые_рощи"} {
		v, _ := cat.ResolveBiomeView(id)
		hz, ok := v["horizon"].(map[string]any)
		require.True(t, ok, "%q: горизонт пресета `ледяные`", id)
		assert.Len(t, asMapList(hz["layers"]), 2, "%q: 2 пояса горизонта", id)
	}

	// `азотно-ледяная_тундра` — семейство `травяные`, без зелёной травы/кустов
	// (§4.8.9 п.10); горизонт — семейный `травяные` (2 пояса).
	tundra, _ := cat.ResolveBiomeView("азотно-ледяная_тундра")
	assert.Equal(t, "травяные", tundra["family"], "тундра: семейство `травяные` (§4.8.1)")
	for _, dec := range asMapList(tundra["decor"]) {
		assert.NotContains(t, []any{"grass", "bush"}, dec["prim"], "тундра: без травы/кустов")
	}
	hzT, ok := tundra["horizon"].(map[string]any)
	require.True(t, ok, "тундра: горизонт семейства `травяные`")
	assert.Len(t, asMapList(hzT["layers"]), 2, "тундра: 2 пояса горизонта")

	// В1: flow читает lobes/slope/levees (языки льда, §4.7.12).
	led, _ := cat.ResolveBiomeView("ледники")
	flow := reliefLayer(led, "flow")
	require.NotNil(t, flow, "ледники: flow-язык")
	assert.NotNil(t, flow["lobes"], "flow.lobes")
	assert.NotNil(t, flow["slope"], "flow.slope")
	assert.Equal(t, 0.2, flow["levees"], "flow.levees")

	// В5: инеевые рощи — иглы-«стволы» кустами (spike.cluster, §4.8.4).
	roshcha, _ := cat.ResolveBiomeView("инеевые_рощи")
	spl := reliefLayer(roshcha, "spike")
	require.NotNil(t, spl, "инеевые_рощи: spike")
	assert.Equal(t, true, spl["cluster"], "spike.cluster (§4.7.12)")
	assert.Equal(t, true, crystalGlow(roshcha), "инеевые_рощи: crystal.glow (§4.8.9 п.11)")
}

// ==================== РЕЦЕПТЫ ЧК5 — ЭКЗОТИКА (§4.9) ====================

// TestBiomeViewExoticResolve — 10 дельт экзотики (§4.9.4, вставка A) резолвятся
// из справочника, палитра совпадает с color (0 предупреждений, §4.9.9 п.4),
// горизонт пресета `экзотика` (вставка B, §4.9.1) присутствует у 4 биомов
// категории, а у 6 override-биомов свой; `crystal_tree` применён ровно у 2 рощ.
func TestBiomeViewExoticResolve(t *testing.T) {
	cat := GetBiomeCatalog()
	d := cat.ViewDiagnostics()
	require.Empty(t, d.Errors, "ЧК5: ошибок вида нет")
	require.Empty(t, d.Warnings, "ЧК5: предупреждений вида нет (palette.base == color)")

	exotic := []string{
		"кристальные_рощи", "кремниевые_рощи", "металлические_щетинные_поля",
		"карбидно-алмазные_заросли", "струнные_рощи", "радиационные_пустоши",
		"терминаторная_зона", "пружинная_тундра", "химический_иней",
		"пещерный_мир_с_потолком",
	}
	for _, id := range exotic {
		view, source := cat.ResolveBiomeView(id)
		require.Equal(t, "catalog", source, "биом %q: рецепт обязан резолвиться", id)
		require.NotNil(t, view)
		def := cat.BiomeByID(id)
		require.NotNil(t, def)
		base, ok := nestedString(view, "palette", "base")
		require.True(t, ok, "%q: палитра обязана нести base", id)
		assert.Equal(t, def.Color, base, "%q: palette.base == color (§4.9.2)", id)
	}

	// Реестр: `crystal_tree` (31 → 32, §4.9.4 C) + `crater` (32 → 33, Э5.2 §3.1).
	kinds := cat.primKindIndex()
	require.Len(t, kinds, 33, "ЧК5/Э5.2: 33 примитива")
	assert.Equal(t, "decor", kinds["crystal_tree"], "crystal_tree — декоративный")
	assert.Equal(t, "relief2d", kinds["crater"], "crater — 2D-форма (регистрация Э5.2)")

	// Горизонт: пресет `экзотика` (вставка B) наследуют `радиационные_пустоши`
	// и `химический_иней`; свой горизонт — 6 override-биомов (расклад 6/4, §4.9.1).
	exoticPresetHorizon := func(id string) string {
		v, _ := cat.ResolveBiomeView(id)
		hz, _ := v["horizon"].(map[string]any)
		layers := asMapList(hz["layers"])
		require.Len(t, layers, 2, "%q: 2 пояса горизонта", id)
		prof, _ := layers[0]["profile"].(map[string]any)
		prim, _ := prof["prim"].(string)
		return prim
	}
	for _, id := range []string{"радиационные_пустоши", "химический_иней"} {
		assert.Equal(t, "dome", exoticPresetHorizon(id), "%q: горизонт пресета `экзотика` (dome)", id)
	}
	// 4 биома наследуют горизонт своего семейства.
	assert.Equal(t, "crest", exoticPresetHorizon("металлические_щетинные_поля"),
		"щетинные поля: горизонт `скальные пустоши` (crest)")
	assert.Equal(t, "wave", exoticPresetHorizon("пружинная_тундра"),
		"тундра: горизонт `травяные` (wave)")
	// 6 override — свой горизонт (не семейный).
	assert.Equal(t, "spike", exoticPresetHorizon("кристальные_рощи"), "рощи: свой горизонта (spike)")
	assert.Equal(t, "step", exoticPresetHorizon("терминаторная_зона"), "терминатор: свой (step)")

	// 4 «рощи»: `hang: []` — лиан пресета `лесные/чащи` нет; «деревья» — не tree.
	for _, id := range []string{"кристальные_рощи", "кремниевые_рощи", "карбидно-алмазные_заросли", "струнные_рощи"} {
		v, _ := cat.ResolveBiomeView(id)
		assert.Empty(t, asMapList(v["hang"]), "%q: hang заменён (`hang: []`, §4.9.9 п.10)", id)
		for _, dec := range asMapList(v["decor"]) {
			assert.NotContains(t, []any{"tree", "conifer", "palm"}, dec["prim"], "%q: без tree/conifer/palm", id)
		}
	}

	// `кристальные_рощи` и `карбидно-алмазные_заросли` — с `crystal_tree`;
	// `кремниевые_рощи`/`струнные_рощи` — без него (§4.9.4, вставка C).
	for _, id := range []string{"кристальные_рощи", "карбидно-алмазные_заросли"} {
		v, _ := cat.ResolveBiomeView(id)
		ct := decorByPrim(v, "crystal_tree")
		require.NotNil(t, ct, "%q: crystal_tree в декорe", id)
		assert.NotNil(t, ct["crown"], "%q: crystal_tree.crown", id)
		assert.NotNil(t, ct["trunkW"], "%q: crystal_tree.trunkW", id)
		assert.NotNil(t, ct["crownW"], "%q: crystal_tree.crownW", id)
	}
	for _, id := range []string{"кремниевые_рощи", "струнные_рощи"} {
		v, _ := cat.ResolveBiomeView(id)
		assert.Nil(t, decorByPrim(v, "crystal_tree"), "%q: crystal_tree НЕ применяем (§4.9.4)", id)
	}

	// `металлические_щетинные_поля`: плотные тонкие spike (cluster), жизни нет.
	metal, _ := cat.ResolveBiomeView("металлические_щетинные_поля")
	spl := reliefLayer(metal, "spike")
	require.NotNil(t, spl, "щетинные поля: spike")
	assert.Equal(t, true, spl["cluster"], "spike.cluster (§4.7.12)")
	assert.InDelta(t, 0.0, metal["life_density"], 1e-9, "щетинные поля: life_density 0 (§4.9.9 п.11)")

	// `пружинная_тундра`: равнина семейства `травяные`, трава/кусты, life 0.20.
	tundra, _ := cat.ResolveBiomeView("пружинная_тундра")
	assert.Equal(t, "травяные", tundra["family"], "тундра: семейство `травяные`")
	assert.InDelta(t, 0.20, tundra["life_density"], 1e-9, "тундра: life_density 0.20")
	assert.NotNil(t, decorByPrim(tundra, "grass"), "тундра: grass")
	assert.NotNil(t, decorByPrim(tundra, "bush"), "тундра: bush")

	// `пещерный_мир_с_потолком`: временное 1D-чтение — `float: true` (§4.9.6).
	cave, _ := cat.ResolveBiomeView("пещерный_мир_с_потолком")
	relief, _ := cave["relief"].(map[string]any)
	assert.Equal(t, true, relief["float"], "пещера: float true (заготовка свода)")
}

// ==================== 2D-ФОРМЫ Э5.3 — ГРОТЫ И СВОДЫ (§3.4/§3.6) ====================

// TestBiomeViewSculptFormsResolve — дельты Э5.3 (`пещерный_мир_с_потолком`,
// `лавовые_поля`, `магмовый_океан`, `кратеры`) резолвятся, `relief.forms[].prim` —
// вид `relief2d`, бюджет вертикали С ДВУХ СТОРОН проходит (верх — сумма аддитивных,
// низ — сумма глубин вычитающих), сплошной `float`-потолок сохранён, а битый `forms`
// уводит РОВНО один биом в фолбэк (нефатально, §3.3/§B.1.3).
func TestBiomeViewSculptFormsResolve(t *testing.T) {
	cat := GetBiomeCatalog()
	kinds := cat.primKindIndex()
	topOK := viewBaseY - viewChunkTopMargin + viewTopSafety
	bottomOK := viewBaseY + viewChunkHeight - viewChunkTopMargin - viewTopSafety

	for _, id := range []string{"пещерный_мир_с_потолком", "лавовые_поля", "магмовый_океан", "кратеры"} {
		view, source := cat.ResolveBiomeView(id)
		require.Equal(t, "catalog", source, "биом %q: рецепт с формами обязан резолвиться", id)
		relief, ok := view["relief"].(map[string]any)
		require.True(t, ok, "%q: relief", id)
		forms := asMapList(relief["forms"])
		require.NotEmpty(t, forms, "%q: relief.forms (Э5.3, §3.6)", id)
		for i, f := range forms {
			prim, _ := f["prim"].(string)
			assert.Equal(t, "relief2d", kinds[prim], "%q: forms[%d].prim=%q — 2D-форма (§3.1)", id, i, prim)
		}
		top, ok := declaredReliefTop(view)
		require.True(t, ok)
		assert.GreaterOrEqual(t, top, topOK, "%q: верх твёрдого в бюджете (top=%.1f)", id, top)
		bottom, ok := declaredReliefBottom(view)
		require.True(t, ok)
		assert.LessOrEqual(t, bottom, bottomOK, "%q: низ твёрдого в бюджете (bottom=%.1f)", id, bottom)
	}

	// Целевые операторы: свод+ниша у пещерного мира, трубки у лавы/магмы, чаша у кратеров.
	cave, _ := cat.ResolveBiomeView("пещерный_мир_с_потолком")
	relief, _ := cave["relief"].(map[string]any)
	assert.Equal(t, true, relief["float"], "пещерный мир: сплошной float-потолок сохранён (§3.4)")
	assert.NotNil(t, formByPrim(cave, "overhang", 1), "пещерный мир: плита-свод overhang arch=1")
	assert.NotNil(t, formByPrim(cave, "void", 0), "пещерный мир: неглубокая ниша void")

	for _, id := range []string{"лавовые_поля", "магмовый_океан"} {
		v, _ := cat.ResolveBiomeView(id)
		assert.NotNil(t, formByPrim(v, "void", 0), "%q: лавовая трубка void (Э5.3)", id)
	}
	cr, _ := cat.ResolveBiomeView("кратеры")
	assert.NotNil(t, formByPrim(cr, "crater", 0), "кратеры: кольцевая чаша crater")

	// Битый forms (relief1d вместо relief2d) — ровно один биом в фолбэк.
	mut := *cat
	mut.Biomes = append([]BiomeDef{}, cat.Biomes...)
	bi := -1
	for i := range mut.Biomes {
		if mut.Biomes[i].ID == "кратеры" {
			bi = i
			break
		}
	}
	require.GreaterOrEqual(t, bi, 0)
	mut.Biomes[bi].View = map[string]any{
		"family": "скальные пустоши",
		"relief": map[string]any{"forms": []any{map[string]any{"prim": "wave"}}},
	}
	_, source := mut.ResolveBiomeView("кратеры")
	assert.Equal(t, "fallback", source, "битый forms уводит биом в фолбэк (§3.3)")
	other, sOther := mut.ResolveBiomeView("лавовые_поля")
	require.Equal(t, "catalog", sOther, "битый forms не каскадит на соседний биом")
	require.NotNil(t, other)
}

// TestBiomeViewDeclaredReliefBottom — низ бюджета (§3.3): сумма глубин вычитающих
// форм (`void.depth`, впадина `crater.depth`) поверх `baseY + offset`; аддитивные
// (`overhang`, `crater.rim`) и косметический `crack2d` вниз не опускают; дефолты
// клиента (void.depth 40, crater.depth 60) учтены forward-compat; превышение низа
// уводит биом в фолбэк.
func TestBiomeViewDeclaredReliefBottom(t *testing.T) {
	view := map[string]any{"relief": map[string]any{
		"offset": float64(10),
		"forms": []any{
			map[string]any{"prim": "void", "depth": []any{float64(50), float64(110)}},
			map[string]any{"prim": "crater", "depth": []any{float64(60), float64(140)}, "rim": float64(34)},
			map[string]any{"prim": "overhang", "arch": float64(1), "h": float64(30), "opening": float64(80)},
			map[string]any{"prim": "crack2d", "depth": float64(300)},
		},
	}}
	bottom, ok := declaredReliefBottom(view)
	require.True(t, ok)
	// 300 + 10 + 110 + 140 = 560 (overhang/crack2d вниз не опускают).
	assert.InDelta(t, viewBaseY+10+110+140, bottom, 1e-9, "низ = baseY + offset + Σ глубин вычитающих")

	// Дефолты клиента: void без depth → 40, crater без depth → 60.
	def := map[string]any{"relief": map[string]any{
		"forms": []any{map[string]any{"prim": "void"}, map[string]any{"prim": "crater"}},
	}}
	db, ok := declaredReliefBottom(def)
	require.True(t, ok)
	assert.InDelta(t, viewBaseY+40+60, db, 1e-9, "void.depth 40, crater.depth 60 (дефолты клиента)")

	// Превышение низа — ошибка вида, биом в фолбэк.
	cat := *GetBiomeCatalog()
	cat.Biomes = append([]BiomeDef{}, cat.Biomes...)
	cat.Biomes[0].ID = "бездонный_колодец"
	cat.Biomes[0].View = map[string]any{
		"relief": map[string]any{"forms": []any{map[string]any{"prim": "void", "depth": float64(9000)}}},
	}
	_, source := cat.ResolveBiomeView("бездонный_колодец")
	assert.Equal(t, "fallback", source, "пол ниже растра → фолбэк биома (§3.3)")
	d := cat.ViewDiagnostics()
	found := false
	for _, e := range d.Errors {
		if e.Biome == "бездонный_колодец" && e.Field == "relief" {
			found = true
		}
	}
	assert.True(t, found, "превышение низа названо полем relief")
}

// decorByPrim — первая запись decor[] с данным prim (nil, если нет).
func decorByPrim(view map[string]any, prim string) map[string]any {
	for _, d := range asMapList(view["decor"]) {
		if p, _ := d["prim"].(string); p == prim {
			return d
		}
	}
	return nil
}

// reliefLayer — первый слой relief.layers с данным prim.
func reliefLayer(view map[string]any, prim string) map[string]any {
	relief, _ := view["relief"].(map[string]any)
	for _, l := range asMapList(relief["layers"]) {
		if p, _ := l["prim"].(string); p == prim {
			return l
		}
	}
	return nil
}

// crystalGlow — значение glow первой crystal-записи декора (nil, если поля нет).
func crystalGlow(view map[string]any) any {
	for _, d := range asMapList(view["decor"]) {
		if p, _ := d["prim"].(string); p == "crystal" {
			return d["glow"]
		}
	}
	return nil
}
