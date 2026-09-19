package handlers

import (
	"image"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"zorion/cmd/art-studio/config"
	"zorion/cmd/art-studio/generator"
)

// recordingComfy — фейковый ComfySubmitter (98b): записывает промпты из
// воркфлоу (узел "2" CLIPTextEncode → inputs.text), WaitAndDownload пишет
// валидный PNG (постпроцесс не падает на чтении).
type recordingComfy struct {
	mu      sync.Mutex
	prompts []string
}

func (f *recordingComfy) Submit(wf map[string]interface{}) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n, ok := wf["2"].(map[string]interface{}); ok {
		if in, ok := n["inputs"].(map[string]interface{}); ok {
			if txt, ok := in["text"].(string); ok {
				f.prompts = append(f.prompts, txt)
			}
		}
	}
	return "pid-1", nil
}

func (f *recordingComfy) WaitAndDownload(pid, outPath string) (bool, error) {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	fh, err := os.Create(outPath)
	if err != nil {
		return false, err
	}
	defer fh.Close()
	png.Encode(fh, img)
	return true, nil
}

func (f *recordingComfy) promptsCopy() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.prompts...)
}

// newTestStudio — Server арт-студии на временных пулах с фейковым Comfy.
func newTestStudio(t *testing.T) (*Server, *recordingComfy, string) {
	t.Helper()
	fam, err := config.LoadFamilies("../../../config/art/families.json")
	if err != nil {
		t.Fatalf("LoadFamilies: %v", err)
	}
	fc, err := config.LoadForms("../../../config/art/forms.json")
	if err != nil {
		t.Fatalf("LoadForms: %v", err)
	}
	humans, err := config.LoadHumans("../../../config/art/humans.json")
	if err != nil {
		t.Fatalf("LoadHumans: %v", err)
	}
	pool := t.TempDir()
	cfg := &config.StudioConfig{
		PoolRoot:   pool,
		ComfyInput: t.TempDir(),
		Workers:    1,
		MaxCount:   100,
	}
	fake := &recordingComfy{}
	runner := generator.NewRunner(cfg, fc, fam, humans, fake)
	srv := NewServer(cfg, fc, fam, humans, runner, []byte("<html></html>"))
	return srv, fake, pool
}

// waitJobDone ждёт завершения джоба: status.json существует И Running=false
// (последняя запись джоба — больше файлов не пишет) И промпт записан фейком.
// Проверка существования файла обязательна: если стартовая запись статуса
// упала (Windows-гонка rename, см. WriteStatus), status.json ещё не создан —
// без неё тест вернулся бы раньше, чем джоб дописал файлы пула.
func waitJobDone(t *testing.T, fake *recordingComfy, pool string) {
	t.Helper()
	statusPath := filepath.Join(pool, "races_pool", "status.json")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		st := generator.ReadStatus(filepath.Join(pool, "races_pool"))
		if !st.Running && len(fake.promptsCopy()) > 0 && fileExists(statusPath) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	st := generator.ReadStatus(filepath.Join(pool, "races_pool"))
	t.Fatalf("джоб не завершился за 5 с: status=%+v prompts=%d", st, len(fake.promptsCopy()))
}

// TestHandleGenRefTags — 98b: /genref парсит query-параметр tags и передаёт
// в промпт кандидатов (полная цепочка: хендлер → GenRef → genRefJob →
// BuildPromptWide → воркфлоу).
func TestHandleGenRefTags(t *testing.T) {
	srv, fake, pool := newTestStudio(t)
	req := httptest.NewRequest("GET", "/genref?fam=F2&n=1&morph=&size=512&tags=my+tag", nil)
	rr := httptest.NewRecorder()
	srv.handleGenRef(rr, req)
	waitJobDone(t, fake, pool)
	ps := fake.promptsCopy()
	if len(ps) == 0 {
		t.Fatal("промпт не отправлен в Comfy")
	}
	if !strings.Contains(ps[0], "my tag") {
		t.Errorf("tags не в промпте: %s", ps[0])
	}
}

// TestHandleGenVarTags — 98b: /genvar парсит tags (вариации от эталона).
func TestHandleGenVarTags(t *testing.T) {
	srv, fake, pool := newTestStudio(t)
	// эталон расы F2/5 обязателен для /genvar (иначе ранний выход до джоба)
	os.MkdirAll(filepath.Join(pool, "races_pool"), 0755)
	os.WriteFile(filepath.Join(pool, "races_pool", "ref_F2_r5.png"), []byte("x"), 0644)
	req := httptest.NewRequest("GET", "/genvar?fam=F2&race=5&n=1&size=512&tags=my+tag", nil)
	rr := httptest.NewRecorder()
	srv.handleGenVar(rr, req)
	waitJobDone(t, fake, pool)
	ps := fake.promptsCopy()
	if len(ps) == 0 {
		t.Fatal("промпт не отправлен в Comfy")
	}
	if !strings.Contains(ps[0], "my tag") {
		t.Errorf("tags не в промпте: %s", ps[0])
	}
}

// TestHandleGenRefOverride — 98b-дополнение 2: /genref с prompt_override —
// все N промптов = override (BuildPromptWide не вызывается).
func TestHandleGenRefOverride(t *testing.T) {
	srv, fake, pool := newTestStudio(t)
	req := httptest.NewRequest("GET", "/genref?fam=F2&n=3&morph=&size=512&prompt_override=my+override+prompt", nil)
	rr := httptest.NewRecorder()
	srv.handleGenRef(rr, req)
	waitJobDone(t, fake, pool)
	ps := fake.promptsCopy()
	if len(ps) != 3 {
		t.Fatalf("промптов %d, want 3", len(ps))
	}
	for i, p := range ps {
		if p != "my override prompt" {
			t.Errorf("промпт %d = %q, want override", i, p)
		}
	}
}

// TestHandleGenVarOverride — /genvar с prompt_override — все N = override.
func TestHandleGenVarOverride(t *testing.T) {
	srv, fake, pool := newTestStudio(t)
	os.MkdirAll(filepath.Join(pool, "races_pool"), 0755)
	os.WriteFile(filepath.Join(pool, "races_pool", "ref_F2_r5.png"), []byte("x"), 0644)
	req := httptest.NewRequest("GET", "/genvar?fam=F2&race=5&n=3&size=512&prompt_override=my+override+prompt", nil)
	rr := httptest.NewRecorder()
	srv.handleGenVar(rr, req)
	waitJobDone(t, fake, pool)
	ps := fake.promptsCopy()
	if len(ps) != 3 {
		t.Fatalf("промптов %d, want 3", len(ps))
	}
	for i, p := range ps {
		if p != "my override prompt" {
			t.Errorf("промпт %d = %q, want override", i, p)
		}
	}
}

// TestHandleGenRefAutoVariety — без prompt_override промпты разные (авто,
// BuildPromptWide на каждый i): разброс картинок сохраняется.
func TestHandleGenRefAutoVariety(t *testing.T) {
	srv, fake, pool := newTestStudio(t)
	req := httptest.NewRequest("GET", "/genref?fam=F2&n=3&morph=&size=512", nil)
	rr := httptest.NewRecorder()
	srv.handleGenRef(rr, req)
	waitJobDone(t, fake, pool)
	ps := fake.promptsCopy()
	if len(ps) != 3 {
		t.Fatalf("промптов %d, want 3", len(ps))
	}
	seen := map[string]bool{}
	for _, p := range ps {
		seen[p] = true
	}
	if len(seen) < 2 {
		t.Errorf("авто-промпты одинаковые (%d уникальных): %v", len(seen), ps)
	}
}