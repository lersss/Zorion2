package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"

	"zorion/internal/models"
	"zorion/internal/routegame"
	"zorion/internal/ship"
	"zorion/internal/travel"
)

// accelerator_grid_handlers.go — доска v9 «Планшет» мини-игры «Прокладка
// маршрута»: публичный слой поля в offer и разведка секторов
// (POST /api/accelerator/scan). Спека
// 2026-09-25-ускоритель-и-мини-игра-прокладка-маршрута §14 (14.1 наследуемое,
// 14.3 секторы/σ, 14.4 разведка, 14.8 что видит игрок). Старая (v1) модель поля
// и оценки полилинии удалена — прод-потребителей у неё не было; offer/boost/scan
// работают на модели v9 «Планшет».
//
// Разделение слоёв: публичная доска (маяки, объекты, клетки и подписи σ)
// выводится из seed; скрытое содержимое секторов — из серверного secret.
// secret, реализованный слой (realized) и содержимое НЕ отдаются клиенту.
//
// Награду в v9-пути задаёт кривая самой модели (bonusCostGrid, может быть
// отрицательной), а не параметр модуля: ship.AcceleratorBonus и
// params.bonus_max в v9 НЕ участвуют. Оставлены по развилке создателя —
// прогрессия модуля (§14.14 спеки ускорителя), прод-путей на них нет.
//
// Файл держит сегмент/задачу (seed, hash, puzzle); DTO — в
// accelerator_grid_dto.go, публичная доска — accelerator_grid_board.go,
// ручка разведки — accelerator_grid_scan.go.

// acceleratorGridPings — импульсы разведки на сегмент (§14.4 R3; значение
// эталонного харнесса v9 — DefaultV9Config.Pings). На будущее — параметр тира
// модуля (accel_N); сейчас константа.
const acceleratorGridPings = 2

// acceleratorPuzzleKind — вид задачи в player_route_puzzle (расширяемость kind).
const acceleratorPuzzleKind = ship.AcceleratorGameRoute

// errGridNoField — поле сегмента не удалось собрать за отведённые попытки
// (гейт §14.2 L_naive > L_safe не выполнен): offer/scan отвечают отказом.
var errGridNoField = errors.New("routegame: no acceptable grid field")

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
