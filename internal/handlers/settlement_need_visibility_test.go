// internal/handlers/settlement_need_visibility_test.go
// T13 (витрина/strip, спека 2026-09-24-потребление-по-товарам §10.3): строка
// нужды живёт ВНУТРИ блока арифметики (поля позиции), поэтому существующая
// очистка — playerSettlements при выключенной настройке и
// stripSnapshotSettlementSecrets в снимке — убирает её вместе с блоком, без
// правки занятого planet_visibility.go.
package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"zorion/internal/models"
)

// needArithmeticPlanet — поселение со строкой нужды внутри арифметики.
func needArithmeticPlanet() models.Planet {
	return models.Planet{
		ID: "p1",
		Settlements: []models.Settlement{{
			ID: "s1", Population: 1000, TypeName: "Аутпост",
			Arithmetic: []models.SettlementPositionArithmetic{{
				Position: "очищенная вода", NormPerDayPerBillion: 2e7,
				ProducedPerDay: 0.01, ConsumedPerDay: 20, NetPerDay: -19.99,
				Need: "жажда", Effect: "жажда", CoveredShare: 0.0005, DeficitShare: 0.9995,
			}},
		}},
	}
}

// presence + настройка false: строка нужды скрыта вместе с блоком арифметики.
func TestNeedArithmeticHiddenForPlayerWhenSettingOff(t *testing.T) {
	out := playerSettlements(needArithmeticPlanet().Settlements, false)
	require.Len(t, out, 1)
	assert.Nil(t, out[0].Arithmetic, "настройка выключена → блока (и строки нужды) нет")
}

// presence + настройка true: строка нужды видна игроку (внутри блока).
func TestNeedArithmeticVisibleForPlayerWhenSettingOn(t *testing.T) {
	out := playerSettlements(needArithmeticPlanet().Settlements, true)
	require.Len(t, out, 1)
	require.Len(t, out[0].Arithmetic, 1)
	assert.Equal(t, "жажда", out[0].Arithmetic[0].Need)
	assert.InDelta(t, 0.9995, out[0].Arithmetic[0].DeficitShare, 1e-9)
}

// snapshot: строка нужды не замораживается — чистится всегда (§10.3).
func TestNeedArithmeticHiddenInSnapshot(t *testing.T) {
	p := needArithmeticPlanet()
	stripSnapshotSettlementSecrets(&p)
	require.Len(t, p.Settlements, 1)
	assert.Nil(t, p.Settlements[0].Arithmetic, "снимок не несёт строку нужды (§10.3)")
}
