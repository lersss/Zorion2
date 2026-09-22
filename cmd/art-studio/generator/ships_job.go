package generator

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"zorion/cmd/art-studio/comfy"
	"zorion/cmd/art-studio/config"
	"zorion/cmd/art-studio/postproc"
)

// Параметры автопроверки кадра (рецепт 2026-09-21): корабль не ближе 8 px к
// краю И вытянутость силуэта λ1/λ2 ≥ 1.3 (3/4-вид; фронтальный ≈ 1.0).
const (
	shipFrameTries  = 6
	shipFrameMargin = 8
	shipMinElong    = 1.3
)

// shipTask — задача генерации: раса + выбранный тип корабля (entry — плоская
// запись с наследованными texture/silhouette/blocked).
type shipTask struct {
	race  string
	typ   string // имя типа корабля; пусто — легаси-раса без типов
	entry config.ShipEntry
}

// buildShipTasks — список задач «раса × тип». typeName задан — только этот тип;
// пусто: allTypes — все типы расы (batch), иначе первый тип (у легаси-расы без
// types — единственный безымянный). Ошибка, если расы/типа нет в ships.json.
func buildShipTasks(ships config.ShipsConfig, races []string, typeName string, allTypes bool) ([]shipTask, error) {
	var tasks []shipTask
	for _, race := range races {
		e, ok := ships[race]
		if !ok {
			return nil, fmt.Errorf("нет расы %s в ships.json", race)
		}
		if typeName != "" {
			t, ok := e.ResolveShipType(typeName)
			if !ok {
				return nil, fmt.Errorf("нет типа %s у расы %s", typeName, race)
			}
			tasks = append(tasks, shipTask{race: race, typ: t.Type, entry: e.ForType(t)})
			continue
		}
		if allTypes {
			for _, t := range e.ShipTypes() {
				tasks = append(tasks, shipTask{race: race, typ: t.Type, entry: e.ForType(t)})
			}
			continue
		}
		t, ok := e.ResolveShipType("")
		if !ok {
			return nil, fmt.Errorf("нет типа у расы %s", race)
		}
		tasks = append(tasks, shipTask{race: race, typ: t.Type, entry: e.ForType(t)})
	}
	return tasks, nil
}

// GenShips — генерация n кандидатов корабля расы (рецепт 2026-09-21,
// /ships/gen). typeName — тип корабля (starship/…); пусто — первый тип
// (у легаси-расы без types — единственный безымянный). tags — доп. теги (в конец
// промпта); prompt1Override — ручной промпт txt2img на все N (пусто —
// авто-промпт); prompt2Override — ручной промпт Hi-Res (пусто — авто: промпт
// txt2img + хвост детализации); hires — включить этап детализации; size —
// финальный размер кандидата (200 — полный, 100 — эскиз: латент 512 и меньший
// steps, быстрее); keep — не чистить пул перед стартом (накопительный прогон).
// Ошибка, если конфиг кораблей не подключён, расы/типа нет в ships.json.
func (r *Runner) GenShips(race, typeName string, n int, tags, prompt1Override, prompt2Override string, hires bool, size int, keep bool) (string, Status) {
	ships, dict := r.shipsSnapshot()
	if ships == nil || dict == nil {
		return "конфиг кораблей не подключён", Status{}
	}
	tasks, err := buildShipTasks(ships, []string{race}, typeName, false)
	if err != nil {
		return err.Error(), Status{}
	}
	started, st := r.TryStart(func(ctx *JobCtx) {
		ctx.genShipsJob(tasks, n, tags, prompt1Override, prompt2Override, hires, size, keep)
	})
	if !started {
		return fmt.Sprintf("Уже идёт генерация: %d/%d", st.Done, st.Total), st
	}
	return fmt.Sprintf("Корабли расы %s: %d шт", race, n), Status{}
}

// GenShipsBatch — пачка: per кандидатов на каждый тип каждой расы
// (/ships/genbatch; 10 рас × 3 = 30 задач, 2 воркера). Типы расы разворачиваются
// (люди ×4); у легаси-расы без types — один безымянный тип. Параметры — как
// GenShips (keep — накопительный прогон).
func (r *Runner) GenShipsBatch(races []string, per int, tags, p1o, p2o string, hires bool, size int, keep bool) (string, Status) {
	ships, dict := r.shipsSnapshot()
	if ships == nil || dict == nil {
		return "конфиг кораблей не подключён", Status{}
	}
	tasks, err := buildShipTasks(ships, races, "", true)
	if err != nil {
		return err.Error(), Status{}
	}
	started, st := r.TryStart(func(ctx *JobCtx) {
		ctx.genShipsJob(tasks, per, tags, p1o, p2o, hires, size, keep)
	})
	if !started {
		return fmt.Sprintf("Уже идёт генерация: %d/%d", st.Done, st.Total), st
	}
	return fmt.Sprintf("Пачка кораблей: %d рас × %d = %d", len(races), per, len(tasks)*per), Status{}
}

// genShipsJob — конвейер на кандидата (рецепт 2026-09-21): (1) txt2img
// (Juggernaut XL, модель из ships.model; ShipTxt2ImgWorkflow) → сырой кадр;
// (2) автопроверка кадра (tools/ship_sprite_cut.py --frame-check): корабль
// касается края или силуэт не вытянут → следующий seed (≤ shipFrameTries,
// все негодны — берётся последний кадр); (3) Hi-Res (ShipHiResWorkflow) — по
// запросу hires; (4) вырез/нормализация (tools/ship_sprite_cut.py, hyst
// 12/40 + fill_holes + --no-orient: без пиксельного доворота) → sNN.png;
// (5) метки авто-фильтра + мета (промпты, статистика попыток, тип корабля) +
// начальная пара (A, F) из подсказки авто-носа. Мягкий СТОП, 2 воркера,
// локальный rand.New на вызов (AGENTS.md §0). keep — не чистить пул перед
// стартом: новые кандидаты добавляются к уже сгенерированным (накопительный
// прогон «пилот → остальные»).
func (c *JobCtx) genShipsJob(tasks []shipTask, per int, tags, p1o, p2o string, hires bool, size int, keep bool) {
	pool := c.PoolPath("ships_pool")
	os.MkdirAll(pool, 0755)
	if !keep {
		// чистка пула перед стартом (sNN.png + meta.json + сырые кадры)
		clearShipsPool(pool)
	}
	total := len(tasks) * per
	c.WriteStatus("ships_pool", Status{Running: true, Done: 0, Total: total, Current: "старт..."})
	_, dict := c.r.shipsSnapshot()
	sp := c.r.shipParams()
	// эскиз (size < 200): латент 512 и вдвое меньше steps — заметно быстрее
	steps := sp.Steps
	wfSize := 1024
	if size < 200 {
		steps = steps / 2
		if steps < 10 {
			steps = 10
		}
		wfSize = 512
	}
	var numMu sync.Mutex
	nextNum := nextShipNum(pool)
	done := c.runParallel("ships_pool", total, func(i int) bool {
		task := tasks[i/per]
		race := task.race
		entry := task.entry
		// локальный rand на вызов (AGENTS.md §0)
		rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(i)))
		prompt1 := p1o
		if prompt1 == "" {
			prompt1 = BuildShipTxt2ImgPrompt(rng, entry, tags)
		}
		prompt2 := p2o
		if prompt2 == "" {
			prompt2 = BuildShipHiResPrompt(prompt1)
		}
		neg := ShipNegRace(entry)
		seed := int64(rng.Intn(999999999) + 1)
		// (1) txt2img + (2) автопроверка кадра: негодный кадр → следующий seed
		raw := ""
		attempts, rejected := 0, 0
		fc := postproc.ShipFrame{}
		for attempt := 0; attempt < shipFrameTries; attempt++ {
			p := filepath.Join(pool, fmt.Sprintf("_raw_%02d_%d.png", i+1, attempt))
			wf := comfy.ShipTxt2ImgWorkflow(sp.Model, prompt1, neg, int(seed)+attempt, steps, sp.Cfg, wfSize, "ship_pool")
			pid, err := c.r.comfy.Submit(wf)
			if err != nil {
				return false
			}
			okDl, err := c.r.comfy.WaitAndDownload(pid, p)
			if err != nil || !okDl {
				os.Remove(p)
				return false
			}
			attempts++
			frame, err := postproc.ShipFrameCheck(c.r.cfg.PythonCmd, p, shipFrameMargin, shipMinElong)
			if err != nil {
				os.Remove(p)
				return false
			}
			fc = frame
			raw = p
			if frame.OK {
				break
			}
			rejected++
			if attempt == shipFrameTries-1 {
				break // все попытки негодны — берём последний кадр
			}
			os.Remove(p)
		}
		// (3) Hi-Res (этап детализации) — по запросу (рецепт: на финалистах)
		if hires && raw != "" {
			hrName := "ship_hr_" + filepath.Base(raw)
			if err := copyFile(raw, filepath.Join(c.r.cfg.ComfyInput, hrName)); err == nil {
				hr := filepath.Join(pool, fmt.Sprintf("_hr_%02d.png", i+1))
				wf := comfy.ShipHiResWorkflow(sp.Model, prompt2, neg, hrName, int(seed)+attempts-1,
					sp.Hires.Steps, sp.Hires.Cfg, sp.Hires.Denoise, sp.Hires.Upscaler, sp.Hires.Scale, "ship_pool")
				if pid, err := c.r.comfy.Submit(wf); err == nil {
					if okHr, err := c.r.comfy.WaitAndDownload(pid, hr); err == nil && okHr {
						os.Remove(raw)
						raw = hr
					} else {
						os.Remove(hr)
					}
				}
			}
		}
		// (4) вырез и нормализация → sNN.png
		numMu.Lock()
		nn := nextNum
		nextNum++
		numMu.Unlock()
		out := filepath.Join(pool, fmt.Sprintf("s%02d.png", nn))
		orient, err := postproc.ShipSpriteCut(c.r.cfg.PythonCmd, raw, out, size)
		if err != nil {
			os.Remove(raw)
			return false
		}
		os.Remove(raw)
		// (5) метки авто-фильтра (страховка, не авто-отклонение): раса без
		// тёплых слов в texture — «холодная» для метки «палитра»
		cold := dict != nil && !hasWarmMarker(entry.Texture, dict.WarmMarkers)
		labels := ShipCandidateLabelsFile(out, cold)
		item := ShipMetaItem{
			File: filepath.Base(out), Race: race, Type: task.typ, RaceName: entry.RaceName,
			Seed: seed + int64(attempts-1), Texture: entry.Texture,
			Prompt1: prompt1, Prompt2: prompt2, Labels: labels, Size: size,
			Frame: &ShipFrameStat{Attempts: attempts, Rejected: rejected, Touch: fc.Touch, Elong: fc.Elong},
		}
		if orient.Reason != "" {
			item.Orient = &orient
			// начальная пара (A, F) по подсказке авто-носа (спека §3.3): пиксели
			// выреза не довёрнуты (--no-orient), «правильная» ориентация —
			// метаданные. Пишем её сразу в item — одной записью под общим
			// shipMetaMu (meta.json пула), без окна «дописали, потом задали пару».
			a, f := ShipHintPair(orient.Angle, orient.Mirror, orient.Ambiguous)
			item.Angle = a
			item.Flip = f
		}
		appendShipMeta(filepath.Join(pool, "meta.json"), item)
		return true
	}, func(d int) {
		c.WriteStatus("ships_pool", Status{Running: true, Done: d, Total: total, Current: "корабли рас..."})
	})
	c.RemoveStopFlag("ships_pool")
	cur := "готово"
	if done < total {
		cur = "остановлено"
	}
	c.WriteStatus("ships_pool", Status{Running: false, Done: done, Total: total, Current: cur})
}

// makeSilhouette вызывает tools/make_ship_silhouettes.py (Python-подпроцесс,
// паттерн rembg, спека §3.1): вход — разобранный spec-JSON (парсер в Go),
// выход — silhouettes/<slug>.png 1024×1024. Прежний конвейер «силуэт →
// ControlNet» выведен из джоба (рецепт 2026-09-21) — функция оставлена до
// отдельного решения об уборке (спека 2026-09-21 §12).
func (c *JobCtx) makeSilhouette(race string, spec SilhouetteSpec, outPath string) error {
	specJSON, err := SilhouetteSpecJSON(spec)
	if err != nil {
		return err
	}
	tmp := filepath.Join(os.TempDir(), "ship_spec_"+race+".json")
	if err := os.WriteFile(tmp, specJSON, 0644); err != nil {
		return err
	}
	cmd := exec.Command(c.r.cfg.PythonCmd, "tools/make_ship_silhouettes.py", "--spec="+tmp, "--out="+outPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("make_ship_silhouettes: %v: %s", err, string(out))
	}
	return nil
}

// clearShipsPool удаляет кандидатов (s*.png), meta.json и сырые кадры
// (_raw_*, _hr_*) пула кораблей. silhouettes/ (кэш прежнего конвейера) не
// трогаем — уборка отдельным решением.
func clearShipsPool(pool string) {
	entries, err := os.ReadDir(pool)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "s") && strings.HasSuffix(name, ".png") {
			os.Remove(filepath.Join(pool, name))
			continue
		}
		if strings.HasPrefix(name, "_raw_") || strings.HasPrefix(name, "_hr_") {
			os.Remove(filepath.Join(pool, name))
			continue
		}
		if name == "meta.json" {
			// удаление меты пула — под общим замком (иначе параллельный
			// HTTP-хендлер студии может писать в только что удалённый файл)
			shipMetaMu.Lock()
			os.Remove(filepath.Join(pool, name))
			shipMetaMu.Unlock()
		}
	}
}

// nextShipNum — следующий свободный номер sNN.png в пуле (max + 1).
func nextShipNum(pool string) int {
	max := 0
	entries, err := os.ReadDir(pool)
	if err != nil {
		return 1
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "s") && strings.HasSuffix(name, ".png") {
			var n int
			if _, err := fmt.Sscanf(name, "s%d.png", &n); err == nil && n > max {
				max = n
			}
		}
	}
	return max + 1
}
