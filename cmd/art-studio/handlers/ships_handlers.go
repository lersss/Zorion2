package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"zorion/cmd/art-studio/config"
	"zorion/cmd/art-studio/generator"
	"zorion/internal/models"
)

// --- Вкладка «Корабли рас» (спека 2026-09-20-ships-races-generator §6) ---

// handleShipsRaces — GET /ships/races → {races: [{slug, name}]}: 60 рас из
// каталога races/ships/*.md + имена из config/races.json (спека §6.1 п.1).
func (s *Server) handleShipsRaces(w http.ResponseWriter, r *http.Request) {
	var races []map[string]string
	entries, err := os.ReadDir(s.shipsDir())
	if err != nil {
		writeJSON(w, map[string]interface{}{"races": races})
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".md")
		races = append(races, map[string]string{"slug": slug, "name": s.raceNameBySlug[slug]})
	}
	sort.Slice(races, func(i, j int) bool { return races[i]["slug"] < races[j]["slug"] })
	writeJSON(w, map[string]interface{}{"races": races})
}

// handleShipsInfo — GET /ships/info?race=&type= → {race, race_name, family,
// type, types, texture, silhouette, blocked} (из ships.json; фолбек — из
// лор-файла, спека §6.2). type — тип корабля (starship/…); пусто — первый
// (у легаси-расы — единственный безымянный).
func (s *Server) handleShipsInfo(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	race := q.Get("race")
	typeName := q.Get("type")
	entry, ok := s.shipsEntry(race)
	if !ok {
		// фолбек: парсим лор-файл напрямую (пачка ещё не пересобрана)
		lore, err := s.parseShipLore(race)
		if err != nil {
			writeJSON(w, map[string]string{"error": err.Error()})
			return
		}
		entry = config.ShipEntry{
			RaceName: s.raceNameBySlug[race], Family: lore.Family,
			Texture: lore.Texture, Silhouette: lore.Silhouette, Blocked: lore.Blocked, Types: lore.Types,
		}
	}
	t, ok := entry.ResolveShipType(typeName)
	if !ok {
		writeJSON(w, map[string]string{"error": "нет типа " + typeName + " у расы " + race})
		return
	}
	writeJSON(w, map[string]interface{}{
		"race": race, "race_name": entry.RaceName, "family": entry.Family,
		"type": t.Type, "types": entry.ShipTypeNames(),
		"texture": t.Texture, "silhouette": t.Silhouette, "blocked": t.Blocked,
	})
}

// handleShipsPrompt — GET /ships/prompt?race=&type=&tags=&seed= → {prompt1,
// prompt2, race, race_name, type}: сборка без генерации (паттерн /prompt 98b,
// спека §6.2). type — тип корабля (пусто — первый). prompt1 — txt2img по
// рецепту 2026-09-21, prompt2 — этап Hi-Res.
func (s *Server) handleShipsPrompt(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	race := q.Get("race")
	entry, ok := s.shipsEntry(race)
	if !ok {
		writeJSON(w, map[string]string{"error": "нет расы " + race + " в ships.json"})
		return
	}
	t, ok := entry.ResolveShipType(q.Get("type"))
	if !ok {
		writeJSON(w, map[string]string{"error": "нет типа " + q.Get("type") + " у расы " + race})
		return
	}
	seed := time.Now().UnixNano()
	if sv := q.Get("seed"); sv != "" {
		if n, err := strconv.ParseInt(sv, 10, 64); err == nil {
			seed = n
		}
	}
	rng := rand.New(rand.NewSource(seed))
	prompt1 := generator.BuildShipTxt2ImgPrompt(rng, entry.ForType(t), q.Get("tags"))
	prompt2 := generator.BuildShipHiResPrompt(prompt1)
	writeJSON(w, map[string]interface{}{"prompt1": prompt1, "prompt2": prompt2, "race": race, "race_name": entry.RaceName, "type": t.Type})
}

// handleShipsGen — GET /ships/gen?race=&type=&n=&tags=&prompt1_override=
// &prompt2_override=&hires=&size=&keep= → {msg} (спека §6.2; tags/override —
// 98b, size — эскиз/полный, 98c; hires — этап детализации, рецепт 2026-09-21;
// type — тип корабля, пусто — первый тип расы; keep=1 — не чистить пул).
func (s *Server) handleShipsGen(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	n := clampCount(atoiDefault(q.Get("n"), 3), s.cfg.MaxCount)
	size := clampShipSize(atoiDefault(q.Get("size"), 200))
	msg, _ := s.runner.GenShips(q.Get("race"), q.Get("type"), n, q.Get("tags"), q.Get("prompt1_override"), q.Get("prompt2_override"), s.shipHires(q.Get("hires")), size, q.Get("keep") == "1")
	writeJSON(w, map[string]string{"msg": msg})
}

// handleShipsGenBatch — GET /ships/genbatch?races=<CSV>&per=&tags=&prompt1_override=
// &prompt2_override=&hires=&size=&keep= → {msg} (пачка рас; типы расы
// разворачиваются, люди ×4; спека §6.2; tags/override — 98b на все расы пачки,
// size — эскиз/полный, 98c; keep=1 — не чистить пул, накопительный прогон).
func (s *Server) handleShipsGenBatch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var races []string
	for _, rc := range strings.Split(q.Get("races"), ",") {
		if rc = strings.TrimSpace(rc); rc != "" {
			races = append(races, rc)
		}
	}
	per := atoiDefault(q.Get("per"), 3)
	size := clampShipSize(atoiDefault(q.Get("size"), 200))
	msg, _ := s.runner.GenShipsBatch(races, per, q.Get("tags"), q.Get("prompt1_override"), q.Get("prompt2_override"), s.shipHires(q.Get("hires")), size, q.Get("keep") == "1")
	writeJSON(w, map[string]string{"msg": msg})
}

// shipHires — включён ли этап детализации: приоритет — явный query-параметр
// (UI всегда передаёт его явно), иначе — ships.hires.enabled из studio.json
// (спека 2026-09-21 §6.1 М-10).
func (s *Server) shipHires(v string) bool {
	if v != "" {
		return v == "1" || v == "true"
	}
	return s.cfg.ShipParams().Hires.Enabled
}

// clampShipSize — допустимый финальный размер кандидата корабля: 200 (полный)
// или 100 (эскиз, 98c); иное → полный.
func clampShipSize(n int) int {
	if n != 100 && n != 200 {
		return 200
	}
	return n
}

// handleShipsVote — GET /ships/vote?file=&vote=like|dislike|clear → {msg}
// (98c): вердикт создателя по кандидату — только метка в meta.json пула,
// файл не перемещается; переживает рестарт студии (meta.json — уже файл).
func (s *Server) handleShipsVote(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	file := filepath.Base(q.Get("file"))
	vote := q.Get("vote")
	if vote != "like" && vote != "dislike" && vote != "clear" {
		writeJSON(w, map[string]string{"msg": "vote = like|dislike|clear"})
		return
	}
	pool := filepath.Join(s.cfg.PoolRoot, "ships_pool")
	if !generator.SetShipVote(pool, file, vote) {
		writeJSON(w, map[string]string{"msg": "нет кандидата " + file + " в мете пула"})
		return
	}
	msg := "👍 Нравится: " + file
	if vote == "dislike" {
		msg = "👎 Не нравится: " + file
	} else if vote == "clear" {
		msg = "Вердикт снят: " + file
	}
	writeJSON(w, map[string]string{"msg": msg})
}

// shipListItem — запись GET /ships/list: кандидат пула + текущая пара (A, F).
// Студия строит из пары CSS-трансформ превью (спека 2026-09-21 §4.2): без неё
// сетка и миниатюры пула показывали бы недовёрнутую картинку.
type shipListItem struct {
	File     string  `json:"file"`
	Race     string  `json:"race"`
	RaceName string  `json:"race_name"`
	Num      string  `json:"num"`
	Labels   string  `json:"labels"`
	Vote     string  `json:"vote"`
	Angle    float64 `json:"angle"`
	Flip     bool    `json:"flip"`
}

// handleShipsList — GET /ships/list → [{file, race, race_name, num, labels,
// vote, angle, flip}]: кандидаты пула (спека §6.2 + пара (A, F), §4.2).
func (s *Server) handleShipsList(w http.ResponseWriter, r *http.Request) {
	pool := filepath.Join(s.cfg.PoolRoot, "ships_pool")
	byFile := map[string]generator.ShipMetaItem{}
	for _, m := range generator.ReadShipMeta(pool) {
		byFile[m.File] = m
	}
	entries, _ := os.ReadDir(pool)
	var files []string
	for _, e := range entries {
		name := e.Name()
		// только готовые кандидаты sNN.png (сырые кадры _raw_*/_hr_* — не в список)
		if strings.HasPrefix(name, "s") && strings.HasSuffix(name, ".png") {
			files = append(files, name)
		}
	}
	sort.Strings(files)
	out := []shipListItem{}
	for _, name := range files {
		m := byFile[name]
		out = append(out, shipListItem{
			File: name, Race: m.Race, RaceName: m.RaceName,
			Num:    strings.TrimSuffix(strings.TrimPrefix(name, "s"), ".png"),
			Labels: strings.Join(m.Labels, ", "),
			Vote:   m.Vote,
			Angle:  m.Angle, Flip: m.Flip,
		})
	}
	writeJSON(w, out)
}

// handleShipsImg — GET /ships/img/<file> → PNG кандидата (спека §6.2).
func (s *Server) handleShipsImg(w http.ResponseWriter, r *http.Request) {
	fname := filepath.Base(r.URL.Path[len("/ships/img/"):])
	fp := filepath.Join(s.cfg.PoolRoot, "ships_pool", fname)
	servePNG(w, fp)
}

// shipIngameItem — запись витрины «В игре»: корабль игрового реестра.
// Своя DTO над models.ShipSprite: angle/flip обязаны быть в JSON всегда
// (в модели omitempty), плюс race_name из config/races.json.
type shipIngameItem struct {
	File     string  `json:"file"`
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Race     string  `json:"race"`
	RaceName string  `json:"race_name"`
	Angle    float64 `json:"angle"`
	Flip     bool    `json:"flip"`
}

// handleShipsIngame — GET /ships/ingame → [{file, id, name, race, race_name,
// angle, flip}]: витрина всех кораблей, уже лежащих в игре. Реестр читается
// файлом при запросе (models.ReadShipRegistry), а не из package-переменной:
// удаление видно в студии сразу, без перезапуска. Состав — ровно содержимое
// файла (нейтральный последним). Ошибка чтения — 500 с текстом.
func (s *Server) handleShipsIngame(w http.ResponseWriter, r *http.Request) {
	ships, err := models.ReadShipRegistry(s.shipRegistry())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]string{"error": "реестр кораблей: " + err.Error()})
		return
	}
	out := make([]shipIngameItem, 0, len(ships))
	for _, sp := range ships {
		out = append(out, shipIngameItem{
			File: sp.File, ID: sp.ID, Name: sp.Name, Race: sp.Race,
			RaceName: s.raceNameBySlug[sp.Race], Angle: sp.Angle, Flip: sp.Flip,
		})
	}
	writeJSON(w, out)
}

// handleShipsIngameImg — GET /ships/ingame/img/<file> → PNG из игровой папки
// спрайтов. filepath.Base — защита от выхода из папки.
func (s *Server) handleShipsIngameImg(w http.ResponseWriter, r *http.Request) {
	fname := filepath.Base(r.URL.Path[len("/ships/ingame/img/"):])
	fp := filepath.Join(s.gameSpritesDir(), fname)
	servePNG(w, fp)
}

// shipDeleteSteps — какие шаги удаления выполнены (ответ /ships/ingame/delete).
type shipDeleteSteps struct {
	Registry bool `json:"registry"`
	Meta     bool `json:"meta"`
	Accepted bool `json:"accepted"`
	Sprite   bool `json:"sprite"`
}

// handleShipsIngameDelete — POST /ships/ingame/delete?file=<name>: физическое
// удаление одного корабля из игры (спека 2026-09-25 §4): запись реестра →
// запись ships_meta.json → PNG accepted → PNG игры (последним, инвариант «нет
// записи реестра без PNG игры»). Мета/accepted связываются с игровым PNG по
// sha256 (хэш считается ДО удаления), фолбэк по имени. Нейтральный и людской
// дефолт не удаляются. Операция адресуется именем файла и домешивается: повтор
// по уже частично удалённому файлу не отдаёт глухой 404.
func (s *Server) handleShipsIngameDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		writeJSON(w, map[string]interface{}{"ok": false, "error": "только POST"})
		return
	}
	file := r.URL.Query().Get("file")
	if !validShipDeleteName(file) {
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]interface{}{"ok": false, "error": "некорректное имя файла: " + file})
		return
	}
	if file == models.NeutralShip.File || file == models.DefaultHumanShip {
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]interface{}{"ok": false, "error": "защищённый корабль: " + file})
		return
	}

	var steps shipDeleteSteps
	fail := func(step, msg string) {
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]interface{}{"ok": false, "error": msg, "step": step})
	}

	// sha256 игрового PNG — считается ДО удаления, пока файл на месте (связывание
	// меты/accepted с игровым именем; при отсутствии файла — фолбэк по имени).
	gameDir := s.gameSpritesDir()
	gamePNG := filepath.Join(gameDir, file)
	gameHash := ""
	if pathInsideDir(gameDir, gamePNG) {
		if h, err := fileSHA256(gamePNG); err == nil {
			gameHash = h
		}
	}

	// Известное ограничение (принято в спеке): read-modify-write реестра и меты
	// не сериализован — при одновременных удалениях возможно lost update.
	// Студия односеансная, общего лока на файлы не вводим.
	//
	// шаг 1: реестр — убрать запись и записать атомарно.
	ships, err := models.ReadShipRegistry(s.shipRegistry())
	if err != nil {
		fail("registry", "реестр кораблей: "+err.Error())
		return
	}
	kept := make([]models.ShipSprite, 0, len(ships))
	race := ""
	found := false
	for _, sp := range ships {
		if sp.File == file {
			found = true
			race = sp.Race
			continue
		}
		kept = append(kept, sp)
	}
	if found {
		data, err := json.MarshalIndent(struct {
			Version int                 `json:"version"`
			Ships   []models.ShipSprite `json:"ships"`
		}{Version: 1, Ships: kept}, "", "  ")
		if err != nil {
			fail("registry", "реестр: "+err.Error())
			return
		}
		if err := writeFileAtomic(s.shipRegistry(), data); err != nil {
			fail("registry", "реестр: "+err.Error())
			return
		}
		steps.Registry = true
	}

	// шаг 2: запись ships_meta.json, связанная с файлом (по хэшу, фолбэк по имени).
	accDir := s.acceptedShipsDir()
	metaRemoved, mrace, err := removeShipsMeta(filepath.Join(accDir, "ships_meta.json"), file, gameHash, accDir)
	if err != nil {
		fail("meta", "мета: "+err.Error())
		return
	}
	if metaRemoved {
		steps.Meta = true
	}
	if race == "" {
		race = mrace
	}

	// шаг 3: PNG приёмки, связанные с файлом (по хэшу, фолбэк по имени).
	if removed, err := removeAcceptedPNGs(accDir, file, gameHash); err != nil {
		fail("accepted", "accepted PNG: "+err.Error())
		return
	} else if removed {
		steps.Accepted = true
	}

	// шаг 4: PNG игры — последним.
	if pathInsideDir(gameDir, gamePNG) {
		if err := os.Remove(gamePNG); err == nil {
			steps.Sprite = true
		} else if !os.IsNotExist(err) {
			fail("sprite", "PNG игры: "+err.Error())
			return
		}
	}

	if !found && !steps.Meta && !steps.Accepted && !steps.Sprite {
		w.WriteHeader(http.StatusNotFound)
		writeJSON(w, map[string]interface{}{"ok": false, "error": "удалять нечего: " + file})
		return
	}

	raceLeft := 0
	if race != "" {
		for _, sp := range kept {
			if sp.Race == race {
				raceLeft++
			}
		}
	}
	writeJSON(w, map[string]interface{}{
		"ok": true, "file": file, "race": race,
		"ships_left": len(kept), "race_ships_left": raceLeft,
		"steps": steps,
	})
}

// validShipDeleteName — имя файла пригодно к удалению: непустое, без разделителей
// пути (в т.ч. декодированного %5C), без обхода (filepath.Base не срезает) и без
// управляющих символов (в т.ч. NUL): иначе os.Remove на шаге спрайта вернёт
// «invalid argument» и операция отдаст 500 вместо 400.
func validShipDeleteName(file string) bool {
	if file == "" || strings.ContainsAny(file, `/\`) || strings.Contains(file, "..") {
		return false
	}
	if strings.ContainsFunc(file, unicode.IsControl) {
		return false
	}
	return filepath.Base(file) == file
}

// pathInsideDir — путь p лежит внутри каталога dir (защита шагов 3/4 от обхода).
func pathInsideDir(dir, p string) bool {
	d := filepath.Clean(dir)
	c := filepath.Clean(p)
	return strings.HasPrefix(c, d+string(os.PathSeparator))
}

// fileSHA256 — hex-sha256 файла (связывание меты/accepted с игровым PNG).
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// writeFileAtomic — записать файл атомарно: temp в том же каталоге + os.Rename
// (на Windows os.Rename заменяет существующий файл; при сбое — fallback через
// .bak, не удаляя целевой файл до успешного повтора).
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".ships_tmp_*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		// Прямой rename не прошёл. Целевой файл НЕ удаляем до успешного повтора
		// (иначе при сбое повтора система осталась бы без файла): отводим его в
		// path+".bak", ставим tmp на место, при успехе убираем .bak; если повтор
		// не прошёл — возвращаем .bak на место. Не удалось и это — ошибка, но
		// данные не потеряны: рабочий файл лежит в path+".bak".
		bak := path + ".bak"
		if berr := os.Rename(path, bak); berr != nil {
			os.Remove(tmpName)
			return err
		}
		if rerr := os.Rename(tmpName, path); rerr != nil {
			os.Remove(tmpName)
			if restoreErr := os.Rename(bak, path); restoreErr != nil {
				return fmt.Errorf("запись %s: повтор rename: %v; возврат .bak: %v", path, rerr, restoreErr)
			}
			return rerr
		}
		os.Remove(bak)
	}
	return nil
}

// shipLinkedToGame — связана ли запись (мета или PNG приёмки) с игровым файлом:
// по имени (фолбэк) либо по sha256 (имя приёмки может отличаться от игрового).
func shipLinkedToGame(name, gameFile, gameHash, acceptedDir string) bool {
	if name == gameFile {
		return true
	}
	if gameHash == "" {
		return false
	}
	h, err := fileSHA256(filepath.Join(acceptedDir, name))
	if err != nil {
		return false
	}
	return h == gameHash
}

// removeShipsMeta — убрать из ships_meta.json записи, связанные с игровым файлом
// (по sha256, фолбэк по имени). Возвращает «были ли удалены записи», расу первой
// удалённой записи и ошибку записи. Файла/записей нет — не ошибка.
func removeShipsMeta(metaPath, gameFile, gameHash, acceptedDir string) (bool, string, error) {
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, "", nil
		}
		return false, "", err
	}
	// Читаем мету как []map[string]any (UseNumber — большие seed не теряют
	// точность int64), а не как []generator.ShipMetaItem: так сохраняются ВСЕ
	// поля записи, включая неизвестные (будущие), а не только известные.
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var items []map[string]any
	if err := dec.Decode(&items); err != nil {
		return false, "", err
	}
	kept := make([]map[string]any, 0, len(items))
	removed := false
	race := ""
	for _, m := range items {
		file, _ := m["file"].(string)
		if shipLinkedToGame(file, gameFile, gameHash, acceptedDir) {
			removed = true
			if race == "" {
				if r, ok := m["race"].(string); ok {
					race = r
				}
			}
			continue
		}
		kept = append(kept, m)
	}
	if !removed {
		return false, "", nil
	}
	out, err := json.Marshal(kept)
	if err != nil {
		return false, "", err
	}
	if err := writeFileAtomic(metaPath, out); err != nil {
		return false, "", err
	}
	return true, race, nil
}

// removeAcceptedPNGs — удалить PNG приёмки, связанные с игровым файлом (по
// sha256, фолбэк по имени), только внутри каталога acceptedDir. Возвращает
// «был ли удалён хотя бы один файл».
func removeAcceptedPNGs(acceptedDir, gameFile, gameHash string) (bool, error) {
	entries, err := os.ReadDir(acceptedDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	removed := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".png") {
			continue
		}
		if !shipLinkedToGame(e.Name(), gameFile, gameHash, acceptedDir) {
			continue
		}
		p := filepath.Join(acceptedDir, e.Name())
		if !pathInsideDir(acceptedDir, p) {
			continue
		}
		if err := os.Remove(p); err != nil {
			if os.IsNotExist(err) {
				continue // файла уже нет — это не «удалён» (steps.accepted не врёт)
			}
			return removed, err
		}
		removed = true
	}
	return removed, nil
}

// handleShipsAct — GET /ships/act?file=&what=accept|reject|rotate&angle=<deg>|
// rot90|rot180|flipH|setangle&angle=<abs>|auto|fit → {msg, angle, flip}
// (спека 2026-09-21 §4.1). Действия ориентации (rotate/rot90/rot180/flipH/
// setangle/auto) пишут пару (A, F) в meta.json пула и НЕ трогают пиксели
// файла; файл перезаписывает только fit («вписать в кадр»). accept копирует
// кандидата байт-в-байт (+ гвард эскиза и маркер orient_meta), reject переносит
// в ships_rejected/. Ответ несёт текущую пару, чтобы клиент обновил слайдер.
func (s *Server) handleShipsAct(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	file := filepath.Base(q.Get("file"))
	what := q.Get("what")
	angle := atofDefault(q.Get("angle"), 0)
	pool := filepath.Join(s.cfg.PoolRoot, "ships_pool")
	src := filepath.Join(pool, file)
	msg := "?"
	if _, err := os.Stat(src); err == nil {
		switch what {
		case "accept":
			msg = acceptShipFile(pool, filepath.Join(s.cfg.PoolRoot, "final_accepted", "ships"), file)
		case "reject":
			rej := filepath.Join(s.cfg.PoolRoot, "ships_rejected")
			os.MkdirAll(rej, 0755)
			os.Rename(src, filepath.Join(rej, filepath.Base(file)))
			generator.RemoveShipMeta(pool, filepath.Base(file))
			msg = "Удалено: " + file
		case "rotate":
			generator.UpdateShipAngle(pool, file, angle, false)
			msg = fmt.Sprintf("Повёрнуто на %g°: %s", angle, file)
		case "rot90":
			generator.UpdateShipAngle(pool, file, 90, false)
			msg = "Повёрнуто на 90°: " + file
		case "rot180":
			generator.UpdateShipAngle(pool, file, 180, false)
			msg = "Повёрнуто на 180°: " + file
		case "flipH":
			generator.UpdateShipAngle(pool, file, 0, true)
			msg = "Отражено: " + file
		case "setangle":
			_, flip, _ := generator.ShipOrientOf(pool, file)
			if generator.SetShipOrient(pool, file, angle, flip) {
				msg = fmt.Sprintf("Угол %g°: %s", angle, file)
			} else {
				msg = "нет меты для " + file
			}
		case "auto":
			if a, f, err := s.shipAutoPair(file); err != nil {
				msg = "Ошибка: " + err.Error()
			} else if generator.SetShipOrient(pool, file, a, f) {
				msg = fmt.Sprintf("Авто: %g°: %s", a, file)
			} else {
				msg = "нет меты для " + file
			}
		case "fit":
			if err := transformShipImage(src); err != nil {
				msg = "Ошибка: " + err.Error()
			} else {
				msg = "Вписано в кадр: " + file
			}
		}
	}
	out := map[string]interface{}{"msg": msg}
	if a, f, ok := generator.ShipOrientOf(pool, file); ok {
		out["angle"] = a
		out["flip"] = f
	}
	writeJSON(w, out)
}

// acceptShipFile принимает кандидата корабля: race_<slug>_<type>.png для расы с
// типом корабля (тип из меты файла) либо race_<slug>_NN.png (первый свободный
// номер по slug) — плюс ships_meta.json рядом с файлами (спека §5).
// Раса/тип — из меты файла (не из выбранного в UI). Файл копируется
// байт-в-байт (пиксели не трогаются). Гвард эскиза (100×100 в игре
// апскейлится = мыло, §4.4) и маркер orient_meta: pair (A, F) не запечена в
// пиксели.
func acceptShipFile(pool, acceptDir, file string) string {
	src := filepath.Join(pool, file)
	var meta *generator.ShipMetaItem
	// индекс, а не `for _, m := range`: go.mod — go 1.21, переменная цикла одна
	// на все итерации, `&m` указывает на последний элемент (приёмка брала мету
	// последнего кандидата пула — чужую расу/seed/промпт).
	metaList := generator.ReadShipMeta(pool)
	for i := range metaList {
		if metaList[i].File == file {
			meta = &metaList[i]
			break
		}
	}
	if meta == nil {
		return "нет меты для " + file
	}
	if meta.Size == 100 {
		return "эскиз 100×100 — для игры нужен 200×200"
	}
	os.MkdirAll(acceptDir, 0755)
	dst := filepath.Join(acceptDir, shipAcceptName(acceptDir, meta.Race, meta.Type))
	if err := copyFile(src, dst); err != nil {
		return "Ошибка: " + err.Error()
	}
	os.Remove(src)
	generator.RemoveShipMeta(pool, file)
	// в ships_meta.json — имя принятого файла (спека §5: file — файл корабля)
	// + пара (A, F) из меты пула (применяется при показе), дата и маркер
	// orient_meta (§4.4).
	meta.File = filepath.Base(dst)
	meta.Date = time.Now().Format("2006-01-02")
	meta.OrientMeta = true
	appendShipsMeta(filepath.Join(acceptDir, "ships_meta.json"), *meta)
	return "Принято: " + filepath.Base(dst)
}

// shipAcceptName — имя принятого корабля: race_<slug>_<type>.png для расы с
// типом (тип уникален; при повторной приёмке — race_<slug>_<type>_NN.png),
// иначе race_<slug>_NN.png (первый свободный номер).
func shipAcceptName(dir, slug, typ string) string {
	if typ == "" {
		return fmt.Sprintf("race_%s_%02d.png", slug, nextShipAcceptNum(dir, slug))
	}
	base := fmt.Sprintf("race_%s_%s", slug, typ)
	if _, err := os.Stat(filepath.Join(dir, base+".png")); err != nil {
		return base + ".png"
	}
	for n := 2; ; n++ {
		cand := fmt.Sprintf("%s_%02d.png", base, n)
		if _, err := os.Stat(filepath.Join(dir, cand)); err != nil {
			return cand
		}
	}
}

// nextShipAcceptNum — первый свободный номер race_<slug>_NN.png в папке.
func nextShipAcceptNum(dir, slug string) int {
	n := 1
	for {
		if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("race_%s_%02d.png", slug, n))); err != nil {
			return n
		}
		n++
	}
}

// appendShipsMeta дописывает запись в ships_meta.json (принятые корабли).
func appendShipsMeta(path string, item generator.ShipMetaItem) {
	data, err := os.ReadFile(path)
	var all []generator.ShipMetaItem
	if err == nil {
		json.Unmarshal(data, &all)
	}
	all = append(all, item)
	out, err := json.Marshal(all)
	if err != nil {
		return
	}
	os.WriteFile(path, out, 0644)
}

// parseShipLore — машинная проекция лор-файла корабля (фолбек /ships/info,
// когда пачка ещё не пересобрана в ships.json).
func (s *Server) parseShipLore(slug string) (config.ShipLore, error) {
	data, err := os.ReadFile(filepath.Join(s.shipsDir(), slug+".md"))
	if err != nil {
		return config.ShipLore{}, fmt.Errorf("нет лор-файла %s/%s.md", s.shipsDir(), slug)
	}
	return config.ParseShipSection(string(data))
}

// shipsDir — каталог лор-файлов кораблей (имена файлов = slug расы).
// Поле Server (как loreDir): в тестах переопределяется на относительный путь.
func (s *Server) shipsDir() string {
	if s.shipsDirPath != "" {
		return s.shipsDirPath
	}
	return "docs/gamedesign/races/ships"
}

// gameSpritesDir — каталог игровых спрайтов кораблей (витрина «В игре»).
// Поле Server (как shipsDirPath): в тестах переопределяется на относительный
// путь; имя отличается от shipsDirPath (тот занят каталогом лора).
func (s *Server) gameSpritesDir() string {
	if s.gameSpritesDirPath != "" {
		return s.gameSpritesDirPath
	}
	return "web/static/sprites"
}

// shipRegistry — файл реестра кораблей игры (чтение витрины + запись при
// удалении). Поле Server: в тестах переопределяется на temp-файл.
func (s *Server) shipRegistry() string {
	if s.shipRegistryPath != "" {
		return s.shipRegistryPath
	}
	return "config/ships_registry.json"
}

// acceptedShipsDir — каталог принятых PNG + ships_meta.json (удаление корабля).
// Поле Server: в тестах переопределяется на temp-каталог.
func (s *Server) acceptedShipsDir() string {
	if s.acceptedShipsDirPath != "" {
		return s.acceptedShipsDirPath
	}
	return "ai_drafts/final_accepted/ships"
}

// --- Режим приёмки кораблей: подсказка носа, счётчик ---

// shipOrient — ответ orient-режима tools/ship_sprite_cut.py
// (profile_orientation): angle — поворот главной оси в конвенции PIL
// (ПОЛОЖИТЕЛЬНЫЙ — против часовой), mirror — предлагаемое зеркало (нос влево),
// ambiguous — авто не уверено (human reads «авто: не уверен»), reason — почему.
type shipOrient struct {
	Angle     float64 `json:"angle"`
	Mirror    bool    `json:"mirror"`
	Ambiguous bool    `json:"ambiguous"`
	Reason    string  `json:"reason"`
}

// shipOrientTimeout — предел ожидания Python-подсказки. Зависший python НЕ
// должен подвешивать /ships/auto (и UI, который его ждёт): по истечении
// возвращается внятная ошибка. Переменная (не const) — тест подменяет её
// коротким значением.
var shipOrientTimeout = 20 * time.Second

// shipOrientHint — подсказка авто-ориентации через Python-процесс
// (tools/ship_sprite_cut.py --orient-only --report). Единственный источник
// детекции носа — стабильный скрипт студии (тот же, что у выреза; спайк вышел
// из рабочего пути, PITFALLS). Ошибка — Python недоступен/парсинг/таймаут.
func shipOrientHint(pythonCmd, src string) (shipOrient, error) {
	var info shipOrient
	tmp, err := os.CreateTemp("", "ship_orient_*.json")
	if err != nil {
		return info, err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)
	ctx, cancel := context.WithTimeout(context.Background(), shipOrientTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, pythonCmd, "tools/ship_sprite_cut.py", src, "--orient-only", "--report", tmpPath)
	// WaitDelay: на Windows Kill убивает только прямой процесс (cmd.exe), а
	// внук (python под .cmd/шима) может держать пайп вывода открытым — без
	// WaitDelay CombinedOutput ждёт ЕГО завершения, и «таймаут» не срабатывает.
	cmd.WaitDelay = 2 * time.Second
	if out, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return info, fmt.Errorf("ориентация: таймаут %s: %s", shipOrientTimeout, strings.TrimSpace(string(out)))
		}
		return info, fmt.Errorf("ориентация: %v: %s", err, strings.TrimSpace(string(out)))
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return info, err
	}
	var rep struct {
		Orient shipOrient `json:"orient"`
	}
	if err := json.Unmarshal(data, &rep); err != nil {
		return info, err
	}
	return rep.Orient, nil
}

// shipAutoPair — пара (A, F) по подсказке авто-носа кандидата: Python-подсказка
// (shipOrientHint) → канонизатор generator.ShipHintPair (спека §3.3). Ошибка —
// Python недоступен/парсинг/таймаут.
func (s *Server) shipAutoPair(file string) (float64, bool, error) {
	src := filepath.Join(s.cfg.PoolRoot, "ships_pool", file)
	info, err := shipOrientHint(s.cfg.PythonCmd, src)
	if err != nil {
		return 0, false, err
	}
	a, f := generator.ShipHintPair(info.Angle, info.Mirror, info.Ambiguous)
	return a, f, nil
}

// handleShipsAuto — GET /ships/auto?file= → {angle, flip, ambiguous, reason}
// (при ошибке Python — {error}): ПОДСКАЗКА авто-носа, уже в конвенции показа
// (пара (A, F), §3.3) — равна значению what=auto и тому, что покажет слайдер.
// Решение всё равно за человеком.
func (s *Server) handleShipsAuto(w http.ResponseWriter, r *http.Request) {
	file := filepath.Base(r.URL.Query().Get("file"))
	info, err := shipOrientHint(s.cfg.PythonCmd, filepath.Join(s.cfg.PoolRoot, "ships_pool", file))
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	a, f := generator.ShipHintPair(info.Angle, info.Mirror, info.Ambiguous)
	writeJSON(w, map[string]interface{}{
		"angle": a, "flip": f, "ambiguous": info.Ambiguous, "reason": info.Reason,
	})
}

// handleShipsAccepted — GET /ships/accepted?race= → {race, accepted}: число
// принятых кораблей расы в реестре ships_meta.json (счётчик «принято N из M»).
func (s *Server) handleShipsAccepted(w http.ResponseWriter, r *http.Request) {
	race := r.URL.Query().Get("race")
	n := 0
	for _, m := range readShipsAccepted(filepath.Join(s.cfg.PoolRoot, "final_accepted", "ships")) {
		if m.Race == race {
			n++
		}
	}
	writeJSON(w, map[string]interface{}{"race": race, "accepted": n})
}

// readShipsAccepted — записи ships_meta.json в каталоге принятых кораблей.
// Файл называется ships_meta.json (не meta.json), поэтому generator.ReadShipMeta
// (читает meta.json) здесь не подходит.
func readShipsAccepted(dir string) []generator.ShipMetaItem {
	data, err := os.ReadFile(filepath.Join(dir, "ships_meta.json"))
	if err != nil {
		return nil
	}
	var m []generator.ShipMetaItem
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}