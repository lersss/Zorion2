// internal/generator/planet/composition_compatibility.go
package planet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// CompatibilityMatrix — хранит "запрещённые пары" для поверхности и недр.
// Формат JSON: "форма": ["несовместимая1", "несовместимая2", ...].
// Симметрия подразумевается: если A запрещает B, то и B запрещает A.
type CompatibilityMatrix struct {
	Surface    map[string][]string `json:"surface"`
	Subterrain map[string][]string `json:"subterrain"`

	// Внутренние индексы для быстрой проверки
	surfacePairs    map[string]bool
	subterrainPairs map[string]bool
}

var (
	compatMatrix *CompatibilityMatrix
	compatMu     sync.RWMutex
)

// LoadCompatibilityMatrix — загружает матрицу из JSON. Если файла нет,
// использует встроенные дефолты.
func LoadCompatibilityMatrix(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		compatMu.Lock()
		compatMatrix = defaultCompatibilityMatrix()
		compatMu.Unlock()
		return nil
	}

	var m CompatibilityMatrix
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	m.buildIndexes()

	compatMu.Lock()
	compatMatrix = &m
	compatMu.Unlock()
	return nil
}

// IsCompatible — совместимы ли две формы в категории.
// По умолчанию true, если пара не указана в матрице.
// category: "surface" | "subterrain"
func IsCompatible(category, a, b string) bool {
	compatMu.RLock()
	defer compatMu.RUnlock()

	if compatMatrix == nil {
		return !autoIncompatible(category, a, b)
	}
	if a == b {
		return true
	}
	key := pairKey(a, b)
	switch category {
	case "surface":
		if compatMatrix.surfacePairs[key] {
			return false
		}
	case "subterrain":
		if compatMatrix.subterrainPairs[key] {
			return false
		}
	}
	return !autoIncompatible(category, a, b)
}

// autoIncompatible — авто-часть матрицы несовместимости (99.2.28 §5.6):
// пары с разными жидкими средами из {вода, метан, аммиак, co2} запрещены
// на одной поверхности; лава/магмовый океан несовместимы с водными.
// Считается от справочника биомов (liquid_medium/volcanism).
func autoIncompatible(category, a, b string) bool {
	if category != "surface" || a == b {
		return false
	}
	cat := GetBiomeCatalog()
	ba := cat.BiomeByID(a)
	bb := cat.BiomeByID(b)
	if ba == nil || bb == nil {
		return false
	}
	la, lb := ba.LiquidMedium, bb.LiquidMedium
	if la != lb && liquidSet[la] && liquidSet[lb] {
		return true
	}
	// Лава/магмовый океан vs водные (крио-водные — в ручной матрице).
	if (ba.Volcanism == "hot" || ba.Volcanism == "magma") && lb == "вода" {
		return true
	}
	if (bb.Volcanism == "hot" || bb.Volcanism == "magma") && la == "вода" {
		return true
	}
	return false
}

// liquidSet — жидкие среды, участвующие в авто-несовместимости.
var liquidSet = map[string]bool{"вода": true, "метан": true, "аммиак": true, "co2": true}

// IsCompositionValid — все ли пары форм в композиции совместимы
func IsCompositionValid(category string, c Composition) bool {
	forms := make([]string, 0, len(c))
	for k, v := range c {
		if v > 0.01 {
			forms = append(forms, k)
		}
	}
	for i := 0; i < len(forms); i++ {
		for j := i + 1; j < len(forms); j++ {
			if !IsCompatible(category, forms[i], forms[j]) {
				return false
			}
		}
	}
	return true
}

// GetCompatibilityMatrix — текущая матрица (для API/админки)
func GetCompatibilityMatrix() *CompatibilityMatrix {
	compatMu.RLock()
	defer compatMu.RUnlock()
	return compatMatrix
}

// ==================== ВНУТРЕННЕЕ ====================

// pairKey — ключ пары (отсортированные имена через "|")
func pairKey(a, b string) string {
	if a > b {
		a, b = b, a
	}
	return a + "|" + b
}

// buildIndexes — строит внутренние индексы для быстрой проверки
func (m *CompatibilityMatrix) buildIndexes() {
	m.surfacePairs = make(map[string]bool)
	m.subterrainPairs = make(map[string]bool)

	for a, list := range m.Surface {
		for _, b := range list {
			m.surfacePairs[pairKey(a, b)] = true
		}
	}
	for a, list := range m.Subterrain {
		for _, b := range list {
			m.subterrainPairs[pairKey(a, b)] = true
		}
	}
}

// ==================== ДЕФОЛТНАЯ МАТРИЦА ====================

func defaultCompatibilityMatrix() *CompatibilityMatrix {
	m := &CompatibilityMatrix{
		Surface: map[string][]string{
			SurfaceLavaFields: {
				SurfaceGlaciers, SurfaceFrozenGases, SurfaceOceans, SurfaceLakes,
				SurfaceForests, SurfaceJungles, SurfaceSwamps, SurfaceMeadows,
				SurfaceCoralReefs,
			},
			SurfaceGlaciers: {
				SurfaceJungles, SurfaceSwamps,
			},
			SurfaceFrozenGases: {
				SurfaceJungles, SurfaceForests,
			},
			SurfaceCraters: {
				SurfaceForests, SurfaceJungles, SurfaceSwamps, SurfaceMeadows,
				SurfaceCoralReefs,
			},
		},
		Subterrain: map[string][]string{
			SubterrainMagmaChambers: {
				SubterrainGroundIce, SubterrainCoalSeams, SubterrainOilPockets,
				SubterrainGasPockets, SubterrainGroundwater, SubterrainSaltDomes,
			},
			SubterrainRadioactiveZones: {
				SubterrainOilPockets, SubterrainCoalSeams,
				SubterrainGasPockets, SubterrainGroundwater,
			},
		},
	}
	m.buildIndexes()
	return m
}