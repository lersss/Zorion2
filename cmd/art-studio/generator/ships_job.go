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
	"zorion/cmd/art-studio/postproc"
)

// GenShips — генерация n кандидатов корабля расы (спека §6.2 /ships/gen).
// tags — доп. теги (98b, в конец обоих авто-промптов); prompt1Override/
// prompt2Override — ручные строки на все N (98b-дополнение 2: непустая —
// используется как есть, пустая — авто-промпт); size — финальный размер
// кандидата (200 — полный, 100 — эскиз: быстрее и легче, ловит форму/стиль,
// 98c).
// Ошибка, если конфиг кораблей не подключён или расы нет в ships.json.
func (r *Runner) GenShips(race string, n int, tags, prompt1Override, prompt2Override string, size int) (string, Status) {
	ships, dict := r.shipsSnapshot()
	if ships == nil || dict == nil {
		return "конфиг кораблей не подключён", Status{}
	}
	if _, ok := ships[race]; !ok {
		return "нет расы " + race + " в ships.json", Status{}
	}
	started, st := r.TryStart(func(ctx *JobCtx) {
		ctx.genShipsJob([]string{race}, n, tags, prompt1Override, prompt2Override, size)
	})
	if !started {
		return fmt.Sprintf("Уже идёт генерация: %d/%d", st.Done, st.Total), st
	}
	return fmt.Sprintf("Корабли расы %s: %d шт", race, n), Status{}
}

// GenShipsBatch — пачка: per кандидатов на каждую расу (спека §6.2
// /ships/genbatch; 10 рас × 3 = 30 задач, 2 воркера). Параметры — как
// GenShips (tags/override на все расы пачки, size — эскиз/полный).
func (r *Runner) GenShipsBatch(races []string, per int, tags, p1o, p2o string, size int) (string, Status) {
	ships, dict := r.shipsSnapshot()
	if ships == nil || dict == nil {
		return "конфиг кораблей не подключён", Status{}
	}
	for _, race := range races {
		if _, ok := ships[race]; !ok {
			return "нет расы " + race + " в ships.json", Status{}
		}
	}
	started, st := r.TryStart(func(ctx *JobCtx) {
		ctx.genShipsJob(races, per, tags, p1o, p2o, size)
	})
	if !started {
		return fmt.Sprintf("Уже идёт генерация: %d/%d", st.Done, st.Total), st
	}
	return fmt.Sprintf("Пачка кораблей: %d рас × %d", len(races), per), Status{}
}

// genShipsJob — конвейер на кандидата (спека §6.3): (1) силуэт — если
// silhouettes/<slug>.png нет, вызов tools/make_ship_silhouettes.py
// (--spec=<JSON> --out=<path>; парсер ТЗ живёт в Go — TDD-требование);
// (2) копия силуэта в ComfyUI/input (ship_sil_<slug>.png); (3) ShipStage1Workflow
// (промпт1) → raw1; (4) ShipStage2Workflow (промпт2) → raw2; (5) process_ship.py
// raw2 → sNN.png; (6) мета в meta.json. Мягкий СТОП, 2 воркера, локальный
// rand.New на вызов (AGENTS.md §0). tags/override — 98b (ручная строка на
// все N); size < 200 — эскиз: меньше steps этапов + латент этапов 512
// (ImageScale в воркфлоу, ~4x быстрее) + финальный размер canvas×canvas (98c).
func (c *JobCtx) genShipsJob(races []string, per int, tags, p1o, p2o string, size int) {
	pool := c.PoolPath("ships_pool")
	os.MkdirAll(pool, 0755)
	os.MkdirAll(filepath.Join(pool, "silhouettes"), 0755)
	// чистка пула перед стартом (кроме silhouettes/ — кэш силуэтов, спека §5)
	clearShipsPool(pool)
	total := len(races) * per
	c.WriteStatus("ships_pool", Status{Running: true, Done: 0, Total: total, Current: "старт..."})
	ships, dict := c.r.shipsSnapshot()
	// эскиз (size < 200): меньше steps этапов — заметно быстрее полного (98c)
	steps := c.r.cfg.Steps
	if size < 200 {
		steps = steps / 2
		if steps < 10 {
			steps = 10
		}
	}
	// латент этапов: эскиз — 512 (ImageScale в воркфлоу, ~4x быстрее),
	// полный — 1024 (как раньше)
	wfSize := 1024
	if size < 200 {
		wfSize = 512
	}
	var metaMu sync.Mutex
	var numMu sync.Mutex
	nextNum := nextShipNum(pool)
	done := c.runParallel("ships_pool", total, func(i int) bool {
		race := races[i/per]
		entry, ok := ships[race]
		if !ok {
			return false
		}
		// локальный rand на вызов (AGENTS.md §0)
		rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(i)))
		seed := rng.Intn(999999999) + 1
		spec := ParseSilhouette(entry.Silhouette, dict)
		// override (98b-дополнение 2): ручная строка на все N; иначе —
		// авто-промпт с tags (98b)
		prompt1 := p1o
		if prompt1 == "" {
			prompt1 = BuildShipPrompt1(rng, race, entry, spec, dict, tags)
		}
		prompt2 := p2o
		if prompt2 == "" {
			prompt2 = BuildShipPrompt2(rng, entry, dict, tags)
		}
		// (1) силуэт (кэш silhouettes/<slug>.png)
		silPath := filepath.Join(pool, "silhouettes", race+".png")
		if _, err := os.Stat(silPath); err != nil {
			if err := c.makeSilhouette(race, spec, silPath); err != nil {
				return false
			}
		}
		// (2) копия силуэта в ComfyUI/input
		silName := "ship_sil_" + race + ".png"
		if err := copyFile(silPath, filepath.Join(c.r.cfg.ComfyInput, silName)); err != nil {
			return false
		}
		// (3) этап 1: форма (ControlNet Canny + img2img)
		raw1 := filepath.Join(pool, fmt.Sprintf("_raw_%02d_1.png", i+1))
		wf1 := comfy.ShipStage1Workflow(c.r.cfg.Checkpoint, prompt1, LightNeg(), silName, seed, steps, c.r.cfg.Cfg, c.r.cfg.CNStrength, wfSize, "ship_pool")
		pid, err := c.r.comfy.Submit(wf1)
		if err != nil {
			return false
		}
		ok1, err := c.r.comfy.WaitAndDownload(pid, raw1)
		if err != nil || !ok1 {
			os.Remove(raw1)
			return false
		}
		// (4) этап 2: текстура (img2img от raw1)
		raw1Name := filepath.Base(raw1)
		if err := copyFile(raw1, filepath.Join(c.r.cfg.ComfyInput, raw1Name)); err != nil {
			os.Remove(raw1)
			return false
		}
		raw2 := filepath.Join(pool, fmt.Sprintf("_raw_%02d_2.png", i+1))
		wf2 := comfy.ShipStage2Workflow(c.r.cfg.Checkpoint, prompt2, LightNeg(), raw1Name, seed, steps, c.r.cfg.CfgImg, 0.5, wfSize, "ship_pool")
		pid2, err := c.r.comfy.Submit(wf2)
		if err != nil {
			os.Remove(raw1)
			return false
		}
		ok2, err := c.r.comfy.WaitAndDownload(pid2, raw2)
		if err != nil || !ok2 {
			os.Remove(raw1)
			os.Remove(raw2)
			return false
		}
		// (5) пост-обработка process_ship.py → sNN.png (canvas — эскиз/полный)
		numMu.Lock()
		nn := nextNum
		nextNum++
		numMu.Unlock()
		out := filepath.Join(pool, fmt.Sprintf("s%02d.png", nn))
		if err := postproc.ProcessShip(c.r.cfg.PythonCmd, raw2, out, size); err != nil {
			os.Remove(raw1)
			os.Remove(raw2)
			return false
		}
		os.Remove(raw1)
		os.Remove(raw2)
		// (5а) метки авто-фильтра (страховка, не авто-отклонение; диагноз
		// визуального аудита, п.7) — показываются на превью
		labels := ShipCandidateLabelsFile(out, specColdHull(spec))
		// (6) мета
		metaMu.Lock()
		appendShipMeta(filepath.Join(pool, "meta.json"), ShipMetaItem{
			File: filepath.Base(out), Race: race, RaceName: entry.RaceName, Seed: int64(seed),
			Texture: entry.Texture, Prompt1: prompt1, Prompt2: prompt2, Labels: labels, Size: size,
		})
		metaMu.Unlock()
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
// выход — silhouettes/<slug>.png 1024×1024.
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

// clearShipsPool удаляет s*.png и meta.json пула кораблей (silhouettes/ —
// кэш силуэтов — не трогаем, спека §5).
func clearShipsPool(pool string) {
	entries, err := os.ReadDir(pool)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "s") && strings.HasSuffix(name, ".png") {
			os.Remove(filepath.Join(pool, name))
		}
		if name == "meta.json" {
			os.Remove(filepath.Join(pool, name))
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