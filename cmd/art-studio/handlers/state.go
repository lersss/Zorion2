package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NextAcceptNumber — первый свободный номер NN в папке (NN.png).
// Автонумерация принятых — по существующим файлам (спека 67a.1 §8).
func NextAcceptNumber(dir string) int {
	n := 1
	for {
		if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("%02d.png", n))); err != nil {
			return n
		}
		n++
	}
}

// NextAcceptHumans — первый свободный номер race_f1_humans_NN.png.
func NextAcceptHumans(dir string) int {
	n := 1
	for {
		if _, err := os.Stat(filepath.Join(dir, fmt.Sprintf("race_f1_humans_%02d.png", n))); err != nil {
			return n
		}
		n++
	}
}

// RefFileName — имя файла эталона: ref_<FAM>_r<race_id>.png.
func RefFileName(fam, raceID string) string {
	return fmt.Sprintf("ref_%s_r%s.png", fam, raceID)
}

// ParseRefFile разбирает имя файла эталона ref_<FAM>_r<id>.png → (fam, raceID).
func ParseRefFile(name string) (string, string) {
	base := strings.TrimSuffix(name, ".png")
	rest := strings.TrimPrefix(base, "ref_")
	idx := strings.Index(rest, "_r")
	if idx < 0 {
		return "", ""
	}
	return rest[:idx], rest[idx+2:]
}