package handlers

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"zorion/cmd/art-studio/config"
	"zorion/cmd/art-studio/generator"
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

// handleShipsInfo — GET /ships/info?race= → {race, race_name, family, texture,
// silhouette, blocked} (из ships.json; фолбек — из лор-файла, спека §6.2).
func (s *Server) handleShipsInfo(w http.ResponseWriter, r *http.Request) {
	race := r.URL.Query().Get("race")
	entry, ok := s.shipsEntry(race)
	if !ok {
		// фолбек: парсим лор-файл напрямую (пачка ещё не пересобрана)
		texture, silhouette, blocked, family, err := s.parseShipLore(race)
		if err != nil {
			writeJSON(w, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, map[string]interface{}{
			"race": race, "race_name": s.raceNameBySlug[race], "family": family,
			"texture": texture, "silhouette": silhouette, "blocked": blocked,
		})
		return
	}
	writeJSON(w, map[string]interface{}{
		"race": race, "race_name": entry.RaceName, "family": entry.Family,
		"texture": entry.Texture, "silhouette": entry.Silhouette, "blocked": entry.Blocked,
	})
}

// handleShipsPrompt — GET /ships/prompt?race=&tags=&seed= → {prompt1, prompt2,
// race, race_name}: сборка без генерации (паттерн /prompt 98b, спека §6.2).
func (s *Server) handleShipsPrompt(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	race := q.Get("race")
	entry, ok := s.shipsEntry(race)
	if !ok {
		writeJSON(w, map[string]string{"error": "нет расы " + race + " в ships.json"})
		return
	}
	dict := s.shipDictRef()
	if dict == nil {
		writeJSON(w, map[string]string{"error": "словарь кораблей не подключён"})
		return
	}
	seed := time.Now().UnixNano()
	if sv := q.Get("seed"); sv != "" {
		if n, err := strconv.ParseInt(sv, 10, 64); err == nil {
			seed = n
		}
	}
	rng := rand.New(rand.NewSource(seed))
	spec := generator.ParseSilhouette(entry.Silhouette, dict)
	prompt1 := generator.BuildShipPrompt1(rng, race, entry, spec, dict, q.Get("tags"))
	prompt2 := generator.BuildShipPrompt2(rng, entry, dict, q.Get("tags"))
	writeJSON(w, map[string]interface{}{"prompt1": prompt1, "prompt2": prompt2, "race": race, "race_name": entry.RaceName})
}

// handleShipsGen — GET /ships/gen?race=&n=&tags=&prompt1_override=&prompt2_override=&size=
// → {msg} (спека §6.2; tags/override — 98b, size — эскиз/полный, 98c).
func (s *Server) handleShipsGen(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	n := clampCount(atoiDefault(q.Get("n"), 3), s.cfg.MaxCount)
	size := clampShipSize(atoiDefault(q.Get("size"), 200))
	msg, _ := s.runner.GenShips(q.Get("race"), n, q.Get("tags"), q.Get("prompt1_override"), q.Get("prompt2_override"), size)
	writeJSON(w, map[string]string{"msg": msg})
}

// handleShipsGenBatch — GET /ships/genbatch?races=<CSV>&per=&tags=&prompt1_override=
// &prompt2_override=&size= → {msg} (пачка 10 рас × 3 = 30 задач, спека §6.2;
// tags/override — 98b на все расы пачки, size — эскиз/полный, 98c).
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
	msg, _ := s.runner.GenShipsBatch(races, per, q.Get("tags"), q.Get("prompt1_override"), q.Get("prompt2_override"), size)
	writeJSON(w, map[string]string{"msg": msg})
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

// handleShipsList — GET /ships/list → [{file, race, race_name, num}]:
// кандидаты пула (спека §6.2).
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
		if strings.HasSuffix(name, ".png") && !strings.HasPrefix(name, "_raw_") {
			files = append(files, name)
		}
	}
	sort.Strings(files)
	out := []map[string]string{}
	for _, name := range files {
		m := byFile[name]
		out = append(out, map[string]string{
			"file": name, "race": m.Race, "race_name": m.RaceName,
			"num":    strings.TrimSuffix(strings.TrimPrefix(name, "s"), ".png"),
			"labels": strings.Join(m.Labels, ", "),
			"vote":   m.Vote,
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

// handleShipsAct — GET /ships/act?file=&what=accept|reject → {msg} (спека §6.2):
// принять → final_accepted/ships/race_<slug>_NN.png + ships_meta.json;
// удалить → ships_rejected/.
func (s *Server) handleShipsAct(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	file := q.Get("file")
	what := q.Get("what")
	pool := filepath.Join(s.cfg.PoolRoot, "ships_pool")
	src := filepath.Join(pool, filepath.Base(file))
	msg := "?"
	switch what {
	case "accept":
		if _, err := os.Stat(src); err == nil {
			msg = acceptShipFile(pool, filepath.Join(s.cfg.PoolRoot, "final_accepted", "ships"), file)
		}
	case "reject":
		if _, err := os.Stat(src); err == nil {
			rej := filepath.Join(s.cfg.PoolRoot, "ships_rejected")
			os.MkdirAll(rej, 0755)
			os.Rename(src, filepath.Join(rej, filepath.Base(file)))
			generator.RemoveShipMeta(pool, filepath.Base(file))
			msg = "Удалено: " + file
		}
	}
	writeJSON(w, map[string]string{"msg": msg})
}

// acceptShipFile принимает кандидата корабля: race_<slug>_NN.png (первый
// свободный номер по slug) + ships_meta.json рядом с файлами (спека §5).
// Раса — из меты файла (не из выбранной в UI).
func acceptShipFile(pool, acceptDir, file string) string {
	src := filepath.Join(pool, file)
	var meta *generator.ShipMetaItem
	for _, m := range generator.ReadShipMeta(pool) {
		if m.File == file {
			meta = &m
		}
	}
	if meta == nil {
		return "нет меты для " + file
	}
	os.MkdirAll(acceptDir, 0755)
	dst := filepath.Join(acceptDir, fmt.Sprintf("race_%s_%02d.png", meta.Race, nextShipAcceptNum(acceptDir, meta.Race)))
	if err := copyFile(src, dst); err != nil {
		return "Ошибка: " + err.Error()
	}
	os.Remove(src)
	generator.RemoveShipMeta(pool, file)
	// в ships_meta.json — имя принятого файла (спека §5: file — файл корабля)
	meta.File = filepath.Base(dst)
	appendShipsMeta(filepath.Join(acceptDir, "ships_meta.json"), *meta)
	return "Принято: " + filepath.Base(dst)
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
func (s *Server) parseShipLore(slug string) (texture, silhouette string, blocked []string, family string, err error) {
	data, err := os.ReadFile(filepath.Join(s.shipsDir(), slug+".md"))
	if err != nil {
		return "", "", nil, "", fmt.Errorf("нет лор-файла %s/%s.md", s.shipsDir(), slug)
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