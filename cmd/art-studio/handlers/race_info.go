package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"zorion/cmd/art-studio/config"
)

// handleRaceInfo — GET /race-info?fam=<F>&race=<id> → {race, name, basis, lore}.
// lore — полный текст docs/gamedesign/races/<slug>.md (маппинг арт-id → slug
// через config/races.json: имя расы из families.json без числового префикса
// «NN » ↔ name в races.json). Если слаг/файл не найден — lore = basis (фолбек).
func (s *Server) handleRaceInfo(w http.ResponseWriter, r *http.Request) {
	fam := r.URL.Query().Get("fam")
	race := r.URL.Query().Get("race")
	f, ok := s.family(fam)
	if !ok {
		writeJSON(w, map[string]string{"race": race, "name": "", "basis": "", "lore": ""})
		return
	}
	var rc config.Race
	found := false
	for _, x := range f.Races {
		if x.ID == race {
			rc = x
			found = true
			break
		}
	}
	if !found {
		writeJSON(w, map[string]string{"race": race, "name": "", "basis": "", "lore": ""})
		return
	}
	// имя расы families.json вида «51 Архивариусы» → без префикса «51 » → «Архивариусы»
	clean := strings.TrimSpace(strings.TrimPrefix(rc.Name, rc.ID+" "))
	slug := s.raceSlug[clean]
	lore := rc.Basis
	if slug != "" {
		if data, err := os.ReadFile(filepath.Join(s.loreDir, slug+".md")); err == nil {
			lore = filterLore(string(data))
		}
	}
	writeJSON(w, map[string]string{"race": race, "name": rc.Name, "basis": rc.Basis, "lore": lore})
}

// loadRaceSlug читает config/races.json и строит map name→slug (id).
// Если файл не найден/невалиден — пустая map (фолбек на basis).
func loadRaceSlug(path string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var doc struct {
		Races []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"races"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return out
	}
	for _, rc := range doc.Races {
		out[rc.Name] = rc.ID
	}
	return out
}

// loadRaceNames читает config/races.json и строит map slug (id) → name
// (вкладка «Корабли рас»: имена рас для селекта и ships.json).
func loadRaceNames(path string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var doc struct {
		Races []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"races"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return out
	}
	for _, rc := range doc.Races {
		out[rc.ID] = rc.Name
	}
	return out
}

// firstOnlySections — секции лора, где нужен только первый абзац
// (вторые абзацы — наука/цифры, решение 79b §5).
var firstOnlySections = map[string]bool{
	"Кто это":         true,
	"Как живут":       true,
	"Сосуществование": true,
}

// filterLore — отбор лора по правилу 79b (решение 5): шапка (заголовок,
// **Семейство:**/**ID:**/**Основа:**) убирается; «Кто это»/«Как живут»/
// «Сосуществование» — только первый абзац; «Живость»/«Происхождение»/
// «Характер»/«Зачем (ниша)» — целиком. Заголовки секций сохраняются
// читаемыми, markdown чистится (cleanLore).
func filterLore(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	var section string
	var pars [][]string // абзацы текущей секции (разделены пустыми строками)
	var cur []string    // строки текущего абзаца
	flushPar := func() {
		if len(cur) > 0 {
			pars = append(pars, cur)
			cur = nil
		}
	}
	flushSection := func() {
		if section == "" {
			return
		}
		flushPar()
		if firstOnlySections[section] && len(pars) > 1 {
			pars = pars[:1]
		}
		out = append(out, section)
		for _, p := range pars {
			out = append(out, strings.Join(p, " "))
		}
		pars = nil
	}
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "## ") {
			flushSection()
			section = strings.TrimSpace(strings.TrimPrefix(t, "## "))
			pars = nil // шапка файла (до первой секции) не входит в лор
			cur = nil
			continue
		}
		if t == "" {
			flushPar()
			continue
		}
		cur = append(cur, t)
	}
	flushSection()
	return cleanLore(strings.Join(out, "\n"))
}

// cleanLore — лёгкая чистка markdown для читаемости в панели: убирает маркеры
// заголовков (#), жирного (**), кода (`) и горизонтальные линии (---);
// текст заголовков секций («## Внешность» → «Внешность») сохраняется.
func cleanLore(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if t == "" || t == "---" {
			continue
		}
		t = strings.TrimLeft(t, "#")
		t = strings.TrimSpace(t)
		t = strings.ReplaceAll(t, "**", "")
		t = strings.ReplaceAll(t, "`", "")
		out = append(out, t)
	}
	return strings.Join(out, "\n")
}
