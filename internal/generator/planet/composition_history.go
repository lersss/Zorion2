// internal/generator/planet/composition_history.go
//
// Этап 2 протопылевого облака (спека
// 2026-09-22-облако-этап-2-состав-и-история-формирования.md §4): состав
// каменистой/ледяной планеты перестаёт быть функцией текущей орбиты —
// резервуары + конденсация a_ice(x_form, x_ice(t_form)), миграция как ролл
// места формирования x_form, потеря мантии (выражается ТОЛЬКО в составе —
// масса заморожена этапом 1), эпоха формирования t_form, снеговая линия
// внутрь x_ice(t).
//
// compositionByZone (cascade.go) СОХРАНЕНА и обслуживает пояса
// (planet_data_belt.go); runCascade вызывает compositionFromHistory только
// для планет без MassOverride (калибровки/близнецы остаются на прежней
// функции, §1.3 спеки — 0 новых роллов).
//
// Масса в состав НЕ входит: состав — функция (x_now, x_form, x_ice(t_form), L)
// и нигде не читает mass/M_диск/giantOrbit (исключение — маркер
// stripped_embryo, читает retentionFactor, §6.4).
package planet

import (
	"math"

	"zorion/internal/models"
)

// ==================== КОНСТАНТЫ (спека §4.2–§4.6) ====================

const (
	// Снеговая линия во времени x_ice(t) (§4.2):
	// x_ice = x_ice_late + (x_ice_early − x_ice_late)·exp(−t/τ), t в млн лет.
	snowLineLate  = 2.7 // позднее/базовое значение (= база r_ice = 2.7·√L)
	snowLineEarly = 5.5 // ранний край (t = 0)
	snowTauMyr    = 0.5 // время остывания диска, млн лет

	// Конденсация (§4.6): плавный переход a_ice = σ((x_form − x_ice)/δ).
	iceDelta = 0.7

	// Поздняя эпоха — остаточная «поздняя вуаль» (§4.3): резервуара льда нет.
	lateVailAIce = 0.05

	// Эпоха формирования t_form (§4.3): дискретные полосы одного ролла u_t.
	epochEarlyHi = 0.05 // [0, 0.05) — ранняя
	epochLateHi  = 0.08 // [0.05, 0.08) — поздняя; [0.08, 1) — основная

	// Миграция (§4.4): гейт мира + per-планета (Ф2 = б).
	migGateStrongHi   = 0.02 // u_sys ∈ [0, 0.02) — сильная (2%)
	migGateModerateHi = 0.17 // [0.02, 0.17) — умеренная (15%); далее нет
	migInwardP        = 0.85 // направление внутрь (доминирующий путь)
	migStrongLo       = 2.0
	migStrongHi       = 10.0
	migModerateLo     = 1.3
	migModerateHi     = 1.9
	migMarkerMin      = 1.3 // f ≥ 1.3 → маркер migrated

	// Потеря мантии (§4.5): ролл u_loss, гейт p_loss.
	lossPLoss     = 0.25 // потеря активируется при u_loss < p_loss
	lossMin       = 0.10 // L = 0.10 + 0.50·u_l, u_l = u_loss/p_loss
	lossSpan      = 0.50
	lossResidueHi = 0.33 // u_l ≥ 0.33 → остаток iron, иначе bare_ice
	lossMarkerMin = 0.20 // L ≥ 0.20 → маркер ice_lost

	// Доли (объёмные, нормируются к 1, §4.6).
	iceJLo      = 0.40
	iceJHi      = 0.65
	iceWMax     = 0.65
	ironKLo     = 0.18
	ironKSpan   = 0.27
	ironIceDamp = 0.6
	shareFloor  = 0.02

	// Профиль p (§4.8): входной параметр, сейчас всегда 1.5 (ролл отложен до
	// перелива). Приоритет места формирования P(x_form) сейчас равномерный —
	// ProfileP не читается, задел на подстановку без переделки состава.
	cloudProfileP = 1.5
)

// Режим миграции мира (гейт Ф2 = б, §4.4): 0 — нет, 1 — умеренная, 2 — сильная.
const (
	migrationNone     = 0
	migrationModerate = 1
	migrationStrong   = 2
)

// formationParams — параметры модели формирования (§4.8).
type formationParams struct {
	ProfileP float64
}

// defaultFormationParams — дефолт (p = 1.5).
func defaultFormationParams() formationParams {
	return formationParams{ProfileP: cloudProfileP}
}

// ==================== ЧИСТЫЕ ФУНКЦИИ МОДЕЛИ ====================

// snowLineAt — снеговая линия x_ice(t) (§4.2): монотонно не возрастает,
// x_ice(0) = 5.5 → x_ice(t ≥ 5) ≈ 2.70. t — млн лет. НЕ функция sp.AgeGyr
// (диск живёт 1–10 млн лет и рассеян во всех системах, §4.2).
func snowLineAt(tMyr float64) float64 {
	if tMyr < 0 {
		tMyr = 0
	}
	return snowLineLate + (snowLineEarly-snowLineLate)*math.Exp(-tMyr/snowTauMyr)
}

// iceAvailability — вероятность конденсации льда a_ice = σ((x_form − x_ice)/δ)
// (§4.6); поздняя эпоха — остаточная вуаль (резервуара льда нет).
func iceAvailability(xForm, xIce float64, late bool) float64 {
	if late {
		return lateVailAIce
	}
	return 1 / (1 + math.Exp(-(xForm-xIce)/iceDelta))
}

// formationEpochRoll — результат ролла эпохи (§4.3).
type formationEpochRoll struct {
	tFormMyr float64 // млн лет от начала аккреции
	xIce     float64 // x_ice(t_form); у поздней не используется (a_ice = 0.05)
	early    bool
	late     bool
}

// formationEpoch — эпоха формирования по одному роллу uT (§4.3): ранняя
// [0, 0.05) / поздняя [0.05, 0.08) / основная [0.08, 1). Второго ролла нет:
// u_e — тот же uT, перемасштабированный внутри полосы.
func formationEpoch(uT float64) formationEpochRoll {
	switch {
	case uT < epochEarlyHi:
		t := 0.1 + 0.7*(uT/epochEarlyHi)
		return formationEpochRoll{tFormMyr: t, xIce: snowLineAt(t), early: true}
	case uT < epochLateHi:
		t := 10 + 90*((uT-epochEarlyHi)/(epochLateHi-epochEarlyHi))
		return formationEpochRoll{tFormMyr: t, xIce: snowLineLate, late: true}
	default:
		t := 1.5 + 3.5*((uT-epochLateHi)/(1-epochLateHi))
		return formationEpochRoll{tFormMyr: t, xIce: snowLineAt(t)}
	}
}

// migrationShift — per-планетный ролл миграции (§4.4): направление (внутрь
// P = 0.85) и фактор из одного uMig. inward → x_form = x_now·f (тело родилось
// дальше), outward → x_form = x_now/f (родилось ближе).
func migrationShift(xNow, uMig float64, strong bool) (xForm, f float64, inward bool) {
	inward = uMig < migInwardP
	var uF float64
	if inward {
		uF = uMig / migInwardP
	} else {
		uF = (uMig - migInwardP) / (1 - migInwardP)
	}
	lo, hi := migModerateLo, migModerateHi
	if strong {
		lo, hi = migStrongLo, migStrongHi
	}
	f = lo + (hi-lo)*uF
	if inward {
		return xNow * f, f, true
	}
	return xNow / f, f, false
}

// mantleLoss — потеря ледяной мантии (§4.5): условие (сдуло звездой ИЛИ
// ободрано при миграции) и величина из одного u_loss. Возвращает остаток
// (iron/bare_ice) и долю L; L = 0 — потери нет.
func mantleLoss(uLoss, aIce, xNow, f float64) (residue string, L float64) {
	swept := aIce >= 0.5 && xNow < snowLineLate // ледяное тело, мигрировавшее внутрь
	stripped := f >= 3.0                        // сильная миграция: срыв оболочки
	if (!swept && !stripped) || uLoss >= lossPLoss {
		return "", 0
	}
	uL := uLoss / lossPLoss
	L = lossMin + lossSpan*uL
	if uL >= lossResidueHi {
		return "iron", L
	}
	return "bare_ice", L
}

// normalizeShares — нормирует объёмные доли к 1 с полом 0.02 (§4.6): доли
// ниже пола фиксируются на полу, остаток распределяется по остальным.
func normalizeShares(rock, iron, ice float64) (float64, float64, float64) {
	vals := [3]float64{rock, iron, ice}
	total := 0.0
	for i := range vals {
		if vals[i] < 0 {
			vals[i] = 0
		}
		total += vals[i]
	}
	if total <= 0 {
		return 0.7, 0.25, 0.05
	}
	for i := range vals {
		vals[i] /= total
	}
	fixed := 0.0
	freeSum := 0.0
	var free []int
	for i := range vals {
		if vals[i] < shareFloor {
			vals[i] = shareFloor
			fixed += shareFloor
		} else {
			free = append(free, i)
			freeSum += vals[i]
		}
	}
	if fixed > 0 && freeSum > 0 {
		rem := 1 - fixed
		for _, i := range free {
			vals[i] = vals[i] / freeSum * rem
		}
	}
	return vals[0], vals[1], vals[2]
}

// compositionFromHistory — состав каменистой/ледяной планеты и история
// формирования (§4.3–§4.6). xNow — нормированное текущее расстояние (aNormOf;
// для P-планет r_P без √L, §4.1). Роллы uT/uMig/uLoss/uJ/uIron — по одному
// Float64; migrationMode — гейт мира (Ф2 = б); orbitIndex и giantOrbit — для
// маркера stripped_embryo. Порядок записей фиксирован (§6.2):
// formed_early/formed_late → migrated → ice_lost → stripped_embryo.
//
// p — параметр профиля (§4.8): принимается входным, ролл отложен до перелива
// (приоритет места формирования сейчас равномерный — p не читается).
func compositionFromHistory(
	xNow float64,
	uT, uMig, uLoss, uJ, uIron float64,
	migrationMode int,
	orbitIndex, giantOrbit int,
	p formationParams,
) (rock, iron, ice float64, history []models.PlanetFormationEvent) {
	_ = p // профиль p — задел §4.8 (см. комментарий выше)

	ep := formationEpoch(uT)

	xForm := xNow
	f := 1.0
	inward := false
	if migrationMode > 0 {
		xForm, f, inward = migrationShift(xNow, uMig, migrationMode == migrationStrong)
	}

	// --- Доли (§4.6) ---
	aIce := iceAvailability(xForm, ep.xIce, ep.late)
	wIce := clamp(aIce*(iceJLo+uJ*(iceJHi-iceJLo)), 0, iceWMax)
	kIron := (ironKLo + ironKSpan*uIron) * (1 - ironIceDamp*aIce)
	wIron := (1 - wIce) * kIron
	wRock := 1 - wIce - wIron

	// --- История: эпоха ---
	if ep.early {
		history = append(history, models.PlanetFormationEvent{
			Type: "formed_early", TFormMyr: ep.tFormMyr, XIce: ep.xIce,
		})
	} else if ep.late {
		history = append(history, models.PlanetFormationEvent{
			Type: "formed_late", TFormMyr: ep.tFormMyr,
		})
	}

	// --- История: миграция ---
	if migrationMode > 0 && f >= migMarkerMin {
		dir := "inward"
		if !inward {
			dir = "outward"
		}
		history = append(history, models.PlanetFormationEvent{
			Type: "migrated", XForm: xForm, XNow: xNow, Direction: dir, Factor: f,
		})
	}

	// --- Потеря мантии (§4.5): только состав, не масса ---
	if residue, L := mantleLoss(uLoss, aIce, xNow, f); L > 0 {
		if residue == "iron" {
			wIce *= 1 - L // обнажённое Fe-ядро: снимается вся оболочка (ice + rock)
			wRock *= 1 - L
		} else {
			wRock *= 1 - L // обнажённый лёд: снимается rock + iron
			wIron *= 1 - L
		}
		if L >= lossMarkerMin {
			history = append(history, models.PlanetFormationEvent{
				Type: "ice_lost", Fraction: L, Residue: residue,
			})
		}
	}

	// --- История: сорванный эмбрион (§6.4): по факту обрезки гигантом,
	// роллов не добавляет ---
	if orbitIndex > 0 && giantOrbit > 0 && retentionFactor(orbitIndex, giantOrbit) < 1 {
		history = append(history, models.PlanetFormationEvent{
			Type: "stripped_embryo", GiantOrbit: giantOrbit,
		})
	}

	rock, iron, ice = normalizeShares(wRock, wIron, wIce)
	return rock, iron, ice, history
}

// rollMigrationMode — гейт миграции мира (§4.4, Ф2 = б): один ролл на мир,
// до цикла орбит (рядом с rollCloudBudget/rollGasReservoir). Возвращает
// migrationNone/Moderate/Strong.
func (g *Generator) rollMigrationMode() int {
	u := g.rng.Float64()
	switch {
	case u < migGateStrongHi:
		return migrationStrong
	case u < migGateModerateHi:
		return migrationModerate
	default:
		return migrationNone
	}
}
