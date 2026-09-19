package model

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SchemaVersion — версия формата state.json (спека 99a.1 §5).
const SchemaVersion = 1

// DefaultCategories — стартовое удобство: 13 позиций §5.2.1
// (решение создателя 2026-09-18, спека 99a.1 §5.1). Множество остаётся
// свободным — просто предзаполнено.
var DefaultCategories = []string{
	"продовольствие",
	"топливо",
	"конструкционные материалы",
	"химикаты",
	"полимеры",
	"комплектующие",
	"детали",
	"медикаменты",
	"товары быта",
	"оборудование",
	"оружие",
	"броня",
	"корабли и модули",
}

// NewState создаёт пустое состояние с посеянными категориями.
func NewState() *State {
	st := &State{SchemaVersion: SchemaVersion}
	for i, name := range DefaultCategories {
		st.Categories = append(st.Categories, Category{ID: fmt.Sprintf("c%d", i+1), Name: name})
	}
	return st
}

// LoadState читает state.json; если файла нет — новое состояние с
// посеянными категориями (память переживает перезапуск, спека 99a.1 §12.2).
func LoadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewState(), nil
		}
		return nil, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if st.SchemaVersion == 0 {
		st.SchemaVersion = SchemaVersion
	}
	// нормализация количества (99a.2 §6.1): слоты с quantity < 1 → 1
	// (старые слоты без миграции — zero value трактуется как 1; файл не
	// переписывается до первой мутации)
	for i := range st.Goods {
		for j := range st.Goods[i].Recipe {
			if st.Goods[i].Recipe[j].Quantity < 1 {
				st.Goods[i].Recipe[j].Quantity = 1
			}
		}
	}
	return &st, nil
}

// SaveState пишет state.json атомарно (tmp + rename, спека 99a.1 §12.2).
// Windows: os.Rename поверх открытого файла падает «Access is denied» —
// читатель (UI-поллинг /api/state) держит файл открытым микросекунды;
// окно короткое, поэтому rename ретраится (паттерн арт-студии WriteStatus,
// docs/PITFALLS.md «Go и конкурентность»).
func SaveState(path string, st *State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	var lastErr error
	for i := 0; i < 5; i++ {
		if err := os.Rename(tmp, path); err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(5 * time.Millisecond)
	}
	return lastErr
}