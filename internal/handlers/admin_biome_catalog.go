// internal/handlers/admin_biome_catalog.go
// Справочник биомов — админка (99.2.28 §22, UI-спека 99.2.28-ui §11):
//
//	GET  /admin/biome-catalog          — справочник (биомы/недры/правила типов/
//	                                      параметры токсичности) + полосы климатов
//	PATCH /admin/biome-catalog         — полная замена справочника (admin)
//	POST /admin/biome-catalog/reset    — сброс к заводскому сиду (admin)
//	GET  /admin/planet-archetypes      — полосы климатов (веса/списки)
//	PATCH /admin/planet-archetypes     — полная замена полос (admin)
//
// Права: правка — admin; чтение — admin + skycomposer (AdminAuth на роуте,
// гейт §22.10). Атомарная запись файла (tmp + rename, паттерн race_balancer),
// hot-reload store (RWMutex), ошибка записи — лог (правки сессионные,
// сервер не падает).
package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"unicode/utf8"

	"zorion/internal/auth"
	"zorion/internal/generator/planet"
)

// biomeCatalogResponse — GET /admin/biome-catalog: справочник + полосы климатов
// (climate_bands — из config/planet_archetypes.json, веса/списки §22.4).
type biomeCatalogResponse struct {
	Biomes          []planet.BiomeDef          `json:"biomes"`
	SubterrainTypes []planet.SubterrainTypeDef `json:"subterrain_types"`
	PlanetTypes     []planet.PlanetTypeRule    `json:"planet_types"`
	FallbackType    string                     `json:"fallback_type"`
	Params          planet.CatalogParams       `json:"params"`
	// Рецепт вида (спека 2026-09-23 §7): GET обязан нести секции и диагностику —
	// PATCH — полная замена, поэтому сохранение обязано переносить их без потерь.
	ViewFamilies    []map[string]any        `json:"view_families"`
	ViewPrimitives  []planet.ViewPrimitive  `json:"view_primitives"`
	ViewDiagnostics planet.ViewDiagnostics  `json:"view_diagnostics"`
	ClimateBands    *planet.ArchetypeConfig `json:"climate_bands"`
}

// biomeCatalogResponseFromStore — сборка ответа из текущего store.
func biomeCatalogResponseFromStore() biomeCatalogResponse {
	cat := planet.GetBiomeCatalog()
	families := cat.ViewFamilies
	if families == nil {
		families = []map[string]any{}
	}
	prims := cat.ViewPrimitives
	if prims == nil {
		prims = []planet.ViewPrimitive{}
	}
	return biomeCatalogResponse{
		Biomes:          cat.Biomes,
		SubterrainTypes: cat.SubterrainTypes,
		PlanetTypes:     cat.PlanetTypes,
		FallbackType:    cat.FallbackType,
		Params:          cat.Params,
		ViewFamilies:    families,
		ViewPrimitives:  prims,
		ViewDiagnostics: cat.ViewDiagnostics(),
		ClimateBands:    planet.GetArchetypes(),
	}
}

// requireAdminRole — правка справочника — только admin (99.2.28 §22.10);
// чтение — admin + skycomposer (AdminAuth на роуте). 403 при не-admin.
func requireAdminRole(w http.ResponseWriter, r *http.Request) bool {
	role, ok := r.Context().Value(auth.RoleKey).(string)
	if !ok || role != string(auth.RoleAdmin) {
		writeJSONError(w, "Недостаточно прав", http.StatusForbidden)
		return false
	}
	return true
}

// HandleBiomeCatalog — диспетчер GET/PATCH для /admin/biome-catalog.
func (h *AdminHandlers) HandleBiomeCatalog(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.GetBiomeCatalog(w, r)
	case http.MethodPatch:
		if !requireAdminRole(w, r) {
			return
		}
		h.PatchBiomeCatalog(w, r)
	default:
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
	}
}

// GetBiomeCatalog — GET /admin/biome-catalog: справочник + полосы климатов.
func (h *AdminHandlers) GetBiomeCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSONStatus(w, http.StatusOK, biomeCatalogResponseFromStore())
}

// PatchBiomeCatalog — PATCH /admin/biome-catalog: полная замена справочника.
// Валидация — инвариант 17 (дубли id, min>max, bands не пусты и из э/ж/у/х,
// признаки §0, bands↔t_range); атомарная запись файла; hot-reload store.
// Ошибка записи — лог, правки сессионные (сервер не падает).
func (h *AdminHandlers) PatchBiomeCatalog(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	// json.Decode не отвергает невалидный UTF-8 (заменяет на U+FFFD) —
	// mojibake-каталог (cp1251) прошёл бы Validate и записался в файл.
	if !utf8.Valid(body) {
		writeJSONError(w, "Тело запроса не в UTF-8 (ожидается UTF-8)", http.StatusBadRequest)
		return
	}
	var cat planet.BiomeCatalog
	if err := json.Unmarshal(body, &cat); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if err := cat.Validate(); err != nil {
		writeJSONError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := planet.RebuildBiomeCatalog(&cat); err != nil {
		writeJSONError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := planet.SaveBiomeCatalog(&cat); err != nil {
		log.Printf("⚠️ Справочник биомов: запись файла: %v (правки сессионные)", err)
	}
	writeJSONStatus(w, http.StatusOK, biomeCatalogResponseFromStore())
}

// ResetBiomeCatalog — POST /admin/biome-catalog/reset: сброс к заводскому
// сиду (store + файл, «Сбросить к заводским» UI-спека §22.7).
func (h *AdminHandlers) ResetBiomeCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	if !requireAdminRole(w, r) {
		return
	}
	seed := planet.SeedBiomeCatalog()
	if err := planet.RebuildBiomeCatalog(seed); err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := planet.SaveBiomeCatalog(seed); err != nil {
		log.Printf("⚠️ Справочник биомов: запись файла при сбросе: %v (правки сессионные)", err)
	}
	writeJSONStatus(w, http.StatusOK, biomeCatalogResponseFromStore())
}

// HandlePlanetArchetypes — диспетчер GET/PATCH для /admin/planet-archetypes.
func (h *AdminHandlers) HandlePlanetArchetypes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.GetPlanetArchetypes(w, r)
	case http.MethodPatch:
		if !requireAdminRole(w, r) {
			return
		}
		h.PatchPlanetArchetypes(w, r)
	default:
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
	}
}

// GetPlanetArchetypes — GET /admin/planet-archetypes: полосы климатов
// (веса base_surface/base_subterrain + allowed-списки; whitelist биомов —
// в bands биома, не здесь — находка @critic №1).
func (h *AdminHandlers) GetPlanetArchetypes(w http.ResponseWriter, r *http.Request) {
	writeJSONStatus(w, http.StatusOK, planet.GetArchetypes())
}

// PatchPlanetArchetypes — PATCH /admin/planet-archetypes: полная замена полос
// климатов. Валидация: climates не пуст, id уникальны, веса ≥ 0. Атомарная
// запись файла + hot-reload store (RWMutex).
func (h *AdminHandlers) PatchPlanetArchetypes(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	// Та же защита от mojibake, что в PatchBiomeCatalog: json.Decode не
	// отвергает невалидный UTF-8 — cp1251-полосы записались бы в файл.
	if !utf8.Valid(body) {
		writeJSONError(w, "Тело запроса не в UTF-8 (ожидается UTF-8)", http.StatusBadRequest)
		return
	}
	var cfg planet.ArchetypeConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if err := validateArchetypes(&cfg); err != nil {
		writeJSONError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := planet.RebuildArchetypes(&cfg); err != nil {
		writeJSONError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if err := planet.SaveArchetypes(&cfg); err != nil {
		log.Printf("⚠️ Полосы климатов: запись файла: %v (правки сессионные)", err)
	}
	writeJSONStatus(w, http.StatusOK, planet.GetArchetypes())
}

// validateArchetypes — серверная валидация полос климатов (дубликат
// клиентской): climates не пуст, id не пуст и уникален, веса ≥ 0.
func validateArchetypes(cfg *planet.ArchetypeConfig) error {
	if len(cfg.Climates) == 0 {
		return fmt.Errorf("полосы климатов: climates пуст")
	}
	seen := make(map[string]bool, len(cfg.Climates))
	for i := range cfg.Climates {
		c := &cfg.Climates[i]
		if c.ID == "" {
			return fmt.Errorf("полоса #%d: пустой id", i)
		}
		if seen[c.ID] {
			return fmt.Errorf("полоса %q: дубликат id", c.ID)
		}
		seen[c.ID] = true
		for form, w := range c.BaseSurface {
			if w < 0 {
				return fmt.Errorf("полоса %q: вес %q < 0", c.ID, form)
			}
		}
		for form, w := range c.BaseSubterrain {
			if w < 0 {
				return fmt.Errorf("полоса %q: вес %q < 0", c.ID, form)
			}
		}
	}
	return nil
}
