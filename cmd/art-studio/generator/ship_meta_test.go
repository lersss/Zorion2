package generator

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// TestShipMetaConcurrentWrites — общий замок на meta.json пула кораблей
// (AGENTS.md §0, идея 2026-09-21 «замок на мету пула кораблей»): конкурентные
// «правь запись» и «допиши запись» не теряют обновлений. Операция — чтение
// файла → правка слайса → запись: без замка побеждает последний писатель и
// чужие +1/дописывания теряются. Гонять под -race.
func TestShipMetaConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	const n = 12     // записей, каждую инкрементит своя горутина
	const extra = 12 // параллельно дописываемых записей
	const bumps = 50 // +1 на запись (итог 50 — без wrap)

	for i := 0; i < n; i++ {
		appendShipMeta(filepath.Join(dir, "meta.json"), ShipMetaItem{File: fmt.Sprintf("s%02d.png", i), Race: "humans"})
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			f := fmt.Sprintf("s%02d.png", i)
			for k := 0; k < bumps; k++ {
				UpdateShipAngle(dir, f, 1, false)
			}
		}(i)
	}
	for i := 0; i < extra; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			appendShipMeta(filepath.Join(dir, "meta.json"), ShipMetaItem{File: fmt.Sprintf("x%02d.png", i), Race: "humans"})
		}(i)
	}
	wg.Wait()

	meta := ReadShipMeta(dir)
	if len(meta) != n+extra {
		t.Fatalf("записей = %d, want %d (потеряны дописывания)", len(meta), n+extra)
	}
	angles := map[string]float64{}
	for _, m := range meta {
		angles[m.File] = m.Angle
	}
	for i := 0; i < n; i++ {
		f := fmt.Sprintf("s%02d.png", i)
		if angles[f] != bumps {
			t.Errorf("потеря обновления: %s Angle = %v, want %d", f, angles[f], bumps)
		}
	}
}
