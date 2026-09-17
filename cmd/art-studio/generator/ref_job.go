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
// size — 512 (быстро, дефолт) или 1024. morph — морф генерации
// ("" = не-антропо, иначе один из 7 морфов). personage — эксперимент 82a:
// субъект «a personage» вместо «an abstract object» в не-антропо ветке.
func (r *Runner) GenRef(famID string, n int, morph string, size int, personage bool) (string, Status) {
	fam, ok := r.families[famID]
	if !ok {
		return "нет семейства " + famID, Status{}
	}
	started, st := r.TryStart(func(ctx *JobCtx) {
		ctx.genRefJob(famID, fam, n, morph, size, personage)
	})
	if !started {
		return fmt.Sprintf("Уже идёт генерация: %d/%d", st.Done, st.Total), st
	}
	suffix := ""
	if morph != "" {
		suffix = ", морф " + morph
	}
	return fmt.Sprintf("Генерация %d кандидатов (все расы %s%s)...", n, famID, suffix), Status{}
}

func (c *JobCtx) genRefJob(famID string, fam config.Family, n int, morph string, size int, personage bool) {
	pool := c.PoolPath("races_pool")
	refdir := filepath.Join(pool, "ref_cands")
	os.MkdirAll(refdir, 0755)
	clearDir(refdir)
	c.WriteStatus("races_pool", Status{Running: true, Done: 0, Total: n, Current: "кандидаты эталона (широкий поиск)..."})
	var metaMu sync.Mutex
	candMeta := map[string]CandMeta{}
	done := c.runParallel("races_pool", n, func(i int) bool {
		// локальный rand на вызов (AGENTS.md §0: общий *rand.Rand не потокобезопасен)
		rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(i)))
		seed := rng.Intn(999999999) + 1
		prompt, rid, rname := BuildPromptWide(rng, fam, -1, famID, morph, c.r.forms, personage)
		raw := filepath.Join(pool, fmt.Sprintf("_raw_ref_%02d.png", i+1))
		out := filepath.Join(refdir, fmt.Sprintf("c%02d.png", i+1))
		wf := comfy.Txt2ImgWorkflow(c.r.cfg.Checkpoint, prompt, NegFor(fam, morph), seed, c.r.cfg.Steps, c.r.cfg.Cfg, size, "race_pool")
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
		os.Remove(raw)
		metaMu.Lock()
		candMeta[fmt.Sprintf("c%02d.png", i+1)] = CandMeta{RaceID: rid, Race: rname, Seed: int64(seed), Prompt: prompt}
		writeCandMeta(filepath.Join(refdir, "meta.json"), candMeta)
		metaMu.Unlock()
		return true
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