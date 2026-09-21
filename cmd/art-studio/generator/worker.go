package generator

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"zorion/cmd/art-studio/config"
	"zorion/cmd/art-studio/postproc"
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

// ShipMetaItem — запись meta.json пула кораблей / ships_meta.json принятых
// (спека 2026-09-20-ships-races-generator §5): [{file, race, race_name, seed,
// texture, prompt1, prompt2}]. Labels — метки авто-фильтра кандидата
// (палитра/форма/текстура, страховка, не авто-отклонение). Vote — вердикт
// создателя (like/dislike/"", 98c: только метка, файл не перемещается).
// Size — финальный размер кандидата (200 — полный, 100 — эскиз, 98c).
// Angle — накопленный ручной поворот приёмки в градусах по часовой,
// Flip — ручное зеркало по горизонтали, Date — дата приёмки (только у
// записей ships_meta.json принятых; в meta.json пула — трансформ кандидата).
type ShipMetaItem struct {
	File     string   `json:"file"`
	Race     string   `json:"race"`
	Type     string   `json:"type,omitempty"` // тип корабля расы (starship/…); пусто — легаси
	RaceName string   `json:"race_name"`
	Seed     int64    `json:"seed"`
	Texture  string   `json:"texture"`
	Prompt1  string   `json:"prompt1"`
	Prompt2  string   `json:"prompt2"`
	Labels   []string `json:"labels,omitempty"`
	Vote     string   `json:"vote,omitempty"`
	Size     int      `json:"size,omitempty"`
	Angle    float64  `json:"angle,omitempty"`
	Flip     bool     `json:"flip,omitempty"`
	Date     string   `json:"date,omitempty"`
	// OrientMeta — маркер «пара (A, F) не запечена в пиксели» (пишет приёмка
	// с 2026-09-21, спека §4.4): её надо применять при показе. Записи без
	// маркера (принятые до правки) считаются «пиксели уже довёрнуты» — импорт
	// читает их пару как (0, false).
	OrientMeta bool `json:"orient_meta,omitempty"`
	// Frame — статистика автопроверки кадра (рецепт 2026-09-21): попытки
	// txt2img на кандидата, отбраковки, проверка последнего кадра.
	Frame *ShipFrameStat `json:"frame,omitempty"`
	// Orient — подсказка авто-ориентации (рецепт 2026-09-21): финальную
	// сторону решает человек в приёмке, авто — только подсказка в мете.
	Orient *postproc.ShipOrient `json:"orient,omitempty"`
}

// ShipFrameStat — статистика автопроверки кадра (tools/ship_sprite_cut.py
// --frame-check): attempts — число попыток txt2img (1..6), rejected — сколько
// кадров не прошло проверку (край/вытянутость; при исчерпании попыток равен
// attempts — последний кадр взят за неимением лучшего), touch — стороны
// касания края, elong — вытянутость силуэта последнего кадра.
type ShipFrameStat struct {
	Attempts int      `json:"attempts"`
	Rejected int      `json:"rejected"`
	Touch    []string `json:"touch,omitempty"`
	Elong    float64  `json:"elong"`
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
	ships    config.ShipsConfig        // вкладка «Корабли рас» (nil — не подключена)
	shipDict *config.ShipDictConfig    // словари кораблей (nil — не подключены)
	racesPath string                   // config/races.json (валидация ключей ships.json)
	comfy    ComfySubmitter
}

// NewRunner создаёт Runner.
func NewRunner(cfg *config.StudioConfig, forms *config.FormsConfig, families config.FamiliesConfig, humans *config.HumansConfig, comfy ComfySubmitter) *Runner {
	return &Runner{cfg: cfg, forms: forms, families: families, humans: humans, comfy: comfy}
}

// SetShips подключает конфиги кораблей (вкладка «Корабли рас»).
func (r *Runner) SetShips(ships config.ShipsConfig, dict *config.ShipDictConfig, racesPath string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ships = ships
	r.shipDict = dict
	r.racesPath = racesPath
}

// SetCheckpoint — смена чекпоинта SDXL на сессию (селект «Модель» в UI).
// Диск (studio.json) НЕ переписывается: рестарт студии вернёт cfg.Checkpoint.
// Применяется к следующим генерациям (джобы читают актуальное значение).
func (r *Runner) SetCheckpoint(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cfg.Checkpoint = name
}

// GetCheckpoint — текущий чекпоинт SDXL.
func (r *Runner) GetCheckpoint() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cfg.Checkpoint
}

// checkpoint — текущий чекпоинт под r.mu (SetCheckpoint может заменить его
// в памяти; джобы читают актуальное значение на каждый воркфлоу, не кэшируют).
func (r *Runner) checkpoint() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cfg.Checkpoint
}

// shipsSnapshot — конфиги кораблей под r.mu (ReloadShips может заменить их
// в памяти; джоб читает снапшот при старте).
func (r *Runner) shipsSnapshot() (config.ShipsConfig, *config.ShipDictConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ships, r.shipDict
}

// shipParams — параметры кораблей под r.mu (блок ships в studio.json +
// дефолты, config.StudioConfig.ShipParams). Общий чекпоинт студии корабли не
// читают: у вкладки своя модель.
func (r *Runner) shipParams() config.ShipsParams {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cfg.ShipParams()
}

// ReloadShips перечитывает ships.json с диска и заменяет конфиг в памяти
// (кнопка «Пересобрать промт»: машинная проекция texture/silhouette/blocked
// обновилась).
func (r *Runner) ReloadShips(path string) error {
	rp := r.racesPath
	if rp == "" {
		rp = "config/races.json"
	}
	ships, err := config.LoadShipsRaces(path, rp)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.ships = ships
	r.mu.Unlock()
	return nil
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
	ships := ReadStatus(filepath.Join(r.cfg.PoolRoot, "ships_pool"))
	if races.Running {
		return races
	}
	if humans.Running {
		return humans
	}
	if ships.Running {
		return ships
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

// shipMetaMu — общий замок на файл meta.json пула кораблей (AGENTS.md §0,
// идея 2026-09-21 «замок на мету пула кораблей»): операции «прочитать файл →
// правка → записать» под ним сериализуются, иначе две операции читают один
// файл и побеждает последняя — чужая правка (угол/голос) теряется. Под
// замком ходят ВСЕ читатели и писатели меты пула: джоб генерации
// (ships_job.go) и HTTP-хендлеры студии (/ships/act, /ships/list).
var shipMetaMu sync.Mutex

// readShipMetaUnlocked — чтение meta.json пула кораблей без замка; вызывать
// только из функций, уже держащих shipMetaMu.
func readShipMetaUnlocked(poolDir string) []ShipMetaItem {
	data, err := os.ReadFile(filepath.Join(poolDir, "meta.json"))
	if err != nil {
		return nil
	}
	var m []ShipMetaItem
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}

// appendShipMeta дописывает запись в meta.json пула кораблей (под shipMetaMu).
func appendShipMeta(path string, item ShipMetaItem) {
	shipMetaMu.Lock()
	defer shipMetaMu.Unlock()
	all := readShipMetaUnlocked(filepath.Dir(path))
	all = append(all, item)
	out, err := json.Marshal(all)
	if err != nil {
		return
	}
	os.WriteFile(path, out, 0644)
}

// ReadShipMeta читает meta.json пула кораблей (под shipMetaMu).
func ReadShipMeta(poolDir string) []ShipMetaItem {
	shipMetaMu.Lock()
	defer shipMetaMu.Unlock()
	return readShipMetaUnlocked(poolDir)
}

// RemoveShipMeta удаляет из meta.json пула кораблей запись с данным файлом
// (под shipMetaMu).
func RemoveShipMeta(poolDir, file string) {
	shipMetaMu.Lock()
	defer shipMetaMu.Unlock()
	all := readShipMetaUnlocked(poolDir)
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

// SetShipVote — вердикт создателя по кандидату корабля (like/dislike/clear)
// в meta.json пула (98c: файл не перемещается, только метка; переживает
// рестарт студии — meta.json уже файл). clear → пустая метка. Возвращает
// false, если кандидата нет в мете (под shipMetaMu).
func SetShipVote(poolDir, file, vote string) bool {
	shipMetaMu.Lock()
	defer shipMetaMu.Unlock()
	all := readShipMetaUnlocked(poolDir)
	found := false
	for i := range all {
		if all[i].File == file {
			if vote == "clear" {
				all[i].Vote = ""
			} else {
				all[i].Vote = vote
			}
			found = true
			break
		}
	}
	if !found {
		return false
	}
	data, err := json.Marshal(all)
	if err != nil {
		return false
	}
	os.WriteFile(filepath.Join(poolDir, "meta.json"), data, 0644)
	return true
}

// shipAngleNorm — привести угол к конвенции показа (спека 2026-09-21 §3.1):
// диапазон (−180, 180], шаг записи 0.1; −0.0 нормализуется к 0. Единственное
// место wrap'а — все писатели пары держат интервал через него.
func shipAngleNorm(a float64) float64 {
	for a > 180 {
		a -= 360
	}
	for a <= -180 {
		a += 360
	}
	v := math.Round(a*10) / 10
	if v == 0 {
		return 0
	}
	return v
}

// ShipHintPair — канонизатор подсказки авто-носа (profile_orientation) в пару
// показа (A, F) по спеке 2026-09-21 §3.3: angle — конвенция PIL (положительный
// против часовой), v = −angle — тот же поворот по часовой; доворот меньше 1°
// не делаем (A = 0); зеркало (mirror && !ambiguous) независимо от угла и меняет
// знак угла. Единственное место перевода PIL-знака в показный: джоб выреза,
// GET /ships/auto и what=auto зовут эту функцию. Результат — уже в конвенции
// показа (равен записи SetShipOrient и слайдеру).
func ShipHintPair(angle float64, mirror, ambiguous bool) (float64, bool) {
	v := -angle
	a := 0.0
	if math.Abs(v) > 1 {
		a = v
	}
	f := mirror && !ambiguous
	if f {
		a = -a
	}
	return shipAngleNorm(a), f
}

// SetShipOrient — абсолютная установка пары (A, F) кандидата в meta.json пула
// (приёмка кораблей: слайдер setangle, авто-пара, начальная пара из джоба).
// A приводится к (−180, 180] с шагом 0.1. Возвращает false, если кандидата
// нет в мете (под shipMetaMu).
func SetShipOrient(poolDir, file string, angle float64, flip bool) bool {
	shipMetaMu.Lock()
	defer shipMetaMu.Unlock()
	all := readShipMetaUnlocked(poolDir)
	found := false
	for i := range all {
		if all[i].File != file {
			continue
		}
		all[i].Angle = shipAngleNorm(angle)
		all[i].Flip = flip
		found = true
		break
	}
	if !found {
		return false
	}
	data, err := json.Marshal(all)
	if err != nil {
		return false
	}
	os.WriteFile(filepath.Join(poolDir, "meta.json"), data, 0644)
	return true
}

// ShipOrientOf — текущая пара (A, F) кандидата из meta.json пула (ответ
// /ships/act и тесты); ok=false, если кандидата нет в мете (под shipMetaMu).
func ShipOrientOf(poolDir, file string) (angle float64, flip bool, ok bool) {
	shipMetaMu.Lock()
	defer shipMetaMu.Unlock()
	for _, m := range readShipMetaUnlocked(poolDir) {
		if m.File == file {
			return m.Angle, m.Flip, true
		}
	}
	return 0, false, false
}

// UpdateShipAngle — накопить ручной поворот кандидата в meta.json пула
// (приёмка кораблей): deg — добавка в градусах по часовой, результат
// приводится к интервалу (−180, 180]; flip=true — переключить зеркало
// (зеркало меняет знак угла). Возвращает false, если кандидата нет в мете
// (под shipMetaMu).
func UpdateShipAngle(poolDir, file string, deg float64, flip bool) bool {
	shipMetaMu.Lock()
	defer shipMetaMu.Unlock()
	all := readShipMetaUnlocked(poolDir)
	found := false
	for i := range all {
		if all[i].File != file {
			continue
		}
		a := all[i].Angle + deg
		if flip {
			a = -a
			all[i].Flip = !all[i].Flip
		}
		all[i].Angle = shipAngleNorm(a)
		found = true
		break
	}
	if !found {
		return false
	}
	data, err := json.Marshal(all)
	if err != nil {
		return false
	}
	os.WriteFile(filepath.Join(poolDir, "meta.json"), data, 0644)
	return true
}

// writeCandMeta пишет ref_cands/meta.json целиком.
func writeCandMeta(path string, m map[string]CandMeta) {
	data, err := json.Marshal(m)
	if err != nil {
		return
	}
	os.WriteFile(path, data, 0644)
}