package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zorion/cmd/art-studio/config"
)

// TestAggregateStatus — /status-агрегация обоих пулов, приоритет running
// (DoD 67a.1 §10, спека §6.1).
func TestAggregateStatus(t *testing.T) {
	dir := t.TempDir()
	races := filepath.Join(dir, "races_pool")
	humans := filepath.Join(dir, "humans_pool")
	os.MkdirAll(races, 0755)
	os.MkdirAll(humans, 0755)

	// оба пула без файлов → не running
	st := AggregateStatus(races, humans)
	if st.Running {
		t.Error("пустые пулы: running = true")
	}

	// races running → приоритет
	WriteStatus(races, Status{Running: true, Done: 3, Total: 8, Current: "x"})
	WriteStatus(humans, Status{Running: true, Done: 1, Total: 4, Current: "y"})
	st = AggregateStatus(races, humans)
	if !st.Running || st.Done != 3 || st.Total != 8 {
		t.Errorf("races-приоритет: %+v", st)
	}

	// только humans running
	WriteStatus(races, Status{Running: false, Done: 8, Total: 8, Current: "готово"})
	st = AggregateStatus(races, humans)
	if !st.Running || st.Done != 1 || st.Total != 4 {
		t.Errorf("humans: %+v", st)
	}

	// оба не running → races (последний статус)
	WriteStatus(humans, Status{Running: false, Done: 4, Total: 4, Current: "готово"})
	st = AggregateStatus(races, humans)
	if st.Running || st.Current != "готово" {
		t.Errorf("idle: %+v", st)
	}
}

// TestWriteStatusAtomic — status.json пишется и читается.
func TestWriteStatusAtomic(t *testing.T) {
	dir := t.TempDir()
	WriteStatus(dir, Status{Running: true, Done: 2, Total: 8, Current: "тест"})
	st := ReadStatus(dir)
	if !st.Running || st.Done != 2 || st.Total != 8 || st.Current != "тест" {
		t.Errorf("прочитан статус: %+v", st)
	}
	if _, err := os.Stat(filepath.Join(dir, "status.json.tmp")); err == nil {
		t.Error("tmp-файл не удалён после rename")
	}
}

// TestAppendPoolMeta — мета пишется инкрементально (спека 67a.1 §8).
func TestAppendPoolMeta(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.json")
	appendPoolMeta(path, MetaItem{File: "r01.png", Seed: 1, Family: "F2", Race: "5 Аммиачники", RaceID: "5", Prompt: "p1"})
	appendPoolMeta(path, MetaItem{File: "r02.png", Seed: 2, Family: "F2", Race: "6 Крио-лесные", RaceID: "6", Prompt: "p2"})
	all := ReadPoolMeta(dir)
	if len(all) != 2 {
		t.Fatalf("мета = %d записей, want 2", len(all))
	}
	if all[1].File != "r02.png" || all[1].RaceID != "6" {
		t.Errorf("вторая запись: %+v", all[1])
	}
}

// testFamiliesJSON — минимальный families.json для тестов Runner.
const testFamiliesJSON = `{
  "F1": {
    "name": "Тест",
    "races": [
      {
        "id": "1",
        "name": "1 Тест",
        "basis": "тест",
        "forms": ["a test form one", "a test form two"],
        "materials": ["test material one", "test material two"],
        "glows": ["test glow one", "test glow two"],
        "appearance": "old appearance",
        "blocked": ["old"]
      }
    ],
    "extra": ["no face"],
    "anchor": ["anchored"],
    "scene": "on flat background",
    "neg": "text"
  }
}`

// TestRunnerReloadFamilies — ReloadFamilies перечитывает families.json с диска
// и заменяет конфиг в памяти (кнопка «Пересобрать промт»): после reload
// r.families содержит обновлённые appearance/blocked; невалидный файл —
// ошибка без замены конфига.
func TestRunnerReloadFamilies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "families.json")
	if err := os.WriteFile(path, []byte(testFamiliesJSON), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	fam, err := config.LoadFamilies(path)
	if err != nil {
		t.Fatalf("LoadFamilies: %v", err)
	}
	r := NewRunner(&config.StudioConfig{}, nil, fam, nil, nil)
	if got := r.families["F1"].Races[0].Appearance; got != "old appearance" {
		t.Fatalf("до reload: appearance = %q", got)
	}
	// файл изменён (машинная проекция обновилась) → reload
	updated := strings.Replace(testFamiliesJSON, "old appearance", "new appearance", 1)
	updated = strings.Replace(updated, `"blocked": ["old"]`, `"blocked": ["new1", "new2"]`, 1)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("WriteFile updated: %v", err)
	}
	if err := r.ReloadFamilies(path); err != nil {
		t.Fatalf("ReloadFamilies: %v", err)
	}
	rc := r.families["F1"].Races[0]
	if rc.Appearance != "new appearance" {
		t.Errorf("после reload: appearance = %q, want new", rc.Appearance)
	}
	if len(rc.Blocked) != 2 || rc.Blocked[0] != "new1" || rc.Blocked[1] != "new2" {
		t.Errorf("после reload: blocked = %v", rc.Blocked)
	}
	// невалидный файл → ошибка, конфиг не заменён
	if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
		t.Fatalf("WriteFile broken: %v", err)
	}
	if err := r.ReloadFamilies(path); err == nil {
		t.Error("невалидный файл: ошибки нет")
	}
	if rc2 := r.families["F1"].Races[0]; rc2.Appearance != "new appearance" {
		t.Errorf("после неудачного reload: appearance = %q, want new (конфиг не заменён)", rc2.Appearance)
	}
}