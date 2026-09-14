// internal/generator/ship/assembly_test.go
// Тесты детерминированной сборки схемы (спека §7.1, инвариант И3/И2/И4):
// детерминизм, цвет из палитры, ровно 5 деталей, индексы в пределах
// каталога, пустой каталог → пустые id (фолбэк И4).
package ship

import (
	"testing"

	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

func TestAssemblyFromSeedDeterministic(t *testing.T) {
	palette := []string{"#a", "#b", "#c"}
	order := Categories
	byCat := map[string][]models.ShipPart{
		"hull":   {{ID: "hull_1"}, {ID: "hull_2"}, {ID: "hull_3"}},
		"nose":   {{ID: "nose_1"}, {ID: "nose_2"}},
		"wings":  {{ID: "wings_1"}},
		"engine": {{ID: "engine_1"}, {ID: "engine_2"}, {ID: "engine_3"}, {ID: "engine_4"}},
		"tail":   {{ID: "tail_1"}, {ID: "tail_2"}, {ID: "tail_3"}},
	}
	seed := []byte("agent-11111111-1111-1111-1111-111111111111")
	a := AssemblyFromSeed(seed, palette, order, byCat)
	b := AssemblyFromSeed(seed, palette, order, byCat)
	require.Equal(t, a, b, "сборка детерминирована (И3)")
	require.Contains(t, palette, a.Color, "цвет из палитры")
	require.Len(t, a.Parts, 5, "ровно 5 деталей (И2)")
	for _, cat := range order {
		require.NotEmpty(t, a.Parts[cat], "категория %s заполнена", cat)
		ids := make([]string, len(byCat[cat]))
		for i, p := range byCat[cat] {
			ids[i] = p.ID
		}
		require.Contains(t, ids, a.Parts[cat], "id ∈ каталог")
	}
}

func TestAssemblyFromSeedDifferentSeeds(t *testing.T) {
	palette := []string{"#a", "#b", "#c", "#d", "#e", "#f", "#g", "#h", "#i", "#j", "#k", "#l"}
	order := Categories
	byCat := map[string][]models.ShipPart{}
	for _, cat := range order {
		for i := 0; i < 10; i++ {
			byCat[cat] = append(byCat[cat], models.ShipPart{ID: cat + "_" + string(rune('a'+i))})
		}
	}
	// 1000 разных seed'ов не должны давать пары-дубли чаще случайного
	// (спека §9: ≈ 4 200 пар из 5·10⁹ — здесь проверяем, что выборки
	// действительно разные по цветам/деталям на заметной выборке).
	seen := map[string]bool{}
	for i := int64(0); i < 1000; i++ {
		v := AssemblyFromSeed([]byte("seed-"+itoa(i)), palette, order, byCat)
		seen[v.Color+v.Parts["hull"]+v.Parts["nose"]+v.Parts["wings"]+v.Parts["engine"]+v.Parts["tail"]] = true
	}
	require.GreaterOrEqual(t, len(seen), 900, "ожидалось заметное разнообразие схем")
}

func TestAssemblyFromSeedEmptyCatalog(t *testing.T) {
	v := AssemblyFromSeed([]byte("x"), []string{"#3b82f6"}, Categories, map[string][]models.ShipPart{})
	require.Equal(t, "#3b82f6", v.Color)
	for _, cat := range Categories {
		require.Equal(t, "", v.Parts[cat], "пустой каталог → пустой id (фолбэк И4)")
	}
}

func TestAssemblyFromSeedEmptyPalette(t *testing.T) {
	v := AssemblyFromSeed([]byte("x"), nil, Categories, map[string][]models.ShipPart{})
	require.Equal(t, "", v.Color, "без палитры — без цвета (клиент рисует фолбэк)")
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}