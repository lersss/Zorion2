// internal/economy/settlement/race_balancer.go
// Расовые R-кривые (спека 99.2.23, Вариант Б — материализация): у каждой
// расы своя кривая R от среды, генерируемая из карточки ОДИН раз и
// сохраняемая как настраиваемый объект. Две копии: factory («заводские
// настройки», снимок из карточки) и active (текущая, настроенная — то, что
// использует R-модель). Хранение — файл `config/race_balancer.json`
// (env RACE_BALANCER_FILE) + in-memory store (паттерн пресетов 99.2.17 §5:
// атомарная запись tmp + os.Rename, битый JSON → лог, сервер не падает).
// Спец-случай humans: NULL/"humans" — человеческая модель ровно как сейчас
// (глобальный store 99.2.17); записи "humans" в расовом store не создаются.
// Файлы пакета: race_balancer.go (store + доступ), race_derive.go (генерация
// из карточки §3.3), race_balancer_file.go (загрузка/сохранение файла §4.4),
// race_balancer_validate.go (валидация §3.1).
package settlement

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"zorion/internal/races"
)

// RaceCurves — материализуемый объект расы: reproduction + 4 кривые
// (resilience уже вшит в Y при генерации — отдельной ручки нет, §2.4.5).
type RaceCurves struct {
	Reproduction float64                    `json:"reproduction"`
	Curves       map[string]*ComponentCurve `json:"curves"` // heat/cold/gravity/radiation
}

// RaceRecord — запись расы в config/race_balancer.json (§3.1): одна на расу.
type RaceRecord struct {
	RaceID    string     `json:"race_id"`
	CardHash  string     `json:"card_hash"` // sha256 канонического JSON карточки в момент генерации
	Factory   RaceCurves `json:"factory"`   // заводские настройки (снимок из карточки)
	Active    RaceCurves `json:"active"`    // текущая (настроенная) — использует R-модель
	UpdatedAt time.Time  `json:"updated_at"`
}

// raceBalancerStore — in-memory store расовых кривых. Чтение (R-модель на
// каждом вызове): RLock + ссылка на неизменяемый объект; запись (админка,
// редко): Lock + атомарная замена всей записи (клон) — старые объекты не
// мутируются, читать их после RUnlock безопасно.
type raceBalancerStore struct {
	mu      sync.RWMutex
	records map[string]*RaceRecord
	path    string
}

// raceBalancerCurveStore — синглтон store (имя переменной отличается от
// типа: в Go тип и переменная пакета не могут называться одинаково;
// конвенция как balancerCurveStore / popSettings).
var raceBalancerCurveStore = &raceBalancerStore{
	records: map[string]*RaceRecord{},
}

// raceComponents — порядок компонент кривых (map — нестабильный).
var raceComponents = []string{"heat", "cold", "gravity", "radiation"}

// ==================== Хэши и сравнение ====================

// CardHash — sha256 канонического JSON карточки (json.Marshal(Race) — порядок
// полей фиксирован структурой) в момент генерации (§3.1).
func CardHash(race *races.Race) string {
	data, err := json.Marshal(race)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// FactoryHash — sha256 канонического JSON factory (для меты GET curve §4.3).
func FactoryHash(rc *RaceCurves) string {
	data, err := json.Marshal(rc)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// RaceCurvesEqual — поэлементное сравнение factory и active (признак
// «не тронута»; флаг manual не хранится — производное сравнение, §2.4.4).
func RaceCurvesEqual(a, b *RaceCurves) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Reproduction != b.Reproduction {
		return false
	}
	for _, comp := range raceComponents {
		ca, cb := a.Curves[comp], b.Curves[comp]
		if ca == nil || cb == nil {
			if ca != cb {
				return false
			}
			continue
		}
		if len(ca.Nodes) != len(cb.Nodes) || len(ca.Bends) != len(cb.Bends) {
			return false
		}
		for i := range ca.Nodes {
			if ca.Nodes[i] != cb.Nodes[i] {
				return false
			}
		}
		for i := range ca.Bends {
			if ca.Bends[i] != cb.Bends[i] {
				return false
			}
		}
	}
	return true
}

// ==================== Store: чтение для R-модели ====================

// GetRaceActiveCurves — active-кривые расы (то, что использует R-модель,
// близнецы и предпросмотр). Возвращает ссылку на неизменяемый объект:
// записи заменяются атомарно под Lock (мутации клонируют), читать после
// RUnlock безопасно. ok = false — расы нет в store (гвард §3.2).
func GetRaceActiveCurves(raceID string) (*RaceCurves, bool) {
	raceBalancerCurveStore.mu.RLock()
	defer raceBalancerCurveStore.mu.RUnlock()
	rec, ok := raceBalancerCurveStore.records[raceID]
	if !ok {
		return nil, false
	}
	return &rec.Active, true
}

// SampleRaceCurve — оцифровка active-кривой расы в точках xs (для отрисовки
// в балансер-UI: единый источник математики — сервер считает evaluateCurve,
// клиент не дублирует). ok = false — расы нет в store или компоненты нет.
func SampleRaceCurve(raceID, component string, xs []float64) ([]float64, bool) {
	rc, ok := GetRaceActiveCurves(raceID)
	if !ok {
		return nil, false
	}
	c := rc.Curves[component]
	if c == nil {
		return nil, false
	}
	out := make([]float64, len(xs))
	for i, x := range xs {
		out[i] = evaluateCurve(c.Nodes, c.Bends, x)
	}
	return out, true
}

// ==================== Store: CRUD для админки ====================

// GetRaceRecordMeta — копия записи расы (для хендлеров; мутации безопасны).
func GetRaceRecordMeta(raceID string) (*RaceRecord, bool) {
	raceBalancerCurveStore.mu.RLock()
	defer raceBalancerCurveStore.mu.RUnlock()
	rec, ok := raceBalancerCurveStore.records[raceID]
	if !ok {
		return nil, false
	}
	return cloneRaceRecord(rec), true
}

// SetRaceCurve — сохранить active-кривую компоненты расы (PUT curve §4.3).
// Валидация как PUT 99.2.17 §6, но БЕЗ диапазона X компоненты (у расовых
// кривых свои диапазоны — сдвиг из карточки). Атомарная замена записи +
// запись файла (ошибка записи — лог, правка сессионная).
func SetRaceCurve(raceID, component string, curve ComponentCurve) error {
	if !ValidComponent(component) {
		return fmt.Errorf("неизвестная компонента %q (heat/cold/gravity/radiation)", component)
	}
	if err := validateRaceCurve(curve); err != nil {
		return err
	}
	raceBalancerCurveStore.mu.Lock()
	defer raceBalancerCurveStore.mu.Unlock()
	rec, ok := raceBalancerCurveStore.records[raceID]
	if !ok {
		return fmt.Errorf("раса %q не найдена в расовом store", raceID)
	}
	newRec := cloneRaceRecord(rec)
	newRec.Active.Curves[component] = cloneCurve(&curve)
	newRec.UpdatedAt = time.Now().UTC()
	raceBalancerCurveStore.records[raceID] = newRec
	if err := writeRaceBalancerFileLocked(); err != nil {
		log.Printf("race balancer: запись файла: %v (правка сессионная)", err)
	}
	return nil
}

// SetRaceReproduction — сохранить active.reproduction (PUT reproduction §4.3).
// Валидация: множитель > 0.
func SetRaceReproduction(raceID string, reproduction float64) error {
	if reproduction <= 0 {
		return fmt.Errorf("reproduction должен быть > 0, получили %v", reproduction)
	}
	raceBalancerCurveStore.mu.Lock()
	defer raceBalancerCurveStore.mu.Unlock()
	rec, ok := raceBalancerCurveStore.records[raceID]
	if !ok {
		return fmt.Errorf("раса %q не найдена в расовом store", raceID)
	}
	newRec := cloneRaceRecord(rec)
	newRec.Active.Reproduction = reproduction
	newRec.UpdatedAt = time.Now().UTC()
	raceBalancerCurveStore.records[raceID] = newRec
	if err := writeRaceBalancerFileLocked(); err != nil {
		log.Printf("race balancer: запись файла: %v (правка сессионная)", err)
	}
	return nil
}

// RegenerateRaceFactory — «Сгенерировать из карточки» (POST generate §4.3):
// factory = вывод из текущей карточки, card_hash обновляется; active НЕ
// трогается (приоритет ручной правки, §2.4.4).
func RegenerateRaceFactory(raceID string) error {
	race := races.ByID(raceID)
	if race == nil {
		return fmt.Errorf("раса %q не найдена в каталоге", raceID)
	}
	if raceID == "humans" {
		return fmt.Errorf("humans — спец-случай, заводские не выводятся из карточки")
	}
	curves, err := deriveRaceCurves(race)
	if err != nil {
		return err
	}
	raceBalancerCurveStore.mu.Lock()
	defer raceBalancerCurveStore.mu.Unlock()
	rec, ok := raceBalancerCurveStore.records[raceID]
	if !ok {
		return fmt.Errorf("раса %q не найдена в расовом store", raceID)
	}
	newRec := cloneRaceRecord(rec)
	newRec.Factory = curves
	newRec.CardHash = CardHash(race)
	newRec.UpdatedAt = time.Now().UTC()
	raceBalancerCurveStore.records[raceID] = newRec
	if err := writeRaceBalancerFileLocked(); err != nil {
		log.Printf("race balancer: запись файла: %v (правка сессионная)", err)
	}
	return nil
}

// ResetRaceToFactory — «Вернуть заводские» (POST reset-factory §4.3):
// active = factory (deep copy).
func ResetRaceToFactory(raceID string) error {
	raceBalancerCurveStore.mu.Lock()
	defer raceBalancerCurveStore.mu.Unlock()
	rec, ok := raceBalancerCurveStore.records[raceID]
	if !ok {
		return fmt.Errorf("раса %q не найдена в расовом store", raceID)
	}
	newRec := cloneRaceRecord(rec)
	newRec.Active = cloneRaceCurves(&rec.Factory)
	newRec.UpdatedAt = time.Now().UTC()
	raceBalancerCurveStore.records[raceID] = newRec
	if err := writeRaceBalancerFileLocked(); err != nil {
		log.Printf("race balancer: запись файла: %v (правка сессионная)", err)
	}
	return nil
}

// RaceStatus — строка списка рас (GET status §4.3): для селектора расы в UI
// и пометки «заводские устарели» (по card_hash).
type RaceStatus struct {
	RaceID              string `json:"race_id"`
	Name                string `json:"name"`
	HasRecord           bool   `json:"has_record"`
	CardHashOK          bool   `json:"card_hash_ok"`
	ActiveEqualsFactory bool   `json:"active_equals_factory"`
}

// RaceStatusList — статус всех рас каталога (кроме humans — спец-случай).
func RaceStatusList() []RaceStatus {
	out := make([]RaceStatus, 0, len(races.Catalog()))
	for _, race := range races.Catalog() {
		if race.ID == "humans" {
			continue
		}
		st := RaceStatus{RaceID: race.ID, Name: race.Name}
		raceBalancerCurveStore.mu.RLock()
		rec, ok := raceBalancerCurveStore.records[race.ID]
		raceBalancerCurveStore.mu.RUnlock()
		if ok {
			st.HasRecord = true
			st.CardHashOK = rec.CardHash == CardHash(race)
			st.ActiveEqualsFactory = RaceCurvesEqual(&rec.Active, &rec.Factory)
		}
		out = append(out, st)
	}
	return out
}

// ==================== Клонирование ====================

func cloneRaceCurves(rc *RaceCurves) RaceCurves {
	out := RaceCurves{
		Reproduction: rc.Reproduction,
		Curves:       make(map[string]*ComponentCurve, len(rc.Curves)),
	}
	for k, c := range rc.Curves {
		out.Curves[k] = cloneCurve(c)
	}
	return out
}

func cloneRaceRecord(rec *RaceRecord) *RaceRecord {
	out := *rec
	out.Factory = cloneRaceCurves(&rec.Factory)
	out.Active = cloneRaceCurves(&rec.Active)
	return &out
}