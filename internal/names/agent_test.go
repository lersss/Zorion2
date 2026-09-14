// internal/names/agent_test.go
// Тесты генератора имён NPC-агентов (спека 26a.1 §3): формат «Имя Фамилия»,
// комбинаторное пространство ≥ 1 млн пар, уникальность 100к имён на одной map.
package names

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Комбинаторное пространство (спека §3.1): имён = слог1 × слог2,
// фамилий = основа × окончание (включая пустое), пар = имён × фамилий.
// ≥ 1 млн — защита от деградации пулов при правках (запас ×75 от цели 100к).
func TestAgentNameSpace(t *testing.T) {
	first := len(agentFirstSyllable1) * len(agentFirstSyllable2)
	last := len(agentLastBase) * len(agentLastEnding)
	pairs := first * last
	require.GreaterOrEqual(t, pairs, 1_000_000,
		"комбинаторное пространство пар «имя-фамилия» ≥ 1 млн (спека §3.1)")
	require.GreaterOrEqual(t, first, 3000, "имён ≥ 3360 (спека §3.1)")
	require.GreaterOrEqual(t, last, 2000, "фамилий ≥ 2250 (спека §3.1)")
}

// Формат: ровно два слова «Имя Фамилия», каждое с заглавной буквы.
func TestGenerateAgentNameFormat(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	used := map[string]bool{}
	for i := 0; i < 2000; i++ {
		name := GenerateAgentName(rng, used)
		parts := strings.Fields(name)
		require.Len(t, parts, 2, "имя из двух слов: %q", name)
		for _, p := range parts {
			require.NotEmpty(t, p)
			require.GreaterOrEqual(t, p[0], byte('A'))
			require.LessOrEqual(t, p[0], byte('Z'), "капитализация: %q", p)
		}
	}
}

// Уникальность: 100к генераций на одной map — дублей 0 (спека §3.2: пара
// «имя-фамилия» уникальна целиком; заполняемость пространства ~1.3%).
func TestGenerateAgentNameUniqueness(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	used := map[string]bool{}
	seen := map[string]bool{}
	const n = 100_000
	for i := 0; i < n; i++ {
		name := GenerateAgentName(rng, used)
		require.False(t, seen[name], "дубль на итерации %d: %q", i, name)
		seen[name] = true
	}
	require.Len(t, seen, n)
}