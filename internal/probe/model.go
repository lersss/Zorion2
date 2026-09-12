// internal/probe/model.go
package probe

import (
	"math"
	"math/rand"
)

// Режимы N_crit.
const (
	NCritConstant    = "constant"
	NCritProportional = "proportional"
	NCritPlanet      = "planet"
)

// Режимы hg.
const (
	HgLinked   = "linked"
	HgSeparate = "separate"
)

// Атмосферы, для которых atmo_penalty даёт штраф. Остальные — штрафа нет.
const (
	AtmoNitrogenOxygen = "азотно-кислородная"
	AtmoCO2            = "углекислая"
	AtmoToxic          = "ядовитая"
)

// Классы планет для группового режима.
const (
	ClassEarthlike = "землеподобные"
	ClassIce       = "ледяные"
	ClassVolcanic  = "вулканические"
	ClassGas       = "газовые"
	ClassOther     = "прочие"
)

// CurveParams — параметры кривой смертности. Точка в пространстве пробы.
// Пресет — именованный набор таких параметров.
type CurveParams struct {
	PBase          float64    `json:"p_base"`
	KBase          float64    `json:"k_base"`
	Alpha          float64    `json:"alpha"`
	NCritMode      string     `json:"n_crit_mode"`
	NCrit          float64    `json:"n_crit"`
	NDead          float64    `json:"n_dead"`
	T0             float64    `json:"t0"`
	HgMode         string     `json:"hg_mode"`
	TMin           float64    `json:"t_min"`
	TMax           float64    `json:"t_max"`
	AtmoPenalty    [3]float64 `json:"atmo_penalty"`
	WaterThreshold float64    `json:"water_threshold"`
	WaterPenalty   float64    `json:"water_penalty"`
}

// DefaultCurveParams — стартовые значения. Рабочие диапазоны — задача пробы.
func DefaultCurveParams() CurveParams {
	return CurveParams{
		PBase:          500000,
		KBase:          0.05,
		Alpha:          1,
		NCritMode:      NCritConstant,
		NCrit:          10000,
		NDead:          100,
		T0:             0,
		HgMode:         HgLinked,
		TMin:           200,
		TMax:           350,
		AtmoPenalty:    [3]float64{1.0, 0.5, 0.2},
		WaterThreshold: 10,
		WaterPenalty:   0.5,
	}
}

// Curve — вычисленная кривая одного поселения.
type Curve struct {
	PlanetID string  `json:"planet_id"`
	WorldID  string  `json:"world_id"`
	Class    string  `json:"class"`
	HTemp    float64 `json:"h_temp"`
	HAtmo    float64 `json:"h_atmo"`
	HWater   float64 `json:"h_water"`
	HPlanet  float64 `json:"h_planet"`
	P0       float64 `json:"p0"`
	K        float64 `json:"k"`
	NCrit    float64 `json:"n_crit"`
	T        float64 `json:"t"`
	Alpha    float64 `json:"alpha"`
	NDead    float64 `json:"n_dead"`
	T0       float64 `json:"t0"`
	Lifetime float64 `json:"lifetime"`
}

// hTemp — 1 внутри комфортной зоны, линейно убывает до 0.1 за границей.
// Ширина затухания — ширина комфортной зоны (TMax - TMin).
func (cp CurveParams) hTemp(temp float64) float64 {
	if temp >= cp.TMin && temp <= cp.TMax {
		return 1.0
	}
	width := cp.TMax - cp.TMin
	if width <= 0 {
		return 0.1
	}
	d := 0.0
	if temp < cp.TMin {
		d = (cp.TMin - temp) / width
	} else {
		d = (temp - cp.TMax) / width
	}
	if d > 1 {
		return 0.1
	}
	return 1 - 0.9*d
}

// hAtmo — штраф за атмосферу из atmo_penalty, остальные — без штрафа.
func (cp CurveParams) hAtmo(atmosphere string) float64 {
	switch atmosphere {
	case AtmoNitrogenOxygen:
		return cp.AtmoPenalty[0]
	case AtmoCO2:
		return cp.AtmoPenalty[1]
	case AtmoToxic:
		return cp.AtmoPenalty[2]
	}
	return 1.0
}

// hWater — 1 выше порога воды, иначе штраф.
func (cp CurveParams) hWater(waterPercent float64) float64 {
	if waterPercent > cp.WaterThreshold {
		return 1.0
	}
	return cp.WaterPenalty
}

// Habitability — пригодность планеты и три фактора.
//
// linked:   h = temp × atmo × water
// separate: h = atmo × water (температура уходит в g)
func (cp CurveParams) Habitability(temp, waterPercent float64, atmosphere string) (hTemp, hAtmo, hWater, hPlanet float64) {
	hTemp = cp.hTemp(temp)
	hAtmo = cp.hAtmo(atmosphere)
	hWater = cp.hWater(waterPercent)
	if cp.HgMode == HgSeparate {
		hPlanet = clamp(hAtmo*hWater, 0.1, 1.0)
	} else {
		hPlanet = clamp(hTemp*hAtmo*hWater, 0.1, 1.0)
	}
	return
}

// G — множитель темпа смертности.
//
// linked:   g = 1/h_planet (жёсткая связь старта и темпа)
// separate: g = 1/h_temp  (темп от температуры, старт от воды и атмосферы)
func (cp CurveParams) G(hPlanet, hTemp float64) float64 {
	if cp.HgMode == HgSeparate {
		return clamp(1.0/hTemp, 0.1, 10)
	}
	return clamp(1.0/hPlanet, 0.1, 10)
}

// NCritValue — порог скученности по режиму.
//
// constant:     значение N_crit как есть
// proportional: N_crit = N_crit × p0
// planet:       N_crit = N_crit × h_planet
func (cp CurveParams) NCritValue(p0, hPlanet float64) float64 {
	switch cp.NCritMode {
	case NCritProportional:
		return cp.NCrit * p0
	case NCritPlanet:
		return cp.NCrit * hPlanet
	}
	return cp.NCrit
}

// ComputeCurve — кривая поселения на планете. rnd — локальный rand на вызов.
func (cp CurveParams) ComputeCurve(planetID, worldID, class string, temp, waterPercent float64, atmosphere string, rnd *rand.Rand) Curve {
	hTemp, hAtmo, hWater, hPlanet := cp.Habitability(temp, waterPercent, atmosphere)

	p0 := cp.PBase * hPlanet * (0.7 + rnd.Float64()*0.6)
	k := cp.KBase * cp.G(hPlanet, hTemp)
	nCrit := cp.NCritValue(p0, hPlanet)
	if nCrit <= 0 {
		nCrit = 1
	}

	alpha := cp.Alpha
	if alpha < 0 {
		alpha = 0
	}

	t := 0.0
	if alpha > 0 && k > 0 {
		t = math.Pow(p0/nCrit, alpha) / (alpha * k)
	}

	return Curve{
		PlanetID: planetID,
		WorldID:  worldID,
		Class:    class,
		HTemp:    hTemp,
		HAtmo:    hAtmo,
		HWater:   hWater,
		HPlanet:  hPlanet,
		P0:       p0,
		K:        k,
		NCrit:    nCrit,
		T:        t,
		Alpha:    alpha,
		NDead:    cp.NDead,
		T0:       cp.T0,
		Lifetime: cp.Lifetime(p0, k, t),
	}
}

// Lifetime — время пересечения порога N_dead, сутки от основания.
// p0 <= N_dead — поселение мертво с рождения.
func (cp CurveParams) Lifetime(p0, k, t float64) float64 {
	if p0 <= cp.NDead {
		return 0
	}
	if cp.Alpha > 0 {
		if k <= 0 {
			return math.Inf(1)
		}
		tt := t * (1 - math.Pow(cp.NDead/p0, cp.Alpha))
		if tt < 0 {
			tt = 0
		}
		return cp.T0 + tt
	}
	// α = 0: экспонента, нуля не достигает, обрезается порогом N_dead.
	if k <= 0 {
		return math.Inf(1)
	}
	return cp.T0 + math.Log(p0/cp.NDead)/k
}

// PopulationAt — население в момент t (сутки от основания поселения).
func (cp CurveParams) PopulationAt(c Curve, t float64) float64 {
	if c.P0 <= cp.NDead {
		return 0
	}
	if t < c.T0 {
		return c.P0
	}
	tt := t - c.T0

	var p float64
	if c.Alpha > 0 {
		frac := 1 - tt/c.T
		if frac <= 0 {
			return 0
		}
		p = c.P0 * math.Pow(frac, 1.0/c.Alpha)
	} else {
		p = c.P0 * math.Exp(-c.K*tt)
	}
	if p <= cp.NDead {
		return 0
	}
	return p
}

// ClassForType — класс для группового режима по геймдизайнерскому типу планеты.
func ClassForType(gdType string) string {
	switch gdType {
	case "землеподобная":
		return ClassEarthlike
	case "ледяная":
		return ClassIce
	case "вулканическая":
		return ClassVolcanic
	case "газовый гигант":
		return ClassGas
	}
	return ClassOther
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}