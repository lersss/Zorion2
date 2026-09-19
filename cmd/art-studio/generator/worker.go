package generator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"zorion/cmd/art-studio/config"
)

// Status — содержимое status.json (спека 67a.1 §9.2).
type Status struct {
	Running bool   `json:"running"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Current string `json:"current"`
}

// MetaItem — запись meta.json пула рас: [{file, seed, family, race, race_id, prompt, ref?, size?}].
type MetaItem struct {
	File   string `json:"file"`
	Seed   int64  `json:"seed"`
	Family string `json:"family"`
	Race   string `json:"race"`
	RaceID string `json:"race_id"`
	Prompt string `json:"prompt"`
	Ref    bool   `json:"ref,omitempty"`
	Size   int    `json:"size,omitempty"`
}

// HumanMetaItem — запись meta.json пула людей: [{file, seed, gender, prompt}].
type HumanMetaItem struct {
	File   string `json:"file"`
	Seed   int64  `json:"seed"`
	Gender string `json:"gender"`
	Prompt string `json:"prompt"`
}

// CandMeta — запись ref_cands/meta.json (ключ — имя файла кандидата).
type CandMeta struct {
	RaceID string `json:"race_id"`
	Race   string `json:"race"`
	Seed   int64  `json:"seed"`
	Prompt string `json:"prompt"`
}

// ComfySubmitter — абстракция ComfyUI для тестов (реализация — comfy.Client).
type ComfySubmitter interface {
	Submit(wf map[string]interface{}) (string, error)
	WaitAndDownload(pid, outPath string) (bool, error)
}

// Runner — владелец активной генерации (одна на процесс, спека 67a.1 §9.2).
type Runner struct {
	mu      sync.Mutex
	running bool

	cfg      *config.StudioConfig
	forms    *config.FormsConfig
	families config.FamiliesConfig
	humans   *config.HumansConfig
	comfy    ComfySubmitter
}

// NewRunner создаёт Runner.
func NewRunner(cfg *config.StudioConfig, forms *config.FormsConfig, families config.FamiliesConfig, humans *config.HumansConfig, comfy ComfySubmitter) *Runner {
	return &Runner{cfg: cfg, forms: forms, families: families, humans: humans, comfy: comfy}
}

// family возвращает семейство по id (чтение под r.mu: ReloadFamilies может
// заменить конфиг в памяти — без мьютекса concurrent map read/write).
func (r *Runner) family(famID string) (config.Family, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.families[famID]
	return f, ok
}

// ReloadFamilies перечитывает families.json с диска и заменяет конфиг в памяти
// (кнопка «Пересобрать промт»: машинная проекция appearance/blocked обновилась).
// Замена под r.mu: джобы читают families только при старте (GenRef/GenVar),
// хендлеры — при запросе; гонки на чтение во время замены нет.
func (r *Runner) ReloadFamilies(path string) error {
	fam, err := config.LoadFamilies(path)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.families = fam
	r.mu.Unlock()
	return nil
}

// TryStart запускает джоб, если ни одна генерация не активна.
// Проверка+действие атомарны (инвариант 67a.1 §11.2); учитывает status.json
// обоих пулов (переживает рестарт студии). Возвращает false + текущий статус,
// если генерация уже идёт.
func (r *Runner) TryStart(job func(ctx *JobCtx)) (bool, Status) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return false, r.readStatusAny()
	}
	if st := r.readStatusAny(); st.Running {
		return false, st
	}
	r.running = true
	go func() {
		defer func() {
			r.mu.Lock()
			r.running = false
			r.mu.Unlock()
		}()
		job(&JobCtx{r: r})
	}()
	return true, Status{}
}

// ReadStatusAny — статус генерации: приоритет running, иначе races_pool
// (спека 67a.1 §6.1: агрегирует оба пула).
func (r *Runner) ReadStatusAny() Status {
	return r.readStatusAny()
}

// AggregateStatus — статус генерации из двух пулов: приоритет running,
// иначе races_pool (спека 67a.1 §6.1 /status).
func AggregateStatus(racesPool, humansPool string) Status {
	races := ReadStatus(racesPool)
	humans := ReadStatus(humansPool)
	if races.Running {
		return races
	}
	if humans.Running {
		return humans
	}
	return races
}

func (r *Runner) readStatusAny() Status {
	races := ReadStatus(filepath.Join(r.cfg.PoolRoot, "races_pool"))
	humans := ReadStatus(filepath.Join(r.cfg.PoolRoot, "humans_pool"))
	if races.Running {
		return races
	}
	if humans.Running {
		return humans
	}
	return races
}

// PoolPath — путь к пулу (races_pool / humans_pool).
func (r *Runner) PoolPath(name string) string {
	return filepath.Join(r.cfg.PoolRoot, name)
}

// JobCtx — контекст одного джоба генерации.
type JobCtx struct {
	r *Runner
}

// PoolPath — путь к пулу (races_pool / humans_pool).
func (c *JobCtx) PoolPath(name string) string {
	return c.r.PoolPath(name)
}

// WriteStatus пишет status.json пула.
func (c *JobCtx) WriteStatus(pool string, s Status) {
	WriteStatus(filepath.Join(c.r.cfg.PoolRoot, pool), s)
}

// StopFlag — существует ли stop.flag в пуле.
func (c *JobCtx) StopFlag(pool string) bool {
	_, err := os.Stat(filepath.Join(c.r.cfg.PoolRoot, pool, "stop.flag"))
	return err == nil
}

// RemoveStopFlag удаляет stop.flag пула.
func (c *JobCtx) RemoveStopFlag(pool string) {
	os.Remove(filepath.Join(c.r.cfg.PoolRoot, pool, "stop.flag"))
}

// runParallel выполняет total задач в workers горутинах с мягким стопом:
// stop.flag проверяется перед взятием задачи, текущие задачи догенерируются
// (спека 67a.1 §5.3). onDone вызывается после каждой задачи (под мьютексом).
// Возвращает число выполненных задач.
func (c *JobCtx) runParallel(pool string, total int, work func(i int) bool, onDone func(done int)) int {
	workers := c.r.cfg.Workers
	if workers < 1 {
		workers = 1
	}
	var mu sync.Mutex
	next := 0
	done := 0
	stopped := false
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				if stopped {
					mu.Unlock()
					return
				}
				if c.StopFlag(pool) {
					stopped = true
					mu.Unlock()
					return
				}
				if next >= total {
					mu.Unlock()
					return
				}
				i := next
				next++
				mu.Unlock()
				work(i)
				mu.Lock()
				done++
				if onDone != nil {
					onDone(done)
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return done
}

// WriteStatus пишет status.json атомарно (tmp + rename, спека 67a.1 §3).
// Windows: os.Rename поверх открытого файла падает «Access is denied» —
// читатель (UI-поллинг /status, тесты) держит файл открытым микросекунды;
// окно короткое, поэтому rename ретраится (иначе status.json застревает
// на старом значении — UI видит «running» вечно).
func WriteStatus(poolDir string, s Status) {
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	tmp := filepath.Join(poolDir, "status.json.tmp")
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return
	}
	dst := filepath.Join(poolDir, "status.json")
	for i := 0; i < 5; i++ {
		if err := os.Rename(tmp, dst); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// ReadStatus читает status.json пула (пустой Status, если файла нет).
func ReadStatus(poolDir string) Status {
	data, err := os.ReadFile(filepath.Join(poolDir, "status.json"))
	if err != nil {
		return Status{}
	}
	var s Status
	if err := json.Unmarshal(data, &s); err != nil {
		return Status{}
	}
	return s
}

// CreateStopFlag создаёт stop.flag в пуле (мягкий стоп, спека 67a.1 §9.2).
func CreateStopFlag(poolDir string) {
	f, err := os.Create(filepath.Join(poolDir, "stop.flag"))
	if err == nil {
		f.Close()
	}
}

// ReadPoolMeta читает meta.json пула рас.
func ReadPoolMeta(poolDir string) []MetaItem {
	data, err := os.ReadFile(filepath.Join(poolDir, "meta.json"))
	if err != nil {
		return nil
	}
	var m []MetaItem
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}

// ReadCandMeta читает ref_cands/meta.json (объект {file: {...}}).
func ReadCandMeta(refDir string) map[string]CandMeta {
	data, err := os.ReadFile(filepath.Join(refDir, "meta.json"))
	if err != nil {
		return nil
	}
	m := map[string]CandMeta{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}

// appendPoolMeta дописывает запись в meta.json пула рас (инкрементально,
// спека 67a.1 §8: приём во время генерации попадает в правильную папку).
func appendPoolMeta(path string, item MetaItem) {
	all := ReadPoolMeta(filepath.Dir(path))
	all = append(all, item)
	data, err := json.Marshal(all)
	if err != nil {
		return
	}
	os.WriteFile(path, data, 0644)
}

// RemovePoolMeta удаляет из meta.json пула запись с данным именем файла
// (эталон-вариант уходит из пула аватаров — решение создателя 2026-09-17).
func RemovePoolMeta(poolDir, file string) {
	all := ReadPoolMeta(poolDir)
	out := all[:0]
	for _, m := range all {
		if m.File != file {
			out = append(out, m)
		}
	}
	if len(out) == len(all) {
		return
	}
	data, err := json.Marshal(out)
	if err != nil {
		return
	}
	os.WriteFile(filepath.Join(poolDir, "meta.json"), data, 0644)
}

// appendHumanMeta дописывает запись в meta.json пула людей.
func appendHumanMeta(path string, item HumanMetaItem) {
	data, err := os.ReadFile(path)
	var all []HumanMetaItem
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

// writeCandMeta пишет ref_cands/meta.json целиком.
func writeCandMeta(path string, m map[string]CandMeta) {
	data, err := json.Marshal(m)
	if err != nil {
		return
	}
	os.WriteFile(path, data, 0644)
}