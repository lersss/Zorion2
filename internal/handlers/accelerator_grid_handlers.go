package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sort"
	"time"

	"zorion/internal/auth"
	"zorion/internal/models"
	"zorion/internal/repository"
	"zorion/internal/routegame"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

// accelerator_grid_handlers.go — доска v9 «Планшет» мини-игры «Прокладка
// маршрута»: публичный слой поля в offer и разведка секторов
// (POST /api/accelerator/scan). Спека
// 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута §14 (14.1 наследуемое,
// 14.3 секторы/σ, 14.4 разведка, 14.8 что видит игрок). Старая (v1) модель
// (field.go/evaluate.go) в offer больше не участвует; boost остаётся на ней
// (подэтап B, A5).
//
// Разделение слоёв: публичная доска (маяки, объекты, клетки и подписи σ)
// выводится из seed; скрытое содержимое секторов — из серверного secret.
// secret, реализованный слой (realized) и содержимое НЕ отдаются клиенту.
//
// Награду в v9-пути задаёт кривая самой модели (bonusCostGrid, может быть
// отрицательной), а не параметр модуля: ship.AcceleratorBonus и
// params.bonus_max в v9 НЕ участвуют (оставлены для старого boost, подэтап B).

// acceleratorGridPings — импульсы разведки на сегмент (§14.4 R3; значение
// эталонного харнесса v9 — DefaultV9Config.Pings). На будущее — параметр тира
// модуля (accel_N); сейчас константа.
const acceleratorGridPings = 2

// acceleratorPuzzleKind — вид задачи в player_route_puzzle (расширяемость kind).
const acceleratorPuzzleKind = ship.AcceleratorGameRoute

// errGridNoField — поле сегмента не удалось собрать за отведённые попытки
// (гейт §14.2 L_naive > L_safe не выполнен): offer/scan отвечают отказом.
var errGridNoField = errors.New("routegame: no acceptable grid field")

// acceleratorOfferResponse — контракт offer (§3.4/§14.8): публичная доска v9 +
// импульсы и вскрытые секторы; прежние верхние поля сохранены. Без прогноза
// прибытия и секунд результата (решение 14). Скрытые слои не отдаются.
type acceleratorOfferResponse struct {
	Fingerprint        string                `json:"fingerprint"`
	Game               string                `json:"game"`
	Passport           routegame.Passport    `json:"passport"`
	Board              acceleratorBoard      `json:"board"`
	PingsLeft          int                   `json:"pings_left"`
	Revealed           []acceleratorRevealed `json:"revealed"`
	RemainingS         int                   `json:"remaining_s"`
	MinRemainingBoostS int                   `json:"min_remaining_boost_s"`
	CooldownRemainingS *int64                `json:"cooldown_remaining_s"`
}

// acceleratorBoard — публичный слой поля v9 (§14.8): геометрия, объекты и
// подписи секторов. Цены клеток — в Visible (n×n). Реализованный слой и
// содержимое секторов НЕ входят.
type acceleratorBoard struct {
	N          int                  `json:"n"`
	Start      int                  `json:"start"`
	Finish     int                  `json:"finish"`
	Beacons    []int                `json:"beacons"`
	Visible    []float64            `json:"visible"`
	Lane       []int                `json:"lane"`
	Wall       []int                `json:"wall"`
	Mud        []int                `json:"mud"`
	Gate       []int                `json:"gate"`
	Bridge     []int                `json:"bridge"`
	Current    []acceleratorCurrent `json:"current"`
	DeadEnd    []int                `json:"dead_end"`
	Bottleneck []int                `json:"bottleneck"`
	Sectors    []acceleratorSector  `json:"sectors"`
	Mode       string               `json:"mode"`
}

// acceleratorCurrent — одностороннее течение: клетка и направление (0=+i, 1=−i,
// 2=+j, 3=−j).
type acceleratorCurrent struct {
	Cell int `json:"cell"`
	Dir  int `json:"dir"`
}

// acceleratorSector — публичный вид сектора: клетки, подпись σ и видимое
// окружение. Скрытого содержимого нет.
type acceleratorSector struct {
	Cells        []int  `json:"cells"`
	Sig          int    `json:"sig"`           // 0 Тихий, 1 Ровный, 2 Гулкий
	SigName      string `json:"sig_name"`      // «Тихий»/«Ровный»/«Гулкий»
	Surround     int    `json:"surround"`      // 0 Ничего, 1 Кордон, 2 Обрыв, 3 Мгла, 4 Течение
	SurroundName string `json:"surround_name"` // «Ничего»/«Кордон»/«Обрыв»/«Мгла»/«Течение»
}

// acceleratorRevealed — вскрытый сектор (элемент revealed из БД).
type acceleratorRevealed struct {
	Sector  int    `json:"sector"`
	Content string `json:"content"`
}

// acceleratorScanRequest — тело POST /api/accelerator/scan: fingerprint
// текущего сегмента + индекс сектора.
type acceleratorScanRequest struct {
	Fingerprint string `json:"fingerprint"`
	Sector      int    `json:"sector"`
}

// acceleratorScanResponse — результат вскрытия: содержимое сектора, остаток
// импульсов и актуальный список вскрытых.
type acceleratorScanResponse struct {
	Fingerprint string                `json:"fingerprint"`
	Sector      int                   `json:"sector"`
	Content     string                `json:"content"`
	PingsLeft   int                   `json:"pings_left"`
	Revealed    []acceleratorRevealed `json:"revealed"`
}

// acceleratorSegmentSeed — seed поля сегмента: hash(from,to) + start_time
// (§14.1: поле честно разное от перелёта, start_time в seed).
func acceleratorSegmentSeed(flight *travel.TravelInfo) int64 {
	return routegame.HashSeed(flight.FromWorld, flight.ToWorld) ^ flight.StartTime.UnixMilli()
}

// acceleratorSegmentHash — отпечаток сегмента для привязки player_route_puzzle
// (тот же fingerprint, что уходит клиенту, — sha256). Смена сегмента
// (разворот/перебазирование) даёт другой отпечаток → задача пересоздаётся.
func acceleratorSegmentHash(flight *travel.TravelInfo) []byte {
	sum := sha256.Sum256([]byte(acceleratorFingerprint(flight)))
	return sum[:]
}

// acceleratorGridPuzzle — состояние задачи текущего сегмента: строка
// player_route_puzzle создаётся/пересоздаётся при смене сегмента (новый secret
// и layout, revealed сброшен, импульсы = acceleratorGridPings). Возвращает
// строку и детерминированное поле (пересобрано из seed+secret — то же, что
// сохранено, переживает рестарт). Секрет — только серверный, клиенту не идёт.
//
// Смена сегмента идёт через Repository.Ensure (условный upsert + перечитывание):
// конкурентные запросы одного игрока (гонка Get+Replace) сходятся к одной
// строке-победителю, и поле пересобирается из ЕЁ secret — одна доска (§14.1).
func (h *TravelHandlers) acceleratorGridPuzzle(userID string, flight *travel.TravelInfo, dist float64, passport routegame.Passport) (*models.RoutePuzzle, routegame.GridField, error) {
	ctx := context.Background()
	hash := acceleratorSegmentHash(flight)
	seed := acceleratorSegmentSeed(flight)
	st, err := h.routePuzzleRepo.Get(ctx, userID, acceleratorPuzzleKind)
	if err != nil {
		return nil, routegame.GridField{}, err
	}
	if st != nil && bytes.Equal(st.SegmentHash, hash) {
		field, ok := routegame.GenerateGridField(seed, st.Secret, dist, passport)
		if !ok {
			return nil, routegame.GridField{}, errGridNoField
		}
		return st, field, nil
	}
	secret, candidate, ok := acceleratorNewPuzzleField(flight, dist, passport)
	if !ok {
		return nil, routegame.GridField{}, errGridNoField
	}
	layout, err := json.Marshal(candidate.Layout())
	if err != nil {
		return nil, routegame.GridField{}, err
	}
	canonical, err := h.routePuzzleRepo.Ensure(ctx, &models.RoutePuzzle{
		UserID:      userID,
		Kind:        acceleratorPuzzleKind,
		SegmentHash: hash,
		Secret:      secret,
		Layout:      layout,
		PingsLeft:   acceleratorGridPings,
	})
	if err != nil {
		return nil, routegame.GridField{}, err
	}
	if canonical == nil || !bytes.Equal(canonical.SegmentHash, hash) {
		// Параллельный запрос успел сменить сегмент — доска текущего не собрана.
		return nil, routegame.GridField{}, errGridNoField
	}
	field, ok := routegame.GenerateGridField(seed, canonical.Secret, dist, passport)
	if !ok {
		return nil, routegame.GridField{}, errGridNoField
	}
	return canonical, field, nil
}

// acceleratorGridSegment — общая сборка сегмента для offer/scan/boost: паспорт
// (dist — от старта сегмента до цели; пояса — только видимые) и
// детерминированное поле v9 из seed сегмента + серверного secret/layout задачи
// (acceleratorGridPuzzle). Единая точка, чтобы offer/scan/boost не разъехались.
func (h *TravelHandlers) acceleratorGridSegment(userID string, flight *travel.TravelInfo, from, to *models.World) (routegame.Passport, *models.RoutePuzzle, routegame.GridField, error) {
	dist := acceleratorSegmentDist(flight, to)
	passport := routegame.BuildPassport(dist, *from, *to, visibleBelts(h.acceleratorBelts(flight.ToWorld)))
	st, field, err := h.acceleratorGridPuzzle(userID, flight, dist, passport)
	return passport, st, field, err
}

// acceleratorNewPuzzleField — новый secret и поле под него (до 8 попыток:
// гейт §14.2 зависит от содержимого секторов, а оно — от secret; при неудаче
// берём другой secret).
func acceleratorNewPuzzleField(flight *travel.TravelInfo, dist float64, passport routegame.Passport) ([]byte, routegame.GridField, bool) {
	seed := acceleratorSegmentSeed(flight)
	for try := 0; try < 8; try++ {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return nil, routegame.GridField{}, false
		}
		if field, ok := routegame.GenerateGridField(seed, secret, dist, passport); ok {
			return secret, field, true
		}
	}
	return nil, routegame.GridField{}, false
}

// acceleratorGridBoard — публичный слой поля. Клетки объектов — срезами в
// детерминированном порядке (map'ы RNG-нестабильны, JSON должен быть стабилен);
// цены — в Visible.
func acceleratorGridBoard(field routegame.GridField) acceleratorBoard {
	b := acceleratorBoard{
		N:          field.N,
		Start:      field.Start,
		Finish:     field.Finish,
		Beacons:    append([]int{}, field.Beacons...),
		Visible:    append([]float64{}, field.Visible...),
		Lane:       gridCellList(field.Lane),
		Wall:       gridCellList(field.Wall),
		Mud:        gridMudCellList(field.Mud),
		Gate:       gridCellList(field.Gate),
		Bridge:     gridCellList(field.Bridge),
		DeadEnd:    gridCellList(field.DeadEnd),
		Bottleneck: gridCellList(field.Bottleneck),
		Sectors:    make([]acceleratorSector, 0, len(field.Sectors)),
		Mode:       field.Mode,
	}
	for c, d := range field.CurrentDir {
		b.Current = append(b.Current, acceleratorCurrent{Cell: c, Dir: d})
	}
	sort.Slice(b.Current, func(i, j int) bool { return b.Current[i].Cell < b.Current[j].Cell })
	for _, s := range field.Sectors {
		b.Sectors = append(b.Sectors, acceleratorSector{
			Cells:        append([]int{}, s.Cells...),
			Sig:          int(s.Sig),
			SigName:      s.Sig.String(),
			Surround:     int(s.Surround),
			SurroundName: s.Surround.String(),
		})
	}
	return b
}

// gridCellList — ключи map[int]bool отсортированным срезом (стабильный JSON).
func gridCellList(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for c := range m {
		out = append(out, c)
	}
	sort.Ints(out)
	return out
}

// gridMudCellList — клетки топи (цена — в Visible) отсортированным срезом.
func gridMudCellList(m map[int]float64) []int {
	out := make([]int, 0, len(m))
	for c := range m {
		out = append(out, c)
	}
	sort.Ints(out)
	return out
}

// acceleratorRevealedList — revealed из БД в список вскрытых; пусто/битый JSON
// → пустой не-nil список (клиент получает []).
func acceleratorRevealedList(raw []byte) []acceleratorRevealed {
	out := make([]acceleratorRevealed, 0)
	if len(raw) == 0 {
		return out
	}
	var parsed []acceleratorRevealed
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed == nil {
		return out
	}
	return parsed
}

// AcceleratorScan — POST /api/accelerator/scan (§14.4): вскрыть сектор серверным
// secret (RevealGridSector) и атомарно списать импульс (Repository.Reveal).
// Гейты — как у offer/boost (нет полёта, модуль/игра, active, fingerprint), но
// БЕЗ порогов остатка и отката: разведка их не тратит. Импульсов нет → 409.
func (h *TravelHandlers) AcceleratorScan(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(auth.UserIDKey).(string)
	if !ok || userID == "" {
		writeJSONError(w, "Не авторизован", http.StatusUnauthorized)
		return
	}
	var req acceleratorScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Некорректный запрос", http.StatusBadRequest)
		return
	}
	now := time.Now()
	if !h.allowAccelRequest(userID, now) {
		writeAcceleratorRefusal(w, http.StatusTooManyRequests, "rate_limited", 0)
		return
	}
	flight := h.travelManager.GetFlight(userID)
	if flight == nil {
		writeAcceleratorRefusal(w, http.StatusConflict, "no_flight", 0)
		return
	}
	if h.accelRepo == nil || h.worldRepo == nil || h.routePuzzleRepo == nil {
		writeJSONError(w, "Ускоритель недоступен", http.StatusInternalServerError)
		return
	}
	if req.Fingerprint != acceleratorFingerprint(flight) {
		writeAcceleratorRefusal(w, http.StatusConflict, "changed", 0)
		return
	}
	state, err := h.accelRepo.GetState(userID)
	if err != nil {
		log.Printf("⚠️ accelerator scan: state (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось прочитать состояние ускорителя", http.StatusInternalServerError)
		return
	}
	cooldownLeft := models.CooldownRemaining(state.LastBoostAt, state.LastCooldownMin, now)
	user, err := h.userRepo.GetByID(userID)
	if err != nil || user == nil {
		writeJSONError(w, "Пользователь не найден", http.StatusNotFound)
		return
	}
	_, cfg, found, valid := ship.AcceleratorModule(user.Equipment)
	// Разведка не тратит ни остаток, ни откат — единый гейт берётся без порогов
	// (minRemainingS=0, checkCooldown=false) при общем порядке причин §3.4/§4.1.
	if reason := acceleratorTravelReason(found, valid, ship.AcceleratorGameRegistered(cfg.Game), models.BoostActive(state.LastBoostAt, flight.StartTime), 0, 0, cooldownLeft, false); reason != "" {
		writeAcceleratorRefusal(w, http.StatusConflict, reason, cooldownLeft)
		return
	}
	target, err := h.worldRepo.GetByID(flight.ToWorld)
	if err != nil || target == nil {
		writeJSONError(w, "Мир назначения не найден", http.StatusNotFound)
		return
	}
	from, err := h.worldRepo.GetByID(flight.FromWorld)
	if err != nil || from == nil {
		writeJSONError(w, "Мир отправления не найден", http.StatusNotFound)
		return
	}
	_, st, _, err := h.acceleratorGridSegment(userID, flight, from, target)
	if err != nil {
		if errors.Is(err, errGridNoField) {
			writeAcceleratorRefusal(w, http.StatusConflict, "no_field", cooldownLeft)
			return
		}
		log.Printf("⚠️ accelerator scan: puzzle (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось получить доску", http.StatusInternalServerError)
		return
	}
	var layout routegame.GridLayout
	if err := json.Unmarshal(st.Layout, &layout); err != nil {
		log.Printf("⚠️ accelerator scan: layout (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось прочитать доску", http.StatusInternalServerError)
		return
	}
	if req.Sector < 0 || req.Sector >= len(layout.Sectors) {
		writeAcceleratorRefusal(w, http.StatusBadRequest, "bad_sector", cooldownLeft)
		return
	}
	content, err := routegame.RevealGridSector(st.Secret, layout, req.Sector)
	if err != nil {
		log.Printf("⚠️ accelerator scan: reveal (user %s, sector %d): %v", userID, req.Sector, err)
		writeJSONError(w, "Не удалось вскрыть сектор", http.StatusInternalServerError)
		return
	}
	contentJSON, err := json.Marshal(content)
	if err != nil {
		writeJSONError(w, "Не удалось вскрыть сектор", http.StatusInternalServerError)
		return
	}
	revealed, pingsLeft, err := h.routePuzzleRepo.Reveal(context.Background(), userID, acceleratorPuzzleKind, req.Sector, string(contentJSON))
	if errors.Is(err, repository.ErrNoPingsLeft) {
		writeAcceleratorRefusal(w, http.StatusConflict, "no_pings", cooldownLeft)
		return
	}
	if err != nil {
		log.Printf("⚠️ accelerator scan: reveal db (user %s): %v", userID, err)
		writeJSONError(w, "Не удалось списать импульс", http.StatusInternalServerError)
		return
	}
	writeJSON(w, acceleratorScanResponse{
		Fingerprint: acceleratorFingerprint(flight),
		Sector:      req.Sector,
		Content:     content,
		PingsLeft:   pingsLeft,
		Revealed:    acceleratorRevealedList(revealed),
	})
}
