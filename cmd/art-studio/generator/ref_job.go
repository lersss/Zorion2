package generator

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"zorion/cmd/art-studio/comfy"
	"zorion/cmd/art-studio/config"
	"zorion/cmd/art-studio/postproc"
)

// GenRef — кандидаты эталона (широкий поиск по всему семейству, спека 67a.1
// §6.1 /genref). Очищает ref_cands/ перед стартом (разовая пачка).
// race — выбранная раса (идея 2026-09-20): непустая — кандидаты от ЭТОЙ расы
// (фиксированный raceIdx в BuildPromptWide, мета RaceID = выбранная раса);
// пустая — текущее поведение: случайная раса на каждого кандидата (raceIdx=-1).
// size — 512 (быстро, дефолт) или 1024. morph — морф генерации
// ("" = не-антропо, иначе один из 7 морфов). personage — эксперимент 82a:
// субъект «a personage» вместо «an abstract object» в не-антропо ветке.
// tags — дополнительные теги с фронта (98b), передаются в BuildPromptWide.
// promptOverride — хирургическая правка (98b-дополнение 2): непустая строка
// используется как промпт для ВСЕХ N кандидатов (BuildPromptWide не
// вызывается); пустая — авто-промпт на каждый i (разнообразие).
func (r *Runner) GenRef(famID, race string, n int, morph string, size int, personage bool, tags string, promptOverride string) (string, Status) {
	fam, ok := r.family(famID)
	if !ok {
		return "нет семейства " + famID, Status{}
	}
	// поиск raceIdx по id (как GenVar в race_job.go); пустая race — raceIdx=-1
	raceIdx := -1
	if race != "" {
		for i, rc := range fam.Races {
			if rc.ID == race {
				raceIdx = i
				break
			}
		}
		if raceIdx < 0 {
			return "нет расы " + race + " в " + famID, Status{}
		}
	}
	started, st := r.TryStart(func(ctx *JobCtx) {
		ctx.genRefJob(famID, fam, raceIdx, n, morph, size, personage, tags, promptOverride)
	})
	if !started {
		return fmt.Sprintf("Уже идёт генерация: %d/%d", st.Done, st.Total), st
	}
	suffix := ""
	if morph != "" {
		suffix = ", морф " + morph
	}
	if raceIdx >= 0 {
		return fmt.Sprintf("Генерация %d кандидатов (раса %s%s)...", n, fam.Races[raceIdx].Name, suffix), Status{}
	}
	return fmt.Sprintf("Генерация %d кандидатов (все расы %s%s)...", n, famID, suffix), Status{}
}

func (c *JobCtx) genRefJob(famID string, fam config.Family, raceIdx, n int, morph string, size int, personage bool, tags string, promptOverride string) {
	pool := c.PoolPath("races_pool")
	refdir := filepath.Join(pool, "ref_cands")
	os.MkdirAll(refdir, 0755)
	clearDir(refdir)
	c.WriteStatus("races_pool", Status{Running: true, Done: 0, Total: n, Current: "кандидаты эталона (широкий поиск)..."})
	var metaMu sync.Mutex
	candMeta := map[string]CandMeta{}
	// runWithRetry — ретрай работы кандидата: Windows Defender при сканировании
	// делает свежесозданные файл/папку невидимыми для других операций на
	// 200–400+ мс — work(i) спорадически возвращает false (RemoveBG «не найден
	// файл», WaitAndDownload «не найден путь»), теряя кандидата (флак
	// TestHandleGenRefRace 2026-09-20; прод-живучесть генераций на Windows).
	// До 5 попыток с паузой 500 мс перед отказом. Побочные эффекты попыток не
	// конфликтуют: refdir очищен один раз до runParallel (clearDir); cNN.png
	// пишется только в конце УСПЕШНОЙ попытки (после NormalizeAlphaFile) — при
	// неудаче его нет; raw перезаписывается (WaitAndDownload пишет заново по
	// тому же пути, os.Create перезапишет), а перед каждой неудачей raw
	// удаляется (os.Remove) — остатки не мешают повторной попытке. seed/prompt
	// генерируются ЗАНОВО на каждую попытку (rng пере-инициализируется) —
	// промпт/сид не дублируются между попытками.
	runWithRetry := func(i int) bool {
		for attempt := 0; attempt < 5; attempt++ {
			if attempt > 0 {
				time.Sleep(500 * time.Millisecond)
			}
			// локальный rand на вызов (AGENTS.md §0: общий *rand.Rand не потокобезопасен)
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(i)))
			seed := rng.Intn(999999999) + 1
			// override (98b-дополнение 2): фиксированная строка на все N; иначе —
			// авто-промпт на каждый i (разнообразие). Мета: выбранная раса
			// (raceIdx >= 0) или случайная (raceIdx = -1, как BuildPromptWide).
			var prompt, rid, rname string
			if promptOverride != "" {
				prompt = promptOverride
				if raceIdx >= 0 {
					rid, rname = fam.Races[raceIdx].ID, fam.Races[raceIdx].Name
				} else {
					ri := rng.Intn(len(fam.Races))
					rid, rname = fam.Races[ri].ID, fam.Races[ri].Name
				}
			} else {
				prompt, rid, rname = BuildPromptWide(rng, fam, raceIdx, famID, morph, c.r.forms, personage, tags)
			}
			raw := filepath.Join(pool, fmt.Sprintf("_raw_ref_%02d.png", i+1))
			out := filepath.Join(refdir, fmt.Sprintf("c%02d.png", i+1))
			wf := comfy.Txt2ImgWorkflow(c.r.checkpoint(), prompt, NegFor(fam, morph), seed, c.r.cfg.Steps, c.r.cfg.Cfg, size, "race_pool")
			pid, err := c.r.comfy.Submit(wf)
			if err != nil {
				continue
			}
			ok, err := c.r.comfy.WaitAndDownload(pid, raw)
			if err != nil || !ok {
				os.Remove(raw)
				continue
			}
			if err := postproc.RemoveBG(c.r.cfg.PythonCmd, c.r.cfg.RembgCLI, raw, out); err != nil {
				os.Remove(raw)
				continue
			}
			// полупрозрачный низ после rembg просвечивал фон (бледный низ) —
			// силуэт делаем полностью непрозрачным (решение создателя 2026-09-17)
			if err := postproc.NormalizeAlphaFile(out, 40); err != nil {
				os.Remove(raw)
				continue
			}
			os.Remove(raw)
			metaMu.Lock()
			candMeta[fmt.Sprintf("c%02d.png", i+1)] = CandMeta{RaceID: rid, Race: rname, Seed: int64(seed), Prompt: prompt}
			writeCandMeta(filepath.Join(refdir, "meta.json"), candMeta)
			metaMu.Unlock()
			return true
		}
		return false
	}
	done := c.runParallel("races_pool", n, func(i int) bool {
		return runWithRetry(i)
	}, func(d int) {
		c.WriteStatus("races_pool", Status{Running: true, Done: d, Total: n, Current: "кандидат..."})
	})
	c.RemoveStopFlag("races_pool")
	cur := "готово"
	if done < n {
		cur = "остановлено"
	}
	c.WriteStatus("races_pool", Status{Running: false, Done: done, Total: n, Current: cur})
}

// clearDir удаляет всё содержимое папки.
func clearDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		os.RemoveAll(filepath.Join(dir, e.Name()))
	}
}