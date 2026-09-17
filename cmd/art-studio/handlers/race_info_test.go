package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zorion/cmd/art-studio/config"
)

// TestRaceSlugMapping — маппинг арт-id → slug лора (79b): имя расы из
// families.json без числового префикса «NN » ↔ name в races.json → slug;
// для выбранных рас F2/F4/F5/F10 slug непустой и файл лора существует;
// несуществующее имя — пустой slug (фолбек на basis).
func TestRaceSlugMapping(t *testing.T) {
	fam, err := config.LoadFamilies("../../../config/art/families.json")
	if err != nil {
		t.Fatalf("LoadFamilies: %v", err)
	}
	slugMap := loadRaceSlug("../../../config/races.json")
	if len(slugMap) == 0 {
		t.Fatal("loadRaceSlug: пустая map")
	}
	cases := []struct{ fam, id, wantSlug string }{
		{"F2", "5", "ammonia"},
		{"F4", "15", "sulfur_nests"},
		{"F5", "23", "saltfolk"},      // солевые: «Кшарры» — маппится по имени
		{"F5", "25", "supercritical"},
		{"F5", "49", "salt_bridge"},   // солевые: «Маргулы» — маппится по имени
		{"F10", "51", "archivists"},
	}
	for _, c := range cases {
		f, ok := fam[c.fam]
		if !ok {
			t.Fatalf("нет семейства %s", c.fam)
		}
		var name string
		for _, rc := range f.Races {
			if rc.ID == c.id {
				name = rc.Name
				break
			}
		}
		if name == "" {
			t.Fatalf("%s/%s: нет расы", c.fam, c.id)
		}
		clean := strings.TrimSpace(strings.TrimPrefix(name, c.id+" "))
		slug := slugMap[clean]
		if slug == "" {
			t.Errorf("%s/%s (%s): slug пуст", c.fam, c.id, clean)
			continue
		}
		if c.wantSlug != "" && slug != c.wantSlug {
			t.Errorf("%s/%s (%s) → slug %q, want %q", c.fam, c.id, clean, slug, c.wantSlug)
		}
		if _, err := os.Stat(filepath.Join("../../../docs/gamedesign/races", slug+".md")); err != nil {
			t.Errorf("%s/%s: файл лора %s.md не найден", c.fam, c.id, slug)
		}
	}
	// фолбек: несуществующее имя → пустой slug
	if slugMap["Несуществующая раса"] != "" {
		t.Errorf("фолбек: slugMap[несуществующая] = %q, want пусто", slugMap["Несуществующая раса"])
	}
}

// TestFilterLore — отбор лора (79b решение 5): второй (научный) абзац
// «Кто это» отсутствует, «Живость» (Сцена + Суть) входит целиком.
func TestFilterLore(t *testing.T) {
	arch, err := os.ReadFile("../../../docs/gamedesign/races/archivists.md")
	if err != nil {
		t.Fatalf("archivists.md: %v", err)
	}
	lore := filterLore(string(arch))
	if strings.Contains(lore, "Материал корпусов") {
		t.Errorf("archivists: научный абзац «Кто это» не отфильтрован")
	}
	if !strings.Contains(lore, "Единственная раса роботов") {
		t.Errorf("archivists: первый абзац «Кто это» отсутствует")
	}
	if !strings.Contains(lore, "Сцена.") || !strings.Contains(lore, "Суть.") {
		t.Errorf("archivists: «Живость» не целиком (нет Сцена./Суть.)")
	}
	if strings.Contains(lore, "Семейство:") || strings.Contains(lore, "Основа:") {
		t.Errorf("archivists: шапка-метаданные не убрана")
	}

	amm, err := os.ReadFile("../../../docs/gamedesign/races/ammonia.md")
	if err != nil {
		t.Fatalf("ammonia.md: %v", err)
	}
	lore2 := filterLore(string(amm))
	if strings.Contains(lore2, "Растворитель — жидкий NH₃") {
		t.Errorf("ammonia: научный абзац «Кто это» не отфильтрован")
	}
	if !strings.Contains(lore2, "Аммиачная биохимия") {
		t.Errorf("ammonia: первый абзац «Кто это» отсутствует")
	}
	if !strings.Contains(lore2, "Сцена.") || !strings.Contains(lore2, "Суть.") {
		t.Errorf("ammonia: «Живость» не целиком (нет Сцена./Суть.)")
	}
}

// TestCleanLore — чистка markdown: маркеры #/**/` и --- убираются,
// текст заголовков секций сохраняется.
func TestCleanLore(t *testing.T) {
	in := "# Архивариусы (archivists)\n\n**Основа:** железо\n\n## Кто это\n\n---\n\nТекст `с кодом` и **жирным**."
	want := "Архивариусы (archivists)\nОснова: железо\nКто это\nТекст с кодом и жирным."
	if got := cleanLore(in); got != want {
		t.Errorf("cleanLore:\n got %q\nwant %q", got, want)
	}
}