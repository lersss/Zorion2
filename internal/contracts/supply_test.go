package contracts

import (
	"reflect"
	"testing"
)

func TestDeficit(t *testing.T) {
	cases := []struct {
		name           string
		target, actual float64
		want           float64
	}{
		{"нехватка", 100, 40, 60},
		{"полный буфер", 100, 100, 0},
		{"переполнение", 100, 130, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Deficit(tc.target, tc.actual); got != tc.want {
				t.Fatalf("Deficit(%v, %v) = %v, want %v", tc.target, tc.actual, got, tc.want)
			}
		})
	}
}

func TestSliceShares(t *testing.T) {
	cases := []struct {
		name       string
		deficit    float64
		capacities []float64
		want       []Share
	}{
		{"нулевой дефицит", 0, []float64{20, 100}, nil},
		{"отрицательный дефицит", -5, []float64{20, 100}, nil},
		{"дробный меньше единицы", 0.9, []float64{20, 100}, nil},
		{"не больше наименьшего трюма — одна доля", 15, []float64{20, 100}, []Share{{1, 15, 20}}},
		{"равно наименьшему трюму — одна доля", 20, []float64{20, 100}, []Share{{1, 20, 20}}},
		{"пустой список вместимостей", 30, nil, []Share{{1, 30, 0}}},
		{"равные доли 200 = 100+100", 200, []float64{20, 100}, []Share{{1, 100, 100}, {2, 100, 100}}},
		{"110 = 100+10, обе в свою вместимость", 110, []float64{20, 100}, []Share{{1, 100, 100}, {2, 10, 20}}},
		{"250 — три доли", 250, []float64{20, 100}, []Share{{1, 100, 100}, {2, 100, 100}, {3, 50, 100}}},
		{"дробный дефицит округляется вниз", 110.9, []float64{20, 100}, []Share{{1, 100, 100}, {2, 10, 20}}},
		{"дубликаты и неположительные вместимости — нормализация", 110, []float64{100, 20, 100, -5, 0}, []Share{{1, 100, 100}, {2, 10, 20}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SliceShares(tc.deficit, tc.capacities); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("SliceShares(%v, %v) = %+v, want %+v", tc.deficit, tc.capacities, got, tc.want)
			}
		})
	}
}

// TestSliceSharesInvariants — Σ долей = ⌊дефицит⌋, каждая доля ≥ 1 и ≤ своей Capacity.
func TestSliceSharesInvariants(t *testing.T) {
	cases := []struct {
		deficit    float64
		capacities []float64
	}{
		{110, []float64{20, 100}},
		{200, []float64{20, 100}},
		{250, []float64{20, 100}},
		{500, []float64{20, 100, 200, 500}},
		{1, []float64{20, 100}},
		{333.7, []float64{100}},
		{77, []float64{20}},
	}
	for _, tc := range cases {
		got := SliceShares(tc.deficit, tc.capacities)
		want := int64(tc.deficit) // floor для положительных
		var sum int64
		for i, s := range got {
			if s.Quantity < 1 {
				t.Fatalf("deficit=%v caps=%v: доля %d объёмом %d < 1", tc.deficit, tc.capacities, i, s.Quantity)
			}
			if s.Index != i+1 {
				t.Fatalf("deficit=%v caps=%v: Index=%d, want %d", tc.deficit, tc.capacities, s.Index, i+1)
			}
			if float64(s.Quantity) > s.Capacity {
				t.Fatalf("deficit=%v caps=%v: доля %d объёмом %d > вместимости %v", tc.deficit, tc.capacities, i, s.Quantity, s.Capacity)
			}
			sum += s.Quantity
		}
		if sum != want {
			t.Fatalf("deficit=%v caps=%v: Σ долей = %d, want %d", tc.deficit, tc.capacities, sum, want)
		}
	}
}

// TestSliceSharesDeterminism — два вызова с тем же входом дают одинаковый результат.
func TestSliceSharesDeterminism(t *testing.T) {
	first := SliceShares(250, []float64{20, 100})
	second := SliceShares(250, []float64{20, 100})
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("нарезка недетерминирована: %+v != %+v", first, second)
	}
}

func TestShareRewardMonotonic(t *testing.T) {
	const base = 10.0
	prev := ShareReward(1, base)
	for q := int64(2); q <= 500; q++ {
		got := ShareReward(q, base)
		if got < prev {
			t.Fatalf("награда убывает: q=%d даёт %d < %d (q-1)", q, got, prev)
		}
		prev = got
	}
}

func TestShareRewardBounded(t *testing.T) {
	bases := []float64{1, 10, 100.5}
	for _, base := range bases {
		for q := int64(1); q <= 500; q++ {
			got := ShareReward(q, base)
			cap := base * float64(q) * (1 + RewardBonusMax)
			if float64(got) > cap {
				t.Fatalf("base=%v q=%d: награда %d превышает премию сверху %v", base, q, got, cap)
			}
		}
	}
}
