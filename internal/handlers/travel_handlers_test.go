// internal/handlers/travel_handlers_test.go
package handlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalcTravelDuration(t *testing.T) {
	tests := []struct {
		name string
		dist float64
		want time.Duration
	}{
		{
			name: "короткое расстояние: время ниже капа, равно dist*0.3",
			dist: 50,
			want: 15 * time.Second,
		},
		{
			name: "большое расстояние: кап 20 секунд",
			dist: 100,
			want: 20 * time.Second,
		},
		{
			name: "граница капа (dist=70 -> 21s raw, режется до 20s)",
			dist: 70,
			want: 20 * time.Second,
		},
		{
			name: "минимум 3 секунды для очень близких миров",
			dist: 1,
			want: 3 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, calcTravelDuration(tt.dist))
		})
	}
}