package generator

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"zorion/cmd/art-studio/comfy"
	"zorion/cmd/art-studio/postproc"
)

// GenHumans — генерация людей (txt2img по humans.json, спека 67a.1 §6.2
// /humans/gen). Файлы hNN_m.png / hNN_f.png.
func (r *Runner) GenHumans(n int) (string, Status) {
	started, st := r.TryStart(func(ctx *JobCtx) {
		ctx.genHumansJob(n)
	})
	if !started {
		return fmt.Sprintf("Уже идёт генерация: %d/%d", st.Done, st.Total), st
	}
	return fmt.Sprintf("Запущена генерация %d шт (прогресс на панели)", n), Status{}
}

func (c *JobCtx) genHumansJob(n int) {
	pool := c.PoolPath("humans_pool")
	os.MkdirAll(pool, 0755)
	c.WriteStatus("humans_pool", Status{Running: true, Done: 0, Total: n, Current: "старт..."})
	var metaMu sync.Mutex
	done := c.runParallel("humans_pool", n, func(i int) bool {
		// локальный rand на вызов (AGENTS.md §0: общий *rand.Rand не потокобезопасен)
		rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(i)))
		seed := rng.Intn(999999999) + 1
		prompt, gender := BuildHumanPrompt(rng, c.r.humans)
		sex := "m"
		if gender == "woman" {
			sex = "f"
		}
		raw := filepath.Join(pool, fmt.Sprintf("_raw_%02d.png", i+1))
		out := filepath.Join(pool, fmt.Sprintf("h%02d_%s.png", i+1, sex))
		wf := comfy.Txt2ImgWorkflow(c.r.checkpoint(), prompt, c.r.humans.Neg, seed, c.r.humans.Params.Steps, c.r.humans.Params.Cfg, 1024, "human_pool")
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
		appendHumanMeta(filepath.Join(pool, "meta.json"), HumanMetaItem{
			File: filepath.Base(out), Seed: int64(seed), Gender: gender, Prompt: prompt,
		})
		metaMu.Unlock()
		return true
	}, func(d int) {
		c.WriteStatus("humans_pool", Status{Running: true, Done: d, Total: n, Current: "люди..."})
	})
	c.RemoveStopFlag("humans_pool")
	cur := "готово"
	if done < n {
		cur = "остановлено"
	}
	c.WriteStatus("humans_pool", Status{Running: false, Done: done, Total: n, Current: cur})
}