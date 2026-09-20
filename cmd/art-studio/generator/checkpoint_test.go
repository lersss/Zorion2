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

// checkpointFakeComfy — фейковый ComfySubmitter: записывает чекпоинты из
// воркфлоу (узел "1" CheckpointLoaderSimple → inputs.ckpt_name),
// WaitAndDownload пишет валидный PNG (постпроцесс не падает на чтении).
type checkpointFakeComfy struct {
	mu          sync.Mutex
	checkpoints []string
}

func (f *checkpointFakeComfy) Submit(wf map[string]interface{}) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n, ok := wf["1"].(map[string]interface{}); ok {
		if in, ok := n["inputs"].(map[string]interface{}); ok {
			if ck, ok := in["ckpt_name"].(string); ok {
				f.checkpoints = append(f.checkpoints, ck)
			}
		}
	}
	return "pid-1", nil
}

func (f *checkpointFakeComfy) WaitAndDownload(pid, outPath string) (bool, error) {
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

func (f *checkpointFakeComfy) checkpointsCopy() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.checkpoints...)
}

// TestRunnerSetCheckpointJobUsesNew — SetCheckpoint меняет чекпоинт, и джоб
// после установки использует НОВЫЙ чекпоинт (не кэширует cfg.Checkpoint).
func TestRunnerSetCheckpointJobUsesNew(t *testing.T) {
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
		PythonCmd:  writeFakeShipPython(t), // фейк копирует in → out (эмуляция rembg)
		RembgCLI:   "rembg_cli.py",
		Checkpoint: "juggernaut-xl-v9.safetensors",
	}
	fake := &checkpointFakeComfy{}
	runner := NewRunner(cfg, nil, nil, humans, fake)

	// смена чекпоинта на сессию
	runner.SetCheckpoint("dreamshaper-xl-v1.safetensors")
	if got := runner.GetCheckpoint(); got != "dreamshaper-xl-v1.safetensors" {
		t.Fatalf("GetCheckpoint = %q, want dreamshaper-xl-v1.safetensors", got)
	}

	msg, _ := runner.GenHumans(1)
	if !strings.Contains(msg, "Запущена генерация") {
		t.Fatalf("msg = %q", msg)
	}
	waitJobDone(t, filepath.Join(pool, "humans_pool"))

	cks := fake.checkpointsCopy()
	if len(cks) != 1 || cks[0] != "dreamshaper-xl-v1.safetensors" {
		t.Errorf("джоб использовал чекпоинты %v, want [dreamshaper-xl-v1.safetensors]", cks)
	}
}