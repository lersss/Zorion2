// internal/regionprofile/catalog.go — каталог классов профилей.
package regionprofile

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"zorion/internal/models"
)

// catalog — загруженный каталог классов (read-only после LoadProfiles;
// загрузка при старте до горутин — блокировка не нужна, паттерн archetypeCache).
var catalog []*Profile

// LoadProfiles — загружает каталог классов из директории (один JSON на класс,
// паттерн config/anomalies/). surfaceForms/subterrainForms — точные константы
// composition_forms.go (спека §9): неизвестные имена отклоняются.
func LoadProfiles(dir string, surfaceForms, subterrainForms []string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dir %s: %w", dir, err)
	}
	var profiles []*Profile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return fmt.Errorf("read %s: %w", e.Name(), err)
		}
		var p Profile
		if err := json.Unmarshal(data, &p); err != nil {
			return fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		if err := p.Validate(); err != nil {
			return fmt.Errorf("%s: %w", e.Name(), err)
		}
		// Списки форм передаёт main.go (composition_forms.go); пустые —
		// валидация имён пропущена (тесты без planet, цикл galaxy↔planet).
		if len(surfaceForms) > 0 || len(subterrainForms) > 0 {
			if err := p.ValidateForms(surfaceForms, subterrainForms); err != nil {
				return fmt.Errorf("%s: %w", e.Name(), err)
			}
		}
		profiles = append(profiles, &p)
	}
	catalog = profiles
	return nil
}

// Profiles — текущий каталог классов (read-only).
func Profiles() []*Profile {
	return catalog
}

// ByID — класс по ключу (nil, если нет в каталоге).
func ByID(id string) *Profile {
	for _, p := range catalog {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// PickClass — взвешенный выбор класса по weight (дефолт 1.0; редкие классы —
// вес < 1, спека §9). Пустой каталог — nil.
func PickClass(rng *rand.Rand) *Profile {
	if len(catalog) == 0 {
		return nil
	}
	total := 0.0
	for _, p := range catalog {
		total += classWeight(p)
	}
	r := rng.Float64() * total
	for _, p := range catalog {
		r -= classWeight(p)
		if r <= 0 {
			return p
		}
	}
	return catalog[len(catalog)-1]
}

func classWeight(p *Profile) float64 {
	if p.Weight <= 0 {
		return 1.0
	}
	return p.Weight
}

// NearestRegionIndex — индекс ближайшего региона к точке (спека §10: привязка
// планет к региону по координатам, та же функция, что в генерации звёзд).
func NearestRegionIndex(x, y float64, regions []*models.Region) int {
	if len(regions) == 0 {
		return -1
	}
	best := 0
	bestDist := math.Inf(1)
	for i, r := range regions {
		dx := r.CenterX - x
		dy := r.CenterY - y
		d := dx*dx + dy*dy
		if d < bestDist {
			bestDist = d
			best = i
		}
	}
	return best
}

func errInvalid(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
}