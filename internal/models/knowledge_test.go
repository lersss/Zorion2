// internal/models/knowledge_test.go
// Протухание знания о планете (спека 77a §8.2): актуальность считается на
// чтении — запись «актуальна», если scanned_at ≥ now − 7 дней (И8/И10).
package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPlanetKnowledgeIsFresh(t *testing.T) {
	now := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		scannedAt time.Time
		want      bool
	}{
		{
			name:      "свежее знание (только что отсканировано) → актуально",
			scannedAt: now,
			want:      true,
		},
		{
			name:      "знание 6 дней назад → актуально",
			scannedAt: now.Add(-6 * 24 * time.Hour),
			want:      true,
		},
		{
			name:      "граница ровно 7 дней → актуально (scanned_at ≥ now − срок)",
			scannedAt: now.Add(-7 * 24 * time.Hour),
			want:      true,
		},
		{
			name:      "знание старше 7 дней → устарело",
			scannedAt: now.Add(-8 * 24 * time.Hour),
			want:      false,
		},
		{
			name:      "знание 7 дней и 1 секунда → устарело",
			scannedAt: now.Add(-7*24*time.Hour - time.Second),
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := &PlanetKnowledge{ScannedAt: tt.scannedAt}
			require.Equal(t, tt.want, k.IsFresh(now))
		})
	}
}

// nil-знание (нет записи) — не актуально.
func TestPlanetKnowledgeIsFreshNil(t *testing.T) {
	var k *PlanetKnowledge
	require.False(t, k.IsFresh(time.Now()), "nil-знание не может быть актуальным")
}