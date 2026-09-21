package handlers

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"

	"zorion/cmd/art-studio/config"
	"zorion/cmd/art-studio/generator"
)

// handleShipsRebuild — GET /ships/rebuild?race= (пусто = массово) →
// {ok, updated[], skipped[]}: пересборка ships.json из лор-файлов
// races/ships/*.md (паттерн rebuild_prompt.go/families_patch.go, спека §6.1
// п.3) + reload без рестарта (Server + Runner).
func (s *Server) handleShipsRebuild(w http.ResponseWriter, r *http.Request) {
	race := r.URL.Query().Get("race")
	if race == "" {
		s.shipsRebuildAll(w)
		return
	}
	if _, err := s.rebuildShipRace(race); err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	if err := s.reloadShips(s.shipsPath); err != nil {
		writeJSON(w, map[string]string{"error": "файл записан, но перезагрузка конфига не удалась: " + err.Error()})
		return
	}
	if err := s.runner.ReloadShips(s.shipsPath); err != nil {
		writeJSON(w, map[string]string{"error": "файл записан, но перезагрузка runner не удалась: " + err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "updated": []string{race}, "skipped": []string{}})
}

// rebuildShipRace — пересборка записи одной расы: лор-файл → texture/
// silhouette/blocked/family/типы → запись в ships.json (валидация как в
// ParseShips). Ошибка — с человеческой причиной (для skipped в массовом).
func (s *Server) rebuildShipRace(slug string) (config.ShipEntry, error) {
	lore, err := s.parseShipLore(slug)
	if err != nil {
		return config.ShipEntry{}, err
	}
	entry := config.ShipEntry{
		RaceName:   s.raceNameBySlug[slug],
		Family:     lore.Family,
		Texture:    lore.Texture,
		Silhouette: lore.Silhouette,
		Blocked:    lore.Blocked,
		Types:      lore.Types,
	}
	if err := updateShipsRace(s.shipsPath, slug, entry, s.racesPathOr("config/races.json")); err != nil {
		return config.ShipEntry{}, err
	}
	return entry, nil
}

// racesPathOr — путь к config/races.json (в тестах переопределяется).
func (s *Server) racesPathOr(def string) string {
	if s.racesPath != "" {
		return s.racesPath
	}
	return def
}

// shipsRebuildAll — массовый режим (race пуст): цикл по всем лор-файлам
// каталога races/ships/, каждая раса пересобирается независимо. Расы без
// раздела/маркеров — не ошибка всей операции: попадают в skipped с причиной.
// Перезагрузка конфига в памяти (Server + Runner) — ОДИН раз после цикла.
func (s *Server) shipsRebuildAll(w http.ResponseWriter) {
	type updatedRace struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	type skippedRace struct {
		Slug   string `json:"slug"`
		Reason string `json:"reason"`
	}
	var updated []updatedRace
	var skipped []skippedRace
	entries, err := os.ReadDir(s.shipsDir())
	if err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "updated": updated, "skipped": skipped, "error": "нет каталога " + s.shipsDir()})
		return
	}
	var slugs []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			slugs = append(slugs, strings.TrimSuffix(e.Name(), ".md"))
		}
	}
	sort.Strings(slugs)
	for _, slug := range slugs {
		entry, err := s.rebuildShipRace(slug)
		if err != nil {
			skipped = append(skipped, skippedRace{slug, err.Error()})
			continue
		}
		updated = append(updated, updatedRace{slug, entry.RaceName})
	}
	if err := s.reloadShips(s.shipsPath); err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "updated": updated, "skipped": skipped, "error": "файл записан, но перезагрузка конфига не удалась: " + err.Error()})
		return
	}
	if err := s.runner.ReloadShips(s.shipsPath); err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "updated": updated, "skipped": skipped, "error": "файл записан, но перезагрузка runner не удалась: " + err.Error()})
		return
	}
	if len(updated) == 0 {
		reasons := make([]string, 0, len(skipped))
		for _, sk := range skipped {
			reasons = append(reasons, sk.Slug+": "+sk.Reason)
		}
		writeJSON(w, map[string]interface{}{"ok": false, "updated": updated, "skipped": skipped, "error": "не пересобрано ни одной расы: " + strings.Join(reasons, "; ")})
		return
	}
	// мягкий кросс-конфиг-тест (спека §3.4): мёртвые токены blocked — warning
	// в лог и в UI-отчёт, не ошибка
	dead := s.deadBlockedTokens()
	for slug, toks := range dead {
		fmt.Printf("ships: мёртвые токены blocked %s: %v\n", slug, toks)
	}
	writeJSON(w, map[string]interface{}{"ok": true, "updated": updated, "skipped": skipped, "dead_tokens": dead})
}

// deadBlockedTokens — мёртвые токены blocked по текущему ships.json
// (warning-отчёт для UI, спека §3.4).
func (s *Server) deadBlockedTokens() map[string][]string {
	s.shipsMu.RLock()
	ships := s.ships
	dict := s.shipDict
	s.shipsMu.RUnlock()
	if ships == nil || dict == nil {
		return nil
	}
	return generator.DeadBlockedTokens(ships, dict)
}