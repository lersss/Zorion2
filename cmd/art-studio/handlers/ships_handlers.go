package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
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
// prompt1 — txt2img по рецепту 2026-09-21, prompt2 — этап Hi-Res.
func (s *Server) handleShipsPrompt(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	race := q.Get("race")
	entry, ok := s.shipsEntry(race)
	if !ok {
		writeJSON(w, map[string]string{"error": "нет расы " + race + " в ships.json"})
		return
	}
	seed := time.Now().UnixNano()
	if sv := q.Get("seed"); sv != "" {
		if n, err := strconv.ParseInt(sv, 10, 64); err == nil {
			seed = n
		}
	}
	rng := rand.New(rand.NewSource(seed))
	prompt1 := generator.BuildShipTxt2ImgPrompt(rng, entry, q.Get("tags"))
	prompt2 := generator.BuildShipHiResPrompt(prompt1)
	writeJSON(w, map[string]interface{}{"prompt1": prompt1, "prompt2": prompt2, "race": race, "race_name": entry.RaceName})
}

// handleShipsGen — GET /ships/gen?race=&n=&tags=&prompt1_override=&prompt2_override=&hires=&size=
// → {msg} (спека §6.2; tags/override — 98b, size — эскиз/полный, 98c;
// hires — этап детализации, рецепт 2026-09-21).
func (s *Server) handleShipsGen(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	n := clampCount(atoiDefault(q.Get("n"), 3), s.cfg.MaxCount)
	size := clampShipSize(atoiDefault(q.Get("size"), 200))
	msg, _ := s.runner.GenShips(q.Get("race"), n, q.Get("tags"), q.Get("prompt1_override"), q.Get("prompt2_override"), s.shipHires(q.Get("hires")), size)
	writeJSON(w, map[string]string{"msg": msg})
}

// handleShipsGenBatch — GET /ships/genbatch?races=<CSV>&per=&tags=&prompt1_override=
// &prompt2_override=&hires=&size= → {msg} (пачка 10 рас × 3 = 30 задач, спека
// §6.2; tags/override — 98b на все расы пачки, size — эскиз/полный, 98c).
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
	msg, _ := s.runner.GenShipsBatch(races, per, q.Get("tags"), q.Get("prompt1_override"), q.Get("prompt2_override"), s.shipHires(q.Get("hires")), size)
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
		// только готовые кандидаты sNN.png (сырые кадры _raw_*/_hr_* — не в список)
		if strings.HasPrefix(name, "s") && strings.HasSuffix(name, ".png") {
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

// handleShipsAct — GET /ships/act?file=&what=accept|reject|rot90|rot180|flipH|rotate&angle=<deg>|fit
// → {msg} (спека §6.2; ручная приёмка): принять →
// final_accepted/ships/race_<slug>_NN.png + ships_meta.json (раса, угол,
// отражение, seed, промпт, дата); удалить → ships_rejected/; rot90/rot180/
// flipH/rotate (произвольный угол по часовой)/fit («вписать в кадр») —
// перезапись кандидата на месте с ре-нормализацией 200×200 (ships_edit.go),
// накопленный угол/зеркало пишутся в meta.json пула.
func (s *Server) handleShipsAct(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	file := filepath.Base(q.Get("file"))
	what := q.Get("what")
	angle := atofDefault(q.Get("angle"), 0)
	pool := filepath.Join(s.cfg.PoolRoot, "ships_pool")
	src := filepath.Join(pool, file)
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
	case "rot90", "rot180", "flipH", "rotate", "fit":
		if _, err := os.Stat(src); err == nil {
			if err := transformShipImage(src, what, angle); err != nil {
				msg = "Ошибка: " + err.Error()
			} else {
				switch what {
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
				default:
					msg = "Вписано в кадр: " + file
				}
			}
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
	os.MkdirAll(acceptDir, 0755)
	dst := filepath.Join(acceptDir, fmt.Sprintf("race_%s_%02d.png", meta.Race, nextShipAcceptNum(acceptDir, meta.Race)))
	if err := copyFile(src, dst); err != nil {
		return "Ошибка: " + err.Error()
	}
	os.Remove(src)
	generator.RemoveShipMeta(pool, file)
	// в ships_meta.json — имя принятого файла (спека §5: file — файл корабля)
	// + ручной трансформ приёмки (угол/отражение из меты пула) и дата.
	meta.File = filepath.Base(dst)
	meta.Date = time.Now().Format("2006-01-02")
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

// --- Режим приёмки кораблей: подсказка носа, живой предпросмотр, счётчик ---

// shipOrient — ответ orient-режима tools/spike_ship_sprite_cut.py
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
// (tools/spike_ship_sprite_cut.py --orient-only --report). Единственный
// источник детекции носа — тот же скрипт, что у конвейера; Go лишь читает
// готовый JSON. Ошибка — Python недоступен/парсинг/таймаут.
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
	cmd := exec.CommandContext(ctx, pythonCmd, "tools/spike_ship_sprite_cut.py", src, "--orient-only", "--report", tmpPath)
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

// handleShipsAuto — GET /ships/auto?file= → {angle, mirror, ambiguous, reason}
// (при ошибке Python — {error}): ПОДСКАЗКА авто-определения носа. angle — в
// градусах ПО ЧАСОВОЙ (конвенция слайдера и /ships/act?what=rotate; конвенцию
// PIL инвертируем здесь), решение всё равно за человеком.
func (s *Server) handleShipsAuto(w http.ResponseWriter, r *http.Request) {
	file := filepath.Base(r.URL.Query().Get("file"))
	src := filepath.Join(s.cfg.PoolRoot, "ships_pool", file)
	info, err := shipOrientHint(s.cfg.PythonCmd, src)
	if err != nil {
		writeJSON(w, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, map[string]interface{}{
		"angle": -info.Angle, "mirror": info.Mirror,
		"ambiguous": info.Ambiguous, "reason": info.Reason,
	})
}

// handleShipsPreview — GET /ships/preview?file=&angle= → PNG: кандидат,
// повёрнутый на angle (по часовой) и вписанный в 200×200 (та же
// нормализация, что у /ships/act?what=rotate). Файл в пуле НЕ меняется —
// это живой предпросмотр слайдера без перезагрузки страницы.
func (s *Server) handleShipsPreview(w http.ResponseWriter, r *http.Request) {
	file := filepath.Base(r.URL.Query().Get("file"))
	angle := atofDefault(r.URL.Query().Get("angle"), 0)
	src := filepath.Join(s.cfg.PoolRoot, "ships_pool", file)
	img, err := openImage(src)
	if err != nil {
		http.NotFound(w, nil)
		return
	}
	out, err := transformShip(img, "rotate", angle)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writePNG(w, out)
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