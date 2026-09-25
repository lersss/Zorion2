// internal/models/race_ship_sprites.go
// Расовый реестр кораблей (спека
// docs/specs/2026-09-23-корабли-рас-раса-агентов-и-игрока.md §6.2, подэтапы
// П1/П3). Сам реестр — данные: config/ships_registry.json, читается при старте
// (ship_registry.go). Здесь — константы и потребители реестра.
package models

import (
	"math/rand"
	"time"
)

// RaceHumans — слаг расы игрока по умолчанию (спека §4.2, §6.5).
const RaceHumans = "humans"

// SHIP_VARIANT_SLOTS — число слотов вариантов расы (как у палитры цветов,
// спека §6.2/§6.3, N8). Потребитель — клиентский pickVariant (П3).
const SHIP_VARIANT_SLOTS = 9

// DefaultHumanShip — людской корабль: DEFAULT users.ship_icon (миграция
// 000074) и страховка назначения корабля игроку (спека §4.1, §6.2, §6.5).
// Член реестра RaceShipSprites.
const DefaultHumanShip = "race_humans_starship.png"

// NeutralShip — нейтральный корабль-фолбэк (спека §6.2, §6.4): раса без
// корабля / неизвестная раса / пустой race_id. Член реестра (входит в
// shipSpriteByFile и в ship_options).
var NeutralShip = ShipSprite{ID: "neutral", Name: "Нейтральный", File: "neutral.png", Race: ""}

// RandomShipForRace — случайный файл пула расы игрока (спека §6.5, N4): у
// humans — случайный из людских. Пул пуст/раса неизвестна → DefaultHumanShip
// для humans, иначе NeutralShip (страховка). Рандом — локальный на вызов
// (AGENTS.md §0: общий *rand.Rand не потокобезопасен).
func RandomShipForRace(raceID string) string {
	pool := shipFilesByRace[raceID]
	if len(pool) == 0 {
		if raceID == RaceHumans {
			return DefaultHumanShip
		}
		return NeutralShip.File
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return pool[r.Intn(len(pool))]
}
