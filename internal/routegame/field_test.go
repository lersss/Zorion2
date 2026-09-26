package routegame

import "testing"

func calmPassport() Passport {
	return Passport{
		Dist: 10000,
		From: PassportStar{SpectralClass: "K", StarType: "star", SystemType: "single", Temperature: 4000},
		To:   PassportStar{SpectralClass: "M", StarType: "star", SystemType: "single", Temperature: 3200},
	}
}

func restlessPassport() Passport {
	return Passport{
		Dist: 10000,
		From: PassportStar{StarType: "black_hole", SystemType: "multiple", Temperature: 20000},
		To:   PassportStar{StarType: "neutron", SystemType: "binary", Temperature: 15000},
		DestinationBelts: []PassportBelt{
			{Kind: "asteroid", Name: "Пояс", RadiusAU: 3, WidthAU: 0.5},
		},
	}
}

func TestHashSeed_StableAndDistinct(t *testing.T) {
	a := HashSeed("w1", "w2")
	if a != HashSeed("w1", "w2") {
		t.Fatalf("HashSeed not stable: %d != %d", a, HashSeed("w1", "w2"))
	}
	if a == HashSeed("w1", "w3") {
		t.Errorf("different target world produced same seed")
	}
	if a == HashSeed("w2", "w1") {
		t.Errorf("direction ignored: A→B seed equals B→A")
	}
	// Разделитель: ("ab","c") — не то же, что ("a","bc").
	if HashSeed("ab", "c") == HashSeed("a", "bc") {
		t.Errorf("separator missing: (ab,c) collides with (a,bc)")
	}
}
