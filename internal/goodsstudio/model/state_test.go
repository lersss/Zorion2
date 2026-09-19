package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewStateSeedsCategories — при первом старте посеяны 13 позиций §5.2.1
// (спека 99a.1 §5.1, решение создателя 2026-09-18).
func TestNewStateSeedsCategories(t *testing.T) {
	st := NewState()
	require.Len(t, st.Categories, 13)
	require.Equal(t, "продовольствие", st.Categories[0].Name)
	require.Equal(t, "корабли и модули", st.Categories[12].Name)
	require.Equal(t, SchemaVersion, st.SchemaVersion)
}

// TestLoadStateMissing — нет файла → новое состояние с посеянными
// категориями (память переживает перезапуск, спека 99a.1 §12.2).
func TestLoadStateMissing(t *testing.T) {
	st, err := LoadState(filepath.Join(t.TempDir(), "state.json"))
	require.NoError(t, err)
	require.Len(t, st.Categories, 13)
	require.Empty(t, st.Goods)
}

// TestStateRoundTrip — round-trip state.json: сохранение → загрузка даёт
// то же состояние (переживает перезапуск студии).
func TestStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st := NewState()
	st.Goods = append(st.Goods, Good{
		ID:        "g1",
		Name:      "Штурмовой корабль",
		Category:  "c13",
		Status:    StatusDraft,
		Kind:      KindGood,
		Source:    SourceManual,
		Recipe:    []Slot{{GoodID: "res:zhelezo"}, {}},
		CreatedAt: "2026-09-18T12:00:00Z",
	})
	require.NoError(t, SaveState(path, st))

	loaded, err := LoadState(path)
	require.NoError(t, err)
	require.Equal(t, SchemaVersion, loaded.SchemaVersion)
	require.Len(t, loaded.Categories, 13)
	require.Len(t, loaded.Goods, 1)
	g := loaded.Goods[0]
	require.Equal(t, "Штурмовой корабль", g.Name)
	require.Equal(t, StatusDraft, g.Status)
	// количество нормализовано при загрузке: 0 (старый слот) → 1 (99a.2 §6.1)
	require.Equal(t, []Slot{{GoodID: "res:zhelezo", Quantity: 1}, {Quantity: 1}}, g.Recipe)
}

// TestSaveStateAtomic — атомарная запись: tmp-файл не остаётся после
// успешного сохранения.
func TestSaveStateAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st := NewState()
	require.NoError(t, SaveState(path, st))
	_, err := os.Stat(path + ".tmp")
	require.True(t, os.IsNotExist(err), "tmp-файл должен быть переименован")
}

// TestStateRoundTripAllowResource — round-trip галки «заполнять ресурсом»
// (99a Пакет 4, п.8): слот с AllowResource=true сериализуется/десериализуется
// корректно; слот без галки (zero bool) — false (старые state.json без
// миграции).
func TestStateRoundTripAllowResource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st := NewState()
	st.Goods = append(st.Goods, Good{
		ID:        "g1",
		Name:      "Корабль",
		Category:  "c1",
		Status:    StatusDraft,
		Kind:      KindGood,
		Source:    SourceManual,
		Recipe:    []Slot{{AllowResource: true}, {}},
		CreatedAt: "2026-09-19T12:00:00Z",
	})
	require.NoError(t, SaveState(path, st))

	loaded, err := LoadState(path)
	require.NoError(t, err)
	require.Equal(t, []Slot{{AllowResource: true, Quantity: 1}, {Quantity: 1}}, loaded.Goods[0].Recipe)
}

// TestLoadStateNormalizesQuantity — нормализация количества при загрузке
// (99a.2 §6.1): слоты с quantity < 1 (0 или отсутствует — старые state.json
// без миграции) → 1 в памяти; файл не переписывается до первой мутации.
func TestLoadStateNormalizesQuantity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	st := NewState()
	st.Goods = append(st.Goods, Good{
		ID:        "g1",
		Name:      "Корабль",
		Category:  "c1",
		Status:    StatusDraft,
		Kind:      KindGood,
		Source:    SourceManual,
		Recipe:    []Slot{{GoodID: "res:zhelezo"}, {GoodID: "res:uglerod", Quantity: 3}, {Quantity: 0}},
		CreatedAt: "2026-09-19T12:00:00Z",
	})
	require.NoError(t, SaveState(path, st))

	loaded, err := LoadState(path)
	require.NoError(t, err)
	require.Equal(t, []Slot{
		{GoodID: "res:zhelezo", Quantity: 1}, // 0 → 1
		{GoodID: "res:uglerod", Quantity: 3}, // уже ≥ 1 — не трогаем
		{Quantity: 1},                        // 0 → 1
	}, loaded.Goods[0].Recipe)
}