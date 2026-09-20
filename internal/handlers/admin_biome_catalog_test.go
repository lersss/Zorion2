// internal/handlers/admin_biome_catalog_test.go
// Тесты ручек справочника биомов (99.2.28 §22): GET/PATCH /admin/biome-catalog
// (валидация инварианта 17, атомарная запись файла, hot-reload store),
// POST /admin/biome-catalog/reset (сброс к сиду), GET/PATCH
// /admin/planet-archetypes (полосы климатов), права ролей (правка — admin,
// чтение — admin + skycomposer).
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"zorion/internal/auth"
	"zorion/internal/generator/planet"
)

// withRole — кладёт роль в контекст, как AuthMiddleware (объявлена в
// visibility_handlers_test.go — здесь не дублируется).

// biomeCatalogHarness — temp-файлы справочника и полос + загрузка в store.
// Возвращает пути; t.Cleanup восстанавливает сид справочника.
func biomeCatalogHarness(t *testing.T) (catalogPath, archetypesPath string) {
	t.Helper()
	dir := t.TempDir()

	catalogPath = filepath.Join(dir, "biome_catalog.json")
	seed := planet.SeedBiomeCatalog()
	data, err := json.MarshalIndent(seed, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(catalogPath, data, 0o644))
	require.NoError(t, planet.LoadBiomeCatalog(catalogPath))
	t.Cleanup(planet.ResetBiomeCatalogToSeed)

	archetypesPath = filepath.Join(dir, "planet_archetypes.json")
	arch := planet.ArchetypeConfig{Climates: []planet.ClimateConfig{
		{ID: "умеренный", Name: "Умеренный",
			BaseSurface:         map[string]float64{"горы": 0.5},
			BaseSubterrain:      map[string]float64{"пустая_порода": 0.5},
			AllowedHydrospheres: []string{"океаны"},
			AllowedAtmospheres:  []string{"азотно-кислородная"},
			AllowedBiospheres:   []string{"растительная"},
		},
	}}
	archData, err := json.MarshalIndent(arch, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(archetypesPath, archData, 0o644))
	require.NoError(t, planet.LoadArchetypes(archetypesPath))

	return catalogPath, archetypesPath
}

// validCatalogBody — текущий store как тело PATCH (валидный справочник).
func validCatalogBody(t *testing.T) string {
	t.Helper()
	data, err := json.Marshal(planet.GetBiomeCatalog())
	require.NoError(t, err)
	return string(data)
}

// catalogCopy — копия store (GetBiomeCatalog возвращает живой указатель —
// тесты не должны мутировать store напрямую).
func catalogCopy(t *testing.T) *planet.BiomeCatalog {
	t.Helper()
	var cat planet.BiomeCatalog
	require.NoError(t, json.Unmarshal([]byte(validCatalogBody(t)), &cat))
	return &cat
}

// ==================== GET ====================

func TestAdminBiomeCatalogGet(t *testing.T) {
	biomeCatalogHarness(t)
	h := &AdminHandlers{}
	req := httptest.NewRequest(http.MethodGet, "/admin/biome-catalog", nil)
	rec := execJSON(h.GetBiomeCatalog, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp biomeCatalogResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Biomes, 57, "сид: 57 биомов")
	require.Len(t, resp.SubterrainTypes, 17, "сид: 17 типов недр")
	require.Len(t, resp.PlanetTypes, 9, "сид: 9 правил типов")
	require.NotNil(t, resp.ClimateBands, "полосы климатов в ответе")
	require.Len(t, resp.ClimateBands.Climates, 1, "temp-полосы")
}

// ==================== PATCH: ВАЛИДАЦИЯ (инвариант 17) ====================

func TestAdminBiomeCatalogPatchValid(t *testing.T) {
	catalogPath, _ := biomeCatalogHarness(t)
	h := &AdminHandlers{}

	// Правка weight_base первого биома (копия store — не мутировать живой).
	cat := catalogCopy(t)
	cat.Biomes[0].WeightBase = 7.5
	body, err := json.Marshal(cat)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, "/admin/biome-catalog", strings.NewReader(string(body)))
	req = withRole(req, string(auth.RoleAdmin))
	rec := execJSON(h.PatchBiomeCatalog, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Hot-reload store.
	require.Equal(t, 7.5, planet.GetBiomeCatalog().Biomes[0].WeightBase, "store обновлён")

	// Файл записан атомарно (читается после «рестарта»).
	fileData, err := os.ReadFile(catalogPath)
	require.NoError(t, err)
	var fromFile planet.BiomeCatalog
	require.NoError(t, json.Unmarshal(fileData, &fromFile))
	require.Equal(t, 7.5, fromFile.Biomes[0].WeightBase, "файл записан")
}

func TestAdminBiomeCatalogPatchInvalid(t *testing.T) {
	biomeCatalogHarness(t)
	h := &AdminHandlers{}
	patch := func(mutate func(cat *planet.BiomeCatalog)) *httptest.ResponseRecorder {
		cat := catalogCopy(t)
		mutate(cat)
		body, err := json.Marshal(cat)
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPatch, "/admin/biome-catalog", strings.NewReader(string(body)))
		req = withRole(req, string(auth.RoleAdmin))
		return execJSON(h.PatchBiomeCatalog, req)
	}

	// Дубли id.
	rec := patch(func(cat *planet.BiomeCatalog) {
		cat.Biomes = append(cat.Biomes, cat.Biomes[0])
	})
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, rec.Body.String(), "дубликат")

	// min > max (вода; T-диапазон не трогаем — иначе раньше сработает
	// bands↔t_range, инвариант 17).
	rec = patch(func(cat *planet.BiomeCatalog) {
		cat.Biomes[0].WaterMin, cat.Biomes[0].WaterMax = 50, 10
	})
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, rec.Body.String(), "water_min")

	// bands пусты.
	rec = patch(func(cat *planet.BiomeCatalog) {
		cat.Biomes[0].Bands = nil
	})
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, rec.Body.String(), "bands пусты")

	// Признак атмосферы вне перечня §0.
	rec = patch(func(cat *planet.BiomeCatalog) {
		cat.Biomes[0].AtmosphereOK = []string{"метановая"}
	})
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, rec.Body.String(), "признак атмосферы")

	// bands не пересекаются с t_range (инвариант 17).
	rec = patch(func(cat *planet.BiomeCatalog) {
		cat.Biomes[0].TMin, cat.Biomes[0].TMax = 480, 600
		cat.Biomes[0].Bands = []string{"х"}
	})
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Contains(t, rec.Body.String(), "не пересекаются")

	// Store не изменился после ошибок.
	require.Equal(t, 57, len(planet.GetBiomeCatalog().Biomes))
}

// ==================== ПРАВА РОЛЕЙ ====================

func TestAdminBiomeCatalogRoles(t *testing.T) {
	biomeCatalogHarness(t)
	h := &AdminHandlers{}

	// Чтение — admin + skycomposer.
	for _, role := range []string{string(auth.RoleAdmin), string(auth.RoleSkycomposer)} {
		req := httptest.NewRequest(http.MethodGet, "/admin/biome-catalog", nil)
		req = withRole(req, role)
		rec := execJSON(h.GetBiomeCatalog, req)
		require.Equal(t, http.StatusOK, rec.Code, "GET роль %s", role)
	}

	// Правка — только admin: skycomposer → 403 (через диспетчер — проверка
	// роли в HandleBiomeCatalog, как на роуте).
	req := httptest.NewRequest(http.MethodPatch, "/admin/biome-catalog", strings.NewReader(validCatalogBody(t)))
	req = withRole(req, string(auth.RoleSkycomposer))
	rec := execJSON(h.HandleBiomeCatalog, req)
	require.Equal(t, http.StatusForbidden, rec.Code, "PATCH skycomposer → 403")

	// Без роли в контексте → 403.
	req = httptest.NewRequest(http.MethodPatch, "/admin/biome-catalog", strings.NewReader(validCatalogBody(t)))
	rec = execJSON(h.HandleBiomeCatalog, req)
	require.Equal(t, http.StatusForbidden, rec.Code, "PATCH без роли → 403")

	// admin → не 403 (валидное тело проходит).
	req = httptest.NewRequest(http.MethodPatch, "/admin/biome-catalog", strings.NewReader(validCatalogBody(t)))
	req = withRole(req, string(auth.RoleAdmin))
	rec = execJSON(h.HandleBiomeCatalog, req)
	require.NotEqual(t, http.StatusForbidden, rec.Code, "PATCH admin не 403")
}

// ==================== RESET ====================

func TestAdminBiomeCatalogReset(t *testing.T) {
	catalogPath, _ := biomeCatalogHarness(t)
	h := &AdminHandlers{}

	// Сначала испортим store правкой (копия — не мутировать живой).
	cat := catalogCopy(t)
	cat.Biomes[0].WeightBase = 9.9
	body, _ := json.Marshal(cat)
	req := httptest.NewRequest(http.MethodPatch, "/admin/biome-catalog", strings.NewReader(string(body)))
	req = withRole(req, string(auth.RoleAdmin))
	require.Equal(t, http.StatusOK, execJSON(h.PatchBiomeCatalog, req).Code)

	// Сброс к заводским.
	req = httptest.NewRequest(http.MethodPost, "/admin/biome-catalog/reset", nil)
	req = withRole(req, string(auth.RoleAdmin))
	rec := execJSON(h.ResetBiomeCatalog, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Equal(t, 57, len(planet.GetBiomeCatalog().Biomes), "store = сид")
	fileData, err := os.ReadFile(catalogPath)
	require.NoError(t, err)
	var fromFile planet.BiomeCatalog
	require.NoError(t, json.Unmarshal(fileData, &fromFile))
	require.NotEqual(t, 9.9, fromFile.Biomes[0].WeightBase, "файл = сид")

	// skycomposer → 403.
	req = httptest.NewRequest(http.MethodPost, "/admin/biome-catalog/reset", nil)
	req = withRole(req, string(auth.RoleSkycomposer))
	require.Equal(t, http.StatusForbidden, execJSON(h.ResetBiomeCatalog, req).Code)
}

// ==================== КОДИРОВКА ТЕЛА (UTF-8) ====================

func TestAdminBiomeCatalogPatchRejectsNonUTF8(t *testing.T) {
	biomeCatalogHarness(t)
	h := &AdminHandlers{}

	// cp1251-тело: id первого биома заменён на байты cp1251 («горы» =
	// 0xE3 0xEE 0xF0 0xFB) — невалидный UTF-8. json.Decode заменил бы их
	// на U+FFFD и каталог прошёл бы Validate (замечание ревью).
	body := strings.Replace(validCatalogBody(t), `"id":"горы"`, "\"id\":\"\xe3\xee\xf0\xfb\"", 1)
	require.False(t, utf8.ValidString(body), "тело теста: невалидный UTF-8")

	req := httptest.NewRequest(http.MethodPatch, "/admin/biome-catalog", strings.NewReader(body))
	req = withRole(req, string(auth.RoleAdmin))
	rec := execJSON(h.HandleBiomeCatalog, req)
	require.Equal(t, http.StatusBadRequest, rec.Code, "cp1251-тело → 400, не 200")
	require.Contains(t, rec.Body.String(), "UTF-8")

	// Store не изменился.
	require.Equal(t, 57, len(planet.GetBiomeCatalog().Biomes))
	require.Equal(t, "горы", planet.GetBiomeCatalog().Biomes[0].ID, "id не заменён на U+FFFD")
}

// ==================== ПОЛОСЫ КЛИМАТОВ ====================

func TestAdminPlanetArchetypesGetPatch(t *testing.T) {
	_, archetypesPath := biomeCatalogHarness(t)
	h := &AdminHandlers{}

	// GET — полосы из store.
	req := httptest.NewRequest(http.MethodGet, "/admin/planet-archetypes", nil)
	rec := execJSON(h.GetPlanetArchetypes, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var got planet.ArchetypeConfig
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got.Climates, 1)

	// PATCH — замена полос (веса/списки).
	cfg := planet.ArchetypeConfig{Climates: []planet.ClimateConfig{
		{ID: "холодный", Name: "Холодный",
			BaseSurface:         map[string]float64{"ледники": 0.8},
			BaseSubterrain:      map[string]float64{"подземные_льды": 0.6},
			AllowedHydrospheres: []string{"подлёдная"},
			AllowedAtmospheres:  []string{"метановая"},
			AllowedBiospheres:   []string{"стерильная"},
		},
	}}
	body, err := json.Marshal(cfg)
	require.NoError(t, err)
	req = httptest.NewRequest(http.MethodPatch, "/admin/planet-archetypes", strings.NewReader(string(body)))
	req = withRole(req, string(auth.RoleAdmin))
	rec = execJSON(h.PatchPlanetArchetypes, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Hot-reload store + файл.
	require.Equal(t, "холодный", planet.GetArchetypes().Climates[0].ID)
	fileData, err := os.ReadFile(archetypesPath)
	require.NoError(t, err)
	var fromFile planet.ArchetypeConfig
	require.NoError(t, json.Unmarshal(fileData, &fromFile))
	require.Equal(t, "холодный", fromFile.Climates[0].ID, "файл записан")

	// Невалид: пустые climates → 422.
	req = httptest.NewRequest(http.MethodPatch, "/admin/planet-archetypes",
		strings.NewReader(`{"climates":[]}`))
	req = withRole(req, string(auth.RoleAdmin))
	rec = execJSON(h.PatchPlanetArchetypes, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// Невалид: отрицательный вес → 422.
	req = httptest.NewRequest(http.MethodPatch, "/admin/planet-archetypes",
		strings.NewReader(`{"climates":[{"id":"жаркий","base_surface":{"горы":-1}}]}`))
	req = withRole(req, string(auth.RoleAdmin))
	rec = execJSON(h.PatchPlanetArchetypes, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// skycomposer → 403 (через диспетчер).
	req = httptest.NewRequest(http.MethodPatch, "/admin/planet-archetypes", strings.NewReader(string(body)))
	req = withRole(req, string(auth.RoleSkycomposer))
	rec = execJSON(h.HandlePlanetArchetypes, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
}
