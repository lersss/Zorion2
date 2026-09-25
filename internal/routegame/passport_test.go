package routegame

import (
	"encoding/json"
	"strings"
	"testing"

	"zorion/internal/models"
)

func sampleWorld() models.World {
	return models.World{
		ID:            "w1",
		SpectralClass: "G",
		StarType:      "star",
		SystemType:    "single",
		Temperature:   5778,
		StellarMass:   ptrFloat(1.0),
		Age:           ptrFloat(4.6),
	}
}

func ptrFloat(v float64) *float64 { return &v }

func TestBuildPassport_CopiesStarFields(t *testing.T) {
	from := sampleWorld()
	to := models.World{
		ID:            "w2",
		SpectralClass: "M",
		StarType:      "white_dwarf",
		SystemType:    "binary",
		Temperature:   3200,
	}

	p := BuildPassport(1234.5, from, to, nil)

	if p.Dist != 1234.5 {
		t.Errorf("Dist = %v, want 1234.5", p.Dist)
	}
	if p.From.SpectralClass != "G" || p.From.StarType != "star" ||
		p.From.SystemType != "single" || p.From.Temperature != 5778 {
		t.Errorf("from fields not copied: %+v", p.From)
	}
	if p.To.SpectralClass != "M" || p.To.StarType != "white_dwarf" ||
		p.To.SystemType != "binary" || p.To.Temperature != 3200 {
		t.Errorf("to fields not copied: %+v", p.To)
	}
}

func TestBuildPassport_ExoticStarEmptySpectralClass(t *testing.T) {
	exotic := models.World{StarType: "black_hole", SystemType: "single"}
	p := BuildPassport(10, exotic, exotic, nil)
	if p.From.SpectralClass != "" {
		t.Errorf("exotic spectral_class = %q, want empty", p.From.SpectralClass)
	}
	if p.From.StarType != "black_hole" {
		t.Errorf("star_type = %q, want black_hole", p.From.StarType)
	}
}

func TestBuildPassport_SkipsInvisibleBeltsKeepsOrder(t *testing.T) {
	belts := []models.Belt{
		{ID: "b1", Kind: "asteroid", Name: "Пояс A", RadiusAU: 2.5, WidthAU: 0.4, Visible: true},
		{ID: "b2", Kind: "kuiper", Name: "Скрытый", RadiusAU: 40, WidthAU: 5, Visible: false},
		{ID: "b3", Kind: "debris", Name: "Пояс B", RadiusAU: 1.1, WidthAU: 0.2, Visible: true},
	}

	p := BuildPassport(100, sampleWorld(), sampleWorld(), belts)

	if len(p.DestinationBelts) != 2 {
		t.Fatalf("belts count = %d, want 2", len(p.DestinationBelts))
	}
	if p.DestinationBelts[0].Name != "Пояс A" || p.DestinationBelts[1].Name != "Пояс B" {
		t.Errorf("order/names wrong: %+v", p.DestinationBelts)
	}
	first := p.DestinationBelts[0]
	if first.Kind != "asteroid" || first.Name != "Пояс A" || first.RadiusAU != 2.5 || first.WidthAU != 0.4 {
		t.Errorf("belt fields not copied: %+v", first)
	}
	for _, b := range p.DestinationBelts {
		if b.Name == "Скрытый" {
			t.Errorf("invisible belt leaked: %+v", b)
		}
	}
}

func TestBuildPassport_EmptyBeltsMarshalAsArray(t *testing.T) {
	cases := []struct {
		name  string
		belts []models.Belt
	}{
		{"nil", nil},
		{"empty", []models.Belt{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := BuildPassport(1, sampleWorld(), sampleWorld(), tc.belts)
			if p.DestinationBelts == nil {
				t.Fatalf("DestinationBelts is nil for %s, want non-nil", tc.name)
			}
			raw, err := json.Marshal(p)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if !strings.Contains(string(raw), `"destination_belts":[]`) {
				t.Errorf("destination_belts not []; json=%s", raw)
			}
		})
	}
}

func TestBuildPassport_JSONOmitsForbiddenFields(t *testing.T) {
	from := sampleWorld()
	to := sampleWorld()
	p := BuildPassport(50, from, to, nil)

	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	js := string(raw)
	for _, forbidden := range []string{"stellar_mass", "age", "region"} {
		if strings.Contains(js, forbidden) {
			t.Errorf("forbidden key %q present in passport JSON: %s", forbidden, js)
		}
	}
}

func TestBuildPassport_Deterministic(t *testing.T) {
	from := sampleWorld()
	to := models.World{ID: "w2", SpectralClass: "K", StarType: "star", SystemType: "multiple", Temperature: 4000}
	belts := []models.Belt{
		{Kind: "asteroid", Name: "A", RadiusAU: 3, WidthAU: 0.5, Visible: true},
		{Kind: "oort", Name: "B", RadiusAU: 100, WidthAU: 10, Visible: true},
	}

	first, err := json.Marshal(BuildPassport(9999.25, from, to, belts))
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}
	second, err := json.Marshal(BuildPassport(9999.25, from, to, belts))
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("JSON not deterministic:\nfirst:  %s\nsecond: %s", first, second)
	}
}
