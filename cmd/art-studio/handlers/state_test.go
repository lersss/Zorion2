package handlers

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseRefFile — разбор имени файла эталона ref_<FAM>_r<id>.png
// (DoD 67a.1 §10).
func TestParseRefFile(t *testing.T) {
	cases := []struct{ name, fam, race string }{
		{"ref_F2_r5.png", "F2", "5"},
		{"ref_F2_r48.png", "F2", "48"},
		{"ref_F9_r47.png", "F9", "47"},
	}
	for _, c := range cases {
		fam, race := ParseRefFile(c.name)
		if fam != c.fam || race != c.race {
			t.Errorf("%s → %s/%s, want %s/%s", c.name, fam, race, c.fam, c.race)
		}
	}
	if RefFileName("F2", "5") != "ref_F2_r5.png" {
		t.Errorf("RefFileName = %s, want ref_F2_r5.png", RefFileName("F2", "5"))
	}
}

// TestNextAcceptNumber — автонумерация принятых по существующим файлам
// (спека 67a.1 §8).
func TestNextAcceptNumber(t *testing.T) {
	dir := t.TempDir()
	if n := NextAcceptNumber(dir); n != 1 {
		t.Errorf("пустая папка: n = %d, want 1", n)
	}
	os.WriteFile(filepath.Join(dir, "01.png"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "02.png"), []byte("x"), 0644)
	if n := NextAcceptNumber(dir); n != 3 {
		t.Errorf("n = %d, want 3", n)
	}
}

// TestNextAcceptHumans — автонумерация race_f0_humans_NN.png.
func TestNextAcceptHumans(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "race_f0_humans_01.png"), []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "race_f0_humans_02.png"), []byte("x"), 0644)
	if n := NextAcceptHumans(dir); n != 3 {
		t.Errorf("n = %d, want 3", n)
	}
}

// TestCandInfo — выдержка «материал · форма» из промпта кандидата
// (перенос _cand_info из race_studio.py).
func TestCandInfo(t *testing.T) {
	p := "dramatic cinematic concept art of an ABSTRACT OBJECT, FRONT VIEW, made of liquid ammonia, translucent pale blue, a clustered jagged spire of material, formed of shards, no face..."
	info := CandInfo(p)
	if info != "liquid ammonia · clustered jagged spire" {
		t.Errorf("info = %q", info)
	}
}