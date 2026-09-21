package generator

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"zorion/cmd/art-studio/config"
)

// shipFakeComfy — фейковый ComfySubmitter для кораблей (рецепт 2026-09-21):
// записывает промпты (узел "2" CLIPTextEncode), steps (узел "5" KSampler) и
// факт Hi-Res (узел "13" ImageUpscaleWithModel); WaitAndDownload пишет PNG.
type shipFakeComfy struct {
	mu       sync.Mutex
	prompts  []string
	steps    []int
	upscaled []bool
}

func (f *shipFakeComfy) Submit(wf map[string]interface{}) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n, ok := wf["2"].(map[string]interface{}); ok {
		if in, ok := n["inputs"].(map[string]interface{}); ok {
			if txt, ok := in["text"].(string); ok {
				f.prompts = append(f.prompts, txt)
			}
		}
	}
	if n, ok := wf["5"].(map[string]interface{}); ok {
		if in, ok := n["inputs"].(map[string]interface{}); ok {
			if st, ok := in["steps"].(int); ok {
				f.steps = append(f.steps, st)
			}
		}
	}
	_, hires := wf["13"]
	f.upscaled = append(f.upscaled, hires)
	return "pid-1", nil
}

func (f *shipFakeComfy) WaitAndDownload(pid, outPath string) (bool, error) {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	var fh *os.File
	var err error
	for i := 0; i < 10; i++ {
		fh, err = os.Create(outPath)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		return false, err
	}
	defer fh.Close()
	png.Encode(fh, img)
	return true, nil
}

// writeFakeShipPython — фейковый python для джоба кораблей (рецепт 2026-09-21).
// ВАЖНО: cmd.exe режет аргументы по «=» — фейк учитывает формы:
//
//	ship_sprite_cut.py <in> --frame-check --report <json> --margin N --min-elong E
//	  → %2=in, %3=--frame-check, %5=json; пишем результат frameOk
//	ship_sprite_cut.py <in> <out> --method hyst ... → %2=in, %3=out; копируем
func writeFakeShipPython(t *testing.T, frameOK bool) string {
	t.Helper()
	fp := filepath.Join(t.TempDir(), "fake_ship_python.cmd")
	frame := `{"touch":[],"elong":2.0,"ok":true}`
	if !frameOK {
		frame = `{"touch":["left"],"elong":1.0,"ok":false}`
	}
	script := "@echo off\r\n" +
		"if \"%3\"==\"--frame-check\" goto frame\r\n" +
		"copy %2 %3 >nul 2>&1\r\n" +
		"exit /b 0\r\n" +
		":frame\r\n" +
		"echo " + frame + " > %5\r\n" +
		"exit /b 0\r\n"
	if err := os.WriteFile(fp, []byte(script), 0o644); err != nil {
		t.Fatalf("WriteFile fake python: %v", err)
	}
	return fp
}

// newShipRunner — Runner с фейковыми Comfy/python и одной расой humans.
func newShipRunner(t *testing.T, fake *shipFakeComfy, frameOK bool) (*Runner, string, config.ShipsConfig) {
	t.Helper()
	ships := config.ShipsConfig{
		"humans": {
			RaceName: "Люди", Family: "F1",
			Texture:    "paneled white-grey metal hull with ceramic heat shield tiles, riveted seams, navigation lights, light blue cockpit glass",
			Silhouette: "крыло-корпус в плане: нос справа, корма слева; модули: корпус крем, дюзы тёмные; запас от краёв ~90 px",
			Blocked:    []string{"tentacle", "organic", "crystal", "pyramid", "obelisk", "bioluminescent", "alien"},
		},
	}
	pool := t.TempDir()
	cfg := &config.StudioConfig{
		PoolRoot:   pool,
		ComfyInput: t.TempDir(),
		Workers:    1,
		MaxCount:   100,
		PythonCmd:  writeFakeShipPython(t, frameOK),
	}
	runner := NewRunner(cfg, nil, nil, nil, fake)
	runner.SetShips(ships, loadShipDict(t), "../../../config/races.json")
	return runner, pool, ships
}

// TestGenShips — джоб кораблей (рецепт 2026-09-21): txt2img → frame-check →
// вырез → 2 кандидата s01/s02, мета с промптами без токенов blocked и
// статистикой попыток.
func TestGenShips(t *testing.T) {
	fake := &shipFakeComfy{}
	runner, pool, ships := newShipRunner(t, fake, true)
	msg, _ := runner.GenShips("humans", "", 2, "", "", "", false, 200, false)
	if !strings.Contains(msg, "Корабли расы humans") {
		t.Fatalf("msg = %q", msg)
	}
	waitJobDone(t, filepath.Join(pool, "ships_pool"))
	poolDir := filepath.Join(pool, "ships_pool")
	for _, name := range []string{"s01.png", "s02.png"} {
		if _, err := os.Stat(filepath.Join(poolDir, name)); err != nil {
			t.Errorf("нет кандидата %s: %v", name, err)
		}
	}
	meta := ReadShipMeta(poolDir)
	if len(meta) != 2 {
		t.Fatalf("meta = %d записей, want 2", len(meta))
	}
	for _, m := range meta {
		if m.Race != "humans" || m.RaceName != "Люди" {
			t.Errorf("meta race = %s/%s", m.Race, m.RaceName)
		}
		if m.Prompt1 == "" || m.Prompt2 == "" {
			t.Errorf("пустые промпты: %+v", m)
		}
		// промпт txt2img: субъект из космического пула + texture расы
		if !strings.Contains(m.Prompt1, ships["humans"].Texture) {
			t.Errorf("промпт1 не содержит texture расы: %s", m.Prompt1)
		}
		if !strings.Contains(m.Prompt1, ShipViewAnchor) {
			t.Errorf("промпт1 не содержит якорь ракурса: %s", m.Prompt1)
		}
		// blocked-термы расы — только в негативе, не в позитиве
		for _, tok := range ships["humans"].Blocked {
			if strings.Contains(strings.ToLower(m.Prompt1), tok) {
				t.Errorf("промпт1 содержит blocked-токен %q: %s", tok, m.Prompt1)
			}
		}
		// prompt2 — Hi-Res: промпт1 + хвост детализации
		if !strings.Contains(m.Prompt2, ShipHiresTail) {
			t.Errorf("промпт2 не содержит хвост Hi-Res: %s", m.Prompt2)
		}
		// статистика попыток: кадр годен → 1 попытка, 0 отбраковок
		if m.Frame == nil || m.Frame.Attempts != 1 || m.Frame.Rejected != 0 {
			t.Errorf("frame = %+v, want attempts=1 rejected=0", m.Frame)
		}
	}
	st := ReadStatus(poolDir)
	if st.Running || st.Done != 2 {
		t.Errorf("status = %+v, want done 2", st)
	}
	// без Hi-Res узел апскейла не встречался
	for _, u := range fake.upscaled {
		if u {
			t.Errorf("без hires отправлен Hi-Res-воркфлоу")
		}
	}
}

// TestGenShipsFrameRetry — негодный кадр (касание края / не вытянут): джоб
// берёт следующий seed, ≤ shipFrameTries попыток; в мете — число попыток.
func TestGenShipsFrameRetry(t *testing.T) {
	fake := &shipFakeComfy{}
	runner, pool, _ := newShipRunner(t, fake, false)
	msg, _ := runner.GenShips("humans", "", 1, "", "", "", false, 200, false)
	if !strings.Contains(msg, "Корабли расы humans") {
		t.Fatalf("msg = %q", msg)
	}
	waitJobDone(t, filepath.Join(pool, "ships_pool"))
	meta := ReadShipMeta(filepath.Join(pool, "ships_pool"))
	if len(meta) != 1 {
		t.Fatalf("meta = %d, want 1", len(meta))
	}
	if meta[0].Frame == nil {
		t.Fatalf("нет статистики попыток")
	}
	// все попытки негодны: 6 txt2img-кадров, все 6 забракованы (последний взят
	// за неимением лучшего — так же делал спайк)
	if meta[0].Frame.Attempts != shipFrameTries || meta[0].Frame.Rejected != shipFrameTries {
		t.Errorf("frame = %+v, want attempts=%d rejected=%d", meta[0].Frame, shipFrameTries, shipFrameTries)
	}
	// txt2img вызывался на каждую попытку
	if len(fake.prompts) != shipFrameTries {
		t.Errorf("txt2img вызовов = %d, want %d", len(fake.prompts), shipFrameTries)
	}
}

// TestGenShipsOverrideTags — override доходит до меты и воркфлоу; tags без
// override — в авто-промпт txt2img.
func TestGenShipsOverrideTags(t *testing.T) {
	fake := &shipFakeComfy{}
	runner, pool, _ := newShipRunner(t, fake, true)
	msg, _ := runner.GenShips("humans", "", 2, "extra tag", "MANUAL PROMPT 1", "MANUAL PROMPT 2", false, 200, false)
	if !strings.Contains(msg, "Корабли расы humans") {
		t.Fatalf("msg = %q", msg)
	}
	waitJobDone(t, filepath.Join(pool, "ships_pool"))
	meta := ReadShipMeta(filepath.Join(pool, "ships_pool"))
	if len(meta) != 2 {
		t.Fatalf("meta = %d, want 2", len(meta))
	}
	for _, m := range meta {
		if m.Prompt1 != "MANUAL PROMPT 1" || m.Prompt2 != "MANUAL PROMPT 2" {
			t.Errorf("override не дошёл до генерации: prompt1=%q prompt2=%q", m.Prompt1, m.Prompt2)
		}
	}
	// воркфлоу получил override (2 кандидата × 1 попытка)
	if len(fake.prompts) != 2 {
		t.Fatalf("fake.prompts = %d, want 2", len(fake.prompts))
	}
	for _, p := range fake.prompts {
		if p != "MANUAL PROMPT 1" {
			t.Errorf("воркфлоу получил не-override промпт: %q", p)
		}
	}
	// tags без override: авто-промпт содержит tags
	msg2, _ := runner.GenShips("humans", "", 1, "extra tag", "", "", false, 200, false)
	if !strings.Contains(msg2, "Корабли расы humans") {
		t.Fatalf("msg2 = %q", msg2)
	}
	meta2 := waitShipMetaCount(t, filepath.Join(pool, "ships_pool"), 1)
	if !strings.Contains(meta2[0].Prompt1, "extra tag") {
		t.Errorf("tags не дошли до авто-промпта: %q", meta2[0].Prompt1)
	}
}

// TestGenShipsHires — hires=true: на кандидата отправляется Hi-Res-воркфлоу.
func TestGenShipsHires(t *testing.T) {
	fake := &shipFakeComfy{}
	runner, pool, _ := newShipRunner(t, fake, true)
	msg, _ := runner.GenShips("humans", "", 1, "", "", "", true, 200, false)
	if !strings.Contains(msg, "Корабли расы humans") {
		t.Fatalf("msg = %q", msg)
	}
	waitJobDone(t, filepath.Join(pool, "ships_pool"))
	ups := 0
	for _, u := range fake.upscaled {
		if u {
			ups++
		}
	}
	if ups != 1 {
		t.Errorf("Hi-Res воркфлоу = %d, want 1", ups)
	}
}

// TestGenShipsBatch — пачка: 2 расы × 2 = 4 кандидата; неизвестная раса — ошибка.
func TestGenShipsBatch(t *testing.T) {
	fake := &shipFakeComfy{}
	runner, pool, ships := newShipRunner(t, fake, true)
	ships["coastal"] = config.ShipEntry{
		RaceName: "Прибрежные", Family: "F1",
		Texture:    "salt-crusted coral and stone hull, warm amber lit windows",
		Silhouette: "широкая плоскодонная баржа в плане: нос справа, корма слева; модули: корпус крем, сваи коралл; запас от краёв ~90 px",
		Blocked:    []string{"machine"},
	}
	runner.SetShips(ships, loadShipDict(t), "../../../config/races.json")

	msg, _ := runner.GenShipsBatch([]string{"humans", "coastal"}, 2, "", "", "", false, 200, false)
	if !strings.Contains(msg, "2 рас × 2") {
		t.Fatalf("msg = %q", msg)
	}
	waitJobDone(t, filepath.Join(pool, "ships_pool"))
	meta := ReadShipMeta(filepath.Join(pool, "ships_pool"))
	if len(meta) != 4 {
		t.Fatalf("meta = %d, want 4", len(meta))
	}
	msg2, _ := runner.GenShipsBatch([]string{"nope"}, 1, "", "", "", false, 200, false)
	if !strings.Contains(msg2, "нет расы nope") {
		t.Errorf("msg2 = %q", msg2)
	}
}

// waitJobDone ждёт завершения джоба: status.json существует И Running=false.
func waitJobDone(t *testing.T, poolDir string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		st := ReadStatus(poolDir)
		if !st.Running && st.Total > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("джоб не завершился за 30 с: %+v", ReadStatus(poolDir))
}

// waitShipMetaCount ждёт, пока в мете пула кораблей не будет ровно n записей.
func waitShipMetaCount(t *testing.T, poolDir string, n int) []ShipMetaItem {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		meta := ReadShipMeta(poolDir)
		if len(meta) == n {
			return meta
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("мета пула не достигла %d записей: %d", n, len(ReadShipMeta(poolDir)))
	return nil
}

// TestGenShipsSketch — эскиз (size=100): мета Size=100, steps txt2img уменьшены.
func TestGenShipsSketch(t *testing.T) {
	fake := &shipFakeComfy{}
	runner, pool, _ := newShipRunner(t, fake, true)
	msg, _ := runner.GenShips("humans", "", 1, "", "", "", false, 100, false)
	if !strings.Contains(msg, "Корабли расы humans") {
		t.Fatalf("msg = %q", msg)
	}
	waitJobDone(t, filepath.Join(pool, "ships_pool"))
	meta := ReadShipMeta(filepath.Join(pool, "ships_pool"))
	if len(meta) != 1 {
		t.Fatalf("meta = %d, want 1", len(meta))
	}
	if meta[0].Size != 100 {
		t.Errorf("meta Size = %d, want 100", meta[0].Size)
	}
	if len(fake.steps) != 1 {
		t.Fatalf("fake.steps = %d, want 1", len(fake.steps))
	}
	if fake.steps[0] >= 32 {
		t.Errorf("steps = %d, want < 32 (эскиз)", fake.steps[0])
	}
}

// TestGenShipsKeepPool — keep=true: пул НЕ чистится, кандидаты и мета копятся
// (накопительный прогон «пилот → остальные»); keep=false — пул чистится.
func TestGenShipsKeepPool(t *testing.T) {
	fake := &shipFakeComfy{}
	runner, pool, _ := newShipRunner(t, fake, true)
	poolDir := filepath.Join(pool, "ships_pool")
	// первый прогон: 2 кандидата (пул чист)
	if msg, _ := runner.GenShips("humans", "", 2, "", "", "", false, 200, false); !strings.Contains(msg, "humans") {
		t.Fatalf("msg = %q", msg)
	}
	waitJobDone(t, poolDir)
	// второй прогон с keep: пул сохраняется, номера продолжаются
	if msg, _ := runner.GenShips("humans", "", 1, "", "", "", false, 200, true); !strings.Contains(msg, "humans") {
		t.Fatalf("msg = %q", msg)
	}
	waitShipMetaCount(t, poolDir, 3)
	for _, name := range []string{"s01.png", "s02.png", "s03.png"} {
		if _, err := os.Stat(filepath.Join(poolDir, name)); err != nil {
			t.Errorf("keep: нет кандидата %s: %v", name, err)
		}
	}
	// третий прогон БЕЗ keep: пул чистится (остаётся только новый s01)
	if msg, _ := runner.GenShips("humans", "", 1, "", "", "", false, 200, false); !strings.Contains(msg, "humans") {
		t.Fatalf("msg = %q", msg)
	}
	waitShipMetaCount(t, poolDir, 1)
	if _, err := os.Stat(filepath.Join(poolDir, "s02.png")); err == nil {
		t.Errorf("без keep пул не очищен: s02.png остался")
	}
}

// TestGenShipsType — тип корабля: GenShips с typeName генерирует только этот
// тип (в мете — Type и texture типа); batch разворачивает все типы расы.
func TestGenShipsType(t *testing.T) {
	fake := &shipFakeComfy{}
	runner, pool, ships := newShipRunner(t, fake, true)
	base := ships["humans"]
	base.Types = []config.ShipType{
		{Type: "starship", Texture: "starship hull material"},
		{Type: "fighter", Texture: "fighter hull material"},
	}
	ships["humans"] = base
	runner.SetShips(ships, loadShipDict(t), "../../../config/races.json")

	poolDir := filepath.Join(pool, "ships_pool")
	msg, _ := runner.GenShips("humans", "fighter", 1, "", "", "", false, 200, false)
	if !strings.Contains(msg, "Корабли расы humans") {
		t.Fatalf("msg = %q", msg)
	}
	waitJobDone(t, poolDir)
	meta := ReadShipMeta(poolDir)
	if len(meta) != 1 {
		t.Fatalf("meta = %d, want 1", len(meta))
	}
	if meta[0].Type != "fighter" || meta[0].Texture != "fighter hull material" {
		t.Errorf("meta = %+v, want type fighter / texture fighter", meta[0])
	}
	if !strings.Contains(meta[0].Prompt1, "fighter hull material") {
		t.Errorf("промпт без texture типа: %s", meta[0].Prompt1)
	}
	// неизвестный тип — ошибка без старта джоба
	if msg2, _ := runner.GenShips("humans", "nope", 1, "", "", "", false, 200, false); !strings.Contains(msg2, "нет типа nope") {
		t.Errorf("msg2 = %q, want «нет типа nope»", msg2)
	}
	// batch: все типы расы разворачиваются (1 раса × 1 = 2 задачи)
	msg3, _ := runner.GenShipsBatch([]string{"humans"}, 1, "", "", "", false, 200, false)
	if !strings.Contains(msg3, "= 2") {
		t.Fatalf("msg3 = %q, want 2 задачи", msg3)
	}
	waitShipMetaCount(t, poolDir, 2)
	types := map[string]bool{}
	for _, m := range ReadShipMeta(poolDir) {
		types[m.Type] = true
	}
	if !types["starship"] || !types["fighter"] {
		t.Errorf("batch типы = %v, want starship+fighter", types)
	}
}
