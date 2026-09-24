// internal/integration/content_export_test.go
// Экспорт снимка контента каталога на ЖИВОЙ PostgreSQL (спека
// 2026-09-24-каталог-экспорт-импорт-контента-на-прод §4/§7, итерация И2).
// Проверяет полный путь: сид → снимок из БД → GET-ручка → файл + скачивание.
// Мок не знает про REPEATABLE READ/JSONB/метки — здесь всё на живой БД
// (scratch-схема, helpers_test.go). Без БД тест скипается.
package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/goodsstudio"
	"zorion/internal/handlers"
	"zorion/internal/repository"
)

// TestContentExportHandlerLiveDB — снимок из реальной dev-схемы: секция items
// после producer_types и до producer_items, `code` совпадает с БД, id нет,
// ручка отдаёт файл и пишет его на диск.
func TestContentExportHandlerLiveDB(t *testing.T) {
	s := openMigrated(t)
	require.NoError(t, goodsstudio.Seed(s.db))
	require.NoError(t, goodsstudio.SeedProducers(s.db))

	repo := repository.NewGoodsRepository(s.db)
	rows, err := repo.ContentExport()
	require.NoError(t, err)
	require.Len(t, rows.Goods, 131) // базовый каталог сида
	require.NotEmpty(t, rows.ProducerTypes)
	require.NotEmpty(t, rows.Items)
	require.NotEmpty(t, rows.EffectTypes)
	require.NotZero(t, rows.DefaultTypeID)

	path := filepath.Join(t.TempDir(), "catalog.json")
	h := handlers.NewStudioHandlers(s.db, nil, "")
	h.SetContentExportPath(path)

	rec := httptest.NewRecorder()
	h.ContentExport(rec, httptest.NewRequest(http.MethodGet, "/studio/api/content/export", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	cd := rec.Header().Get("Content-Disposition")
	require.Contains(t, cd, "attachment")
	require.Contains(t, cd, "catalog-")
	require.Contains(t, cd, ".json")

	body := rec.Body.String()
	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(body), &top))
	for _, section := range []string{"categories", "goods", "recipes", "recipe_components",
		"producer_types", "items", "producer_slots", "producer_recipes", "producer_items",
		"effect_types", "generation_config"} {
		require.Containsf(t, top, section, "нет секции %s", section)
	}
	require.Equal(t, "1", string(top["schema_version"]))

	// Никаких id в файле (§4).
	require.False(t, strings.Contains(body, `"id"`), "снимок не должен нести id")

	// Топологический порядок по FK (§4): items после producer_types, до producer_items.
	// Ключи верхнего уровня (отступ 2 пробела) — вложенный counts несёт те же имена.
	require.Less(t, sectionIdx(body, "producer_types"), sectionIdx(body, "items"))
	require.Less(t, sectionIdx(body, "items"), sectionIdx(body, "producer_items"))

	// Метки в файле совпадают с БД (§4: метки совпадают).
	require.Contains(t, body, `"code": "g_0001"`)
	require.Equal(t, "g_0001", mustStr(t, s.db,
		`SELECT code FROM goods ORDER BY id LIMIT 1`))

	// Файл записан и совпадает с отданным телом.
	written, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, body, string(written))
}

// sectionIdx — позиция ключа ВЕРХНЕГО уровня (отступ 2 пробела) — отличает
// секции от одноимённых ключей вложенного counts.
func sectionIdx(body, key string) int {
	return strings.Index(body, "\n  \""+key+"\":")
}

// TestContentExportUnlocksByKeyLiveDB — `items.unlocks` из БД (id-форма) в
// снимке едет по метке/натуральному ключу (§4/T15), внутренних id в файле нет.
func TestContentExportUnlocksByKeyLiveDB(t *testing.T) {
	s := openMigrated(t)
	require.NoError(t, goodsstudio.Seed(s.db))
	require.NoError(t, goodsstudio.SeedProducers(s.db))

	// Предмет-рецепт ссылается на существующие запись-тип (метка p_0001) и
	// ресурсную категорию.
	ptID := mustInt(t, s.db, `SELECT id FROM producer_types WHERE code = 'p_0001'`)
	catID := mustInt(t, s.db, `SELECT id FROM categories WHERE kind = 'resource' ORDER BY id LIMIT 1`)
	catName := mustStr(t, s.db, `SELECT name FROM categories WHERE id = $1`, catID)
	unlocks := fmt.Sprintf(`[{"producer_type_id":%d,"category_id":%d}]`, ptID, catID)
	_, err := s.db.Exec(
		`INSERT INTO items (name, name_norm, slot_type, unlocks) VALUES ('Тест-рецепт','тест-рецепт unlocks','модуль',$1)`,
		unlocks)
	require.NoError(t, err)

	h := handlers.NewStudioHandlers(s.db, nil, "")
	h.SetContentExportPath(filepath.Join(t.TempDir(), "catalog.json"))
	rec := httptest.NewRecorder()
	h.ContentExport(rec, httptest.NewRequest(http.MethodGet, "/studio/api/content/export", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()

	// id-формы в файле нет (§4/T15).
	require.NotContains(t, body, "producer_type_id")
	require.NotContains(t, body, "category_id")

	var snap handlers.ContentSnapshot
	require.NoError(t, json.Unmarshal([]byte(body), &snap))
	var found *handlers.ContentItem
	for i := range snap.Items {
		if snap.Items[i].Name == "Тест-рецепт" {
			found = &snap.Items[i]
		}
	}
	require.NotNil(t, found, "предмет-рецепт не найден в снимке")
	require.Equal(t, []handlers.ContentUnlock{{
		Producer: "p_0001",
		Category: handlers.ContentUnlockCategory{Kind: "resource", Name: catName},
	}}, found.Unlocks)
}
