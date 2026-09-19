package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/cmd/goods-studio/ai"
	"zorion/cmd/goods-studio/config"
	"zorion/cmd/goods-studio/model"
)

// validateWarning — локальная копия validate.Warning для разбора ответа.
type validateWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// newTestServer — сервер с временным state.json и пустым ИИ-клиентом.
func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.StudioConfig{
		Port:        8799,
		DataDir:     dir,
		OpenCodeURL: "http://127.0.0.1:1", // недостижимый — fill не тестируем на успех
		Model:       "test-model",
		TimeoutS:    1,
		MaxRetries:  0,
	}
	st := model.NewState()
	statePath := filepath.Join(dir, "state.json")
	require.NoError(t, model.SaveState(statePath, st))
	srv := NewServer(cfg, st, statePath, ai.NewClient(cfg.OpenCodeURL, cfg.Model, 1, 0), []byte("<html>ui</html>"))
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return srv, ts
}

func doJSON(t *testing.T, method, url string, body interface{}) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req, err := http.NewRequest(method, url, &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// TestCreateGoodDupName — дубликат имени при создании — 409 (спека 99a.1 §6.3).
func TestCreateGoodDupName(t *testing.T) {
	_, ts := newTestServer(t)
	resp := doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "Корабль", "category_id": "c1"})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "  корабль ", "category_id": "c1"})
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	resp.Body.Close()
}

// TestCreateGoodDefaultSlot — новый товар рождается с одним пустым слотом
// (по умолчанию слот один, спека 99a.1 §5.1).
func TestCreateGoodDefaultSlot(t *testing.T) {
	_, ts := newTestServer(t)
	resp := doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "Корабль", "category_id": "c1"})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var g model.Good
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&g))
	resp.Body.Close()
	require.Len(t, g.Recipe, 1)
	require.Empty(t, g.Recipe[0].GoodID)
	require.Equal(t, model.StatusDraft, g.Status)
}

// TestSlotCycle409 — цикл при PUT слота — 409 (спека 99a.1 §6.2).
func TestSlotCycle409(t *testing.T) {
	_, ts := newTestServer(t)
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "A", "category_id": "c1"}).Body.Close()
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "B", "category_id": "c1"}).Body.Close()
	// B содержит A
	resp := doJSON(t, http.MethodPut, ts.URL+"/api/goods/g2/slots/0", map[string]string{"good_id": "g1"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	// A содержит B — цикл
	resp = doJSON(t, http.MethodPut, ts.URL+"/api/goods/g1/slots/0", map[string]string{"good_id": "g2"})
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	resp.Body.Close()
}

// TestResourceReadOnly — PUT/DELETE ресурса — 403 (спека 99a.1 §5.3, инвариант 15).
func TestResourceReadOnly(t *testing.T) {
	srv, ts := newTestServer(t)
	// добавить ресурс в состояние напрямую
	srv.mu.Lock()
	srv.state.Goods = append(srv.state.Goods, model.Good{
		ID: "res:zhelezo", Name: "Железо Fe", Category: "mineral",
		Status: model.StatusResource, Kind: model.KindResource,
		ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "zhelezo"},
	})
	srv.mu.Unlock()

	resp := doJSON(t, http.MethodPut, ts.URL+"/api/goods/res:zhelezo", map[string]string{"name": "Другое"})
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp.Body.Close()
	resp = doJSON(t, http.MethodDelete, ts.URL+"/api/goods/res:zhelezo", nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp.Body.Close()
	// статус ресурса: banned — можно
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/goods/res:zhelezo/status", map[string]string{"status": "banned"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	// approved — нельзя
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/goods/res:zhelezo/status", map[string]string{"status": "approved"})
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp.Body.Close()
}

// TestFillNoEmptySlots — fill без пустых слотов — 400 (спека 99a.1 §7.1).
func TestFillNoEmptySlots(t *testing.T) {
	srv, ts := newTestServer(t)
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "A", "category_id": "c1"}).Body.Close()
	// заполнить единственный слот
	resp := doJSON(t, http.MethodPut, ts.URL+"/api/goods/g1/slots/0", map[string]string{"good_id": "res:zhelezo"})
	require.Equal(t, http.StatusNotFound, resp.StatusCode) // ресурса нет в состоянии
	resp.Body.Close()
	// добавить ресурс и заполнить
	srv.mu.Lock()
	srv.state.Goods = append(srv.state.Goods, model.Good{
		ID: "res:zhelezo", Name: "Железо Fe", Category: "mineral",
		Status: model.StatusResource, Kind: model.KindResource,
		ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "zhelezo"},
	})
	srv.mu.Unlock()
	resp = doJSON(t, http.MethodPut, ts.URL+"/api/goods/g1/slots/0", map[string]string{"good_id": "res:zhelezo"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	resp = doJSON(t, http.MethodPost, ts.URL+"/api/goods/g1/fill", nil)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

// TestFillAlreadyGenerating — вторая генерация — 409 (TryStart, §7.1).
func TestFillAlreadyGenerating(t *testing.T) {
	srv, ts := newTestServer(t)
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "A", "category_id": "c1"}).Body.Close()
	srv.mu.Lock()
	srv.generating = true
	srv.mu.Unlock()
	resp := doJSON(t, http.MethodPost, ts.URL+"/api/goods/g1/fill", nil)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	resp.Body.Close()
}

// TestCategoryDupAndNotEmpty — дубликат категории — 409; удаление непустой
// категории — 409 (спека 99a.1 §9).
func TestCategoryDupAndNotEmpty(t *testing.T) {
	_, ts := newTestServer(t)
	resp := doJSON(t, http.MethodPost, ts.URL+"/api/categories", map[string]string{"name": "продовольствие"})
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	resp.Body.Close()
	// создать товар в c1 и попытаться удалить c1
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "Корабль", "category_id": "c1"}).Body.Close()
	resp = doJSON(t, http.MethodDelete, ts.URL+"/api/categories/c1", nil)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	resp.Body.Close()
}

// TestStateView — GET /api/state: товары с тирами, banned, unused, warnings.
func TestStateView(t *testing.T) {
	srv, ts := newTestServer(t)
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "Корабль", "category_id": "c1"}).Body.Close()
	srv.mu.Lock()
	srv.state.Goods = append(srv.state.Goods,
		model.Good{ID: "res:zhelezo", Name: "Железо Fe", Category: "mineral", Status: model.StatusResource, Kind: model.KindResource,
			ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "zhelezo"}},
		model.Good{ID: "g2", Name: "Забаненный", Category: "c1", Status: model.StatusBanned, Kind: model.KindGood},
	)
	srv.mu.Unlock()
	resp := doJSON(t, http.MethodGet, ts.URL+"/api/state", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var view StateView
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&view))
	resp.Body.Close()
	require.Equal(t, "test-model", view.Model)
	require.Len(t, view.Goods, 3)
	require.Len(t, view.Banned, 1)
	require.Equal(t, "Забаненный", view.Banned[0].Name)
	// unused: Корабль (входящая степень 0, не забанен, не ресурс)
	require.Len(t, view.Unused, 1)
	require.Equal(t, "Корабль", view.Unused[0].Name)
}

// TestExportWritesFile — POST /api/export пишет export.json (дефолтный путь).
func TestExportWritesFile(t *testing.T) {
	srv, ts := newTestServer(t)
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "Корабль", "category_id": "c1"}).Body.Close()
	srv.mu.Lock()
	srv.state.Goods[0].Status = model.StatusApproved
	srv.mu.Unlock()
	resp := doJSON(t, http.MethodPost, ts.URL+"/api/export", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out struct {
		Path     string `json:"path"`
		Warnings []struct {
			Code string `json:"code"`
		} `json:"warnings"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	resp.Body.Close()
	require.Equal(t, filepath.Join(srv.cfg.DataDir, "export.json"), out.Path)
	// файл существует и валиден
	data, err := os.ReadFile(out.Path)
	require.NoError(t, err)
	var ex map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &ex))
	require.Equal(t, float64(1), ex["schema_version"])
}

// TestValidateEndpoint — GET /api/validate возвращает warnings.
func TestValidateEndpoint(t *testing.T) {
	srv, ts := newTestServer(t)
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "Корабль", "category_id": "c1"}).Body.Close()
	// согласованный товар без рецепта → неполная цепочка
	srv.mu.Lock()
	srv.state.Goods[0].Status = model.StatusApproved
	srv.mu.Unlock()
	resp := doJSON(t, http.MethodGet, ts.URL+"/api/validate", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var warnings []validateWarning
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&warnings))
	resp.Body.Close()
	require.NotEmpty(t, warnings)
	require.Equal(t, "incomplete_chain", warnings[0].Code)
}

// TestSlotAllowResourceToggle — PUT /api/goods/{id}/slots/{n}/allow_resource:
// галка «заполнять ресурсом» переключается и видна в /api/state
// (99a Пакет 4, п.8). Несуществующий слот — 404.
func TestSlotAllowResourceToggle(t *testing.T) {
	_, ts := newTestServer(t)
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "Корабль", "category_id": "c1"}).Body.Close()
	// включить галку на слоте 0
	resp := doJSON(t, http.MethodPut, ts.URL+"/api/goods/g1/slots/0/allow_resource", map[string]bool{"allow_resource": true})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	// видна в /api/state
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/state", nil)
	var view StateView
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&view))
	resp.Body.Close()
	require.True(t, view.Goods[0].Recipe[0].AllowResource)
	// выключить
	resp = doJSON(t, http.MethodPut, ts.URL+"/api/goods/g1/slots/0/allow_resource", map[string]bool{"allow_resource": false})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/state", nil)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&view))
	resp.Body.Close()
	require.False(t, view.Goods[0].Recipe[0].AllowResource)
	// несуществующий слот — 404
	resp = doJSON(t, http.MethodPut, ts.URL+"/api/goods/g1/slots/5/allow_resource", map[string]bool{"allow_resource": true})
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// TestDeleteGoodClearsSlots — удаление товара очищает слоты, ссылающиеся
// на него (спека 99a.1 §9).
func TestDeleteGoodClearsSlots(t *testing.T) {
	_, ts := newTestServer(t)
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "A", "category_id": "c1"}).Body.Close()
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "B", "category_id": "c1"}).Body.Close()
	doJSON(t, http.MethodPut, ts.URL+"/api/goods/g2/slots/0", map[string]string{"good_id": "g1"}).Body.Close()
	resp := doJSON(t, http.MethodDelete, ts.URL+"/api/goods/g1", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	// слот g2 очищен
	resp = doJSON(t, http.MethodGet, ts.URL+"/api/state", nil)
	var view StateView
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&view))
	resp.Body.Close()
	for _, g := range view.Goods {
		if g.ID == "g2" {
			require.Empty(t, g.Recipe[0].GoodID)
		}
	}
}

// --- импорт top-каталога (99a.2 §7) ---

const testCatalogJSON = `{
  "schema_version": 1,
  "catalog": "top-goods",
  "generated_at": "2026-09-19T12:00:00Z",
  "summary": {"total": 2, "per_category": {}, "demand": {"continuous": 2, "spike": 0}},
  "goods": [
    {"id": "tg_0001", "name": "Стейк", "category": "продовольствие", "why_top": "статусный стол",
     "constituents": [
       {"name": "Белки", "type": "resource"},
       {"name": "ароматический комплекс", "type": "good"}
     ]},
    {"id": "tg_0002", "name": "Паёк", "category": "продовольствие", "why_top": "стандарт флота",
     "constituents": [
       {"name": "Питательная паста", "type": "good"},
       {"name": "Белки", "type": "resource"}
     ]}
  ]
}`

// importServer — сервер с ресурсами по именам фикстуры и черновиком.
func importServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	srv, ts := newTestServer(t)
	srv.mu.Lock()
	srv.state.Goods = append(srv.state.Goods,
		model.Good{ID: "res:belki", Name: "Белки", Category: "mineral", Status: model.StatusResource, Kind: model.KindResource,
			ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "belki"}},
		model.Good{ID: "g1", Name: "Черновик", Category: "c1", Status: model.StatusDraft, Kind: model.KindGood, Source: model.SourceManual},
	)
	srv.persist() // бэкап берётся из файла — состояние должно быть персистнуто
	srv.mu.Unlock()
	return srv, ts
}

// TestImportEndpoint200 — POST /api/import с фикстурой → 200, отчёт с
// числами, состояние: топ-товары approved, внешние компоненты draft,
// ресурсы сохранены, черновик снесён.
func TestImportEndpoint200(t *testing.T) {
	srv, ts := importServer(t)
	catPath := filepath.Join(t.TempDir(), "top_catalog.json")
	require.NoError(t, os.WriteFile(catPath, []byte(testCatalogJSON), 0644))

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/import", map[string]string{"path": catPath})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out struct {
		Report struct {
			Imported          int      `json:"imported"`
			ComponentsCreated int      `json:"components_created"`
			GoodsRemoved      int      `json:"goods_removed"`
			CategoriesCreated int      `json:"categories_created"`
			Backup            string   `json:"backup"`
			Warnings          []string `json:"warnings"`
		} `json:"report"`
		State StateView `json:"state"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	resp.Body.Close()
	require.Equal(t, 2, out.Report.Imported)
	require.Equal(t, 2, out.Report.ComponentsCreated)
	require.Equal(t, 1, out.Report.GoodsRemoved)
	require.Equal(t, 0, out.Report.CategoriesCreated)
	require.Equal(t, srv.statePath+".bak", out.Report.Backup)
	require.Len(t, out.Report.Warnings, 4) // 2× missing_resource + 2× non_approved_ref

	byID := map[string]GoodView{}
	for _, g := range out.State.Goods {
		byID[g.ID] = g
	}
	require.Equal(t, "approved", byID["tg_0001"].Status)
	require.Equal(t, "draft", byID["ext:ароматический-комплекс"].Status)
	require.Equal(t, "resource", byID["res:belki"].Status)
	require.NotContains(t, byID, "g1")
	// слоты топ-товара: quantity=1, резолв по имени
	require.Equal(t, "res:belki", byID["tg_0001"].Recipe[0].GoodID)
	require.Equal(t, 1, byID["tg_0001"].Recipe[0].Quantity)
	require.Equal(t, "ext:ароматический-комплекс", byID["tg_0001"].Recipe[1].GoodID)
}

// TestImportEndpoint404 — файл не найден → 404.
func TestImportEndpoint404(t *testing.T) {
	_, ts := importServer(t)
	resp := doJSON(t, http.MethodPost, ts.URL+"/api/import", map[string]string{"path": filepath.Join(t.TempDir(), "nope.json")})
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	var e map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&e))
	resp.Body.Close()
	require.Contains(t, e["error"], "файл не найден")
}

// TestImportEndpoint400 — битый файл → 400, состояние не меняется.
func TestImportEndpoint400(t *testing.T) {
	srv, ts := importServer(t)
	catPath := filepath.Join(t.TempDir(), "top_catalog.json")
	require.NoError(t, os.WriteFile(catPath, []byte(`{"schema_version":1,"goods":[`), 0644))
	before := len(srv.state.Goods)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/import", map[string]string{"path": catPath})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
	require.Equal(t, before, len(srv.state.Goods), "состояние не меняется при 400")
}

// TestImportEndpoint409 — активная генерация → 409, состояние не меняется.
func TestImportEndpoint409(t *testing.T) {
	srv, ts := importServer(t)
	catPath := filepath.Join(t.TempDir(), "top_catalog.json")
	require.NoError(t, os.WriteFile(catPath, []byte(testCatalogJSON), 0644))
	srv.mu.Lock()
	srv.generating = true
	srv.mu.Unlock()
	before := len(srv.state.Goods)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/import", map[string]string{"path": catPath})
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	resp.Body.Close()
	require.Equal(t, before, len(srv.state.Goods), "состояние не меняется при 409")
}

// TestImportBackupCreated — перед заменой создаётся state.json.bak
// с прежним состоянием (99a.2 §4.1, инвариант 5).
func TestImportBackupCreated(t *testing.T) {
	srv, ts := importServer(t)
	catPath := filepath.Join(t.TempDir(), "top_catalog.json")
	require.NoError(t, os.WriteFile(catPath, []byte(testCatalogJSON), 0644))

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/import", map[string]string{"path": catPath})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	raw, err := os.ReadFile(srv.statePath + ".bak")
	require.NoError(t, err)
	var bak model.State
	require.NoError(t, json.Unmarshal(raw, &bak))
	// бэкап — прежнее состояние: черновик g1 на месте, топ-товаров нет
	ids := map[string]bool{}
	for _, g := range bak.Goods {
		ids[g.ID] = true
	}
	require.True(t, ids["g1"])
	require.False(t, ids["tg_0001"])
}

// --- количество в слотах (99a.2 §6) ---

// TestSlotQuantityUpdate — PUT {quantity} обновляет; PUT без quantity не
// сбрасывает; quantity < 1 трактуется как 1.
func TestSlotQuantityUpdate(t *testing.T) {
	_, ts := newTestServer(t)
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "A", "category_id": "c1"}).Body.Close()
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "B", "category_id": "c1"}).Body.Close()
	// новый слот по умолчанию quantity=1
	resp := doJSON(t, http.MethodGet, ts.URL+"/api/state", nil)
	var view StateView
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&view))
	resp.Body.Close()
	require.Equal(t, 1, view.Goods[0].Recipe[0].Quantity)

	// PUT с quantity=3
	resp = doJSON(t, http.MethodPut, ts.URL+"/api/goods/g1/slots/0", map[string]interface{}{"good_id": "g2", "quantity": 3})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	// PUT без quantity — количество сохраняется (замена составляющей не сбрасывает)
	resp = doJSON(t, http.MethodPut, ts.URL+"/api/goods/g1/slots/0", map[string]string{"good_id": "g2"})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	// quantity < 1 → 1
	resp = doJSON(t, http.MethodPut, ts.URL+"/api/goods/g1/slots/0", map[string]interface{}{"good_id": "g2", "quantity": 0})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/state", nil)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&view))
	resp.Body.Close()
	require.Equal(t, 1, view.Goods[0].Recipe[0].Quantity)
}

// TestSlotQuantityInState — GET /api/state отдаёт quantity слота (≥ 1).
func TestSlotQuantityInState(t *testing.T) {
	_, ts := newTestServer(t)
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "A", "category_id": "c1"}).Body.Close()
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "B", "category_id": "c1"}).Body.Close()
	doJSON(t, http.MethodPut, ts.URL+"/api/goods/g1/slots/0", map[string]interface{}{"good_id": "g2", "quantity": 7}).Body.Close()
	resp := doJSON(t, http.MethodGet, ts.URL+"/api/state", nil)
	var view StateView
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&view))
	resp.Body.Close()
	require.Equal(t, 7, view.Goods[0].Recipe[0].Quantity)
}

// TestSlotQuantityOnly — PUT {quantity} без good_id (99a.2 §6.5: инпут
// количества шлёт только quantity): количество обновляется, составляющая
// не меняется.
func TestSlotQuantityOnly(t *testing.T) {
	_, ts := newTestServer(t)
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "A", "category_id": "c1"}).Body.Close()
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "B", "category_id": "c1"}).Body.Close()
	doJSON(t, http.MethodPut, ts.URL+"/api/goods/g1/slots/0", map[string]string{"good_id": "g2"}).Body.Close()

	resp := doJSON(t, http.MethodPut, ts.URL+"/api/goods/g1/slots/0", map[string]interface{}{"quantity": 4})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	resp = doJSON(t, http.MethodGet, ts.URL+"/api/state", nil)
	var view StateView
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&view))
	resp.Body.Close()
	require.Equal(t, "g2", view.Goods[0].Recipe[0].GoodID, "составляющая не меняется")
	require.Equal(t, 4, view.Goods[0].Recipe[0].Quantity)
}

// --- импорт деревьев Т8 (schema_version 2) ---

// testTreesJSON — малая фикстура деревьев (2 дерева, общая база s_0001,
// quantity в слотах, ресурсы по именам; все ref замкнуты, тиры сходятся).
const testTreesJSON = `{
  "schema_version": 2,
  "catalog": "top-goods-trees-t8",
  "generated_at": "2026-09-19T18:00:00Z",
  "rework": "тест",
  "trees": [
    {
      "root": {"name": "Топливо «Гелиос»", "tier": 3, "category": "топливо"},
      "nodes": [
        {"id":"h_0001","name":"Топливо «Гелиос»","tier":3,"category":"топливо","constituents":[
          {"ref":"h_0002","name":"Сборки","type":"good"},
          {"name":"Уран-оксид UO₂","type":"resource"}
        ]},
        {"id":"h_0002","name":"Сборки","tier":2,"category":"комплектующие","constituents":[
          {"ref":"s_0001","name":"База","type":"good","quantity":4},
          {"name":"Плутоний Pu","type":"resource"}
        ]},
        {"id":"s_0001","name":"База","tier":1,"category":"комплектующие","constituents":[
          {"name":"Глина","type":"resource"},
          {"name":"Вода H₂O","type":"resource"}
        ]}
      ],
      "leaves_resources": ["Уран-оксид UO₂","Плутоний Pu","Глина","Вода H₂O"]
    },
    {
      "root": {"name": "Микропроцессор «Кремний-9»", "tier": 3, "category": "комплектующие"},
      "nodes": [
        {"id":"k_0001","name":"Микропроцессор «Кремний-9»","tier":3,"category":"комплектующие","constituents":[
          {"ref":"k_0002","name":"Ядра","type":"good"},
          {"name":"Кремний Si","type":"resource"}
        ]},
        {"id":"k_0002","name":"Ядра","tier":2,"category":"комплектующие","constituents":[
          {"ref":"s_0001","name":"База","type":"good","quantity":6},
          {"name":"Графит C","type":"resource"}
        ]},
        {"id":"s_0001","name":"База","tier":1,"category":"комплектующие","constituents":[
          {"name":"Глина","type":"resource"},
          {"name":"Вода H₂O","type":"resource"}
        ]}
      ],
      "leaves_resources": ["Кремний Si","Графит C","Глина","Вода H₂O"]
    }
  ],
  "pool": {"note": "не импортируется"},
  "summary": {"trees": 2}
}`

// treesImportServer — сервер с ресурсами по именам фикстуры деревьев и черновиком.
func treesImportServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	srv, ts := newTestServer(t)
	srv.mu.Lock()
	srv.state.Goods = append(srv.state.Goods,
		model.Good{ID: "res:uran-oksid", Name: "Уран-оксид UO₂", Category: "mineral", Status: model.StatusResource, Kind: model.KindResource,
			ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "uran-oksid"}},
		model.Good{ID: "res:plutoniy", Name: "Плутоний Pu", Category: "mineral", Status: model.StatusResource, Kind: model.KindResource,
			ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "plutoniy"}},
		model.Good{ID: "res:glina", Name: "Глина", Category: "mineral", Status: model.StatusResource, Kind: model.KindResource,
			ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "glina"}},
		model.Good{ID: "res:voda", Name: "Вода H₂O", Category: "mineral", Status: model.StatusResource, Kind: model.KindResource,
			ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "voda"}},
		model.Good{ID: "res:kremniy", Name: "Кремний Si", Category: "mineral", Status: model.StatusResource, Kind: model.KindResource,
			ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "kremniy"}},
		model.Good{ID: "res:grafit", Name: "Графит C", Category: "mineral", Status: model.StatusResource, Kind: model.KindResource,
			ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "grafit"}},
		model.Good{ID: "g1", Name: "Черновик", Category: "c1", Status: model.StatusDraft, Kind: model.KindGood, Source: model.SourceManual},
	)
	srv.persist() // бэкап берётся из файла — состояние должно быть персистнуто
	srv.mu.Unlock()
	return srv, ts
}

// TestImportTreesEndpoint200 — POST /api/import с фикстурой деревьев → 200,
// отчёт: деревья/узлы/база/снесено/категории; состояние: узлы approved,
// дедуп базы, quantity из файла, ресурсы сохранены, черновик снесён.
func TestImportTreesEndpoint200(t *testing.T) {
	srv, ts := treesImportServer(t)
	treesPath := filepath.Join(t.TempDir(), "trees_t8.json")
	require.NoError(t, os.WriteFile(treesPath, []byte(testTreesJSON), 0644))

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/import", map[string]string{"path": treesPath})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out struct {
		Report struct {
			Imported          int      `json:"imported"`
			ComponentsCreated int      `json:"components_created"`
			GoodsRemoved      int      `json:"goods_removed"`
			CategoriesCreated int      `json:"categories_created"`
			Backup            string   `json:"backup"`
			Warnings          []string `json:"warnings"`
			Trees             int      `json:"trees"`
			NodesCreated      int      `json:"nodes_created"`
			BaseDeduplicated  int      `json:"base_deduplicated"`
		} `json:"report"`
		State StateView `json:"state"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	resp.Body.Close()
	require.Equal(t, 2, out.Report.Trees)
	require.Equal(t, 5, out.Report.NodesCreated)
	require.Equal(t, 5, out.Report.Imported)
	require.Equal(t, 1, out.Report.BaseDeduplicated)
	require.Equal(t, 1, out.Report.GoodsRemoved)
	require.Equal(t, 0, out.Report.CategoriesCreated)
	require.Equal(t, srv.statePath+".bak", out.Report.Backup)
	require.Empty(t, out.Report.Warnings) // тиры сходятся, валидаторы чисты

	byID := map[string]GoodView{}
	for _, g := range out.State.Goods {
		byID[g.ID] = g
	}
	require.Equal(t, "approved", byID["h_0001"].Status)
	require.Equal(t, "approved", byID["s_0001"].Status)
	require.Equal(t, "resource", byID["res:uran-oksid"].Status)
	require.NotContains(t, byID, "g1")
	// дедуп базы: s_0001 один раз, рецепт без дублей слотов
	require.Equal(t, "База", byID["s_0001"].Name)
	require.Len(t, byID["s_0001"].Recipe, 2) // Глина + Вода H₂O, без дублей от второго дерева
	// quantity из файла и без quantity → 1
	require.Equal(t, "s_0001", byID["h_0002"].Recipe[0].GoodID)
	require.Equal(t, 4, byID["h_0002"].Recipe[0].Quantity)
	require.Equal(t, "h_0002", byID["h_0001"].Recipe[0].GoodID)
	require.Equal(t, 1, byID["h_0001"].Recipe[0].Quantity)
	require.Equal(t, "res:uran-oksid", byID["h_0001"].Recipe[1].GoodID)
}

// TestImportTreesEndpoint404 — файл не найден → 404.
func TestImportTreesEndpoint404(t *testing.T) {
	_, ts := treesImportServer(t)
	resp := doJSON(t, http.MethodPost, ts.URL+"/api/import", map[string]string{"path": filepath.Join(t.TempDir(), "nope.json")})
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	var e map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&e))
	resp.Body.Close()
	require.Contains(t, e["error"], "файл не найден")
}

// TestImportTreesEndpoint400 — битый файл → 400, состояние не меняется.
func TestImportTreesEndpoint400(t *testing.T) {
	srv, ts := treesImportServer(t)
	treesPath := filepath.Join(t.TempDir(), "trees_t8.json")
	require.NoError(t, os.WriteFile(treesPath, []byte(`{"schema_version":2,"trees":[`), 0644))
	before := len(srv.state.Goods)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/import", map[string]string{"path": treesPath})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
	require.Equal(t, before, len(srv.state.Goods), "состояние не меняется при 400")
}

// TestImportTreesEndpoint409 — активная генерация → 409, состояние не меняется.
func TestImportTreesEndpoint409(t *testing.T) {
	srv, ts := treesImportServer(t)
	treesPath := filepath.Join(t.TempDir(), "trees_t8.json")
	require.NoError(t, os.WriteFile(treesPath, []byte(testTreesJSON), 0644))
	srv.mu.Lock()
	srv.generating = true
	srv.mu.Unlock()
	before := len(srv.state.Goods)

	resp := doJSON(t, http.MethodPost, ts.URL+"/api/import", map[string]string{"path": treesPath})
	require.Equal(t, http.StatusConflict, resp.StatusCode)
	resp.Body.Close()
	require.Equal(t, before, len(srv.state.Goods), "состояние не меняется при 409")
}

// TestImportTreesDefaultPath — POST /api/import без path → дефолт
// goods_data/trees_t8.json (в тесте — файл в DataDir).
func TestImportTreesDefaultPath(t *testing.T) {
	srv, ts := treesImportServer(t)
	require.NoError(t, os.WriteFile(filepath.Join(srv.cfg.DataDir, "trees_t8.json"), []byte(testTreesJSON), 0644))
	resp := doJSON(t, http.MethodPost, ts.URL+"/api/import", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

// --- тир-оверрайд (99a.3 §9.1) ---

// tierView — GoodView по id из /api/state.
func tierView(t *testing.T, ts *httptest.Server, id string) GoodView {
	t.Helper()
	resp := doJSON(t, http.MethodGet, ts.URL+"/api/state", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var view StateView
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&view))
	resp.Body.Close()
	for _, g := range view.Goods {
		if g.ID == id {
			return g
		}
	}
	t.Fatalf("товар %s не найден в /api/state", id)
	return GoodView{}
}

// TestTierOverrideSet — PUT tier 7: tier_override=7, tier=7 (эффективный),
// tier_computed=вычисленный.
func TestTierOverrideSet(t *testing.T) {
	_, ts := newTestServer(t)
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "A", "category_id": "c1"}).Body.Close()
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "B", "category_id": "c1"}).Body.Close()
	// B содержит A → вычисленный тир B = 1
	doJSON(t, http.MethodPut, ts.URL+"/api/goods/g2/slots/0", map[string]string{"good_id": "g1"}).Body.Close()

	resp := doJSON(t, http.MethodPut, ts.URL+"/api/goods/g2/tier", map[string]interface{}{"tier": 7})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
	gv := tierView(t, ts, "g2")
	require.Equal(t, 7, gv.Tier)
	require.Equal(t, 1, gv.TierComputed)
	require.NotNil(t, gv.TierOverride)
	require.Equal(t, 7, *gv.TierOverride)
}

// TestTierOverrideClear — PUT tier null: оверрайд снят, tier = вычисленный.
func TestTierOverrideClear(t *testing.T) {
	_, ts := newTestServer(t)
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "A", "category_id": "c1"}).Body.Close()
	doJSON(t, http.MethodPut, ts.URL+"/api/goods/g1/tier", map[string]interface{}{"tier": 5}).Body.Close()
	doJSON(t, http.MethodPut, ts.URL+"/api/goods/g1/tier", map[string]interface{}{"tier": nil}).Body.Close()
	gv := tierView(t, ts, "g1")
	require.Equal(t, 0, gv.Tier) // пустой рецепт → вычисленный 0
	require.Nil(t, gv.TierOverride)
}

// TestTierOverrideNegative400 — тир < 0 → 400.
func TestTierOverrideNegative400(t *testing.T) {
	_, ts := newTestServer(t)
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "A", "category_id": "c1"}).Body.Close()
	resp := doJSON(t, http.MethodPut, ts.URL+"/api/goods/g1/tier", map[string]interface{}{"tier": -1})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}

// TestTierOverrideResource403 — ресурс read-only → 403.
func TestTierOverrideResource403(t *testing.T) {
	srv, ts := newTestServer(t)
	srv.mu.Lock()
	srv.state.Goods = append(srv.state.Goods, model.Good{
		ID: "res:zhelezo", Name: "Железо Fe", Category: "mineral",
		Status: model.StatusResource, Kind: model.KindResource,
		ResourceRef: &model.ResourceRef{Catalog: "real", CatalogID: "zhelezo"},
	})
	srv.mu.Unlock()
	resp := doJSON(t, http.MethodPut, ts.URL+"/api/goods/res:zhelezo/tier", map[string]interface{}{"tier": 3})
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp.Body.Close()
}

// TestTierOverrideNotFound404 — неизвестный id → 404.
func TestTierOverrideNotFound404(t *testing.T) {
	_, ts := newTestServer(t)
	resp := doJSON(t, http.MethodPut, ts.URL+"/api/goods/nope/tier", map[string]interface{}{"tier": 3})
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

// TestTierOverrideSlotView — SlotView.Tier = эффективный тир составляющей
// (99a.3 §9.3): оверрайд составляющей виден в слоте родителя.
func TestTierOverrideSlotView(t *testing.T) {
	_, ts := newTestServer(t)
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "A", "category_id": "c1"}).Body.Close()
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "B", "category_id": "c1"}).Body.Close()
	doJSON(t, http.MethodPut, ts.URL+"/api/goods/g2/slots/0", map[string]string{"good_id": "g1"}).Body.Close()
	doJSON(t, http.MethodPut, ts.URL+"/api/goods/g1/tier", map[string]interface{}{"tier": 4}).Body.Close()
	gv := tierView(t, ts, "g2")
	require.Equal(t, 4, gv.Recipe[0].Tier) // эффективный тир A в слоте B
}

// TestFillPromptUsesComputedTier — промпт «заполнить комплектующие» получает
// ВЫЧИСЛЕННЫЙ тир: оверрайд не влияет на генерацию (99a.3 §4.2, критик №10).
func TestFillPromptUsesComputedTier(t *testing.T) {
	srv, ts := newTestServer(t)
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "A", "category_id": "c1"}).Body.Close()
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "B", "category_id": "c1"}).Body.Close()
	doJSON(t, http.MethodPut, ts.URL+"/api/goods/g2/slots/0", map[string]string{"good_id": "g1"}).Body.Close()
	// оверрайд 9 на B — вычисленный тир B = 1 (A с пустым рецептом = 0)
	doJSON(t, http.MethodPut, ts.URL+"/api/goods/g2/tier", map[string]interface{}{"tier": 9}).Body.Close()
	srv.mu.RLock()
	prompt := srv.buildFillPrompt(1) // g2
	srv.mu.RUnlock()
	require.Contains(t, prompt, "тир: 1")
	require.NotContains(t, prompt, "тир: 9")
}

// --- подгрузка списка (99a.3 §9.2) ---

// bulkReportTest — локальная копия отчёта bulk для разбора ответа.
type bulkReportTest struct {
	Created []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"created"`
	Skipped []struct {
		Line   int    `json:"line"`
		Name   string `json:"name"`
		Reason string `json:"reason"`
	} `json:"skipped"`
	Errors []struct {
		Line   int    `json:"line"`
		Reason string `json:"reason"`
	} `json:"errors"`
}

func doBulk(t *testing.T, ts *httptest.Server, lines []string) (int, bulkReportTest) {
	t.Helper()
	resp := doJSON(t, http.MethodPost, ts.URL+"/api/goods/bulk", map[string]interface{}{"lines": lines})
	var rep bulkReportTest
	json.NewDecoder(resp.Body).Decode(&rep)
	resp.Body.Close()
	return resp.StatusCode, rep
}

// TestBulkCreate — валидные строки создаются: draft, один пустой слот,
// quantity 1, source=manual, категория по имени.
func TestBulkCreate(t *testing.T) {
	_, ts := newTestServer(t)
	status, rep := doBulk(t, ts, []string{"Стальной каркас | конструкционные материалы", "Болт М8 | детали"})
	require.Equal(t, http.StatusOK, status)
	require.Len(t, rep.Created, 2)
	require.Empty(t, rep.Skipped)
	require.Empty(t, rep.Errors)
	gv := tierView(t, ts, rep.Created[0].ID)
	require.Equal(t, "draft", gv.Status)
	require.Equal(t, "manual", gv.Source)
	require.Len(t, gv.Recipe, 1)
	require.Empty(t, gv.Recipe[0].GoodID)
	require.Equal(t, 1, gv.Recipe[0].Quantity)
	// категория по нормализованному имени (c3 = конструкционные материалы)
	require.Equal(t, "c3", gv.Category)
}

// TestBulkCategoryNotFound — ненайденная категория — ошибка строки с номером.
func TestBulkCategoryNotFound(t *testing.T) {
	_, ts := newTestServer(t)
	status, rep := doBulk(t, ts, []string{"X | футур-материалы"})
	require.Equal(t, http.StatusOK, status)
	require.Empty(t, rep.Created)
	require.Len(t, rep.Errors, 1)
	require.Equal(t, 1, rep.Errors[0].Line)
	require.Contains(t, rep.Errors[0].Reason, "категория не найдена")
}

// TestBulkDuplicate — дубликат (существующий и в пачке) — пропуск с
// предупреждением, не ошибка.
func TestBulkDuplicate(t *testing.T) {
	_, ts := newTestServer(t)
	doJSON(t, http.MethodPost, ts.URL+"/api/goods", map[string]string{"name": "Стальной каркас", "category_id": "c1"}).Body.Close()
	status, rep := doBulk(t, ts, []string{"Стальной каркас | детали", "Болт | детали", "болт | детали"})
	require.Equal(t, http.StatusOK, status)
	require.Len(t, rep.Created, 1) // только «Болт»
	require.Len(t, rep.Skipped, 2) // существующий + дубликат в пачке
	require.Empty(t, rep.Errors)
	require.Equal(t, 1, rep.Skipped[0].Line)
	require.Equal(t, 3, rep.Skipped[1].Line)
	require.Contains(t, rep.Skipped[0].Reason, "уже есть")
}

// TestBulkEmptyLineIgnored — пустые строки пропускаются молча и не нумеруются
// (номера остальных — физические индексы в textarea, 1-based).
func TestBulkEmptyLineIgnored(t *testing.T) {
	_, ts := newTestServer(t)
	status, rep := doBulk(t, ts, []string{"", "Болт | детали", "", "X | футур-материалы", ""})
	require.Equal(t, http.StatusOK, status)
	require.Len(t, rep.Created, 1)
	require.Len(t, rep.Errors, 1)
	require.Equal(t, 4, rep.Errors[0].Line) // физический индекс, пустые не нумеруются
}

// TestBulkNoSeparator — нет разделителя «|» — ошибка строки.
func TestBulkNoSeparator(t *testing.T) {
	_, ts := newTestServer(t)
	status, rep := doBulk(t, ts, []string{"просто имя"})
	require.Equal(t, http.StatusOK, status)
	require.Len(t, rep.Errors, 1)
	require.Equal(t, 1, rep.Errors[0].Line)
	require.Contains(t, rep.Errors[0].Reason, "разделителя")
}

// TestBulkEmptyNameCategory — пустые имя/категория — ошибки строк.
func TestBulkEmptyNameCategory(t *testing.T) {
	_, ts := newTestServer(t)
	status, rep := doBulk(t, ts, []string{" | детали", "Болт | "})
	require.Equal(t, http.StatusOK, status)
	require.Len(t, rep.Errors, 2)
	require.Contains(t, rep.Errors[0].Reason, "пустое имя")
	require.Contains(t, rep.Errors[1].Reason, "пустая категория")
}

// TestBulkFirstSeparator — разделитель — первый «|» (99a.3 §9.2, вердикт
// критика М2): всё после первого «|» — категория (остальные «|» в ней).
func TestBulkFirstSeparator(t *testing.T) {
	_, ts := newTestServer(t)
	status, rep := doBulk(t, ts, []string{"Болт | детали", "Кислота A | B | химикаты"})
	require.Equal(t, http.StatusOK, status)
	require.Len(t, rep.Created, 1)
	require.Equal(t, "Болт", rep.Created[0].Name)
	// «Кислота A | B | химикаты»: имя «Кислота A», категория «B | химикаты» —
	// не найдена → ошибка строки (первый «|» — разделитель)
	require.Len(t, rep.Errors, 1)
	require.Equal(t, 2, rep.Errors[0].Line)
	require.Contains(t, rep.Errors[0].Reason, "категория не найдена")
}

// TestBulkPartialSuccess — частичный успех: валидные создаются, проблемные —
// в отчёте, состояние не откатывается.
func TestBulkPartialSuccess(t *testing.T) {
	_, ts := newTestServer(t)
	status, rep := doBulk(t, ts, []string{"Болт | детали", "X | футур-материалы", "Гайка | детали"})
	require.Equal(t, http.StatusOK, status)
	require.Len(t, rep.Created, 2)
	require.Len(t, rep.Errors, 1)
	require.Equal(t, 2, rep.Errors[0].Line)
}

// TestBulkEmptyLines400 — lines пуст → 400.
func TestBulkEmptyLines400(t *testing.T) {
	_, ts := newTestServer(t)
	resp := doJSON(t, http.MethodPost, ts.URL+"/api/goods/bulk", map[string]interface{}{"lines": []string{}})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
}