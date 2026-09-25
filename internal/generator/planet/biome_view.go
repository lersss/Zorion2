// internal/generator/planet/biome_view.go
//
// Рецепт вида биома (спека 2026-09-23 «Мир прогулки: рецепты биомов» §2):
// данные справочника (BiomeDef.view + секции view_families/view_primitives),
// серверный резолв «семейство ⊕ дельта» (§2.3) и нефатальная валидация вида
// (§2.8) с диагностикой, называющей биом и поле. Рецепт НЕ входит в Validate()
// — ошибка вида уводит в фолбэк один биом, а не роняет каталог/генерацию.
package planet

import (
	"fmt"
	"log"
	"strings"
)

// ViewSchemaVersion — версия схемы рецепта (пакет прогулки, поле view_version,
// §2.6): клиент умеет игнорировать незнакомую версию.
const ViewSchemaVersion = 1

// ViewPrimitive — запись реестра view_primitives (§2.1): id примитива и его
// вид — ровно четыре: relief1d / relief2d / decor / placement.
type ViewPrimitive struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

// ViewIssue — диагностика вида (§2.8 п.4): биом + поле + причина.
type ViewIssue struct {
	Biome  string `json:"biome"`
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

// ViewDiagnostics — диагностика вида каталога (§2.8): ошибки (рецепт не
// применяется, биом в фолбэк), предупреждения (рецепт применяется) и биомы без
// рецепта (§2.7 — отсутствие рецепта обязано быть видимым, а не тихим).
type ViewDiagnostics struct {
	Errors   []ViewIssue `json:"view_errors"`
	Warnings []ViewIssue `json:"view_warnings"`
	Missing  []string    `json:"view_missing"`
}

// viewFamilyByID — пресет семейства вида по id (nil, если нет).
func (c *BiomeCatalog) viewFamilyByID(id string) map[string]any {
	for _, f := range c.ViewFamilies {
		if s, _ := f["id"].(string); s == id {
			return f
		}
	}
	return nil
}

// primKindIndex — id примитива → вид (реестр view_primitives).
func (c *BiomeCatalog) primKindIndex() map[string]string {
	idx := make(map[string]string, len(c.ViewPrimitives))
	for _, p := range c.ViewPrimitives {
		idx[p.ID] = p.Kind
	}
	return idx
}

// ResolveBiomeView — резолвленный вид биома и источник (§2.6): "catalog" —
// рецепт есть и валиден; "fallback" — рецепта нет или он с ошибкой (§2.8 п.3).
func (c *BiomeCatalog) ResolveBiomeView(biomeID string) (map[string]any, string) {
	b := c.BiomeByID(biomeID)
	if b == nil || len(b.View) == 0 {
		return nil, "fallback"
	}
	view, errs, _ := c.resolveBiomeView(b)
	if len(errs) > 0 {
		return nil, "fallback"
	}
	return view, "catalog"
}

// resolveBiomeView — слияние пресета семейства и дельты биома (§2.3) +
// проверка вида (§2.8). Возвращает (резолвленный вид, ошибки, предупреждения).
func (c *BiomeCatalog) resolveBiomeView(b *BiomeDef) (map[string]any, []ViewIssue, []ViewIssue) {
	var errs, warns []ViewIssue
	view := b.View
	if fam, _ := view["family"].(string); fam != "" {
		preset := c.viewFamilyByID(fam)
		if preset == nil {
			errs = append(errs, ViewIssue{b.ID, "family", fmt.Sprintf("семейство %q не найдено", fam)})
		} else {
			base := make(map[string]any, len(preset))
			for k, v := range preset {
				if k == "id" || k == "name" {
					continue // id/name — метаданные пресета, не часть рецепта
				}
				base[k] = v
			}
			view = mergeViewMaps(base, view)
		}
	}
	errs = append(errs, c.validateView(b.ID, view)...)
	if base, ok := nestedString(view, "palette", "base"); ok && b.Color != "" && !strings.EqualFold(base, b.Color) {
		warns = append(warns, ViewIssue{b.ID, "palette.base", "не совпадает с color (источник истины — color)"})
	}
	return view, errs, warns
}

// mergeViewMaps — слияние рецепта по трём правилам §2.3: карты — по ключам
// (рекурсивно), всё остальное (скаляр, список) — перекрытие дельтой целиком.
// Ловушка: horizon может задаваться в дельте биома, а не только в пресете семейства — семейный горизонт не единственный источник (при отладке ярусов/дальнего плана).
func mergeViewMaps(preset, delta map[string]any) map[string]any {
	out := make(map[string]any, len(preset)+len(delta))
	for k, v := range preset {
		out[k] = v
	}
	for k, v := range delta {
		if pm, ok := out[k].(map[string]any); ok {
			if dm, ok := v.(map[string]any); ok {
				out[k] = mergeViewMaps(pm, dm)
				continue
			}
		}
		out[k] = v
	}
	return out
}

// nestedString — строковое значение по пути вложенных карт (для диагностики).
func nestedString(m map[string]any, path ...string) (string, bool) {
	cur := m
	for i, key := range path {
		v, ok := cur[key]
		if !ok {
			return "", false
		}
		if i == len(path)-1 {
			s, ok := v.(string)
			return s, ok
		}
		next, ok := v.(map[string]any)
		if !ok {
			return "", false
		}
		cur = next
	}
	return "", false
}

// Запас вертикали чанка (спека 2026-09-23 §6 п.6): растр чанка —
// [baseY − CHUNK_TOP_MARGIN, baseY − CHUNK_TOP_MARGIN + CHUNK_HEIGHT]. Числа
// зеркалят web/static/js/surface/surface_render.js / surface_config.js
// (baseY = 300, CHUNK_TOP_MARGIN = 1200 — поднят 700 → 1200 в Э3 под рецепт гор
// scale 0.55; высота канваса та же, память не растёт). Объявленный профиль
// рецепта не должен выходить за верх растра — иначе земля твёрдая, но не
// нарисована (класс «невидимые стены»). Точная граница с FLOAT_SPAN/24 —
// рантайм-тест T-budget (Э3, tools/surface-profile-check.mjs).
const (
	viewBaseY          = 300.0
	viewChunkTopMargin = 1200.0
	// viewTopSafety — запас до верхней границы растра (спека Э5 §3.3/§6 п.6):
	// объявленный профиль обязан быть НЕ выше `baseY − CHUNK_TOP_MARGIN + 24`;
	// тот же запас применяет рантайм-тест T-budget
	// (tools/surface-profile-check.mjs, MIN_OK). До Э5 серверная проверка была
	// без +24 (запас жил только в тесте) — приводим к общей границе.
	viewTopSafety = 24.0
)

// declaredReliefTop — верхняя (минимальная по y) точка объявленного профиля:
// baseY + offset − Σ(амплитуды вверх)·scale. Амплитуды вверх: база (large,
// 115·ridge), мелкая деталь (40·(1−0.7·flatten)) и слои (amp/h/stepH).
// Числа зеркалят terrainHeight в surface_world.js (§3.1) — при переносе формулы
// на стек слоёв (Э3) обновить здесь же.
func declaredReliefTop(view map[string]any) (float64, bool) {
	relief, ok := view["relief"].(map[string]any)
	if !ok {
		return 0, false
	}
	scale := 1.0
	if v, ok := relief["scale"].(float64); ok && v > 0 {
		scale = v
	}
	rise := 115.0*viewNum(relief["ridge"], 0) + 40.0*(1-0.7*viewNum(relief["flatten"], 0))
	for _, l := range asMapList(relief["layers"]) {
		rise += layerUpAmp(l)
	}
	return viewBaseY + viewNum(relief["offset"], 0) - rise*scale, true
}

// layerUpAmp — объявленная амплитуда слоя вверх: amp/h/stepH (полуразмах, берём
// верх диапазона). depth (вырез) и len/w (горизонталь) вверх не поднимают.
func layerUpAmp(l map[string]any) float64 {
	for _, key := range []string{"amp", "h", "stepH"} {
		if v, ok := l[key]; ok {
			return rangeUpper(v)
		}
	}
	return 0
}

// rangeUpper — верхняя граница [lo,hi] или само число.
func rangeUpper(v any) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	if a, ok := v.([]any); ok && len(a) == 2 {
		if hi, ok := a[1].(float64); ok {
			return hi
		}
	}
	return 0
}

// viewNum — число из JSON-значения или дефолт.
func viewNum(v any, def float64) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return def
}

// validateView — проверка рецепта вида (§2.8 п.2). Ошибки (рецепт не
// применяется): неизвестный примитив / вид блока не совпадает, цвет не
// #RRGGBB, пустой или перевёрнутый диапазон [lo,hi]. Ссылки проверяются по
// реестру view_primitives (список — данные, не дублируется в коде).
func (c *BiomeCatalog) validateView(biomeID string, view map[string]any) []ViewIssue {
	idx := c.primKindIndex()
	var errs []ViewIssue
	checkPrim := func(field string, m map[string]any, want string) {
		prim, _ := m["prim"].(string)
		if prim == "" {
			errs = append(errs, ViewIssue{biomeID, field, "не задан prim"})
			return
		}
		kind, ok := idx[prim]
		if !ok {
			errs = append(errs, ViewIssue{biomeID, field, fmt.Sprintf("примитив %q отсутствует в view_primitives", prim)})
			return
		}
		if kind != want {
			errs = append(errs, ViewIssue{biomeID, field, fmt.Sprintf("примитив %q вида %q, ожидался %q", prim, kind, want)})
		}
	}
	// relief.layers — профильные примитивы relief1d (§2.1/§3.1).
	if relief, ok := view["relief"].(map[string]any); ok {
		for i, l := range asMapList(relief["layers"]) {
			checkPrim(fmt.Sprintf("relief.layers[%d]", i), l, "relief1d")
		}
	}
	// decor / hang — декоративные примитивы decor (§3.2).
	for i, d := range asMapList(view["decor"]) {
		checkPrim(fmt.Sprintf("decor[%d]", i), d, "decor")
	}
	for i, d := range asMapList(view["hang"]) {
		checkPrim(fmt.Sprintf("hang[%d]", i), d, "decor")
	}
	// horizon.layers[].profile — тоже relief1d (§3.6, отдельного вида нет).
	if hz, ok := view["horizon"].(map[string]any); ok {
		for i, l := range asMapList(hz["layers"]) {
			prof, _ := l["profile"].(map[string]any)
			if prof == nil {
				errs = append(errs, ViewIssue{biomeID, fmt.Sprintf("horizon.layers[%d].profile", i), "не задан profile"})
				continue
			}
			checkPrim(fmt.Sprintf("horizon.layers[%d].profile", i), prof, "relief1d")
		}
	}
	// placement.mode — правило размещения (§3.3).
	if pl, ok := view["placement"].(map[string]any); ok {
		if mode, _ := pl["mode"].(string); mode != "" {
			if kind, ok := idx[mode]; !ok {
				errs = append(errs, ViewIssue{biomeID, "placement.mode", fmt.Sprintf("примитив %q отсутствует в view_primitives", mode)})
			} else if kind != "placement" {
				errs = append(errs, ViewIssue{biomeID, "placement.mode", fmt.Sprintf("примитив %q вида %q, ожидался %q", mode, kind, "placement")})
			}
		}
	}
	// palette — цвета #RRGGBB (§3.4).
	if pal, ok := view["palette"].(map[string]any); ok {
		for key, v := range pal {
			s, ok := v.(string)
			if !ok {
				continue
			}
			if _, ok := parseHexColor(s); !ok {
				errs = append(errs, ViewIssue{biomeID, "palette." + key, fmt.Sprintf("цвет %q — не #RRGGBB", s)})
			}
		}
	}
	// Диапазоны [lo,hi] — lo ≤ hi (обход всего рецепта).
	errs = append(errs, validateViewRanges(biomeID, "", view)...)
	// Бюджет вертикали (§6 п.6): объявленный профиль не выходит за верх растра.
	if top, ok := declaredReliefTop(view); ok && top < viewBaseY-viewChunkTopMargin+viewTopSafety {
		errs = append(errs, ViewIssue{biomeID, "relief",
			fmt.Sprintf("объявленный профиль на %.0f px выше запаса вертикали чанка (%.0f px) — увеличьте relief.scale",
				viewBaseY-viewChunkTopMargin+viewTopSafety-top, viewChunkTopMargin)})
	}
	return errs
}

// validateViewRanges — рекурсивный обход рецепта: любой [lo,hi] из двух чисел
// обязан иметь lo ≤ hi (§2.8 п.2 — пустой/перевёрнутый диапазон нечитаем).
func validateViewRanges(biomeID, path string, m map[string]any) []ViewIssue {
	var errs []ViewIssue
	for k, v := range m {
		field := k
		if path != "" {
			field = path + "." + k
		}
		switch t := v.(type) {
		case map[string]any:
			errs = append(errs, validateViewRanges(biomeID, field, t)...)
		case []any:
			if lo, hi, ok := rangeBounds(t); ok && lo > hi {
				errs = append(errs, ViewIssue{biomeID, field, fmt.Sprintf("диапазон [%g,%g]: lo > hi", lo, hi)})
			}
		}
	}
	return errs
}

// rangeBounds — [lo,hi] из двух чисел (ok=false, если это не диапазон).
func rangeBounds(a []any) (lo, hi float64, ok bool) {
	if len(a) != 2 {
		return 0, 0, false
	}
	l, ok1 := a[0].(float64)
	h, ok2 := a[1].(float64)
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return l, h, true
}

// asMapList — список объектов из JSON-значения (nil, если не список карт).
func asMapList(v any) []map[string]any {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, e := range arr {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// ViewDiagnostics — диагностика вида всего каталога (§2.8 п.4): по каждому
// биому — ошибки/предупреждения; отдельно — биомы без рецепта (§2.7).
func (c *BiomeCatalog) ViewDiagnostics() ViewDiagnostics {
	d := ViewDiagnostics{Errors: []ViewIssue{}, Warnings: []ViewIssue{}, Missing: []string{}}
	for i := range c.Biomes {
		b := &c.Biomes[i]
		if len(b.View) == 0 {
			d.Missing = append(d.Missing, b.ID)
			continue
		}
		_, errs, warns := c.resolveBiomeView(b)
		d.Errors = append(d.Errors, errs...)
		d.Warnings = append(d.Warnings, warns...)
	}
	return d
}

// logViewDiagnostics — строка в лог при загрузке/перестройке каталога (§2.8
// п.4): ошибки вида видимы, «тихо ушло в фолбэк» не бывает.
func logViewDiagnostics(cat *BiomeCatalog) {
	d := cat.ViewDiagnostics()
	for _, e := range d.Errors {
		log.Printf("⚠️ Вид биома %q: поле %s — %s (рецепт не применяется, фолбэк)", e.Biome, e.Field, e.Reason)
	}
	for _, w := range d.Warnings {
		log.Printf("ℹ️ Вид биома %q: поле %s — %s", w.Biome, w.Field, w.Reason)
	}
}
