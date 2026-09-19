package generator

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"zorion/cmd/art-studio/comfy"
	"zorion/cmd/art-studio/config"
	"zorion/cmd/art-studio/postproc"
)

// GenVar — вариации расы от эталона (img2img, спека 67a.1 §6.1 /genvar).
// Ошибка, если эталона нет. Возвращает сообщение для UI.
// cnStrength — сила формы (ControlNet strength), cnEnd — до какого шага ControlNet
// активен (end_percent). Обе ручки «близость к эталону» (решение создателя 2026-09-17).
// size — 512 (быстро, дефолт) или 1024. palette — отклонение палитры 0–100.
// personage — эксперимент 82a: субъект «a personage» вместо «an abstract structure».
// tags — дополнительные теги с фронта (98b), передаются в BuildPrompt.
// promptOverride — хирургическая правка (98b-дополнение 2): непустая строка
// используется как промпт для ВСЕХ N вариаций (BuildPrompt не вызывается);
// пустая — авто-промпт на каждый i (разнообразие).
func (r *Runner) GenVar(famID, raceID string, n int, denoise, cnStrength, cnEnd float64, size, palette int, personage bool, tags string, promptOverride string) (string, Status) {
	fam, ok := r.family(famID)
	if !ok {
		return "нет семейства " + famID, Status{}
	}
	raceIdx := -1
	for i, rc := range fam.Races {
		if rc.ID == raceID {
			raceIdx = i
			break
		}
	}
	if raceIdx < 0 {
		return "нет расы " + raceID + " в " + famID, Status{}
	}
	pool := r.PoolPath("races_pool")
	ref := filepath.Join(pool, fmt.Sprintf("ref_%s_r%s.png", famID, raceID))
	if _, err := os.Stat(ref); err != nil {
		st := Status{Running: false, Done: 0, Total: 0, Current: "НЕТ эталона расы " + raceID}
		WriteStatus(pool, st)
		return "НЕТ эталона расы " + raceID + " — сначала сделай эталон", st
	}
	started, st := r.TryStart(func(ctx *JobCtx) {
		ctx.genVarJob(famID, fam, raceIdx, n, ref, denoise, cnStrength, cnEnd, size, palette, personage, tags, promptOverride)
	})
	if !started {
		return fmt.Sprintf("Уже идёт генерация: %d/%d", st.Done, st.Total), st
	}
	return fmt.Sprintf("Вариации от эталона расы: %d шт (denoise %.2f)", n, denoise), Status{}
}

func (c *JobCtx) genVarJob(famID string, fam config.Family, raceIdx, n int, ref string, denoise, cnStrength, cnEnd float64, size, palette int, personage bool, tags string, promptOverride string) {
	pool := c.PoolPath("races_pool")
	// НЕ чистим пул: новая генерация дописывает к существующим вариантам
	// (решение создателя 2026-09-17). Очистка — отдельной кнопкой «Очистить результаты».
	race := fam.Races[raceIdx]
	c.WriteStatus("races_pool", Status{Running: true, Done: 0, Total: n, Current: "вариации расы " + race.Name + "..."})
	refName := fmt.Sprintf("ref_%s_r%s.png", famID, race.ID)
	// копия эталона в ComfyUI/input (спека 67a.1 §5.1)
	if err := copyFile(ref, filepath.Join(c.r.cfg.ComfyInput, refName)); err != nil {
		c.WriteStatus("races_pool", Status{Running: false, Done: 0, Total: n, Current: "ошибка копирования эталона: " + err.Error()})
		return
	}
	var metaMu sync.Mutex
	var numMu sync.Mutex
	nextNum := nextRaceNum(pool) // следующий свободный rNN.png (параллельность — через numMu)
	done := c.runParallel("races_pool", n, func(i int) bool {
		// локальный rand на вызов (AGENTS.md §0: общий *rand.Rand не потокобезопасен)
		rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(i)))
		seed := rng.Intn(999999999) + 1
		// override (98b-дополнение 2): фиксированная строка на все N; иначе —
		// авто-промпт на каждый i (разнообразие). seed остаётся разным — разброс
		// картинок у SDXL сохраняется.
		var prompt, rid, rname string
		if promptOverride != "" {
			prompt = promptOverride
			rid, rname = race.ID, race.Name
		} else {
			prompt, rid, rname = BuildPrompt(rng, fam, raceIdx, famID, c.r.forms, palette, personage, tags)
		}
		raw := filepath.Join(pool, fmt.Sprintf("_raw_%02d.png", i+1))
		numMu.Lock()
		nn := nextNum
		nextNum++
		numMu.Unlock()
		out := filepath.Join(pool, fmt.Sprintf("r%02d.png", nn))
		wf := comfy.Img2ImgWorkflow(c.r.cfg.Checkpoint, prompt, LightNeg(), refName, seed, c.r.cfg.Steps, c.r.cfg.CfgImg, denoise, cnStrength, cnEnd, size, "race_pool")
		pid, err := c.r.comfy.Submit(wf)
		if err != nil {
			return false
		}
		ok, err := c.r.comfy.WaitAndDownload(pid, raw)
		if err != nil || !ok {
			os.Remove(raw)
			return false
		}
		if err := postproc.RemoveBG(c.r.cfg.PythonCmd, c.r.cfg.RembgCLI, raw, out); err != nil {
			os.Remove(raw)
			return false
		}
		// полупрозрачный низ после rembg просвечивал фон (бледный низ) —
		// силуэт делаем полностью непрозрачным (решение создателя 2026-09-17)
		if err := postproc.NormalizeAlphaFile(out, 40); err != nil {
			os.Remove(raw)
			return false
		}
		os.Remove(raw)
		metaMu.Lock()
		appendPoolMeta(filepath.Join(pool, "meta.json"), MetaItem{
			File: filepath.Base(out), Seed: int64(seed), Family: famID, Race: rname, RaceID: rid, Prompt: prompt, Ref: true, Size: size,
		})
		metaMu.Unlock()
		return true
	}, func(d int) {
		c.WriteStatus("races_pool", Status{Running: true, Done: d, Total: n, Current: "вариации расы " + race.Name})
	})
	c.RemoveStopFlag("races_pool")
	cur := "готово"
	if done < n {
		cur = "остановлено"
	}
	c.WriteStatus("races_pool", Status{Running: false, Done: done, Total: n, Current: cur})
}

// clearRacePool удаляет r*.png и meta.json пула (ref_* и ref_cands/ не трогает).
func clearRacePool(pool string) {
	entries, err := os.ReadDir(pool)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "r") && !strings.HasPrefix(name, "ref_") && strings.HasSuffix(name, ".png") {
			os.Remove(filepath.Join(pool, name))
		}
		if name == "meta.json" {
			os.Remove(filepath.Join(pool, name))
		}
	}
}

// nextRaceNum — следующий свободный номер rNN.png в пуле (max существующих + 1).
func nextRaceNum(pool string) int {
	max := 0
	entries, err := os.ReadDir(pool)
	if err != nil {
		return 1
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "r") && strings.HasSuffix(name, ".png") && !strings.HasPrefix(name, "ref_") {
			var n int
			if _, err := fmt.Sscanf(name, "r%d.png", &n); err == nil && n > max {
				max = n
			}
		}
	}
	return max + 1
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}