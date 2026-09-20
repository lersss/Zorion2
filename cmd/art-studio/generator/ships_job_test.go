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

// shipFakeComfy — фейковый ComfySubmitter для кораблей: записывает промпты
// (узел "2" CLIPTextEncode), steps (узел "5" KSampler) и ширину ImageScale
// (узел "15": 512 — эскиз, 0 — узла нет), WaitAndDownload пишет валидный PNG.
type shipFakeComfy struct {
	mu      sync.Mutex
	prompts []string
	steps   []int
	scales  []int
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
	if n, ok := wf["15"].(map[string]interface{}); ok {
		if in, ok := n["inputs"].(map[string]interface{}); ok {
			if w, ok := in["width"].(int); ok {
				f.scales = append(f.scales, w)
			}
		}
	} else {
		f.scales = append(f.scales, 0)
	}
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

// writeFakeShipPython — фейковый python для джоба кораблей. ВАЖНО: cmd.exe
// режет аргументы по «=» (--spec=path → %2=--spec, %3=path) — фейк учитывает
// split-форму; скрипт — %1:
//   make_ship_silhouettes.py --spec=<json> --out=<png> → %2=--spec %3=json %4=--out %5=png
//   process_ship.py <in> <out>                        → копия %2 → %3
func writeFakeShipPython(t *testing.T) string {
	t.Helper()
	fp := filepath.Join(t.TempDir(), "fake_ship_python.cmd")
	script := "@echo off\r\n" +
		"if \"%2\"==\"--spec\" goto spec\r\n" +
		"copy %2 %3 >nul 2>&1\r\n" +
		"exit /b 0\r\n" +
		":spec\r\n" +
		"copy %3 %5 >nul 2>&1\r\n" +
		"exit /b 0\r\n"
	if err := os.WriteFile(fp, []byte(script), 0o644); err != nil {
		t.Fatalf("WriteFile fake python: %v", err)
	}
	return fp
}

// TestGenShips — джоб кораблей: силуэт (кэш silhouettes/), 2 кандидата s01/s02,
// мета с промптами без токенов blocked.
func TestGenShips(t *testing.T) {
	dc, err := config.LoadShipDict("../../../config/art/ship_dict.json")
	if err != nil {
		t.Fatalf("LoadShipDict: %v", err)
	}
	ships := config.ShipsConfig{
		"humans": {
			RaceName: "Люди", Family: "F1",
			Texture:    "paneled white-grey metal hull with ceramic heat shield tiles, riveted seams, navigation lights, subtle weathering, light blue cockpit glass, no organic shapes, no bioluminescence",
			Silhouette: "крыло-корпус в плане: широкий нос (светлая кабина-стекло) справа, сужающаяся корма с дюзами слева; асимметрия по оси «нос-корма»; крылья-стабилизаторы; модули: корпус крем, крылья сталь, дюзы тёмные, кабина светлая; запас от краёв ~90 px",
			Blocked:    []string{"tentacle", "organic", "crystal", "pyramid", "obelisk", "bioluminescent", "alien"},
		},
	}
	pool := t.TempDir()
	cfg := &config.StudioConfig{
		PoolRoot:   pool,
		ComfyInput: t.TempDir(),
		Workers:    1,
		MaxCount:   100,
		PythonCmd:  writeFakeShipPython(t),
	}
	fake := &shipFakeComfy{}
	runner := NewRunner(cfg, nil, nil, nil, fake)
	runner.SetShips(ships, dc, "../../../config/races.json")

	msg, _ := runner.GenShips("humans", 2, "", "", "", 200)
	if !strings.Contains(msg, "Корабли расы humans") {
		t.Fatalf("msg = %q", msg)
	}
	waitJobDone(t, filepath.Join(pool, "ships_pool"))

	poolDir := filepath.Join(pool, "ships_pool")
	// силуэт закэширован
	if _, err := os.Stat(filepath.Join(poolDir, "silhouettes", "humans.png")); err != nil {
		t.Errorf("нет силуэта silhouettes/humans.png: %v", err)
	}
	// кандидаты s01.png, s02.png
	for _, name := range []string{"s01.png", "s02.png"} {
		if _, err := os.Stat(filepath.Join(poolDir, name)); err != nil {
			t.Errorf("нет кандидата %s: %v", name, err)
		}
	}
	// мета: 2 записи, промпты без blocked
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
		// этап 1: детали отфильтрованы по blocked (спека §3.2/§3.4)
		for _, tok := range ships["humans"].Blocked {
			if strings.Contains(m.Prompt1, tok) {
				t.Errorf("промпт этапа 1 содержит blocked-токен %q: %s", tok, m.Prompt1)
			}
		}
		// этап 2: texture расы дословно (не фильтруется, §3.4) — «no organic
		// shapes» легитимно; теги фильтруются (проверено в TestBuildShipPrompt2)
		if !strings.Contains(m.Prompt2, ships["humans"].Texture) {
			t.Errorf("промпт этапа 2 не содержит texture расы: %s", m.Prompt2)
		}
	}
	// статус завершён
	st := ReadStatus(poolDir)
	if st.Running || st.Done != 2 {
		t.Errorf("status = %+v, want done 2", st)
	}
}

// TestGenShipsBatch — пачка: 2 расы × 2 = 4 кандидата; неизвестная раса — ошибка.
func TestGenShipsBatch(t *testing.T) {
	dc, err := config.LoadShipDict("../../../config/art/ship_dict.json")
	if err != nil {
		t.Fatalf("LoadShipDict: %v", err)
	}
	ships := config.ShipsConfig{
		"humans": {
			RaceName: "Люди", Family: "F1",
			Texture:    "paneled white-grey metal hull",
			Silhouette: "крыло-корпус в плане: нос справа, корма слева; модули: корпус крем, дюзы тёмные; запас от краёв ~90 px",
			Blocked:    []string{"tentacle"},
		},
		"coastal": {
			RaceName: "Прибрежные", Family: "F1",
			Texture:    "salt-crusted coral and stone hull, warm amber lit windows",
			Silhouette: "широкая плоскодонная баржа в плане: нос справа, корма слева; модули: корпус крем, сваи коралл; запас от краёв ~90 px",
			Blocked:    []string{"machine"},
		},
	}
	pool := t.TempDir()
	cfg := &config.StudioConfig{
		PoolRoot:   pool,
		ComfyInput: t.TempDir(),
		Workers:    2,
		MaxCount:   100,
		PythonCmd:  writeFakeShipPython(t),
	}
	fake := &shipFakeComfy{}
	runner := NewRunner(cfg, nil, nil, nil, fake)
	runner.SetShips(ships, dc, "../../../config/races.json")

	msg, _ := runner.GenShipsBatch([]string{"humans", "coastal"}, 2, "", "", "", 200)
	if !strings.Contains(msg, "2 рас × 2") {
		t.Fatalf("msg = %q", msg)
	}
	waitJobDone(t, filepath.Join(pool, "ships_pool"))
	meta := ReadShipMeta(filepath.Join(pool, "ships_pool"))
	if len(meta) != 4 {
		t.Fatalf("meta = %d, want 4", len(meta))
	}
	// неизвестная раса — ошибка без запуска
	msg2, _ := runner.GenShipsBatch([]string{"nope"}, 1, "", "", "", 200)
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

// TestGenShipsOverrideTags — 98b-механизм для кораблей: GenShips с override —
// genShipsJob использует ручные строки (мета Prompt1/Prompt2 = override);
// tags без override — в авто-промпты.
func TestGenShipsOverrideTags(t *testing.T) {
	dc, err := config.LoadShipDict("../../../config/art/ship_dict.json")
	if err != nil {
		t.Fatalf("LoadShipDict: %v", err)
	}
	ships := config.ShipsConfig{
		"humans": {
			RaceName: "Люди", Family: "F1",
			Texture:    "paneled white-grey metal hull",
			Silhouette: "крыло-корпус в плане: нос справа, корма слева; модули: корпус крем, дюзы тёмные; запас от краёв ~90 px",
			Blocked:    []string{"tentacle"},
		},
	}
	pool := t.TempDir()
	cfg := &config.StudioConfig{
		PoolRoot:   pool,
		ComfyInput: t.TempDir(),
		Workers:    1,
		MaxCount:   100,
		Steps:      40,
		PythonCmd:  writeFakeShipPython(t),
	}
	fake := &shipFakeComfy{}
	runner := NewRunner(cfg, nil, nil, nil, fake)
	runner.SetShips(ships, dc, "../../../config/races.json")

	// override: ручные строки на все N
	msg, _ := runner.GenShips("humans", 2, "extra tag", "MANUAL PROMPT 1", "MANUAL PROMPT 2", 200)
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
	// воркфлоу тоже получил override (фейк записал промпты: 2 этапа × 2 кандидата)
	if len(fake.prompts) != 4 {
		t.Fatalf("fake.prompts = %d, want 4", len(fake.prompts))
	}
	for _, p := range fake.prompts {
		if p != "MANUAL PROMPT 1" && p != "MANUAL PROMPT 2" {
			t.Errorf("воркфлоу получил не-override промпт: %q", p)
		}
	}

	// tags без override: авто-промпты содержат tags (пул чистится на старте).
	// Ждём мету из 1 записи, а не waitJobDone: status.json после первого джоба
	// ещё показывает Running=false/Total=2 — второй waitJobDone вернулся бы
	// раньше, чем джоб #2 очистил пул (мета = 2 старые записи).
	msg2, _ := runner.GenShips("humans", 1, "extra tag", "", "", 200)
	if !strings.Contains(msg2, "Корабли расы humans") {
		t.Fatalf("msg2 = %q", msg2)
	}
	meta2 := waitShipMetaCount(t, filepath.Join(pool, "ships_pool"), 1)
	if !strings.Contains(meta2[0].Prompt1, "extra tag") || !strings.Contains(meta2[0].Prompt2, "extra tag") {
		t.Errorf("tags не дошли до авто-промптов: %q / %q", meta2[0].Prompt1, meta2[0].Prompt2)
	}
}

// waitShipMetaCount ждёт, пока в мете пула кораблей не будет ровно n записей
// (надёжнее waitJobDone для последовательных джобов: status.json второго
// джоба может ещё не перезаписать финальный статус первого).
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

// TestGenShipsSketch — эскиз (98c): size=100 → мета Size=100, steps этапов
// уменьшены (эскиз заметно быстрее полного).
func TestGenShipsSketch(t *testing.T) {
	dc, err := config.LoadShipDict("../../../config/art/ship_dict.json")
	if err != nil {
		t.Fatalf("LoadShipDict: %v", err)
	}
	ships := config.ShipsConfig{
		"humans": {
			RaceName: "Люди", Family: "F1",
			Texture:    "paneled white-grey metal hull",
			Silhouette: "крыло-корпус в плане: нос справа, корма слева; модули: корпус крем, дюзы тёмные; запас от краёв ~90 px",
			Blocked:    []string{"tentacle"},
		},
	}
	pool := t.TempDir()
	cfg := &config.StudioConfig{
		PoolRoot:   pool,
		ComfyInput: t.TempDir(),
		Workers:    1,
		MaxCount:   100,
		Steps:      40,
		PythonCmd:  writeFakeShipPython(t),
	}
	fake := &shipFakeComfy{}
	runner := NewRunner(cfg, nil, nil, nil, fake)
	runner.SetShips(ships, dc, "../../../config/races.json")

	msg, _ := runner.GenShips("humans", 1, "", "", "", 100)
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
	// steps этапов уменьшены (cfg.Steps=40 → 20): 2 этапа на кандидата
	if len(fake.steps) != 2 {
		t.Fatalf("fake.steps = %d, want 2", len(fake.steps))
	}
	for _, st := range fake.steps {
		if st >= cfg.Steps {
			t.Errorf("steps = %d, want < %d (эскиз)", st, cfg.Steps)
		}
	}
}

// TestGenShipsJobSizePropagates — size доходит до workflow: эскиз (100) →
// ImageScale 512 в обоих этапах (латент 512, ~4x быстрее), полный (200) →
// без ImageScale (regression: эскиз гнал img2img на 1024 — не ускорял).
func TestGenShipsJobSizePropagates(t *testing.T) {
	dc, err := config.LoadShipDict("../../../config/art/ship_dict.json")
	if err != nil {
		t.Fatalf("LoadShipDict: %v", err)
	}
	ships := config.ShipsConfig{
		"humans": {
			RaceName: "Люди", Family: "F1",
			Texture:    "paneled white-grey metal hull",
			Silhouette: "крыло-корпус в плане: нос справа, корма слева; модули: корпус крем, дюзы тёмные; запас от краёв ~90 px",
			Blocked:    []string{"tentacle"},
		},
	}
	pool := t.TempDir()
	cfg := &config.StudioConfig{
		PoolRoot:   pool,
		ComfyInput: t.TempDir(),
		Workers:    1,
		MaxCount:   100,
		Steps:      40,
		PythonCmd:  writeFakeShipPython(t),
	}
	fake := &shipFakeComfy{}
	runner := NewRunner(cfg, nil, nil, nil, fake)
	runner.SetShips(ships, dc, "../../../config/races.json")

	// эскиз: оба этапа с ImageScale 512
	msg, _ := runner.GenShips("humans", 1, "", "", "", 100)
	if !strings.Contains(msg, "Корабли расы humans") {
		t.Fatalf("msg = %q", msg)
	}
	waitJobDone(t, filepath.Join(pool, "ships_pool"))
	if len(fake.scales) != 2 {
		t.Fatalf("fake.scales = %d, want 2 (этапы эскиза)", len(fake.scales))
	}
	for _, w := range fake.scales {
		if w != 512 {
			t.Errorf("эскиз: ImageScale width = %d, want 512", w)
		}
	}

	// полный: без ImageScale (0). Ждём финальный статус второго джоба
	// (Running=false, Total=2): статус первого джоба (Total=1) неотличим по
	// Running=false, но Total различает джобы; после финального статуса джоб
	// больше не пишет в пул (чистый TempDir cleanup).
	msg2, _ := runner.GenShips("humans", 2, "", "", "", 200)
	if !strings.Contains(msg2, "Корабли расы humans") {
		t.Fatalf("msg2 = %q", msg2)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		st := ReadStatus(filepath.Join(pool, "ships_pool"))
		if !st.Running && st.Total == 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if st := ReadStatus(filepath.Join(pool, "ships_pool")); st.Running || st.Total != 2 {
		t.Fatalf("второй джоб не завершился: %+v", st)
	}
	if len(fake.scales) != 6 {
		t.Fatalf("fake.scales = %d, want 6 (2 эскиза + 4 полных)", len(fake.scales))
	}
	for _, w := range fake.scales[2:] {
		if w != 0 {
			t.Errorf("полный: ImageScale width = %d, want 0 (нет масштабирования)", w)
		}
	}
}