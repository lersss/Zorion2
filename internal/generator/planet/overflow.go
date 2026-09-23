// internal/generator/planet/overflow.go
//
// Перелив массы в гигантов (спека
// 2026-09-23-перелив-массы-в-гигантов-и-мини-нептуны, §4): двухпроходный
// пред-слой УРОВНЯ МИРА. Проход 1 — сырые ядра без гиганта (цикл
// «масса ↔ позиция гиганта» K1 разорван); проход 2 — орбита рождения
// g₀ = min{i : M_i⁰ > M_crit}; проходы 3–4 — класс тела по итоговой массе
// M_body (§7.1); проход 5 — пересчёт масс с f_обр ПОСЛЕ фиксации g.
package planet

import "math"

// ==================== КОНСТАНТЫ ПЕРЕЛИВА (§5, §6, §7) ====================

const (
	// overflowOrbitMax — потолок лестницы орбит (PlanetMeans.Max = 8).
	overflowOrbitMax = 8

	// zetaSigma — σ шума аккреции ζ ~ logN(0, 0.6) (§10.1). Единственный
	// источник числа для каскада и пред-слоя перелива.
	zetaSigma = 0.6

	// gasRichFraction — f_rich: доля систем, где диск сохранил газ к моменту
	// достижения ядром M_crit (§7.2). Гипотеза калибровки; f_rich ≤ 0.5 —
	// достаточное условие «гиганты не доминируют» (§5.3).
	gasRichFraction = 0.5

	// envelopeRichKappa — κ_rich: доля газового резервуара, которую связывает
	// оболочка тела на орбите рождения (§7.2). Гипотеза калибровки.
	envelopeRichKappa = 1.0

	// gasReservoirMedian — M_gas0: медиана газового резервуара системы (M⊕) —
	// ОТДЕЛЬНАЯ от твёрдого M_диск величина (§7.2). Калибровка под целевую
	// медиану GasGiantTargetMedian (§10.5): отсечение верхнего хвоста (§7.3 п.5)
	// при σ = 0.9 декады опускает медиану ПРИНЯТОЙ выборки ниже безусловной
	// κ_rich·M_gas0·w_{g₀}, поэтому параметр поднят над оценкой «≈310 при
	// w ≈ 0.12»; фактическая медиана M_body измеряется тестом O10 (§17.8).
	gasReservoirMedian = 5200.0

	// gasReservoirSigmaDecades — σ_gas: ПАРАМЕТР логнормали M_gas, 0.9 ДЕКАДЫ
	// (log10 — та же единица, что у σ эталонной массы гиганта, 99.2.15 §3.2;
	// в натуральных логарифмах это 0.9·ln10 ≈ 2.07). §7.3 п.1: эмпирическая
	// σ выборки M_body ≈ 0.62 декады — другое число (усечение + ядро).
	gasReservoirSigmaDecades = 0.9

	// gasReservoirCandidates — сколько кандидатов M_gas тянется на мир ВСЕГДА
	// (§4.4: поток RNG не ветвится по факту попадания в верхний предел):
	// применяется первый, при котором все M_body ≤ GasGiantMassMax
	// (пересэмплинг верхнего хвоста, §7.3 п.5). Первый кандидат — ролл мира.
	gasReservoirCandidates = 6

	// giantMigrationChance — P_mig: доля гигантов, дошедших до орбит 1–2
	// (горячие юпитеры, §6.3). Калибровка под §13: глобальная частота горячих
	// (P_giant(глоб.)·P_mig) обязана лежать в вилке Wright 2012 [0.5%, 1.0%];
	// при измеренном P_giant вилка даёт P_mig ≈ 0.15 (ссылка §6.3 — «≈0.20»
	// под её собственный P_giant; замер — O8/§17.7; §6.3 синхронизируется
	// дизайнером).
	giantMigrationChance = 0.15

	// minineptuneEnvelopeFrac — f_env: доля «остатка до порога» в тонкой
	// оболочке мини-нептуна (§7.2). Коридор калибровки [0.01, 0.10], канон —
	// середина. Константа, не ролл: счёт роллов пред-слоя фиксирован (§4.4).
	minineptuneEnvelopeFrac = 0.05

	// gasShiftScale — опорная база регионального сдвига: множитель
	// газоудержания 1 + shift/0.3 (§5.3; источник — рескейл §4.5 спеки
	// 2026-09-20). Точка приложения — доля газового режима f_rich, не порог.
	gasShiftScale = 0.3

	// gasShiftKMin/gasShiftKMax — границы k_region (§5.3): [1/3, 5/3].
	// Кламп — защита на будущее расширение диапазона ±0.2; в рабочем диапазоне
	// достигается только на точных границах ∓0.2.
	gasShiftKMin = 1.0 / 3.0
	gasShiftKMax = 5.0 / 3.0
)

// bodyClass — класс тела по итоговой массе M_body (§7.1: единственный
// критерий — масса; режим задаёт лишь формулу оболочки).
type bodyClass int

const (
	bodyRocky bodyClass = iota
	bodyMiniNeptune
	bodyGiant
)

// overflowPlan — результат пред-слоя перелива уровня мира (§4.2).
type overflowPlan struct {
	active     bool
	crit       float64 // M_crit — глобальный порог убегающей аккреции (§5.2)
	birthOrbit int     // g₀ — орбита рождения первичного тела (0 — нет)
	giantOrbit int     // g — орбита гиганта после миграции (0 — гиганта нет)
	gasMode    bool    // u_gas < f_rich_eff — формула оболочки (§7.2)
	coreMass   [overflowOrbitMax + 1]float64
	bodyMass   [overflowOrbitMax + 1]float64
	class      [overflowOrbitMax + 1]bodyClass
}

// ==================== ЧИСТЫЕ ФУНКЦИИ (§5.3, §7.2) ====================

// profileWeight — w_i = c_i/S₀ — нормированная доля профиля диска (§10.1).
// Единый источник для пред-слоя и каскада: значения не зависят от светимости
// (c_i считается по орбитальным индексам, самоподобие по √L).
func profileWeight(orbitIndex int) float64 {
	if orbitIndex < 1 {
		return 0
	}
	return coreMass(orbitRadiusByIndex(orbitIndex), 0) / cloudProfileSum
}

// kRegion — k_region(s) = clamp(1 + s/0.3, 1/3, 5/3) (§5.3): региональный
// множитель газоудержания. Кламп [1/3, 5/3] — защита на будущее расширение
// диапазона ±0.2 (в рабочем диапазоне достигается только на точных границах).
func kRegion(shift float64) float64 {
	return clamp(1+shift/gasShiftScale, gasShiftKMin, gasShiftKMax)
}

// gasRichFractionEffective — f_rich_eff (§5.3): региональный сдвиг
// gas_giant_shift масштабирует ДОЛЮ газового режима, а не порог (решение
// создателя 2026-09-23). Порог M_crit и граница класса 16 — ГЛОБАЛЬНЫЕ
// константы, регион их не трогает. Положительный сдвиг («газовый регион») →
// больше газа → чаще убегание оболочки; отрицательный → больше тел остаются
// мини-нептунами.
func (g *Generator) gasRichFractionEffective() float64 {
	if g.profile == nil {
		return gasRichFraction
	}
	return clamp(gasRichFraction*kRegion(g.profile.GasGiantShift(g.profileIntensity)), 0, 1)
}

// envelopeMass — масса оболочки тела с ядром core на орбите orbit (§7.2):
// газовый режим — κ_rich·M_gas·w_{g₀}; безгазовый — f_env·(16 − M_core).
func envelopeMass(core float64, orbit int, gasMode bool, mGas float64) float64 {
	if gasMode {
		return envelopeRichKappa * mGas * profileWeight(orbit)
	}
	if core >= GasGiantMassMin {
		return 0 // твёрдое ядро ≥ 16 — гигант в любом режиме (§7.1)
	}
	return minineptuneEnvelopeFrac * (GasGiantMassMin - core)
}

// classifyBodyClass — класс тела по итоговой массе M_body (§7.1). Тело с
// ядром ≤ M_crit остаётся каменистым (оболочки нет); выше — мини-нептун
// (M_body ≤ 16) либо газовый гигант (M_body > 16).
func classifyBodyClass(core, body, crit float64) bodyClass {
	if core <= crit {
		return bodyRocky
	}
	if body > GasGiantMassMin {
		return bodyGiant
	}
	return bodyMiniNeptune
}

// ==================== РОЛЛЫ ПРЕД-СЛОЯ (§4.2, §4.4) ====================

// overflowBirthOrbit — орбита рождения g₀ = min{i ∈ [1, count] : M_i⁰ > crit}
// (§4.2 проход 2): порог первым достигает САМОЕ ВНУТРЕННЕЕ ядро, чья зона
// питания достаточно массивна (профиль c_i растёт наружу). 0 — переполнения нет.
func overflowBirthOrbit(cores [overflowOrbitMax + 1]float64, count int, crit float64) int {
	for i := 1; i <= count && i <= overflowOrbitMax; i++ {
		if cores[i] > crit {
			return i
		}
	}
	return 0
}

// rollGasReservoir — M_gas ~ logN(ln M_gas0, σ_gas): ОТДЕЛЬНЫЙ от твёрдого
// M_диск газовый резервуар системы (§7.2) — один нормальный ролл на мир.
// σ задана в ДЕКАДАХ (log10), поэтому множитель — 10^(σ·z), а не e^(σ·z):
// 0.9 декады = 2.07 натуральных логарифма (единица σ эталона 99.2.15 §3.2).
func (g *Generator) rollGasReservoir() float64 {
	return gasReservoirMedian * math.Pow(10, gasReservoirSigmaDecades*g.rng.NormFloat64())
}

// rollOverflow — пред-слой перелива мира (§4.2). Требует уже сролленных
// cloudBudget и gasReservoir (порядок потока §4.4: M_диск → M_gas →
// migrationMode → ζ_1..ζ_n → режим u_gas → миграция).
//
// Роллов здесь: n (ζ) + 2 (режим u_gas, решение о миграции) + выбор орбиты
// мигрировавшего гиганта (0/1) + плавающий пересэмплинг верхнего хвоста
// системного M_gas (§7.3 п.5). От числа орбит зависит только блок ζ.
func (g *Generator) rollOverflow(count int, metallicity, crit float64) overflowPlan {
	p := overflowPlan{active: true, crit: crit}

	// Проход 1 — сырые ядра БЕЗ гиганта: ни f_обр, ни клампа (§4.2).
	metFactor := math.Pow(10, 0.5*metallicity)
	for i := 1; i <= count && i <= overflowOrbitMax; i++ {
		zeta := math.Exp(zetaSigma * g.rng.NormFloat64())
		p.coreMass[i] = g.cloudBudget * profileWeight(i) * zeta * metFactor
	}

	// Проход 2 — орбита рождения: первое (самое внутреннее) ядро выше порога.
	p.birthOrbit = overflowBirthOrbit(p.coreMass, count, crit)

	// Режим газа — один ролл на мир (§7.2). f_rich_eff — региональный
	// множитель k_region (§5.3); M_crit и граница класса 16 регионом не
	// двигаются (глобальные константы).
	p.gasMode = g.rng.Float64() < g.gasRichFractionEffective()

	// Ролл миграции и ролл орбиты миграции тянутся ВСЕГДА (поток не ветвится
	// по факту переполнения, §4.4); орбита применяется, только если мигрирует
	// первичный гигант.
	migrate := g.rng.Float64() < giantMigrationChance
	migrationTarget := 1 + g.rng.Intn(2)
	if migrationTarget > count {
		migrationTarget = 1 // в системе из одной орбиты мигрировать некуда
	}

	// Проходы 3–4 + детерминированный добор верхнего хвоста M_gas (§7.3 п.5).
	g.fillOverflowBodies(&p, count)

	// Проход 3 — миграция: мигрирует только гигант (§6.1); первичный
	// мини-нептун даёт g = 0 (миграции нет, пояс не строится, §4.2).
	if p.birthOrbit > 0 && p.class[p.birthOrbit] == bodyGiant {
		if migrate {
			// Орбиты миграции {1, 2} равномерно (§6.3).
			p.giantOrbit = migrationTarget
		} else {
			p.giantOrbit = p.birthOrbit
		}
		if p.giantOrbit != p.birthOrbit {
			// Гигант занимает орбиту g: ядро (срез твёрдого бюджета) и масса
			// переезжают с орбиты рождения; тело, стоявшее на g, вытеснено.
			// Покинутая g₀ остаётся пустой (abandonedOrbit), поэтому ядро
			// считается в твёрдой сумме ОДИН раз.
			p.coreMass[p.giantOrbit] = p.coreMass[p.birthOrbit]
			p.bodyMass[p.giantOrbit] = p.bodyMass[p.birthOrbit]
		}
		p.class[p.giantOrbit] = bodyGiant
	}
	return p
}

// fillOverflowBodies — массы и классы тел с ядром > M_crit (проходы 3–4) с
// детерминированным отсечением верхнего хвоста (§7.3 п.5). Все
// gasReservoirCandidates кандидатов M_gas тянутся ВСЕГДА — поток RNG не
// зависит от того, попал ли кандидат в предел (§4.4); применяется первый, при
// котором все M_body ≤ GasGiantMassMax (пересэмплинг, а не кламп: кламп дал бы
// пайк ровно на 4131). Класса «брак выборки» нет — M_body ≤ 16 это мини-нептун
// (§7.1), поэтому нижнего пересэмплинга нет.
//
// Предохранитель от зацикливания: число доборов жёстко ограничено
// gasReservoirCandidates — цикла нет. Если все кандидаты вышли за верх
// (теоретический хвост), оболочка у таких тел СНИМАЕТСЯ («газ не удержан»,
// как в безгазовом режиме §7.2): масса остаётся равной ядру, физический верх
// не нарушается и пайка ровно на 4131 не возникает (клампа нет нигде).
func (g *Generator) fillOverflowBodies(p *overflowPlan, count int) {
	candidates := [gasReservoirCandidates]float64{g.gasReservoir}
	for i := 1; i < gasReservoirCandidates; i++ {
		candidates[i] = g.rollGasReservoir()
	}
	for _, cand := range candidates {
		g.gasReservoir = cand
		if !g.applyEnvelopes(p, count, false) {
			return
		}
	}
	g.applyEnvelopes(p, count, true)
}

// applyEnvelopes — заполняет bodyMass/class при текущем g.gasReservoir;
// возвращает true, если хоть одно тело вышло за физический верх (нужен
// следующий кандидат M_gas). dropOver — фолбэк последнего кандидата: оболочка
// тел за верхом снимается вместо пере-ролла.
func (g *Generator) applyEnvelopes(p *overflowPlan, count int, dropOver bool) (overTop bool) {
	for i := 1; i <= count && i <= overflowOrbitMax; i++ {
		core := p.coreMass[i]
		if core <= p.crit {
			p.bodyMass[i], p.class[i] = core, bodyRocky
			continue
		}
		body := core + envelopeMass(core, i, p.gasMode, g.gasReservoir)
		if body > GasGiantMassMax {
			overTop = true
			if dropOver {
				body = core
			}
		}
		p.bodyMass[i] = body
		p.class[i] = classifyBodyClass(core, body, p.crit)
	}
	return overTop
}

// ==================== ПРОХОД 5: ФИНАЛЬНЫЕ МАССЫ (§4.2) ====================

// rockyMass — итоговая масса КАМЕНИСТОГО тела на орбите: кламп f_обр·M_i⁰
// (f_обр применён ПОСЛЕ фиксации g — цикл K1 разорван, §4.3). Для тела ветки
// оболочки не применяется (масса — bodyMass, §7.1).
func (p *overflowPlan) rockyMass(orbit int) float64 {
	if orbit < 1 || orbit > overflowOrbitMax || p.abandonedOrbit(orbit) {
		return 0
	}
	return clamp(retentionFactor(orbit, p.giantOrbit)*p.coreMass[orbit], massMin, massMax)
}

// abandonedOrbit — покинутая орбита рождения g₀ при миграции: планета там не
// формируется (щель миграции, §4.2 проход 5, §9.1). При g₀ = g покинутой
// орбиты нет — ядро гиганта стоит на своей орбите рождения.
func (p *overflowPlan) abandonedOrbit(orbit int) bool {
	if p.giantOrbit <= 0 || p.birthOrbit <= 0 || p.birthOrbit == p.giantOrbit {
		return false
	}
	return orbit == p.birthOrbit
}

