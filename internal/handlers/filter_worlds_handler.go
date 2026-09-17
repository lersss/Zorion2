// internal/handlers/filter_worlds_handler.go
package handlers

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"zorion/internal/auth"
	"zorion/internal/mapcache"
)

// ==================== ТИПЫ ====================

// worldCluster — один кластер миров для карты.
// CellX/CellY — идентификатор ячейки (для отладки).
// X/Y — координаты кластера в мировых координатах.
// Count — сколько миров в ячейке.
// Sample* — данные представителя для cnt=1 (для tooltip и цвета).
// Вид звезды (имя/спектр/температура/тип/координаты) — открытая информация
// для всех ролей (спека 77a §5.5/И11); детали системы (планеты/поселения)
// радиусом не раскрываются — модалка 403 вне радиуса/знания (§11.2).
type worldCluster struct {
	CellX          int64   `json:"cx"`
	CellY          int64   `json:"cy"`
	Count          int     `json:"cnt,omitempty"`
	X              float64 `json:"x"`
	Y              float64 `json:"y"`
	SampleID       string  `json:"sid,omitempty"`
	SampleName     string  `json:"sname,omitempty"`
	SampleSpectral string  `json:"sspec,omitempty"`
	SampleTemp     float64 `json:"stemp,omitempty"`
	// Экзотические типы представителя (99.2.4 §8): stype — тип объекта,
	// systype — тип системы (для цвета/бейджей карты).
	SampleStarType   string `json:"stype,omitempty"`
	SampleSystemType string `json:"systype,omitempty"`
	// SampleStellarMods — модификаторы представителя (35b §6.4): спектры
	// компаньонов (smods) для цвета точек-компаньонов на карте.
	SampleStellarMods map[string]interface{} `json:"smods,omitempty"`
}

// ==================== ЛИМИТЫ ====================

const (
	// Минимальный размер ячейки в мировых координатах.
	minCellSize = 0.5

	// Максимум ячеек по ширине viewport — защита от DoS,
	// если клиент пришлёт микроскопический cell.
	maxCellCols = 250
)

// ==================== ХЕНДЛЕР ====================

// FilterWorldsHandler — возвращает кластеры миров для видимой области карты.
// Клиент присылает границы viewport'а и размер ячейки. Кластеризация
// выполняется в Go по снапшоту mapcache — Postgres из хот-пата карты убран.
func (h *AdminHandlers) FilterWorldsHandler(w http.ResponseWriter, r *http.Request) {
	tStart := time.Now()

	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("🔥 PANIC in FilterWorldsHandler: %v", rec)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}()

	q := r.URL.Query()

	// --- Геометрия viewport ---
	xMin, err := parseFloatParam(q.Get("x_min"))
	if err != nil {
		http.Error(w, "invalid x_min", http.StatusBadRequest)
		return
	}
	xMax, err := parseFloatParam(q.Get("x_max"))
	if err != nil {
		http.Error(w, "invalid x_max", http.StatusBadRequest)
		return
	}
	yMin, err := parseFloatParam(q.Get("y_min"))
	if err != nil {
		http.Error(w, "invalid y_min", http.StatusBadRequest)
		return
	}
	yMax, err := parseFloatParam(q.Get("y_max"))
	if err != nil {
		http.Error(w, "invalid y_max", http.StatusBadRequest)
		return
	}
	cell, err := parseFloatParam(q.Get("cell"))
	if err != nil || cell <= 0 {
		http.Error(w, "invalid cell", http.StatusBadRequest)
		return
	}
	if xMax <= xMin || yMax <= yMin {
		http.Error(w, "invalid bounds (max must be > min)", http.StatusBadRequest)
		return
	}

	// Ограничиваем ячейку снизу: не больше maxCellCols ячеек по ширине.
	minCell := (xMax - xMin) / float64(maxCellCols)
	if cell < minCell {
		cell = minCell
	}
	if cell < minCellSize {
		cell = minCellSize
	}

	// --- Опциональные фильтры ---
	hasPlanets := q.Get("has_planets") == "true"
	hasLife := q.Get("has_life") == "true"
	hasHabitable := q.Get("has_habitable") == "true"
	planetType := q.Get("planet_type")
	resourceCategory := q.Get("resource_category")

	// --- Кластеризация из снапшота карты (без SQL) ---
	tQuery := time.Now()
	clusters := h.mapCache.Snapshot().Query(
		xMin, xMax, yMin, yMax, cell,
		mapcache.Filter{
			HasPlanets:       hasPlanets,
			HasLife:          hasLife,
			HasHabitable:     hasHabitable,
			PlanetType:       planetType,
			ResourceCategory: resourceCategory,
		},
	)
	queryDur := time.Since(tQuery)

	// --- Преобразование в формат ответа ---
	tScan := time.Now()
	out := make([]worldCluster, 0, len(clusters))
	for _, c := range clusters {
		oc := worldCluster{
			CellX:            c.CellX,
			CellY:            c.CellY,
			Count:            c.Count,
			X:                c.X,
			Y:                c.Y,
			SampleSpectral:   c.SampleSpectral,
			SampleTemp:       c.SampleTemp,
			SampleStarType:   c.SampleStarType,
			SampleSystemType: c.SampleSystemType,
		}
		// Sample* — только для cnt=1, чтобы не раздувать payload.
		if c.Count == 1 {
			oc.SampleID = c.SampleID
			oc.SampleName = c.SampleName
			oc.SampleStellarMods = c.SampleStellarMods
		}
		out = append(out, oc)
	}

	// Вид звезды — открытая информация для всех ролей (спека 77a §5.5/И11):
	// имя/спектр/температура/тип/координаты отдаются как админу, радиус радара
	// на вид звезды не влияет. Детали системы (планеты/поселения) закрыты
	// отдельно — модалка 403 вне радиуса/знания (§11.2).
	scanDur := time.Since(tScan)

	// --- Ответ ---
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	var writer io.Writer = w
	useGzip := strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
	if useGzip {
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		writer = gz
	}

	tEncode := time.Now()
	if err := json.NewEncoder(writer).Encode(out); err != nil {
		// Заголовки уже улетели — http.Error нельзя. Просто логируем.
		log.Printf("⚠️ FilterWorlds encode error (клиент отвалился?): %v", err)
		return
	}
	encodeDur := time.Since(tEncode)

	log.Printf(
		"🗺️  FilterWorlds: bounds=(%.0f,%.0f)-(%.0f,%.0f) cell=%.2f → %d кластеров, query=%v scan=%v encode=%v total=%v gzip=%v",
		xMin, yMin, xMax, yMax, cell,
		len(out),
		queryDur.Round(time.Millisecond),
		scanDur.Round(time.Millisecond),
		encodeDur.Round(time.Millisecond),
		time.Since(tStart).Round(time.Millisecond),
		useGzip,
	)
}

// ==================== УТИЛИТЫ ====================

func parseFloatParam(s string) (float64, error) {
	if s == "" {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseFloat(s, 64)
}

// roleFromContext — роль из контекста запроса (AuthMiddleware).
func roleFromContext(r *http.Request) string {
	role, _ := r.Context().Value(auth.RoleKey).(string)
	return role
}
