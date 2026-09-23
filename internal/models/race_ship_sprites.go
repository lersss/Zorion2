// internal/models/race_ship_sprites.go
// Расовый реестр кораблей (спека
// docs/specs/2026-09-23-корабли-рас-раса-агентов-и-игрока.md §6.2, подэтапы
// П1/П3). Полный реестр: 12 людских типизированных записей (4 типа × 3
// варианта) + 105 расовых + нейтральный корабль. Людские записи первыми
// (фиксированный порядок), расовые дописаны В КОНЕЦ в порядке ships_meta.json
// («только в конец», И7). Тип ShipSprite переиспользуется из ship_sprites.go.
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
	// Остальные 105 расовых записей (спека 2026-09-23 §6.1 п.6, §12 П3):
	// дописаны В КОНЕЦ после людского блока — порядок ships_meta.json,
	// «только в конец» (И7). Ориентация — из меты при orient_meta:true.
	{ID: "race_coastal_01", Name: "Прибрежные", File: "race_coastal_01.png", Race: "coastal", Angle: -30.2},
	{ID: "race_sulfur_nests_01", Name: "Серные гнёзда", File: "race_sulfur_nests_01.png", Race: "sulfur_nests", Angle: -33.7, Flip: true},
	{ID: "race_silicon_guardians_01", Name: "Кремниевые стражи", File: "race_silicon_guardians_01.png", Race: "silicon_guardians", Angle: 91.3, Flip: true},
	{ID: "race_methane_plankton_01", Name: "Метан-планктон", File: "race_methane_plankton_01.png", Race: "methane_plankton", Angle: 89.2, Flip: true},
	{ID: "race_ammonia_02", Name: "Аммиачники · 2", File: "race_ammonia_02.png", Race: "ammonia", Angle: -37, Flip: true},
	{ID: "race_ammonia_03", Name: "Аммиачники · 3", File: "race_ammonia_03.png", Race: "ammonia", Angle: -124},
	{ID: "race_archivists_01", Name: "Архивариусы", File: "race_archivists_01.png", Race: "archivists", Angle: -47, Flip: true},
	{ID: "race_archivists_02", Name: "Архивариусы · 2", File: "race_archivists_02.png", Race: "archivists", Angle: -94},
	{ID: "race_arks_01", Name: "Ковчеги", File: "race_arks_01.png", Race: "arks", Angle: -3},
	{ID: "race_arks_02", Name: "Ковчеги · 2", File: "race_arks_02.png", Race: "arks", Angle: 25},
	{ID: "race_ashfolk_01", Name: "Пепельные", File: "race_ashfolk_01.png", Race: "ashfolk", Angle: -49},
	{ID: "race_awakened_01", Name: "Пробуждённые", File: "race_awakened_01.png", Race: "awakened", Angle: -49, Flip: true},
	{ID: "race_carbonic_03", Name: "Углекислые · 3", File: "race_carbonic_03.png", Race: "carbonic", Angle: -47},
	{ID: "race_carbonic_04", Name: "Углекислые · 4", File: "race_carbonic_04.png", Race: "carbonic", Angle: 124, Flip: true},
	{ID: "race_coastal_02", Name: "Прибрежные · 2", File: "race_coastal_02.png", Race: "coastal", Angle: -38},
	{ID: "race_coastal_03", Name: "Прибрежные · 3", File: "race_coastal_03.png", Race: "coastal", Angle: -40, Flip: true},
	{ID: "race_cryo_corals_01", Name: "Крио-кораллы", File: "race_cryo_corals_01.png", Race: "cryo_corals", Angle: -33, Flip: true},
	{ID: "race_cryo_corals_02", Name: "Крио-кораллы · 2", File: "race_cryo_corals_02.png", Race: "cryo_corals", Angle: 180},
	{ID: "race_cryo_forest_01", Name: "Крио-лесные", File: "race_cryo_forest_01.png", Race: "cryo_forest", Angle: -36.6},
	{ID: "race_cryo_forest_02", Name: "Крио-лесные · 2", File: "race_cryo_forest_02.png", Race: "cryo_forest", Angle: -38.1},
	{ID: "race_cryo_swarms_01", Name: "Крио-рои", File: "race_cryo_swarms_01.png", Race: "cryo_swarms", Angle: -6.5},
	{ID: "race_cryo_swarms_02", Name: "Крио-рои · 2", File: "race_cryo_swarms_02.png", Race: "cryo_swarms", Angle: 89.6, Flip: true},
	{ID: "race_crystallites_03", Name: "Кристаллиты · 3", File: "race_crystallites_03.png", Race: "crystallites", Angle: 91.5, Flip: true},
	{ID: "race_crystallites_04", Name: "Кристаллиты · 4", File: "race_crystallites_04.png", Race: "crystallites", Angle: 126.4, Flip: true},
	{ID: "race_deep_dwellers_01", Name: "Глубинники", File: "race_deep_dwellers_01.png", Race: "deep_dwellers", Angle: -38},
	{ID: "race_deep_dwellers_02", Name: "Глубинники · 2", File: "race_deep_dwellers_02.png", Race: "deep_dwellers", Angle: -54},
	{ID: "race_deep_sky_01", Name: "Небесные глубокие", File: "race_deep_sky_01.png", Race: "deep_sky", Angle: 45},
	{ID: "race_diamond_01", Name: "Алмазные", File: "race_diamond_01.png", Race: "diamond", Angle: -43.7},
	{ID: "race_diamond_02", Name: "Алмазные · 2", File: "race_diamond_02.png", Race: "diamond", Angle: -44.9, Flip: true},
	{ID: "race_diskfolk_01", Name: "Дисковые", File: "race_diskfolk_01.png", Race: "diskfolk", Angle: 89.1, Flip: true},
	{ID: "race_diskfolk_02", Name: "Дисковые · 2", File: "race_diskfolk_02.png", Race: "diskfolk", Angle: 91.6, Flip: true},
	{ID: "race_echo_aliens_01", Name: "Эхо чужих", File: "race_echo_aliens_01.png", Race: "echo_aliens", Angle: -27.1, Flip: true},
	{ID: "race_echo_aliens_02", Name: "Эхо чужих · 2", File: "race_echo_aliens_02.png", Race: "echo_aliens", Angle: 2},
	{ID: "race_forges_01", Name: "Горнила", File: "race_forges_01.png", Race: "forges", Angle: -91.6},
	{ID: "race_forges_02", Name: "Горнила · 2", File: "race_forges_02.png", Race: "forges", Angle: -26.8, Flip: true},
	{ID: "race_hot_ash_01", Name: "Жаркие пепловые", File: "race_hot_ash_01.png", Race: "hot_ash", Angle: -49.4},
	{ID: "race_hot_ash_02", Name: "Жаркие пепловые · 2", File: "race_hot_ash_02.png", Race: "hot_ash", Angle: -30, Flip: true},
	{ID: "race_hydrogen_01", Name: "Водородные", File: "race_hydrogen_01.png", Race: "hydrogen", Angle: 2.5, Flip: true},
	{ID: "race_ice_herders_01", Name: "Ледяные пастухи", File: "race_ice_herders_01.png", Race: "ice_herders", Angle: -43.1},
	{ID: "race_ice_herders_02", Name: "Ледяные пастухи · 2", File: "race_ice_herders_02.png", Race: "ice_herders", Angle: 88.3, Flip: true},
	{ID: "race_lava_01", Name: "Лавовые", File: "race_lava_01.png", Race: "lava", Angle: -45, Flip: true},
	{ID: "race_lava_02", Name: "Лавовые · 2", File: "race_lava_02.png", Race: "lava", Angle: -43.7, Flip: true},
	{ID: "race_magnetophages_01", Name: "Магнитофаги", File: "race_magnetophages_01.png", Race: "magnetophages", Angle: 3.3, Flip: true},
	{ID: "race_magnetophages_02", Name: "Магнитофаги · 2", File: "race_magnetophages_02.png", Race: "magnetophages", Angle: 91},
	{ID: "race_methane_fungi_01", Name: "Метан-грибницы", File: "race_methane_fungi_01.png", Race: "methane_fungi", Angle: -43.6},
	{ID: "race_methane_fungi_02", Name: "Метан-грибницы · 2", File: "race_methane_fungi_02.png", Race: "methane_fungi", Angle: -98},
	{ID: "race_methane_plankton_02", Name: "Метан-планктон · 2", File: "race_methane_plankton_02.png", Race: "methane_plankton", Angle: 24, Flip: true},
	{ID: "race_methane_plankton_03", Name: "Метан-планктон · 3", File: "race_methane_plankton_03.png", Race: "methane_plankton", Angle: 5},
	{ID: "race_microcrack_01", Name: "Микротрещинные", File: "race_microcrack_01.png", Race: "microcrack", Angle: 94.9, Flip: true},
	{ID: "race_microcrack_02", Name: "Микротрещинные · 2", File: "race_microcrack_02.png", Race: "microcrack", Angle: -68.7},
	{ID: "race_mist_swarms_01", Name: "Туман-рои", File: "race_mist_swarms_01.png", Race: "mist_swarms", Angle: 116, Flip: true},
	{ID: "race_mist_swarms_02", Name: "Туман-рои · 2", File: "race_mist_swarms_02.png", Race: "mist_swarms", Angle: 38.2, Flip: true},
	{ID: "race_mistfolk_01", Name: "Туманники", File: "race_mistfolk_01.png", Race: "mistfolk", Angle: 5.1, Flip: true},
	{ID: "race_mistfolk_02", Name: "Туманники · 2", File: "race_mistfolk_02.png", Race: "mistfolk", Angle: 31.2},
	{ID: "race_nether_01", Name: "Преисподние", File: "race_nether_01.png", Race: "nether", Angle: -43, Flip: true},
	{ID: "race_nether_02", Name: "Преисподние · 2", File: "race_nether_02.png", Race: "nether", Angle: 95, Flip: true},
	{ID: "race_oceanids_01", Name: "Океаниды", File: "race_oceanids_01.png", Race: "oceanids", Angle: -35.5},
	{ID: "race_oceanids_02", Name: "Океаниды · 2", File: "race_oceanids_02.png", Race: "oceanids", Angle: 39},
	{ID: "race_philosophers_01", Name: "Философы", File: "race_philosophers_01.png", Race: "philosophers", Angle: 33, Flip: true},
	{ID: "race_philosophers_02", Name: "Философы · 2", File: "race_philosophers_02.png", Race: "philosophers", Angle: -44.3, Flip: true},
	{ID: "race_piezo_minds_01", Name: "Пьезо-разум", File: "race_piezo_minds_01.png", Race: "piezo_minds", Angle: 38.6},
	{ID: "race_piezo_minds_02", Name: "Пьезо-разум · 2", File: "race_piezo_minds_02.png", Race: "piezo_minds", Angle: -88.9},
	{ID: "race_pulsar_01", Name: "Пульсарные", File: "race_pulsar_01.png", Race: "pulsar", Angle: -48, Flip: true},
	{ID: "race_pulsar_02", Name: "Пульсарные · 2", File: "race_pulsar_02.png", Race: "pulsar", Angle: -15},
	{ID: "race_radiotolerant_01", Name: "Радиотолерантные", File: "race_radiotolerant_01.png", Race: "radiotolerant", Angle: 87, Flip: true},
	{ID: "race_radiotolerant_02", Name: "Радиотолерантные · 2", File: "race_radiotolerant_02.png", Race: "radiotolerant", Angle: -42.1},
	{ID: "race_salt_bridge_01", Name: "Маргулы", File: "race_salt_bridge_01.png", Race: "salt_bridge", Angle: -52},
	{ID: "race_salt_bridge_02", Name: "Маргулы · 2", File: "race_salt_bridge_02.png", Race: "salt_bridge", Angle: -71},
	{ID: "race_saltfolk_01", Name: "Кшарры", File: "race_saltfolk_01.png", Race: "saltfolk", Angle: -43.4, Flip: true},
	{ID: "race_saltfolk_02", Name: "Кшарры · 2", File: "race_saltfolk_02.png", Race: "saltfolk", Angle: 4},
	{ID: "race_scavengers_01", Name: "Санитары", File: "race_scavengers_01.png", Race: "scavengers", Angle: -84, Flip: true},
	{ID: "race_scavengers_02", Name: "Санитары · 2", File: "race_scavengers_02.png", Race: "scavengers", Angle: -89},
	{ID: "race_silicate_swarms_01", Name: "Силикатные рои", File: "race_silicate_swarms_01.png", Race: "silicate_swarms", Angle: -42.8, Flip: true},
	{ID: "race_silicate_swarms_02", Name: "Силикатные рои · 2", File: "race_silicate_swarms_02.png", Race: "silicate_swarms", Angle: 89},
	{ID: "race_silicon_guardians_02", Name: "Кремниевые стражи · 2", File: "race_silicon_guardians_02.png", Race: "silicon_guardians", Angle: 89.9, Flip: true},
	{ID: "race_silicon_guardians_03", Name: "Кремниевые стражи · 3", File: "race_silicon_guardians_03.png", Race: "silicon_guardians", Angle: -87.2},
	{ID: "race_skyfolk_01", Name: "Небесные", File: "race_skyfolk_01.png", Race: "skyfolk", Angle: -89},
	{ID: "race_skyfolk_02", Name: "Небесные · 2", File: "race_skyfolk_02.png", Race: "skyfolk", Angle: -50, Flip: true},
	{ID: "race_sleepers_01", Name: "Спящие", File: "race_sleepers_01.png", Race: "sleepers", Angle: -56, Flip: true},
	{ID: "race_sleepers_02", Name: "Спящие · 2", File: "race_sleepers_02.png", Race: "sleepers", Angle: 5.6, Flip: true},
	{ID: "race_smokers_01", Name: "Курильщики", File: "race_smokers_01.png", Race: "smokers", Angle: -51, Flip: true},
	{ID: "race_smokers_02", Name: "Курильщики · 2", File: "race_smokers_02.png", Race: "smokers", Angle: 147},
	{ID: "race_spark_01", Name: "Искра", File: "race_spark_01.png", Race: "spark", Angle: 44},
	{ID: "race_spark_02", Name: "Искра · 2", File: "race_spark_02.png", Race: "spark", Angle: 28.7},
	{ID: "race_star_shepherds_01", Name: "Звёздные пастыри", File: "race_star_shepherds_01.png", Race: "star_shepherds", Angle: -141},
	{ID: "race_sublimators_01", Name: "Сублиматоры", File: "race_sublimators_01.png", Race: "sublimators", Angle: -34, Flip: true},
	{ID: "race_sulfur_nests_04", Name: "Серные гнёзда · 4", File: "race_sulfur_nests_04.png", Race: "sulfur_nests", Angle: -48, Flip: true},
	{ID: "race_sulfur_nests_05", Name: "Серные гнёзда · 5", File: "race_sulfur_nests_05.png", Race: "sulfur_nests", Angle: 36, Flip: true},
	{ID: "race_sulfur_steppes_01", Name: "Серные степи", File: "race_sulfur_steppes_01.png", Race: "sulfur_steppes", Angle: 156, Flip: true},
	{ID: "race_sulfur_swarms_01", Name: "Серные рои", File: "race_sulfur_swarms_01.png", Race: "sulfur_swarms", Angle: -139},
	{ID: "race_sulfur_swarms_02", Name: "Серные рои · 2", File: "race_sulfur_swarms_02.png", Race: "sulfur_swarms", Angle: 26, Flip: true},
	{ID: "race_sulfur_wanderers_01", Name: "Серные странники", File: "race_sulfur_wanderers_01.png", Race: "sulfur_wanderers", Angle: -91},
	{ID: "race_sulfur_wanderers_02", Name: "Серные странники · 2", File: "race_sulfur_wanderers_02.png", Race: "sulfur_wanderers", Angle: -13.8},
	{ID: "race_supercritical_01", Name: "Сверхкритические", File: "race_supercritical_01.png", Race: "supercritical", Angle: 165, Flip: true},
	{ID: "race_thermo_swarms_01", Name: "Термо-рои", File: "race_thermo_swarms_01.png", Race: "thermo_swarms", Angle: -145},
	{ID: "race_thermo_swarms_02", Name: "Термо-рои · 2", File: "race_thermo_swarms_02.png", Race: "thermo_swarms", Angle: -141},
	{ID: "race_tidal_01", Name: "Приливные", File: "race_tidal_01.png", Race: "tidal", Angle: 40, Flip: true},
	{ID: "race_tidal_02", Name: "Приливные · 2", File: "race_tidal_02.png", Race: "tidal", Angle: 106, Flip: true},
	{ID: "race_volcanites_01", Name: "Вулканиты", File: "race_volcanites_01.png", Race: "volcanites", Angle: -40.6, Flip: true},
	{ID: "race_vortex_01", Name: "Вихревые", File: "race_vortex_01.png", Race: "vortex", Angle: 90},
	{ID: "race_vortex_02", Name: "Вихревые · 2", File: "race_vortex_02.png", Race: "vortex", Angle: -14},
	{ID: "race_world_machines_01", Name: "Миры-машины", File: "race_world_machines_01.png", Race: "world_machines", Angle: -31, Flip: true},
	{ID: "race_young_01", Name: "Молодые", File: "race_young_01.png", Race: "young", Angle: 57},
	{ID: "race_young_02", Name: "Молодые · 2", File: "race_young_02.png", Race: "young", Angle: 104.3, Flip: true},
	{ID: "race_silicon_threads_01", Name: "Кремниевые нити", File: "race_silicon_threads_01.png", Race: "silicon_threads", Angle: -24.5},
}

// ShipOptions — транспорт реестра для /me.ship_options (спека §6.2, §7.1):
// 117 расовых записей + нейтральный = 118 строк (П3).
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
