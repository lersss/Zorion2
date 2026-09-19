package handlers

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"zorion/cmd/art-studio/config"
)

// handleRebuildPrompt — «Пересобрать промт» (идея 2026-09-20): перечитывает
// лор-файл расы (docs/gamedesign/races/<slug>.md), извлекает из раздела
// «Внешний вид» строки «**Для генератора (appearance):**» и
// «**Для генератора (blocked):**», обновляет машинную проекцию
// (appearance/blocked в config/art/families.json на диске), перезагружает
// конфиг в памяти (Server + Runner) и возвращает {ok, race, race_name,
// appearance, blocked} или {error}. Маппинг раса → файл — по ИМЕНИ
// (id в families.json для F7–F10 ≠ нумерация races.json): чистое имя без
// числового префикса «NN » → raceSlug (config/races.json) → loreDir.
// Если race пуст — массовый режим: пересборка ВСЕХ рас семейства
// (rebuildPromptAll), ответ {ok, updated[], skipped[]}.
func (s *Server) handleRebuildPrompt(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	famID := q.Get("fam")
	raceID := q.Get("race")
	fam, ok := s.family(famID)
	if !ok {
		writeJSON(w, map[string]string{"error": "нет семейства " + famID})
		return
	}
	if raceID == "" {
		s.rebuildPromptAll(w, fam)
		return
	}
	var rc config.Race
	found := false
	for _, x := range fam.Races {
		if x.ID == raceID {
			rc = x
			found = true
			break
		}
	}
	if !found {
		writeJSON(w, map[string]string{"error": "нет расы " + raceID + " в " + famID})
		return
	}
	appearance, blocked, err := s.rebuildRace(rc)
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	if err := s.reloadFamilies(s.familiesPath); err != nil {
		writeJSON(w, map[string]string{"error": "файл записан, но перезагрузка конфига не удалась: " + err.Error()})
		return
	}
	if err := s.runner.ReloadFamilies(s.familiesPath); err != nil {
		writeJSON(w, map[string]string{"error": "файл записан, но перезагрузка runner не удалась: " + err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "race": raceID, "race_name": rc.Name, "appearance": appearance, "blocked": blocked})
}

// rebuildRace — пересборка промта одной расы: лор-файл → appearance/blocked →
// запись в families.json (валидация как в единичном режиме). Ошибка — с
// человеческой причиной (для skipped в массовом режиме).
func (s *Server) rebuildRace(rc config.Race) (appearance string, blocked []string, err error) {
	// имя families.json вида «52 Крио-небесные» → без префикса «52 » → «Крио-небесные»
	clean := strings.TrimSpace(strings.TrimPrefix(rc.Name, rc.ID+" "))
	slug := s.raceSlug[clean]
	if slug == "" {
		return "", nil, errors.New("нет слага для расы «" + clean + "» (config/races.json)")
	}
	lorePath := filepath.Join(s.loreDir, slug+".md")
	data, err := os.ReadFile(lorePath)
	if err != nil {
		return "", nil, errors.New("нет лор-файла " + lorePath)
	}
	appearance, blocked, err = parseAppearanceSection(string(data))
	if err != nil {
		return "", nil, err
	}
	if err := validateAppearanceBlocked(appearance, blocked); err != nil {
		return "", nil, err
	}
	if err := updateFamiliesRace(s.familiesPath, rc.ID, appearance, blocked); err != nil {
		return "", nil, err
	}
	return appearance, blocked, nil
}

// rebuildPromptAll — массовый режим «Пересобрать промт» (race пуст): цикл по
// всем расам семейства, каждая пересобирается независимо (rebuildRace).
// Расы без раздела «Внешний вид»/маркеров, без слага или без лор-файла — не
// ошибка всей операции: попадают в skipped с человеческой причиной, остальные
// пересобираются. Перезагрузка конфига в памяти (Server + Runner) — ОДИН раз
// после цикла в любом случае (файл мог измениться частично — память должна
// совпадать с диском). ok=false только если не пересобрано ни одной расы
// (error — сводка причин).
func (s *Server) rebuildPromptAll(w http.ResponseWriter, fam config.Family) {
	type updatedRace struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Appearance string `json:"appearance"`
	}
	type skippedRace struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Reason string `json:"reason"`
	}
	var updated []updatedRace
	var skipped []skippedRace
	for _, rc := range fam.Races {
		appearance, _, err := s.rebuildRace(rc)
		if err != nil {
			skipped = append(skipped, skippedRace{rc.ID, rc.Name, err.Error()})
			continue
		}
		updated = append(updated, updatedRace{rc.ID, rc.Name, appearance})
	}
	if err := s.reloadFamilies(s.familiesPath); err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "updated": updated, "skipped": skipped, "error": "файл записан, но перезагрузка конфига не удалась: " + err.Error()})
		return
	}
	if err := s.runner.ReloadFamilies(s.familiesPath); err != nil {
		writeJSON(w, map[string]interface{}{"ok": false, "updated": updated, "skipped": skipped, "error": "файл записан, но перезагрузка runner не удалась: " + err.Error()})
		return
	}
	if len(updated) == 0 {
		reasons := make([]string, 0, len(skipped))
		for _, sk := range skipped {
			reasons = append(reasons, sk.Name+": "+sk.Reason)
		}
		writeJSON(w, map[string]interface{}{"ok": false, "updated": updated, "skipped": skipped, "error": "не пересобрано ни одной расы: " + strings.Join(reasons, "; ")})
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "updated": updated, "skipped": skipped})
}

// parseAppearanceSection извлекает из лор-файла расы машинную проекцию раздела
// «Внешний вид» (98a §3): строки «**Для генератора (appearance):** <текст>»
// и «**Для генератора (blocked):** <токены через запятую>» (сплит по запятой,
// trim каждого). Маркеры ищутся только внутри раздела «## Внешний вид».
// Ошибка, если раздела или любого из маркеров нет.
func parseAppearanceSection(md string) (appearance string, blocked []string, err error) {
	lines := strings.Split(md, "\n")
	inSection := false
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "## ") {
			inSection = strings.TrimSpace(strings.TrimPrefix(t, "## ")) == "Внешний вид"
			continue
		}
		if !inSection {
			continue
		}
		if strings.HasPrefix(t, "**Для генератора (appearance):**") {
			appearance = strings.TrimSpace(strings.TrimPrefix(t, "**Для генератора (appearance):**"))
			continue
		}
		if strings.HasPrefix(t, "**Для генератора (blocked):**") {
			rest := strings.TrimSpace(strings.TrimPrefix(t, "**Для генератора (blocked):**"))
			for _, tok := range strings.Split(rest, ",") {
				if tok = strings.TrimSpace(tok); tok != "" {
					blocked = append(blocked, tok)
				}
			}
		}
	}
	if appearance == "" || len(blocked) == 0 {
		return "", nil, errors.New(`у расы нет раздела "Внешний вид"`)
	}
	return appearance, blocked, nil
}

// validateAppearanceBlocked — валидация перед записью в families.json
// (те же правила, что ParseFamilies 98a §4.3, но с человеческой ошибкой):
// appearance непустой после trim, ≤ 200 символов, без \n; blocked — токены
// непустые, без пробелов (нормализация lowercase/trim/дедуп делает
// ParseFamilies при чтении).
func validateAppearanceBlocked(appearance string, blocked []string) error {
	if strings.TrimSpace(appearance) == "" {
		return errors.New("appearance пуст после trim")
	}
	if len(appearance) > 200 {
		return errors.New("appearance " + strconv.Itoa(len(appearance)) + " символов (нужно ≤ 200)")
	}
	if strings.Contains(appearance, "\n") {
		return errors.New("appearance содержит перенос строки")
	}
	seen := map[string]bool{}
	for _, tok := range blocked {
		t := strings.ToLower(strings.TrimSpace(tok))
		if t == "" {
			return errors.New("blocked содержит пустой токен")
		}
		if strings.ContainsAny(t, " \t") {
			return errors.New("blocked токен «" + t + "» содержит пробел")
		}
		if !seen[t] {
			seen[t] = true
		}
	}
	return nil
}