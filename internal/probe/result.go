// internal/probe/result.go
package probe

import (
	"math"
	"sort"
)

// RunOptions — параметры прогона (не входят в пресет).
type RunOptions struct {
	Seed        int64   `json:"seed"`
	SampleSize  int     `json:"sample_size"`
	HorizonDays float64 `json:"horizon_days"`
}

// DefaultRunOptions — стартовые значения прогона.
func DefaultRunOptions() RunOptions {
	return RunOptions{Seed: 42, SampleSize: 0, HorizonDays: 7300}
}

// Result — результат одного прогона. Сохраняется в файл и держится в памяти.
type Result struct {
	ID        string      `json:"id"`
	PresetID  string      `json:"preset_id"`
	CreatedAt string      `json:"created_at"`
	Params    CurveParams `json:"params"`
	Run       RunOptions  `json:"run"`
	Total     int         `json:"total"`
	Stats     Stats       `json:"stats"`
	Curves    []Curve     `json:"curves"`
}

// HistBin — бин гистограммы времени жизни.
type HistBin struct {
	Lo    float64 `json:"lo"`
	Hi    float64 `json:"hi"`
	Count int     `json:"count"`
}

// SurvivalPoint — доля живых поселений в момент t.
type SurvivalPoint struct {
	T     float64 `json:"t"`
	Alive float64 `json:"alive"`
}

// GroupStats — статистика по классу планет.
type GroupStats struct {
	Count          int       `json:"count"`
	MedianDays     float64   `json:"median_days"`
	Q25Days        float64   `json:"q25_days"`
	Q75Days        float64   `json:"q75_days"`
	MinDays        float64   `json:"min_days"`
	MaxDays        float64   `json:"max_days"`
	ExtinctFraction float64  `json:"extinct_fraction"`
	Histogram      []HistBin `json:"histogram"`
}

// Stats — скалярные метрики, гистограмма, кривая выживаемости, группы.
type Stats struct {
	Count           int                   `json:"count"`
	MedianDays      float64               `json:"median_days"`
	Q25Days         float64               `json:"q25_days"`
	Q75Days         float64               `json:"q75_days"`
	MinDays         float64               `json:"min_days"`
	MaxDays         float64               `json:"max_days"`
	ExtinctFraction float64               `json:"extinct_fraction"`
	Histogram       []HistBin             `json:"histogram"`
	Survival        []SurvivalPoint       `json:"survival"`
	Groups          map[string]GroupStats `json:"groups"`
}

// ComputeStats — статистика по кривым. Гистограмма и выживаемость — в
// единицах горизонта прогона; пережившие горизонт — в последний бин.
func ComputeStats(curves []Curve, horizonDays float64) Stats {
	if horizonDays <= 0 {
		horizonDays = 7300
	}
	binWidth := horizonDays / 100.0

	lifetimes := make([]float64, len(curves))
	hist := make([]int, 101) // 100 бинов до горизонта + последний «>горизонт»
	for i, c := range curves {
		lt := c.Lifetime
		if math.IsInf(lt, 1) {
			lt = horizonDays
		}
		lifetimes[i] = lt
		idx := int(lt / binWidth)
		if idx >= 100 {
			idx = 100
		}
		hist[idx]++
	}

	total := len(curves)
	histBins := make([]HistBin, 0, 101)
	for i := 0; i < 100; i++ {
		histBins = append(histBins, HistBin{
			Lo:    float64(i) * binWidth,
			Hi:    float64(i+1) * binWidth,
			Count: hist[i],
		})
	}
	histBins = append(histBins, HistBin{
		Lo:    horizonDays,
		Hi:    horizonDays,
		Count: hist[100],
	})

	alive0 := 0
	for _, lt := range lifetimes {
		if lt > 0 {
			alive0++
		}
	}
	survival := make([]SurvivalPoint, 0, 101)
	for i := 0; i <= 100; i++ {
		t := float64(i) * binWidth
		alive := 0
		for _, lt := range lifetimes {
			if lt > t {
				alive++
			}
		}
		survival = append(survival, SurvivalPoint{T: t, Alive: frac(alive, total)})
	}

	sorted := append([]float64(nil), lifetimes...)
	sort.Float64s(sorted)

	groups := map[string]GroupStats{}
	for _, c := range curves {
		gs := groups[c.Class]
		gs.Count++
		groups[c.Class] = gs
	}
	groupHist := map[string][]int{}
	groupLt := map[string][]float64{}
	for _, c := range curves {
		lt := c.Lifetime
		if math.IsInf(lt, 1) {
			lt = horizonDays
		}
		groupLt[c.Class] = append(groupLt[c.Class], lt)
		idx := int(lt / binWidth)
		if idx >= 100 {
			idx = 100
		}
		groupHist[c.Class] = append(groupHist[c.Class], idx)
	}
	for cls, gs := range groups {
		lt := groupLt[cls]
		gh := make([]int, 101)
		for _, idx := range groupHist[cls] {
			gh[idx]++
		}
		bins := make([]HistBin, 0, 101)
		for i := 0; i < 100; i++ {
			bins = append(bins, HistBin{Lo: float64(i) * binWidth, Hi: float64(i+1) * binWidth, Count: gh[i]})
		}
		bins = append(bins, HistBin{Lo: horizonDays, Hi: horizonDays, Count: gh[100]})
		gs.Histogram = bins
		gs.MedianDays, gs.Q25Days, gs.Q75Days, gs.MinDays, gs.MaxDays = quantiles(lt)
		gs.ExtinctFraction = extinctFrac(lt, horizonDays)
		groups[cls] = gs
	}

	minDays, maxDays := 0.0, 0.0
	if len(sorted) > 0 {
		minDays = sorted[0]
		maxDays = sorted[len(sorted)-1]
	}

	return Stats{
		Count:           total,
		MedianDays:      quantile(sorted, 0.5),
		Q25Days:         quantile(sorted, 0.25),
		Q75Days:         quantile(sorted, 0.75),
		MinDays:         minDays,
		MaxDays:         maxDays,
		ExtinctFraction: extinctFrac(sorted, horizonDays),
		Histogram:       histBins,
		Survival:        survival,
		Groups:          groups,
	}
}

func quantiles(xs []float64) (med, q25, q75, minV, maxV float64) {
	if len(xs) == 0 {
		return 0, 0, 0, 0, 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	return quantile(s, 0.5), quantile(s, 0.25), quantile(s, 0.75), s[0], s[len(s)-1]
}

// quantile — линейная интерполяция порядка p на отсортированном слайсе.
func quantile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	pos := p * float64(n-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	f := pos - float64(lo)
	return sorted[lo]*(1-f) + sorted[hi]*f
}

func extinctFrac(sorted []float64, horizonDays float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	dead := 0
	for _, lt := range sorted {
		if lt <= horizonDays {
			dead++
		}
	}
	return frac(dead, len(sorted))
}

func frac(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}