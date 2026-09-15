// internal/economy/settlement/settings.go
// Настройки рождаемости/СПЖ из админки (спека 99.2.16): СПЖ (дефолт 50 лет)
// и коэффициент рождаемости k (дефолт 2). In-memory: дефолты при рестарте
// (прецедент — internal/npc/settings.go). Конкурентность: ChangeComponents
// читает на каждом вызове (RLock), админка пишет редко (Lock) — RWMutex.
package settlement

import (
	"fmt"
	"sync"
)

// populationSettings — настройки рождаемости/СПЖ из админки (спека 99.2.16).
type populationSettings struct {
	mu                   sync.RWMutex
	lifeExpectancyYears  float64 // СПЖ, лет, дефолт 50 (диапазон 1..500, §3.3)
	birthRateCoefficient float64 // k, дефолт 2 (диапазон 0..10, §3.3)
}

var popSettings = populationSettings{
	lifeExpectancyYears:  50,
	birthRateCoefficient: 2,
}

// LifeExpectancyYears — СПЖ, годы (для админки; значение из настроек).
func LifeExpectancyYears() float64 {
	popSettings.mu.RLock()
	defer popSettings.mu.RUnlock()
	return popSettings.lifeExpectancyYears
}

// LifeExpectancySeconds — средняя продолжительность жизни, сек
// (заменяет константу LifeExpectancySeconds, спека 99.2.16 §3.5):
// годы × 365.25 × 24 × 3600.
func LifeExpectancySeconds() float64 {
	popSettings.mu.RLock()
	defer popSettings.mu.RUnlock()
	return popSettings.lifeExpectancyYears * 365.25 * 24 * 3600
}

// SetLifeExpectancyYears — СПЖ из админки, годы. Валидация: 1..500 (§3.3);
// значения за пределами не клампятся, а отклоняются (ошибка админа видима),
// текущее значение не меняется.
func SetLifeExpectancyYears(y float64) error {
	if y < 1 || y > 500 {
		return fmt.Errorf("СПЖ должна быть в диапазоне 1..500 лет, получили %v", y)
	}
	popSettings.mu.Lock()
	defer popSettings.mu.Unlock()
	popSettings.lifeExpectancyYears = y
	return nil
}

// BirthRateCoefficient — коэффициент рождаемости k (дефолт 2).
func BirthRateCoefficient() float64 {
	popSettings.mu.RLock()
	defer popSettings.mu.RUnlock()
	return popSettings.birthRateCoefficient
}

// SetBirthRateCoefficient — k из админки. Валидация: 0..10 (§3.3);
// k=0 — чистая убыль, отрицательный k (вторая смертность) запрещён.
func SetBirthRateCoefficient(k float64) error {
	if k < 0 || k > 10 {
		return fmt.Errorf("коэффициент рождаемости k должен быть в диапазоне 0..10, получили %v", k)
	}
	popSettings.mu.Lock()
	defer popSettings.mu.Unlock()
	popSettings.birthRateCoefficient = k
	return nil
}

// NaturalChangeRate — естественная компонента изменения за 1 секунду:
// 1/СПЖ (заменяет константу NaturalChangeRate, спека 99.2.16 §3.5).
func NaturalChangeRate() float64 {
	return 1.0 / LifeExpectancySeconds()
}

// NettoPerYearPercent — нетто-темп естественной пары за год, %:
// (1−k)/СПЖ_лет × 100 (спека 99.2.16 §3.6, §6.2 п.12). Знак минус = рост
// (согласовано с r_per_sec: r < 0 → рост); фронт переворачивает знак для
// человекочитаемости.
func NettoPerYearPercent() float64 {
	popSettings.mu.RLock()
	defer popSettings.mu.RUnlock()
	return (1 - popSettings.birthRateCoefficient) / popSettings.lifeExpectancyYears * 100
}

// ResetPopulationSettings — дефолты (тесты; «сброс» в админке не делаем).
func ResetPopulationSettings() {
	popSettings.mu.Lock()
	defer popSettings.mu.Unlock()
	popSettings.lifeExpectancyYears = 50
	popSettings.birthRateCoefficient = 2
}