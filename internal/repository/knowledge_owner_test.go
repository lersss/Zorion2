// Тест владельца в снимке присутствия (спека 2026-09-24-постройка-структур
// §10.3, Г2/T20): snapshotSettlement несёт owner_name (замороженный), а не
// теряет владельца — иначе игрок в snapshot увидел бы поселение без владельца.
package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// TestBuildPresenceSnapshotOwnerName — владелец поселения проезжает в снимок
// (owner_name), в т.ч. у поселения без владельца — пусто, без ошибки.
func TestBuildPresenceSnapshotOwnerName(t *testing.T) {
	at := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	planet := &models.Planet{
		ID: "p1",
		Settlements: []models.Settlement{
			{ID: "s1", RaceID: "humans", RaceName: "Люди", Population: 1000, Stability: 100, OwnerType: "faction", OwnerID: "f1", OwnerName: "Люди"},
			{ID: "s2", Population: 500, Stability: 40}, // без владельца (Г1)
		},
	}

	snap := buildPresenceSnapshot(planet, "presence", at)
	require.Len(t, snap.Settlements, 2)
	require.Equal(t, "Люди", snap.Settlements[0].OwnerName, "owner_name заморожен в снимке")
	require.Empty(t, snap.Settlements[1].OwnerName, "без владельца — пусто, без ошибки")
}
