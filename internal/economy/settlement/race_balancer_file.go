// internal/economy/settlement/race_balancer_file.go
// Файловый слой расовых R-кривых (спека 99.2.23 §4.4): загрузка при старте
// сервера (паттерн 99.2.17 §5 «Старт») и атомарная запись
// config/race_balancer.json (tmp + os.Rename, битый JSON → лог, сервер не
// падает).
package settlement

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"zorion/internal/races"
)

// raceBalancerFile — формат файла (§3.1): плоский список записей.
type raceBalancerFile struct {
	Races []*RaceRecord `json:"races"`
}

// LoadRaceBalancer — старт сервера (паттерн 99.2.17 §5 «Старт»):
// 1) прочитать config/race_balancer.json (если есть); каждая запись проходит
// валидацию §3.1; невалидные пропускаются с логом; 2) для каждой расы
// каталога: записи нет → сгенерировать factory/active из карточки, записать
// в store и файл (атомарно); запись есть → не трогать (даже при устаревшем
// card_hash — только пометка в UI); 3) битый JSON → лог, store пуст, расы
// инициализируются из карточек (файл пересоздаётся при первой записи).
// Сервер НЕ падает.
func LoadRaceBalancer(path string) error {
	if path == "" {
		path = "config/race_balancer.json"
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("race balancer: не удалось создать каталог %s: %w", dir, err)
	}

	raceBalancerCurveStore.mu.Lock()
	raceBalancerCurveStore.path = path
	raceBalancerCurveStore.records = map[string]*RaceRecord{}
	raceBalancerCurveStore.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("race balancer: чтение %s: %w", path, err)
		}
	} else {
		var f raceBalancerFile
		if err := json.Unmarshal(data, &f); err != nil {
			log.Printf("race balancer: битый JSON в %s: %v (store пуст, расы инициализируются из карточек)", path, err)
		} else {
			for _, rec := range f.Races {
				if err := validateRaceRecord(rec); err != nil {
					log.Printf("race balancer: запись %q пропущена: %v", rec.RaceID, err)
					continue
				}
				raceBalancerCurveStore.mu.Lock()
				raceBalancerCurveStore.records[rec.RaceID] = cloneRaceRecord(rec)
				raceBalancerCurveStore.mu.Unlock()
			}
		}
	}

	// Авто-инициализация: раса без записи → factory/active из карточки
	// («один раз при создании расы», §2.4.3). humans — спец-случай, запись
	// не создаётся (§2.3).
	changed := false
	for _, race := range races.Catalog() {
		if race.ID == "humans" {
			continue
		}
		raceBalancerCurveStore.mu.RLock()
		_, exists := raceBalancerCurveStore.records[race.ID]
		raceBalancerCurveStore.mu.RUnlock()
		if exists {
			continue
		}
		curves, err := deriveRaceCurves(race)
		if err != nil {
			log.Printf("race balancer: раса %q: %v", race.ID, err)
			continue
		}
		rec := &RaceRecord{
			RaceID:    race.ID,
			CardHash:  CardHash(race),
			Factory:   curves,
			Active:    cloneRaceCurves(&curves),
			UpdatedAt: time.Now().UTC(),
		}
		raceBalancerCurveStore.mu.Lock()
		raceBalancerCurveStore.records[race.ID] = rec
		raceBalancerCurveStore.mu.Unlock()
		changed = true
	}
	if changed {
		if err := writeRaceBalancerFileLocked(); err != nil {
			log.Printf("race balancer: запись файла: %v", err)
		}
	}
	return nil
}

// writeRaceBalancerFileLocked — атомарная запись файла под Lock: tmp в той же
// директории + os.Rename (как 99.2.17 §5). Ошибка записи — только лог на
// стороне вызывающего (режим отказа: правки сессионные).
func writeRaceBalancerFileLocked() error {
	path := raceBalancerCurveStore.path
	if path == "" {
		return fmt.Errorf("race balancer: путь файла не задан")
	}
	f := raceBalancerFile{Races: make([]*RaceRecord, 0, len(raceBalancerCurveStore.records))}
	for _, rec := range raceBalancerCurveStore.records {
		f.Races = append(f.Races, cloneRaceRecord(rec))
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("race balancer: marshal: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("race balancer: каталог %s: %w", dir, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("race balancer: запись tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("race balancer: rename: %w", err)
	}
	return nil
}