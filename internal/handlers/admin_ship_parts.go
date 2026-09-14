// internal/handlers/admin_ship_parts.go
// Вкладка «Корабли» админки (спека 99.2.15 §6, §10): «теневой» генератор
// деталей кораблей. GET /admin/ship-parts — каталог + метаданные (params,
// created_at) + данные предпросмотра-гейта И9 (нейтральные/крайние детали
// для стрипа и worst-case сборки); POST /admin/ship-parts/generate
// {category, count?, seed?} → 201; DELETE /admin/ship-parts/{id} → 204;
// POST /admin/ship-parts/regenerate-category {category} — перегенерация 10
// форм (ON CONFLICT DO UPDATE + удаление не вошедших в десятку). После любой
// генерации каталог в памяти перезагружается из БД (И7).
package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"zorion/internal/generator/ship"
	"zorion/internal/models"
	"zorion/internal/repository"
)

// maxGenerateCount — потолок пачки генерации (кнопка «10», лимит на защиту
// от случайного 1000-кратного спама).
const maxGenerateCount = 50

// AdminShipPartsHandlers — «теневой» генератор деталей кораблей.
type AdminShipPartsHandlers struct {
	repo    *repository.ShipRepository
	catalog *repository.ShipCatalog
}

func NewAdminShipPartsHandlers(repo *repository.ShipRepository, catalog *repository.ShipCatalog) *AdminShipPartsHandlers {
	return &AdminShipPartsHandlers{repo: repo, catalog: catalog}
}

// adminShipPart — деталь для вкладки: + параметры генерации и дата создания
// (спека §10: «каталог + метаданные (params, created_at)»).
type adminShipPart struct {
	ID        string      `json:"id"`
	Category  string      `json:"category"`
	Name      string      `json:"name"`
	SVG       string      `json:"svg"`
	Params    interface{} `json:"params"`
	CreatedAt time.Time   `json:"created_at"`
}

func toAdminDTO(p models.ShipPart) adminShipPart {
	return adminShipPart{
		ID:        p.ID,
		Category:  p.Category,
		Name:      p.Name,
		SVG:       p.SVG,
		Params:    p.Params,
		CreatedAt: p.CreatedAt,
	}
}

func toAdminDTOs(parts []models.ShipPart) []adminShipPart {
	out := make([]adminShipPart, 0, len(parts))
	for _, p := range parts {
		out = append(out, toAdminDTO(p))
	}
	return out
}

// ==================== СПИСОК: GET /admin/ship-parts ====================

// List — каталог из БД (источник правды для вкладки) + палитра/порядок
// слоёв + данные предпросмотра: три корпуса стрипа (нейтральный/min/max),
// нейтральные детали остальных категорий и worst-case сборки (гейт И9, §6).
func (h *AdminShipPartsHandlers) List(w http.ResponseWriter, r *http.Request) {
	parts, err := h.repo.ListParts()
	if err != nil {
		log.Printf("AdminShipParts List: %v", err)
		writeJSONError(w, "Не удалось загрузить каталог деталей", http.StatusInternalServerError)
		return
	}
	if parts == nil {
		parts = []models.ShipPart{}
	}
	snap := h.catalog.Snapshot()
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"parts":         toAdminDTOs(parts),
		"palette":       snap.Palette(),
		"layerOrder":    snap.LayerOrder(),
		"hulls":         pickHulls(parts),
		"neutral_parts": pickNeutralParts(parts),
		"worst_cases":   worstCases(parts),
	})
}

// ==================== ГЕНЕРАЦИЯ: POST /admin/ship-parts/generate ====================

// Generate — {category, count?, seed?} → 201, список созданных (спека §10).
// count по умолчанию 1 (кнопка «Сгенерировать 1»), seed по умолчанию
// случайный (время). Детали генератора детерминированы: тот же seed и count
// → те же детали с теми же id (INSERT ... ON CONFLICT DO UPDATE безопасен).
func (h *AdminShipPartsHandlers) Generate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Category string `json:"category"`
		Count    int    `json:"count"`
		Seed     int64  `json:"seed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if !validShipCategory(req.Category) {
		writeJSONError(w, "Неизвестная категория", http.StatusBadRequest)
		return
	}
	count := req.Count
	if count <= 0 {
		count = 1
	}
	if count > maxGenerateCount {
		count = maxGenerateCount
	}
	seedBase := req.Seed
	if seedBase == 0 {
		seedBase = time.Now().UnixNano()
	}

	// Сначала генерируем всю пачку, затем пишем: при ошибке генерации
	// ничего не сохраняем (частичных пачек нет).
	generated := make([]models.ShipPart, 0, count)
	for i := 0; i < count; i++ {
		p, err := ship.GeneratePart(req.Category, seedBase+int64(i))
		if err != nil {
			log.Printf("AdminShipParts Generate: %v", err)
			writeJSONError(w, "Ошибка генерации детали", http.StatusInternalServerError)
			return
		}
		generated = append(generated, models.ShipPart{
			ID:       p.ID,
			Category: p.Category,
			Name:     p.Name,
			SVG:      p.SVG,
			Params:   p.Params,
		})
	}
	for i := range generated {
		if err := h.repo.UpsertPart(&generated[i]); err != nil {
			log.Printf("AdminShipParts Generate: Upsert(%s): %v", generated[i].ID, err)
			writeJSONError(w, "Не удалось сохранить деталь", http.StatusInternalServerError)
			return
		}
	}
	if err := h.repo.ReloadCatalog(r.Context(), h.catalog); err != nil {
		log.Printf("AdminShipParts Generate: reload catalog: %v", err)
	}
	writeJSONStatus(w, http.StatusCreated, map[string]interface{}{"parts": toAdminDTOs(generated)})
}

// ==================== УДАЛЕНИЕ: DELETE /admin/ship-parts/{id} ====================

// HandleObject — диспетчер DELETE /admin/ship-parts/{id} → 204.
func (h *AdminShipPartsHandlers) HandleObject(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/ship-parts/")
	if path == "" || path == r.URL.Path || strings.Contains(path, "/") {
		writeJSONError(w, "ID не указан", http.StatusBadRequest)
		return
	}
	if r.Method != http.MethodDelete {
		writeJSONError(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return
	}
	if err := h.repo.DeletePart(path); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, "Деталь не найдена", http.StatusNotFound)
			return
		}
		log.Printf("AdminShipParts Delete(%s): %v", path, err)
		writeJSONError(w, "Не удалось удалить деталь", http.StatusInternalServerError)
		return
	}
	if err := h.repo.ReloadCatalog(r.Context(), h.catalog); err != nil {
		log.Printf("AdminShipParts Delete: reload catalog: %v", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ==================== ПЕРЕГЕНЕРАЦИЯ: POST /admin/ship-parts/regenerate-category ====================

// RegenerateCategory — {category} → 10 новых форм (свежие случайные seed),
// INSERT ... ON CONFLICT DO UPDATE (спека §10); детали категории, не вошедшие
// в новую десятку, удаляются — категория снова ровно 10 форм (спека §9).
func (h *AdminShipPartsHandlers) RegenerateCategory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Category string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректное тело запроса", http.StatusBadRequest)
		return
	}
	if !validShipCategory(req.Category) {
		writeJSONError(w, "Неизвестная категория", http.StatusBadRequest)
		return
	}

	const n = 10
	seedBase := time.Now().UnixNano()
	parts := make([]models.ShipPart, 0, n)
	for i := 0; i < n; i++ {
		p, err := ship.GeneratePart(req.Category, seedBase+int64(i))
		if err != nil {
			log.Printf("AdminShipParts Regenerate: %v", err)
			writeJSONError(w, "Ошибка генерации детали", http.StatusInternalServerError)
			return
		}
		parts = append(parts, models.ShipPart{
			ID:       p.ID,
			Category: p.Category,
			Name:     p.Name,
			SVG:      p.SVG,
			Params:   p.Params,
		})
	}
	keep := make(map[string]bool, n)
	for i := range parts {
		keep[parts[i].ID] = true
		if err := h.repo.UpsertPart(&parts[i]); err != nil {
			log.Printf("AdminShipParts Regenerate: Upsert(%s): %v", parts[i].ID, err)
			writeJSONError(w, "Не удалось сохранить деталь", http.StatusInternalServerError)
			return
		}
	}
	// Убираем детали категории вне новой десятки (категория = ровно 10 форм).
	existing, err := h.repo.PartsByCategory(req.Category)
	if err != nil {
		log.Printf("AdminShipParts Regenerate: PartsByCategory: %v", err)
	} else {
		for _, e := range existing {
			if !keep[e.ID] {
				if err := h.repo.DeletePart(e.ID); err != nil {
					log.Printf("AdminShipParts Regenerate: Delete(%s): %v", e.ID, err)
				}
			}
		}
	}
	if err := h.repo.ReloadCatalog(r.Context(), h.catalog); err != nil {
		log.Printf("AdminShipParts Regenerate: reload catalog: %v", err)
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{"parts": toAdminDTOs(parts)})
}

// ==================== ДАННЫЕ ПРЕДПРОСМОТРА (гейт И9, спека §6) ====================

// validShipCategory — категория из константы генератора (спека §2: список
// категорий — константа в одном месте).
func validShipCategory(cat string) bool {
	for _, c := range ship.Categories {
		if c == cat {
			return true
		}
	}
	return false
}

// paramF — числовой параметр генерации детали (JSONB-числа → float64).
func paramF(p models.ShipPart, key string) float64 {
	v, ok := p.Params[key]
	if !ok {
		return 0
	}
	f, _ := v.(float64)
	return f
}

// byCategory — детали каталога, сгруппированные по категории.
func byCategory(parts []models.ShipPart) map[string][]models.ShipPart {
	out := map[string][]models.ShipPart{}
	for _, p := range parts {
		out[p.Category] = append(out[p.Category], p)
	}
	return out
}

// pickHulls — три корпуса стрипа (спека §6 п.4): нейтральный (ближайший к
// середине габаритов: w≈85, h≈51) + минимального габарита (min w·h) +
// максимального (max w·h). Ключи: neutral/min/max; пусто, если корпусов нет.
func pickHulls(parts []models.ShipPart) map[string]string {
	hulls := byCategory(parts)["hull"]
	if len(hulls) == 0 {
		return map[string]string{}
	}
	out := map[string]string{}
	var minP, maxP, neuP models.ShipPart
	minArea, maxArea := math.MaxFloat64, -1.0
	bestDist := math.MaxFloat64
	for _, p := range hulls {
		w, h := paramF(p, "w"), paramF(p, "h")
		area := w * h
		if area < minArea {
			minArea, minP = area, p
		}
		if area > maxArea {
			maxArea, maxP = area, p
		}
		// Нейтральный: ближайший к середине диапазонов (ширина 65–110
		// эффективно, высота 42–60; спека §3.1).
		if d := math.Abs(w-85) + math.Abs(h-51); d < bestDist {
			bestDist, neuP = d, p
		}
	}
	out["neutral"], out["min"], out["max"] = neuP.ID, minP.ID, maxP.ID
	return out
}

// pickNeutralParts — нейтральные детали придатков для стрипа: ближайшие к
// середине главного габарита категории (нос — длина, крылья — вылет,
// двигатели — длина, хвост — высота; спека §3.1).
func pickNeutralParts(parts []models.ShipPart) map[string]string {
	byCat := byCategory(parts)
	specs := []struct {
		cat, param string
		mid        float64
	}{
		{"nose", "l", 62.5},
		{"wings", "r", 32.5},
		{"engine", "l", 45},
		{"tail", "ht", 35},
	}
	out := map[string]string{}
	for _, s := range specs {
		if p, ok := nearestPart(byCat[s.cat], s.param, s.mid); ok {
			out[s.cat] = p.ID
		}
	}
	return out
}

// nearestPart — деталь с параметром, ближайшим к mid; ok=false, если
// в категории нет деталей.
func nearestPart(parts []models.ShipPart, param string, mid float64) (models.ShipPart, bool) {
	if len(parts) == 0 {
		return models.ShipPart{}, false
	}
	best := parts[0]
	bestD := math.Abs(paramF(best, param) - mid)
	for _, p := range parts[1:] {
		if d := math.Abs(paramF(p, param) - mid); d < bestD {
			best, bestD = p, d
		}
	}
	return best, true
}

// worstCase — сборка у границ габаритов §3.1 для сэмплера предпросмотра.
type worstCase struct {
	Key   string            `json:"key"` // "nose_max_hull_min", ...
	Parts map[string]string `json:"parts"`
}

// worstCases — сэмплер крайних сочетаний (спека §6 п.5): max-нос × min-корпус,
// max-крылья × min-корпус, max-двигатели × min-корпус, max-хвост × min-корпус.
// Остальные категории в сборке — нейтральные.
func worstCases(parts []models.ShipPart) []worstCase {
	hulls := pickHulls(parts)
	minHull := hulls["min"]
	if minHull == "" {
		return nil
	}
	base := map[string]string{"hull": minHull}
	for cat, id := range pickNeutralParts(parts) {
		base[cat] = id
	}
	byCat := byCategory(parts)
	extremes := []struct {
		key, cat, param string
	}{
		{"nose_max_hull_min", "nose", "l"},
		{"wings_max_hull_min", "wings", "r"},
		{"engine_max_hull_min", "engine", "l"},
		{"tail_max_hull_min", "tail", "ht"},
	}
	out := make([]worstCase, 0, len(extremes))
	for _, e := range extremes {
		id := extremePart(byCat[e.cat], e.param)
		if id == "" {
			continue
		}
		combo := make(map[string]string, len(base)+1)
		for k, v := range base {
			combo[k] = v
		}
		combo[e.cat] = id
		out = append(out, worstCase{Key: e.key, Parts: combo})
	}
	return out
}

// extremePart — деталь категории с максимальным значением параметра
// (для worst-case сэмплера; "" — категория пуста).
func extremePart(parts []models.ShipPart, param string) string {
	if len(parts) == 0 {
		return ""
	}
	best := parts[0]
	bestVal := paramF(best, param)
	for _, p := range parts[1:] {
		if v := paramF(p, param); v > bestVal {
			best, bestVal = p, v
		}
	}
	return best.ID
}