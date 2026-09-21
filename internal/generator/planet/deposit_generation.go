// internal/generator/planet/deposit_generation.go
//
// Генерация залежей поверхности (итерация 1 эпика «Экономика поселения:
// залежи + производственная ветка», спека 2026-09-22-поселение-добыча-сырья-
// биома-ленивый-буфер §3.1/§3.2): случайное число залежей из трёх пилотных
// ресурсов каталога, без чтения биомов и легаси-сводки (§3.5). Значения
// дизайн-ручек — заглушки, не калиброваны (@balancetester, §9).
package planet

import (
	"encoding/json"
	"log"
	"math/rand"
	"sync"

	"github.com/google/uuid"
	"zorion/internal/models"
)

// Дизайн-ручки залежей — значения-заглушки (§3.2 спеки; ⚠️ не калиброваны).
const (
	depositsMin       = 0      // нижняя граница числа залежей (0 — ноль залежей допустим)
	depositsMax       = 4      // верхняя граница числа залежей
	depositWealthMin  = 0.2    // нижняя граница богатства
	depositWealthMax  = 1.0    // верхняя граница богатства (0–1, §3.3)
	depositAmountBase = 1000.0 // базовый запас, условные единицы (единиц ресурса в игре ещё нет)
	depositJitter     = 0.5    // размах запаса ±50%: depositAmountBase·(1 + rng·2·jitter − jitter)

	// DepositStratumSurface — слой залежей итерации 1 (§3.3; 'subsurface' — задел).
	DepositStratumSurface = "surface"
)

// pilotDepositNames — три пилотных ресурса (goods.name_norm, §3.1).
var pilotDepositNames = []string{"растения", "мясо", "вода-ресурс"}

// StubDepositWealth — богатство залежи по заглушке ручки §3.2 (равномерно
// в [wealthMin, wealthMax]). Один источник чисел для генерации и админ-ручки.
func StubDepositWealth(rng *rand.Rand) float64 {
	return depositWealthMin + rng.Float64()*(depositWealthMax-depositWealthMin)
}

// StubDepositAmount — запас залежи по заглушке ручки §3.2:
// depositAmountBase·(1 + rng·2·jitter − jitter).
func StubDepositAmount(rng *rand.Rand) float64 {
	return depositAmountBase * (1 + rng.Float64()*2*depositJitter - depositJitter)
}

// SetGoodsIndex — карта «name_norm ресурса → goods.id» для резолва пилотов
// залежей (по образцу SetMeans/SetRaceTuning). Каталог генератор не читает:
// карту подаёт вызывающий. Пустая карта — все имена «неизвестны», залежей нет.
func (g *Generator) SetGoodsIndex(index map[string]int64) {
	g.goodsIndex = index
}

// generateDeposits — наполняет PlanetData.Deposits, если у тела есть
// поверхность (§3.2). Предикат — по данным планеты (surface_composition
// непуста и is_gas_giant ≠ true), не по ветке кода; биомы не читаются.
// Число залежей роллится всегда (даже при пустой карте goods), неизвестный
// пилот пропускается с логом (§3.1). Залежи перезаписываются — повторный
// проход не плодит дубликаты.
func (g *Generator) generateDeposits(planet *PlanetData) {
	if planet == nil || !hasSurface(planet) {
		return
	}
	n := depositsMin
	if depositsMax > depositsMin {
		n = depositsMin + g.rng.Intn(depositsMax-depositsMin+1)
	}
	deposits := make([]models.SurfaceDeposit, 0, n)
	for i := 0; i < n; i++ {
		name := pilotDepositNames[g.rng.Intn(len(pilotDepositNames))]
		goodID, ok := g.goodsIndex[name]
		if !ok {
			logMissingDepositPilot(name)
			continue
		}
		deposits = append(deposits, models.SurfaceDeposit{
			ID:       uuid.New().String(),
			PlanetID: planet.ID,
			GoodID:   goodID,
			Stratum:  DepositStratumSurface,
			Wealth:   StubDepositWealth(g.rng),
			Amount:   StubDepositAmount(g.rng),
		})
	}
	planet.Deposits = deposits
}

// depositMissingLogged — пилоты, отсутствие которых в goodsIndex уже
// залогировано: одноразовый лог на процесс (§3.1) — массовая генерация не
// заваливает вывод повторами. sync.Map — генерация однопоточная, но тесты/
// джобы процесса могут идти параллельно.
var depositMissingLogged sync.Map

// logMissingDepositPilot — одноразовый лог на неизвестный пилот (§3.1):
// первое упоминание печатается, повторы молчат.
func logMissingDepositPilot(name string) {
	if _, loaded := depositMissingLogged.LoadOrStore(name, struct{}{}); loaded {
		return
	}
	log.Printf("⚠️ залежь: ресурс %q не найден в карте goods — пропуск (повторы молчат)", name)
}

// hasSurface — предикат «у тела есть поверхность» по данным планеты (§3.2):
// surface_composition непуста и is_gas_giant ≠ true. Газовый гигант ключа
// surface_composition не имеет; у экзотического остатка композиция есть.
func hasSurface(planet *PlanetData) bool {
	if len(planet.Data) == 0 {
		return false
	}
	var probe struct {
		SurfaceComposition map[string]interface{} `json:"surface_composition"`
		IsGasGiant         bool                   `json:"is_gas_giant"`
	}
	if err := json.Unmarshal(planet.Data, &probe); err != nil {
		return false
	}
	return len(probe.SurfaceComposition) > 0 && !probe.IsGasGiant
}
