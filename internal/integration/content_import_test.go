// internal/integration/content_import_test.go
// Импорт снимка контента на ЖИВОЙ PostgreSQL (спека 2026-09-24-каталог-
// экспорт-импорт-контента-на-прод §5, итерация И3). Мок не знает про
// advisory-лок, FK/ON DELETE, JSONB и частичный UNIQUE — здесь полный путь
// экспорт→импорт на scratch-схемах (helpers_test.go). Без БД тесты скипаются.
package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/goodsstudio"
	"zorion/internal/handlers"
	"zorion/internal/models"
	"zorion/internal/repository"
)

// exportContentSnapshot — сид источника + обогащение (рецепт/состав/привязка) +
// снимок через ручку экспорта (И2).
func exportContentSnapshot(t *testing.T, s *scratchDB) []byte {
	t.Helper()
	require.NoError(t, goodsstudio.Seed(s.db))
	require.NoError(t, goodsstudio.SeedProducers(s.db))
	enrichSource(t, s.db)
	return exportSnapshot(t, s)
}

// enrichSource — сид не создаёт товары kind=good/рецепты; добавляем один товар с
// рецептом, составом и привязкой к постройке — покрытие T3/T6/T15.
func enrichSource(t *testing.T, db *sql.DB) {
	t.Helper()
	catGood := mustInt(t, db, `SELECT id FROM categories WHERE kind='good' ORDER BY id LIMIT 1`)
	var goodID int64
	if err := db.QueryRow(
		`INSERT INTO goods (name, name_norm, category_id, kind, source) VALUES ('Тест-товар','тест-товар импорт',$1,'good','manual') RETURNING id`,
		catGood).Scan(&goodID); err != nil {
		t.Fatalf("enrich good: %v", err)
	}
	compID := mustInt(t, db, `SELECT id FROM goods WHERE kind='resource' ORDER BY id LIMIT 1`)
	var recipeID int64
	if err := db.QueryRow(`INSERT INTO recipes (good_id, complexity) VALUES ($1, 2) RETURNING id`, goodID).Scan(&recipeID); err != nil {
		t.Fatalf("enrich recipe: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO recipe_components (recipe_id, pos, component_id, quantity, reason, allow_resource)
		 VALUES ($1, 0, $2, 2, 'вода', true)`, recipeID, compID); err != nil {
		t.Fatalf("enrich component: %v", err)
	}
	ptID := mustInt(t, db, `SELECT id FROM producer_types WHERE code IS NOT NULL ORDER BY id LIMIT 1`)
	if _, err := db.Exec(`INSERT INTO producer_recipes (producer_type_id, recipe_id) VALUES ($1,$2)`, ptID, recipeID); err != nil {
		t.Fatalf("enrich binding: %v", err)
	}
}

// exportSnapshot — снимок через ручку экспорта без сида (источник уже готов).
func exportSnapshot(t *testing.T, s *scratchDB) []byte {
	t.Helper()
	h := handlers.NewStudioHandlers(s.db, nil, "")
	h.SetContentExportPath(filepath.Join(t.TempDir(), "catalog.json"))
	rec := httptest.NewRecorder()
	h.ContentExport(rec, httptest.NewRequest(http.MethodGet, "/studio/api/content/export", nil))
	require.Equalf(t, http.StatusOK, rec.Code, "экспорт: %s", rec.Body.String())
	return rec.Body.Bytes()
}

// clearContent — полностью чистит контентные таблицы (родители позже детей).
func clearContent(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`DELETE FROM producer_items`,
		`DELETE FROM producer_recipes`,
		`DELETE FROM producer_slots`,
		`DELETE FROM recipe_components`,
		`DELETE FROM recipes`,
		`DELETE FROM goods`,
		`DELETE FROM producer_types WHERE parent_id IS NOT NULL`,
		`DELETE FROM producer_types`,
		`DELETE FROM items`,
		`DELETE FROM effect_types`,
		`DELETE FROM categories`,
		`DELETE FROM generation_config WHERE key = '` + models.DefaultSettlementTypeIDKey + `'`,
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("clearContent %q: %v", q, err)
		}
	}
}

// callImport — вызов ручки импорта с телом {"snapshot": raw}.
func callImport(t *testing.T, db *sql.DB, raw []byte, dryRun bool) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	h := handlers.NewStudioHandlers(db, nil, "")
	h.SetContentExportPath(filepath.Join(t.TempDir(), "catalog.json"))
	url := "/studio/api/content/import"
	if dryRun {
		url += "?dry_run=true"
	}
	body := append([]byte(`{"snapshot":`), raw...)
	body = append(body, '}')
	rec := httptest.NewRecorder()
	h.ContentImport(rec, httptest.NewRequest(http.MethodPost, url, bytes.NewReader(body)))
	var m map[string]interface{}
	require.NoErrorf(t, json.Unmarshal(rec.Body.Bytes(), &m), "ответ не JSON: %s", rec.Body.String())
	return rec, m
}

// applyImport — импорт с ожиданием 200; возвращает отчёт.
func applyImport(t *testing.T, db *sql.DB, raw []byte) map[string]interface{} {
	t.Helper()
	rec, m := callImport(t, db, raw, false)
	require.Equalf(t, http.StatusOK, rec.Code, "импорт не прошёл: %s", rec.Body.String())
	return m
}

// sectionCount — число строк секции из map ответа.
func sectionCount(t *testing.T, m map[string]interface{}, key string) int {
	t.Helper()
	raw, ok := m[key]
	if !ok {
		return 0
	}
	mm, ok := raw.(map[string]interface{})
	if !ok {
		return 0
	}
	total := 0
	for _, v := range mm {
		total += int(v.(float64))
	}
	return total
}

// entriesLen — длина перечня (delete/blocked/unmatched).
func entriesLen(m map[string]interface{}, key string) int {
	raw, ok := m[key]
	if !ok {
		return 0
	}
	l, ok := raw.([]interface{})
	if !ok {
		return 0
	}
	return len(l)
}

// TestContentImportIntoEmptyDB — T3: экспорт → импорт в пустую БД = тот же
// контент (по метке и составу).
func TestContentImportIntoEmptyDB(t *testing.T) {
	src := openMigrated(t)
	raw := exportContentSnapshot(t, src)

	tgt := openMigrated(t)
	clearContent(t, tgt.db)
	applyImport(t, tgt.db, raw)

	srcRepo := repository.NewGoodsRepository(src.db)
	tgtRepo := repository.NewGoodsRepository(tgt.db)
	sr, err := srcRepo.ContentExport()
	require.NoError(t, err)
	tr, err := tgtRepo.ContentExport()
	require.NoError(t, err)
	require.Equal(t, len(sr.Categories), len(tr.Categories), "categories")
	require.Equal(t, len(sr.Goods), len(tr.Goods), "goods")
	require.Equal(t, len(sr.Recipes), len(tr.Recipes), "recipes")
	require.Equal(t, len(sr.Components), len(tr.Components), "recipe_components")
	require.Equal(t, len(sr.ProducerTypes), len(tr.ProducerTypes), "producer_types")
	require.Equal(t, len(sr.Items), len(tr.Items), "items")
	require.Equal(t, len(sr.ProducerSlots), len(tr.ProducerSlots), "producer_slots")
	require.Equal(t, len(sr.ProducerRecipes), len(tr.ProducerRecipes), "producer_recipes")
	require.Equal(t, len(sr.ProducerItems), len(tr.ProducerItems), "producer_items")
	require.Equal(t, len(sr.EffectTypes), len(tr.EffectTypes), "effect_types")
	require.NotZero(t, len(tr.Goods))
	require.Equal(t, int64(0), mustInt(t, tgt.db, `SELECT COUNT(*) FROM goods WHERE code IS NULL`))
	require.Equal(t, "g_0001", mustStr(t, tgt.db, `SELECT code FROM goods ORDER BY id LIMIT 1`))
}

// TestContentImportIdempotent — T4: повторный импорт того же файла = 0/0/0.
func TestContentImportIdempotent(t *testing.T) {
	src := openMigrated(t)
	raw := exportContentSnapshot(t, src)

	tgt := openMigrated(t)
	clearContent(t, tgt.db)
	applyImport(t, tgt.db, raw)

	rec, m := callImport(t, tgt.db, raw, false)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, 0, sectionCount(t, m, "created"), "created не ноль: %v", m)
	require.Equal(t, 0, sectionCount(t, m, "updated"), "updated не ноль: %v", m)
	require.Equal(t, 0, sectionCount(t, m, "deleted"), "deleted не ноль: %v", m)
}

// TestContentImportRestoresDeleted — T5: удалённая в цели запись восстанавливается.
func TestContentImportRestoresDeleted(t *testing.T) {
	src := openMigrated(t)
	raw := exportContentSnapshot(t, src)

	tgt := openMigrated(t)
	clearContent(t, tgt.db)
	applyImport(t, tgt.db, raw)

	lastCode := mustStr(t, tgt.db, `SELECT code FROM goods ORDER BY id DESC LIMIT 1`)
	lastName := mustStr(t, tgt.db, `SELECT name FROM goods WHERE code = $1`, lastCode)
	if _, err := tgt.db.Exec(`DELETE FROM goods WHERE code = $1`, lastCode); err != nil {
		t.Fatalf("delete: %v", err)
	}
	applyImport(t, tgt.db, raw)
	require.Equal(t, lastName, mustStr(t, tgt.db, `SELECT name FROM goods WHERE code = $1`, lastCode))
}

// TestContentImportReferencesResolve — T6: ссылки резолвятся в локальные id.
func TestContentImportReferencesResolve(t *testing.T) {
	src := openMigrated(t)
	require.NoError(t, goodsstudio.Seed(src.db))
	require.NoError(t, goodsstudio.SeedProducers(src.db))
	enrichSource(t, src.db)
	raw := exportSnapshot(t, src)
	require.False(t, bytes.Contains(raw, []byte(`"id"`)), "снимок не должен нести id")

	tgt := openMigrated(t)
	clearContent(t, tgt.db)
	applyImport(t, tgt.db, raw)

	// все ссылки листьев/родителей резолвятся (join'ы непусты и без сирот)
	require.NotZero(t, mustInt(t, tgt.db,
		`SELECT COUNT(*) FROM recipe_components c JOIN recipes r ON r.id=c.recipe_id JOIN goods g ON g.id=r.good_id`))
	require.Equal(t, int64(0), mustInt(t, tgt.db,
		`SELECT COUNT(*) FROM recipe_components c WHERE c.component_id IS NOT NULL
		   AND NOT EXISTS (SELECT 1 FROM goods g WHERE g.id=c.component_id)`))
	require.Equal(t, int64(0), mustInt(t, tgt.db,
		`SELECT COUNT(*) FROM producer_types p WHERE p.parent_id IS NOT NULL
		   AND NOT EXISTS (SELECT 1 FROM producer_types q WHERE q.id=p.parent_id)`))
	require.NotZero(t, mustInt(t, tgt.db,
		`SELECT COUNT(*) FROM producer_recipes pr JOIN recipes r ON r.id=pr.recipe_id JOIN producer_types p ON p.id=pr.producer_type_id`))
	require.Equal(t, int64(0), mustInt(t, tgt.db,
		`SELECT COUNT(*) FROM producer_slots s WHERE NOT EXISTS (SELECT 1 FROM categories c WHERE c.id=s.category_id)`))
	require.Equal(t, int64(0), mustInt(t, tgt.db,
		`SELECT COUNT(*) FROM producer_items pi WHERE NOT EXISTS (SELECT 1 FROM items i WHERE i.id=pi.item_id)`))
}

// TestContentImportAnchorSameRecord — T7: якорь указывает на ту же запись.
func TestContentImportAnchorSameRecord(t *testing.T) {
	src := openMigrated(t)
	raw := exportContentSnapshot(t, src)
	srcAnchorCode := mustStr(t, src.db,
		`SELECT p.code FROM producer_types p JOIN generation_config g ON g.payload::text::bigint = p.id
		 WHERE g.key = $1`, models.DefaultSettlementTypeIDKey)

	tgt := openMigrated(t)
	clearContent(t, tgt.db)
	applyImport(t, tgt.db, raw)

	tgtAnchorCode := mustStr(t, tgt.db,
		`SELECT p.code FROM producer_types p JOIN generation_config g ON g.payload::text::bigint = p.id
		 WHERE g.key = $1`, models.DefaultSettlementTypeIDKey)
	require.Equal(t, srcAnchorCode, tgtAnchorCode)
}

// TestContentImportFullReplacementDelete — T8a: записи нет в файле и она не
// связана с миром — удаляется.
func TestContentImportFullReplacementDelete(t *testing.T) {
	src := openMigrated(t)
	raw := exportContentSnapshot(t, src)

	tgt := openMigrated(t)
	clearContent(t, tgt.db)
	applyImport(t, tgt.db, raw)

	snap := parseSnap(t, raw)
	require.NotEmpty(t, snap.EffectTypes)
	removed := snap.EffectTypes[len(snap.EffectTypes)-1].Code
	snap.EffectTypes = snap.EffectTypes[:len(snap.EffectTypes)-1]
	before := mustInt(t, tgt.db, `SELECT COUNT(*) FROM effect_types`)
	approved := marshalSnap(t, snap)

	rec, m := callImport(t, tgt.db, approved, false)
	require.Equalf(t, http.StatusOK, rec.Code, rec.Body.String())
	require.GreaterOrEqual(t, sectionCount(t, m, "deleted"), 1, "нет удаления: %v", m)
	require.Equal(t, before-1, mustInt(t, tgt.db, `SELECT COUNT(*) FROM effect_types`))
	require.Equal(t, int64(0), mustInt(t, tgt.db, `SELECT COUNT(*) FROM effect_types WHERE code = $1`, removed))
}

// TestContentImportBlockedByWorldRef — T8b: удаление, на которое ссылается
// объект мира (active_effects), блокирует импорт; БД не изменена.
func TestContentImportBlockedByWorldRef(t *testing.T) {
	src := openMigrated(t)
	raw := exportContentSnapshot(t, src)

	tgt := openMigrated(t)
	clearContent(t, tgt.db)
	applyImport(t, tgt.db, raw)

	snap := parseSnap(t, raw)
	require.NotEmpty(t, snap.EffectTypes)
	removed := snap.EffectTypes[len(snap.EffectTypes)-1]
	effID := mustInt(t, tgt.db, `SELECT id FROM effect_types WHERE code = $1`, removed.Code)
	if _, err := tgt.db.Exec(
		`INSERT INTO active_effects (effect_type_id, owner_type, owner_id, source_position)
		 VALUES ($1, 'probe', gen_random_uuid(), 'тест')`, effID); err != nil {
		t.Fatalf("insert active_effects: %v", err)
	}
	snap.EffectTypes = snap.EffectTypes[:len(snap.EffectTypes)-1]
	approved := marshalSnap(t, snap)

	before := mustInt(t, tgt.db, `SELECT COUNT(*) FROM effect_types`)
	rec, m := callImport(t, tgt.db, approved, false)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.NotZero(t, entriesLen(m, "blocked"), "нет blocked: %v", m)
	require.Equal(t, before, mustInt(t, tgt.db, `SELECT COUNT(*) FROM effect_types`), "БД изменилась при блоке")
}

// TestContentImportDryRun — T9: сухой прогон = факт; при dry_run БД не изменена.
func TestContentImportDryRun(t *testing.T) {
	src := openMigrated(t)
	raw := exportContentSnapshot(t, src)

	tgt := openMigrated(t)
	clearContent(t, tgt.db)

	rec, m := callImport(t, tgt.db, raw, true)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "managed", m["mode"])
	require.Equal(t, int64(0), mustInt(t, tgt.db, `SELECT COUNT(*) FROM goods`), "dry_run записал в БД")

	dryCreated := sectionCount(t, m, "create")
	report := applyImport(t, tgt.db, raw)
	require.Equal(t, dryCreated, sectionCount(t, report, "created"), "дифф не совпал с фактом")
}

// TestContentImportBootstrap — T10: цель без меток → сопоставление по имени,
// метки впечатываются, id сохранены; при наличии метки bootstrap запрещён.
func TestContentImportBootstrap(t *testing.T) {
	src := openMigrated(t)
	raw := exportContentSnapshot(t, src)

	tgt := openMigrated(t)
	clearContent(t, tgt.db)
	applyImport(t, tgt.db, raw)

	// снимаем метки у всех четырёх контентных таблиц → цель «бутстрап»
	for _, q := range []string{
		`UPDATE goods SET code = NULL`,
		`UPDATE producer_types SET code = NULL`,
		`UPDATE items SET code = NULL`,
		`UPDATE effect_types SET code = NULL`,
	} {
		if _, err := tgt.db.Exec(q); err != nil {
			t.Fatalf("clear code %q: %v", q, err)
		}
	}
	probeCode := mustStr(t, tgt.db, `SELECT name FROM goods ORDER BY id LIMIT 1`)
	probeID := mustInt(t, tgt.db, `SELECT id FROM goods WHERE name = $1`, probeCode)

	rec, m := callImport(t, tgt.db, raw, false)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "bootstrap", m["mode"])
	require.GreaterOrEqual(t, sectionCount(t, m, "updated"), 1, "метки не впечатаны: %v", m)
	require.Equal(t, probeID, mustInt(t, tgt.db, `SELECT id FROM goods WHERE name = $1`, probeCode), "id изменён")
	require.Equal(t, int64(0), mustInt(t, tgt.db, `SELECT COUNT(*) FROM goods WHERE code IS NULL`))
}

// TestContentImportUnmatchedRefuses — T11: запись цели без метки (смесь режимов)
// → unmatched и отказ; БД не изменена.
func TestContentImportUnmatchedRefuses(t *testing.T) {
	src := openMigrated(t)
	raw := exportContentSnapshot(t, src)

	tgt := openMigrated(t)
	clearContent(t, tgt.db)
	applyImport(t, tgt.db, raw)

	// все метки сняты, одна возвращена → смесь
	for _, q := range []string{
		`UPDATE goods SET code = NULL`,
		`UPDATE producer_types SET code = NULL`,
		`UPDATE items SET code = NULL`,
		`UPDATE effect_types SET code = NULL`,
	} {
		if _, err := tgt.db.Exec(q); err != nil {
			t.Fatalf("clear code %q: %v", q, err)
		}
	}
	if _, err := tgt.db.Exec(`UPDATE goods SET code = 'g_9001' WHERE id = (SELECT id FROM goods ORDER BY id LIMIT 1)`); err != nil {
		t.Fatalf("set code: %v", err)
	}
	before := mustInt(t, tgt.db, `SELECT COUNT(*) FROM goods`)

	rec, m := callImport(t, tgt.db, raw, false)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.Equal(t, "unmatched", m["mode"])
	require.NotZero(t, entriesLen(m, "unmatched"))
	require.Equal(t, before, mustInt(t, tgt.db, `SELECT COUNT(*) FROM goods`), "БД изменена при unmatched")
}

// TestContentImportAtomicRollback — T12: конфликт в снимке → откат, БД целы.
func TestContentImportAtomicRollback(t *testing.T) {
	src := openMigrated(t)
	raw := exportContentSnapshot(t, src)

	tgt := openMigrated(t)
	clearContent(t, tgt.db)
	applyImport(t, tgt.db, raw)

	snap := parseSnap(t, raw)
	require.GreaterOrEqual(t, len(snap.Goods), 2)
	// конфликт имён: код товара A, но имя другого товара → UNIQUE(name_norm)
	snap.Goods[0].Name = snap.Goods[1].Name
	before := mustStr(t, tgt.db, `SELECT name FROM goods WHERE code = $1`, snap.Goods[0].Code)

	rec, _ := callImport(t, tgt.db, marshalSnap(t, snap), false)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.Equal(t, before, mustStr(t, tgt.db, `SELECT name FROM goods WHERE code = $1`, snap.Goods[0].Code),
		"БД изменена при откате")
}

// TestContentImportRenameSameCode — T13: переименование при той же метке
// обновляет ту же запись (id и число строк не меняются).
func TestContentImportRenameSameCode(t *testing.T) {
	src := openMigrated(t)
	raw := exportContentSnapshot(t, src)

	tgt := openMigrated(t)
	clearContent(t, tgt.db)
	applyImport(t, tgt.db, raw)

	snap := parseSnap(t, raw)
	require.NotEmpty(t, snap.Goods)
	code := snap.Goods[0].Code
	idBefore := mustInt(t, tgt.db, `SELECT id FROM goods WHERE code = $1`, code)
	countBefore := mustInt(t, tgt.db, `SELECT COUNT(*) FROM goods`)
	snap.Goods[0].Name = snap.Goods[0].Name + " (переименовано)"
	snap.Goods[0].Code = code

	rec, m := callImport(t, tgt.db, marshalSnap(t, snap), false)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.GreaterOrEqual(t, sectionCount(t, m, "updated"), 1)
	require.Equal(t, idBefore, mustInt(t, tgt.db, `SELECT id FROM goods WHERE code = $1`, code), "id изменился")
	require.Equal(t, countBefore, mustInt(t, tgt.db, `SELECT COUNT(*) FROM goods`), "число строк изменилось")
	require.Equal(t, snap.Goods[0].Name, mustStr(t, tgt.db, `SELECT name FROM goods WHERE code = $1`, code))
}

// TestContentImportUnlocks — T15: unlocks едет по метке/ключу, импорт даёт
// ссылки на локальные записи; id-форма в снимке → отказ.
func TestContentImportUnlocks(t *testing.T) {
	src := openMigrated(t)
	require.NoError(t, goodsstudio.Seed(src.db))
	require.NoError(t, goodsstudio.SeedProducers(src.db))
	// предмет-рецепт с id-формой unlocks в источнике
	ptID := mustInt(t, src.db, `SELECT id FROM producer_types ORDER BY id LIMIT 1`)
	catID := mustInt(t, src.db, `SELECT id FROM categories WHERE kind='resource' ORDER BY id LIMIT 1`)
	unlocks := fmt.Sprintf(`[{"producer_type_id":%d,"category_id":%d}]`, ptID, catID)
	if _, err := src.db.Exec(
		`INSERT INTO items (name, name_norm, slot_type, unlocks) VALUES ('Тест-рецепт','тест-рецепт импорт','модуль',$1)`,
		unlocks); err != nil {
		t.Fatalf("insert item: %v", err)
	}
	raw := exportSnapshot(t, src)
	require.False(t, bytes.Contains(raw, []byte("producer_type_id")), "id-форма утекла в снимок (T15)")

	tgt := openMigrated(t)
	clearContent(t, tgt.db)
	applyImport(t, tgt.db, raw)

	// unlocks в цели ссылается на ЛОКАЛЬНЫЕ (найденные по метке) записи
	var targetUnlocks string
	if err := tgt.db.QueryRow(`SELECT unlocks::text FROM items WHERE name = 'Тест-рецепт'`).Scan(&targetUnlocks); err != nil {
		t.Fatalf("read unlocks: %v", err)
	}
	require.NotEmpty(t, targetUnlocks)
	require.Equal(t, int64(0), mustInt(t, tgt.db,
		`SELECT COUNT(*) FROM items i, jsonb_array_elements(i.unlocks) e
		 WHERE i.name = 'Тест-рецепт'
		   AND (NOT EXISTS (SELECT 1 FROM producer_types p WHERE p.id = (e->>'producer_type_id')::bigint)
		    OR NOT EXISTS (SELECT 1 FROM categories c WHERE c.id = (e->>'category_id')::bigint))`))

	// id-форма в снимке → отказ (не молчаливый пропуск)
	snap := parseSnap(t, raw)
	for i := range snap.Items {
		if snap.Items[i].Name == "Тест-рецепт" {
			snap.Items[i].Unlocks = []handlers.ContentUnlock{{
				ProducerTypeID: int64Ptr(ptID), CategoryID: int64Ptr(catID),
			}}
		}
	}
	rec2, _ := callImport(t, tgt.db, marshalSnap(t, snap), false)
	require.Equal(t, http.StatusBadRequest, rec2.Code, rec2.Body.String())
}

func int64Ptr(v int64) *int64 { return &v }

// TestContentImportIdentityDivergence — T16 (решение В1): цель полностью
// размечена, но метки разошлись с файлом → режим bootstrap: матч по
// (kind, name_norm), метки переназначаются по файлу, id сохранены; после накатки
// последующий импорт — managed, 0/0/0.
func TestContentImportIdentityDivergence(t *testing.T) {
	src := openMigrated(t)
	raw := exportContentSnapshot(t, src)

	tgt := openMigrated(t)
	clearContent(t, tgt.db)
	applyImport(t, tgt.db, raw) // согласованная копия (managed)

	// ломаем тождество: меняем местами метки двух разных товаров
	type g struct {
		id   int64
		name string
		code string
	}
	var gs []g
	rows, err := tgt.db.Query(`SELECT id, name, code FROM goods ORDER BY id LIMIT 2`)
	require.NoError(t, err)
	for rows.Next() {
		var x g
		require.NoError(t, rows.Scan(&x.id, &x.name, &x.code))
		gs = append(gs, x)
	}
	rows.Close()
	require.Len(t, gs, 2)
	if _, err := tgt.db.Exec(`UPDATE goods SET code = NULL WHERE id IN ($1,$2)`, gs[0].id, gs[1].id); err != nil {
		t.Fatalf("null codes: %v", err)
	}
	if _, err := tgt.db.Exec(`UPDATE goods SET code = $1 WHERE id = $2`, gs[1].code, gs[0].id); err != nil {
		t.Fatalf("swap 1: %v", err)
	}
	if _, err := tgt.db.Exec(`UPDATE goods SET code = $1 WHERE id = $2`, gs[0].code, gs[1].id); err != nil {
		t.Fatalf("swap 2: %v", err)
	}

	// сухой прогон видит расхождение: bootstrap + переназначение меток
	recDry, mDry := callImport(t, tgt.db, raw, true)
	require.Equal(t, http.StatusOK, recDry.Code, recDry.Body.String())
	require.Equal(t, "bootstrap", mDry["mode"], "расхождение тождества не распознано: %v", mDry)
	require.NotZero(t, entriesLen(mDry, "remap"), "дифф не показал переназначение меток: %v", mDry)

	// применение: метки восстановлены, id сохранены
	rec, m := callImport(t, tgt.db, raw, false)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "bootstrap", m["mode"])
	require.Equal(t, gs[0].code, mustStr(t, tgt.db, `SELECT code FROM goods WHERE id = $1`, gs[0].id), "метка не впечатана")
	require.Equal(t, gs[0].name, mustStr(t, tgt.db, `SELECT name FROM goods WHERE id = $1`, gs[0].id), "id не сохранён")
	require.Equal(t, int64(0), mustInt(t, tgt.db, `SELECT COUNT(*) FROM goods WHERE code IS NULL`))

	// после накатки — managed и 0/0/0
	rec2, m2 := callImport(t, tgt.db, raw, false)
	require.Equal(t, http.StatusOK, rec2.Code, rec2.Body.String())
	require.Equal(t, "managed", m2["mode"])
	require.Equal(t, 0, sectionCount(t, m2, "created"), "повтор не 0/0/0: %v", m2)
	require.Equal(t, 0, sectionCount(t, m2, "updated"), "повтор не 0/0/0: %v", m2)
	require.Equal(t, 0, sectionCount(t, m2, "deleted"), "повтор не 0/0/0: %v", m2)
}

// TestContentImportUnlocksIdempotent — T4/T15: повторный импорт item с непустым
// `unlocks` и категорией, у которой `name != name_norm` (сырое имя в снимке,
// норма в БД), даёт 0/0/0 — сравнение unlocks устойчиво (баг ревью И3).
func TestContentImportUnlocksIdempotent(t *testing.T) {
	src := openMigrated(t)
	require.NoError(t, goodsstudio.Seed(src.db))
	require.NoError(t, goodsstudio.SeedProducers(src.db))
	ptID := mustInt(t, src.db, `SELECT id FROM producer_types ORDER BY id LIMIT 1`)
	var catID int64
	var catName, catNorm string
	if err := src.db.QueryRow(`SELECT id, name, name_norm FROM categories ORDER BY id LIMIT 1`).Scan(&catID, &catName, &catNorm); err != nil {
		t.Fatalf("read category: %v", err)
	}
	require.NotEqual(t, catName, catNorm, "тест требует категорию с name != name_norm")
	if _, err := src.db.Exec(
		`INSERT INTO items (name, name_norm, slot_type, unlocks)
		 VALUES ('Тест-рецепт unlocked','тест-рецепт unlocked импорт','модуль',
		         jsonb_build_array(jsonb_build_object('producer_type_id', $1::bigint, 'category_id', $2::bigint)))`,
		ptID, catID); err != nil {
		t.Fatalf("insert item: %v", err)
	}
	raw := exportSnapshot(t, src)

	tgt := openMigrated(t)
	clearContent(t, tgt.db)
	applyImport(t, tgt.db, raw)

	rec, m := callImport(t, tgt.db, raw, false)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, 0, sectionCount(t, m, "updated"), "повторный импорт не 0/0/0: %v", m)
	require.Equal(t, 0, sectionCount(t, m, "created"), "повторный импорт не 0/0/0: %v", m)
	require.Equal(t, 0, sectionCount(t, m, "deleted"), "повторный импорт не 0/0/0: %v", m)
}

// --- разбор/сборка снимка ---

func parseSnap(t *testing.T, raw []byte) *handlers.ContentSnapshot {
	t.Helper()
	var snap handlers.ContentSnapshot
	require.NoError(t, json.Unmarshal(raw, &snap))
	return &snap
}

func marshalSnap(t *testing.T, snap *handlers.ContentSnapshot) []byte {
	t.Helper()
	// counts пересобираем, чтобы файл остался консистентным
	snap.Counts = map[string]int{
		"categories": len(snap.Categories), "goods": len(snap.Goods),
		"recipes": len(snap.Recipes), "recipe_components": len(snap.RecipeComponents),
		"producer_types": len(snap.ProducerTypes), "items": len(snap.Items),
		"producer_slots": len(snap.ProducerSlots), "producer_recipes": len(snap.ProducerRecipes),
		"producer_items": len(snap.ProducerItems), "effect_types": len(snap.EffectTypes),
	}
	b, err := json.Marshal(snap)
	require.NoError(t, err)
	return b
}
