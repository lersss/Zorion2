// internal/models/race_ship_sprites.go
// Расовый реестр кораблей (спека
// docs/specs/2026-09-23-корабли-рас-раса-агентов-и-игрока.md §6.2, подэтап П1).
// В П1 реестр — частично: 12 людских типизированных записей (4 типа × 3
// варианта) + нейтральный корабль. Остальные 106 расовых записей дописываются
// В КОНЕЦ в П3 («только в конец», И7). Тип ShipSprite переиспользуется из
// ship_sprites.go.
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

// RaceShipSprites — расовый реестр кораблей. Порядок фиксирован (спека §6.1
// п.6): людские записи первыми, тип starship → cruiser → carrier → fighter,
// внутри типа вариант base → _02 → _03. Новые записи — только в конец (И7).
var RaceShipSprites = []ShipSprite{
	{ID: "race_humans_starship", Name: "Звёздный корабль (люди)", File: "race_humans_starship.png", Race: RaceHumans, Angle: 21.1},
	{ID: "race_humans_starship_02", Name: "Звёздный корабль (люди) · 2", File: "race_humans_starship_02.png", Race: RaceHumans},
	{ID: "race_humans_starship_03", Name: "Звёздный корабль (люди) · 3", File: "race_humans_starship_03.png", Race: RaceHumans, Angle: -39},
	{ID: "race_humans_cruiser", Name: "Крейсер (люди)", File: "race_humans_cruiser.png", Race: RaceHumans, Angle: -31.1},
	{ID: "race_humans_cruiser_02", Name: "Крейсер (люди) · 2", File: "race_humans_cruiser_02.png", Race: RaceHumans, Angle: -45, Flip: true},
	{ID: "race_humans_cruiser_03", Name: "Крейсер (люди) · 3", File: "race_humans_cruiser_03.png", Race: RaceHumans, Angle: 35, Flip: true},
	{ID: "race_humans_carrier", Name: "Носитель (люди)", File: "race_humans_carrier.png", Race: RaceHumans},
	{ID: "race_humans_carrier_02", Name: "Носитель (люди) · 2", File: "race_humans_carrier_02.png", Race: RaceHumans, Angle: -22, Flip: true},
	{ID: "race_humans_carrier_03", Name: "Носитель (люди) · 3", File: "race_humans_carrier_03.png", Race: RaceHumans, Angle: -25, Flip: true},
	{ID: "race_humans_fighter", Name: "Истребитель (люди)", File: "race_humans_fighter.png", Race: RaceHumans, Angle: 21.1, Flip: true},
	{ID: "race_humans_fighter_02", Name: "Истребитель (люди) · 2", File: "race_humans_fighter_02.png", Race: RaceHumans, Angle: -51},
	{ID: "race_humans_fighter_03", Name: "Истребитель (люди) · 3", File: "race_humans_fighter_03.png", Race: RaceHumans, Angle: -38, Flip: true},
}

// ShipOptions — транспорт реестра для /me.ship_options (спека §6.2, §7.1):
// расовые записи + нейтральный. П1–П2 = 12 людских + neutral = 13 строк;
// П3 растёт до 119.
var ShipOptions = append(append([]ShipSprite{}, RaceShipSprites...), NeutralShip)

// shipSpriteByFile — индекс реестра по имени файла (ResolveShipIcon,
// IsValidShipIcon, ShipOrientByFile); включает NeutralShip (спека §6.2, N2).
var shipSpriteByFile = func() map[string]ShipSprite {
	m := make(map[string]ShipSprite, len(RaceShipSprites)+1)
	for _, s := range RaceShipSprites {
		m[s.File] = s
	}
	m[NeutralShip.File] = NeutralShip
	return m
}()

// shipFilesByRace — индекс «раса → файлы» (порядок = порядок реестра); ключ
// "" → нейтральный пул (спека §6.2, N12). Источник состава пула и валидации.
var shipFilesByRace = func() map[string][]string {
	m := map[string][]string{}
	for _, s := range RaceShipSprites {
		m[s.Race] = append(m[s.Race], s.File)
	}
	m[""] = []string{NeutralShip.File}
	return m
}()

// RandomShipForRace — случайный файл пула расы игрока (спека §6.5, N4): у
// humans — случайный из 12 людских. Пул пуст/раса неизвестна → DefaultHumanShip
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
