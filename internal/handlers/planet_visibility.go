// internal/handlers/planet_visibility.go
// Скрытие деталей планет для role=player (спека 77a §6.2/§11.2): поверхность
// и наличие поселений — только при знании в player_planet_knowledge (с датой,
// И8); недра/атмосфера/детали поселений — всегда скрыты (сканер их не
// вскрывает, §6.2). Без знания — Knowledge=nil, клиент показывает
// «нет данных — купить отчёт». admin/skycomposer — без фильтра (И7).
//
// Режимы показа (спека 2026-09-23-орбита-планеты-присутствие-и-снимок §5.1):
// presence (игрок на орбите/поверхности планеты или её спутника) → полные
// данные поверхности и поселений; snapshot (есть data.snapshot) → замороженная
// картина из снимка; scan (знание без снимка) → скан-уровень; none (знания нет)
// → заглушка. Приоритет: presence → snapshot → scan → none.
package handlers

import (
	"encoding/json"
	"time"

	"zorion/internal/models"
	"zorion/internal/repository"
)

// Режимы знания планеты (спека 2026-09-23 §5.1).
const (
	knowledgeModePresence = "presence"
	knowledgeModeSnapshot = "snapshot"
	knowledgeModeScan     = "scan"
)

// applyPlanetVisibility — применяет знание к планетам системы для player и
// выставляет режим показа каждой планеты (§5.1). presenceID — планета
// присутствия игрока (§2.1, спутник — по родителю; "" — присутствия нет):
// у неё полные данные, у остальных — snapshot/scan/none. can_buy_report
// (§5.2) — истина только при присутствии на ДРУГОЙ планете.
func applyPlanetVisibility(userID string, planets []models.Planet, knowledge *repository.KnowledgeRepository, presenceID string, arithmeticVisible bool) []models.Planet {
	now := time.Now()
	out := make([]models.Planet, 0, len(planets))
	for _, p := range planets {
		k, err := knowledge.GetKnowledge(userID, p.ID)
		if err != nil {
			k = nil
		}
		view := buildKnowledgeView(k, now)
		p.CanBuyReport = presenceID != "" && p.ID != presenceID

		switch {
		case presenceID != "" && p.ID == presenceID:
			// Присутствие — живой канал (§5.1): знание не обязательно, режим
			// не зависит от записи в player_planet_knowledge.
			if view == nil {
				view = &models.PlanetKnowledgeView{}
			}
			view.Mode = knowledgeModePresence
			out = append(out, stripPlanetDetails(p, view, arithmeticVisible))
		case view == nil:
			// Знания нет — заглушка «нет данных» (knowledge == nil).
			out = append(out, stripPlanetDetails(p, nil, arithmeticVisible))
		default:
			if snap, ok := parseSnapshot(k); ok {
				// Снимок приоритетнее скана (§5.1, решение О1): свежий скан не
				// перебивает «как было на момент отлёта».
				view.Mode = knowledgeModeSnapshot
				view.SnapshotAt = &snap.at
				view.SnapshotFresh = now.Sub(snap.at) <= models.KnowledgeTTL
				p = applySnapshotToPlanet(p, snap)
			} else {
				view.Mode = knowledgeModeScan
			}
			out = append(out, stripPlanetDetails(p, view, arithmeticVisible))
		}
	}
	return out
}

// buildKnowledgeView — поля знания игрока о планете (спека 77a §8.2 + дельта
// 2026-09-23 §5.2): существующие поля (surface_*/settlements_count/scanned_at/
// fresh) + дата снимка (snapshot_at/snapshot_fresh). nil, если знания нет.
func buildKnowledgeView(k *models.PlanetKnowledge, now time.Time) *models.PlanetKnowledgeView {
	if k == nil {
		return nil
	}
	view := &models.PlanetKnowledgeView{
		ScannedAt: k.ScannedAt,
		Fresh:     k.IsFresh(now),
	}
	if sd, ok := k.Data["surface_dominant"].(string); ok {
		view.SurfaceDominant = sd
	}
	if sc, ok := k.Data["surface_composition"].(map[string]interface{}); ok {
		view.SurfaceComposition = toFloatMap(sc)
	}
	if n, ok := k.Data["settlements_count"].(float64); ok {
		view.SettlementsCount = int(n)
	}
	if snap, ok := parseSnapshot(k); ok {
		at := snap.at
		view.SnapshotAt = &at
		view.SnapshotFresh = now.Sub(at) <= models.KnowledgeTTL
	}
	return view
}

// parsedSnapshot — внутренний разбор data.snapshot (§3.1) для раскладки в уже
// существующие поля ответа (§5.2). Отдельного DTO ответа нет.
type parsedSnapshot struct {
	at                 time.Time
	SurfaceDominant    string
	SurfaceComposition map[string]float64
	Settlements        []models.Settlement
	Factions           []models.PlanetFaction
	Buildings          []models.PlanetBuilding
}

// parseSnapshot — читает data.snapshot из записи знания. ok=false, если снимка
// нет или он нечитаем (тогда планета — скан-уровня). Момент снимка — в at.
func parseSnapshot(k *models.PlanetKnowledge) (parsedSnapshot, bool) {
	if k == nil {
		return parsedSnapshot{}, false
	}
	raw, ok := k.Data["snapshot"]
	if !ok || raw == nil {
		return parsedSnapshot{}, false
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return parsedSnapshot{}, false
	}
	var s struct {
		At                 string                  `json:"at"`
		SurfaceDominant    string                  `json:"surface_dominant"`
		SurfaceComposition map[string]float64      `json:"surface_composition"`
		Settlements        []models.Settlement     `json:"settlements"`
		Factions           []models.PlanetFaction  `json:"factions"`
		Buildings          []models.PlanetBuilding `json:"buildings"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return parsedSnapshot{}, false
	}
	at, err := time.Parse(time.RFC3339, s.At)
	if err != nil {
		return parsedSnapshot{}, false
	}
	return parsedSnapshot{
		at:                 at,
		SurfaceDominant:    s.SurfaceDominant,
		SurfaceComposition: s.SurfaceComposition,
		Settlements:        s.Settlements,
		Factions:           s.Factions,
		Buildings:          s.Buildings,
	}, true
}

// applySnapshotToPlanet — раскладывает содержимое снимка в существующие поля
// планеты (§5.2): поверхность, поселения (models.Settlement — без эффектов и
// лога), фракции, строения; население — сумма поселений снимка.
func applySnapshotToPlanet(p models.Planet, snap parsedSnapshot) models.Planet {
	p.SurfaceDominant = snap.SurfaceDominant
	p.SurfaceComposition = snap.SurfaceComposition
	p.Settlements = snap.Settlements
	p.Factions = snap.Factions
	p.Buildings = snap.Buildings
	var pop int64
	for i := range snap.Settlements {
		pop += int64(snap.Settlements[i].Population)
	}
	p.Population = pop
	return p
}

// stripPlanetDetails — убирает детали планеты из ответа для player в
// соответствии с режимом знания (§5.1). Базовые поля объекта (имя, тип, орбита,
// физика) остаются (спека 77a §5.2). Знание (если есть) — в p.Knowledge.
//
//   - presence: поверхность + поселения (раса/население/стабильность/ветки/
//     лог, эффекты — player-safe DTO), фракции, строения, активные залежи;
//     блок арифметики — только при arithmeticVisible (настройка §8.3);
//   - snapshot: замороженная картина (уже разложена в поля), без эффектов и лога,
//     блок арифметики не замораживается — чистится всегда (§8.3 п.5);
//   - scan/none (и пустой режим — совместимость вызовов): поверхность + счётчик,
//     детали поселений скрыты.
func stripPlanetDetails(p models.Planet, view *models.PlanetKnowledgeView, arithmeticVisible bool) models.Planet {
	p.Knowledge = view
	mode := ""
	if view != nil {
		mode = view.Mode
	}
	// Физика — скрыта всегда, в любом режиме (§1.4): недра/атмосфера/ядро/
	// описание/биомы игроку не показываются ни сканом, ни присутствием.
	p.SubterrainComposition = nil
	// Биомы/недры объектами (99.2.28 §9.3) — тоже скрыты для player: утечка
	// И1 с релиза 99.2.28 (находка 2026-09-20 §10.5). Картинка планеты из
	// биомов при этом остаётся допустимой — вид разрешён создателем (С1/гейт 2).
	p.Biomes = nil
	p.Subterrain = nil
	p.Atmosphere = ""
	p.Hydrosphere = ""
	p.Biosphere = ""
	p.Archetype = ""
	p.AtmosphereData = nil
	p.LiquidWaterPossible = false
	p.OrbitalPeriod = 0
	p.Eccentricity = 0
	p.EscapeVelocity = 0
	p.TidalLock = false
	p.Core = nil
	p.Description = ""
	p.SystemAge = 0
	p.Moons = 0
	p.Radioactive = false
	// Спутники — объекты системы; их детали (поверхность/атмосфера) тоже
	// скрыты для player (консистентность с §5.2, M5: деталей спутника нет).
	for i := range p.Satellites {
		s := &p.Satellites[i]
		s.SurfaceDominant = ""
		s.SurfaceComposition = nil
		s.SubterrainComposition = nil
		s.Atmosphere = ""
		s.Biosphere = ""
		s.Description = ""
	}

	switch mode {
	case knowledgeModePresence:
		// Полные данные присутствия (§5.1): ветки без входа, эффекты —
		// player-safe DTO без нагрузки/порога/силы (§5.4), залежи — активные.
		// Блок арифметики и новые поля веток — только при включённой настройке
		// (§8.3 п.4); type_id/type_name стадии остаются всегда.
		stripBranchInputs(&p)
		p.Settlements = playerSettlements(p.Settlements, arithmeticVisible)
		p.Deposits = filterActiveDeposits(p.Deposits)
	case knowledgeModeSnapshot:
		// Замороженная картина (§3.1): эффектов и лога нет (в снимок не
		// замораживаются); вход веток — никогда; блок арифметики не
		// замораживается — чистится всегда (§8.3 п.5); залежи — как сегодня.
		stripBranchInputs(&p)
		stripSnapshotSettlementSecrets(&p)
		p.Deposits = filterActiveDeposits(p.Deposits)
	default:
		// scan/none: поверхность и наличие поселений — только в knowledge;
		// детали поселений/фракций/строений — за знанием (§6.2).
		p.SurfaceDominant = ""
		p.SurfaceComposition = nil
		p.Population = 0
		stripBranchInputs(&p)
		p.Settlements = nil
		if view == nil {
			p.Factions = nil
			p.Buildings = nil
			// Залежи поверхности (спека 2026-09-22-поселение-... §5.1): без знания
			// о планете игрок залежей не видит — защита в глубину (как фракции/
			// строения); со знанием остаются. attachDeposits зовётся только в
			// GetPlanetsByWorldID — других путей к игроку нет.
			p.Deposits = nil
			// История формирования (спека 2026-09-22-облако-этап-2-... §6.5):
			// значок в карточке виден только со знанием о планете — без знания
			// маркер скрыт (защита в глубину, как фракции/строения).
			p.FormationHistory = nil
		} else {
			// Со знанием игрок видит только активные залежи (спека итерации 3
			// §5.2/п.26): выработанные (amount = 0) скрыты от player и видны
			// только админу (admin идёт мимо stripPlanetDetails).
			p.Deposits = filterActiveDeposits(p.Deposits)
		}
	}
	return p
}

// playerSettlements — витрина поселений игрока в присутствии (§5.1/§5.4):
// копия списка с убранными служебными чек-точками (population_exact/computed_at/
// R-компоненты — §15) и player-safe DTO эффектов. Лог и ветки остаются. Новые
// витринные поля арифметики (блок позиций, число скорости/признак/«забираем» на
// ветке) остаются только при arithmeticVisible — иначе явная очистка (§8.3 п.4).
func playerSettlements(in []models.Settlement, arithmeticVisible bool) []models.Settlement {
	if len(in) == 0 {
		return in
	}
	out := make([]models.Settlement, len(in))
	copy(out, in)
	for i := range out {
		out[i].PopulationExact = 0
		out[i].ComputedAt = time.Time{}
		out[i].RPerSec = 0
		out[i].LambdaPerHour = 0
		out[i].NDead = 0
		out[i].Effects = effectViews(out[i].Effects)
		if !arithmeticVisible {
			out[i].Arithmetic = nil
			out[i].Branches = copyBranches(out[i].Branches)
			for j := range out[i].Branches {
				stripBranchArithmetic(&out[i].Branches[j])
			}
		}
	}
	return out
}

// copyBranches — копия среза веток перед точечной правкой: копия поселений выше
// поверхностная, и очистка не должна писать в исходный срез (иначе витрина
// побочно мутирует планеты запроса — новая точка алиасинга). nil → nil.
func copyBranches(in []models.SettlementBranch) []models.SettlementBranch {
	if in == nil {
		return nil
	}
	out := make([]models.SettlementBranch, len(in))
	copy(out, in)
	return out
}

// stripBranchArithmetic — очистка новых витринных полей ветки (§8.3 п.4): число
// скорости пары, признак «рецепт не в наборе стадии», «забираем», доля залежи.
func stripBranchArithmetic(b *models.SettlementBranch) {
	b.RatePerDayPerBillion = nil
	b.NotInStageSet = false
	b.Take = nil
	b.DepositShare = 0
}

// planetsHaveSettlementArithmetic — есть ли в планетах новые витринные поля
// арифметики (блок позиций или поля ветки). Нужен, чтобы читать настройку
// видимости только когда есть что скрывать (§8.3): иначе лишний запрос к БД на
// каждую карточку без арифметики.
func planetsHaveSettlementArithmetic(planets []models.Planet) bool {
	for i := range planets {
		for j := range planets[i].Settlements {
			s := &planets[i].Settlements[j]
			if len(s.Arithmetic) > 0 {
				return true
			}
			for k := range s.Branches {
				b := &s.Branches[k]
				if b.RatePerDayPerBillion != nil || b.NotInStageSet || len(b.Take) > 0 || b.DepositShare != 0 {
					return true
				}
			}
		}
	}
	return false
}

// stripSnapshotSettlementSecrets — страховка в глубину для снимка: эффектов и
// лога в снимке нет (§3.1, п.3 решения); чек-точки снимок не несёт; блок
// арифметики в снимок не замораживается — чистится ВСЕГДА (§8.3 п.5).
func stripSnapshotSettlementSecrets(p *models.Planet) {
	for i := range p.Settlements {
		s := &p.Settlements[i]
		s.Effects = nil
		s.Log = nil
		s.PopulationExact = 0
		s.ComputedAt = time.Time{}
		s.RPerSec = 0
		s.LambdaPerHour = 0
		s.NDead = 0
		s.Arithmetic = nil
		s.Branches = copyBranches(s.Branches)
		for j := range s.Branches {
			stripBranchArithmetic(&s.Branches[j])
		}
	}
}

// effectViews — player-safe DTO эффектов (§5.4): только name/impact/state.
// Нагрузка/порог/кривая/владелец/сила R(load) игроку не отдаются. Пустой
// список → nil (ключ `effects` не выводится).
func effectViews(v interface{}) interface{} {
	list, ok := v.([]models.ActiveEffect)
	if !ok || len(list) == 0 {
		return nil
	}
	out := make([]models.EffectView, 0, len(list))
	for _, e := range list {
		state := "inactive"
		if e.Enabled {
			state = "active"
		}
		out = append(out, models.EffectView{Name: e.Name, Impact: e.Impact, State: state})
	}
	return out
}

// stripBranchInputs — обнуляет входной буфер веток поселений (спека
// 2026-09-22-поселение-ветка-буферы-переработка §6): вход видит только админ,
// выход остаётся частью деталей поселения. Защита в глубину — вызывается
// безусловно в stripPlanetDetails, на случай будущего потребителя, отдающего
// поселения игроку.
func stripBranchInputs(p *models.Planet) {
	for i := range p.Settlements {
		for j := range p.Settlements[i].Branches {
			p.Settlements[i].Branches[j].Input = nil
		}
	}
}

// filterActiveDeposits — оставляет залежи с запасом > 0 (спека итерации 3
// §5.2/п.26): выработанная залежь (`amount = 0`) скрыта от игрока; админ идёт
// мимо stripPlanetDetails и видит её с пометкой «выработано».
func filterActiveDeposits(deposits []models.SurfaceDeposit) []models.SurfaceDeposit {
	if len(deposits) == 0 {
		return deposits
	}
	out := make([]models.SurfaceDeposit, 0, len(deposits))
	for _, d := range deposits {
		if d.Amount > 0 {
			out = append(out, d)
		}
	}
	return out
}

// applyBeltVisibility — выдача поясов игроку (спека
// 2026-09-22-пояса-малых-тел-этап-2-показ-знание-полёт §4.2/§4.3): только
// visible=true (фильтр ДО маппинга); состав (composition) раскрывается при
// знании — система в радиусе радара (inRadar) ИЛИ присутствие игрока в поясе
// (myPosition: orbit на belt с этим id). Без знания состав не отдаётся.
func applyBeltVisibility(belts []models.Belt, inRadar bool, myPosition *models.CurrentPosition) []models.BeltView {
	out := make([]models.BeltView, 0, len(belts))
	for _, b := range belts {
		if !b.Visible {
			continue
		}
		revealed := inRadar || beltPresence(myPosition, b.ID)
		out = append(out, stripBeltDetails(b, revealed))
	}
	return out
}

// visibleBelts — только видимые игроку пояса (visible=true, §4.2): пояс
// visible=false игроку не отдаётся и не является валидной целью полёта.
func visibleBelts(belts []models.Belt) []models.Belt {
	out := make([]models.Belt, 0, len(belts))
	for _, b := range belts {
		if b.Visible {
			out = append(out, b)
		}
	}
	return out
}

// beltPresence — игрок физически в этом поясе (позиция orbit/mining на belt,
// §4.3 условие 2): присутствие даёт состав без сканера. `mining` (спека поясов
// этап 3 §6.4) — тоже присутствие: иначе во время захода состав/уровень запаса
// внезапно скрылись бы, а при выходе вернулись.
func beltPresence(pos *models.CurrentPosition, beltID string) bool {
	return pos != nil && (pos.Status == "orbit" || pos.Status == "mining") &&
		pos.ObjectType == "belt" && pos.ObjectID == beltID
}

// stripBeltDetails — маппит запись system_belts в BeltView для игрока (спека
// 2026-09-22-пояса-малых-тел-этап-2-показ-знание-полёт §4.2): базовые поля
// (тип/имя/геометрия/типичное тело/масса) открыты; composition — только при
// знании (compositionRevealed, §4.3); data/visible/created_at/updated_at не
// отдаются. models.Belt напрямую игроку не сериализуется.
func stripBeltDetails(b models.Belt, compositionRevealed bool) models.BeltView {
	v := models.BeltView{
		ID:         b.ID,
		WorldID:    b.WorldID,
		Kind:       b.Kind,
		Name:       b.Name,
		OrbitIndex: b.OrbitIndex,
		RadiusAU:   b.RadiusAU,
		WidthAU:    b.WidthAU,
		Mass:       b.Mass,
		BodySizeKm: b.BodySizeKm,
	}
	if compositionRevealed {
		v.Composition = b.Composition
		// Две шкалы запаса (спека поясов этап 3 §8.4) — только при знании, как
		// composition: belt_class по composition.iron; remaining_level по
		// f = iron_remaining/reserve. Точные числа клиенту не отдаются.
		iron := 0.0
		ice := 0.0
		if b.Composition != nil {
			iron = b.Composition["iron"]
			ice = b.Composition["ice"]
		}
		if iron > 0 {
			v.BeltClass = beltClassByIron(iron)
		}
		if b.IronRemaining != nil {
			v.RemainingLevel = beltRemainingLevel(*b.IronRemaining, beltReserve(iron))
		}
		// Лёд (спека 2026-09-24 §5.7): класс по k_ice и уровень остатка запаса
		// льда — тот же гейт знания. Точные числа клиенту не отдаются.
		if ice > 0 {
			v.IceClass = beltIceClassByIce(ice)
		}
		if b.IceRemaining != nil {
			v.RemainingLevelIce = beltRemainingLevel(*b.IceRemaining, beltIceReserve)
		}
	}
	return v
}

// toFloatMap — map[string]interface{} → map[string]float64 (значения-числа).
func toFloatMap(m map[string]interface{}) map[string]float64 {
	out := make(map[string]float64, len(m))
	for k, v := range m {
		if f, ok := v.(float64); ok {
			out[k] = f
		}
	}
	return out
}
